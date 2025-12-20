package adapter

import (
corelog "tunnox-core/internal/core/log"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
)

// Adapter 协议适配器统一接口
type Adapter interface {
	ConnectTo(serverAddr string) error
	ListenFrom(serverAddr string) error
	Name() string
	GetReader() io.Reader
	GetWriter() io.Writer
	Close() error
	SetAddr(addr string)
	GetAddr() string
}

// BaseAdapter 基础适配器，提供通用的连接管理和流处理逻辑
type BaseAdapter struct {
	dispose.Dispose
	name        string
	addr        string
	session     session.Session
	active      bool
	connMutex   sync.RWMutex
	stream      stream.PackageStreamer
	streamMutex sync.RWMutex
	protocol    ProtocolAdapter // 存储具体的协议适配器引用
}

// 协议适配器接口，子类需要实现
type ProtocolAdapter interface {
	Adapter
	// 协议特定的方法
	Dial(addr string) (io.ReadWriteCloser, error)
	Listen(addr string) error            // 直接启动监听，不需要返回监听器
	Accept() (io.ReadWriteCloser, error) // 直接在适配器中实现Accept
	getConnectionType() string
}

func (b *BaseAdapter) GetAddr() string  { return b.addr }
func (b *BaseAdapter) Name() string     { return b.name }
func (b *BaseAdapter) Addr() string     { return b.addr }
func (b *BaseAdapter) SetName(n string) { b.name = n }
func (b *BaseAdapter) SetAddr(a string) { b.addr = a }

// SetSession 设置会话
func (b *BaseAdapter) SetSession(session session.Session) {
	b.session = session
}

// GetSession 获取会话
func (b *BaseAdapter) GetSession() session.Session {
	return b.session
}

// SetProtocolAdapter 设置具体的协议适配器引用
func (b *BaseAdapter) SetProtocolAdapter(pa ProtocolAdapter) {
	b.protocol = pa
}

// ConnectTo 通用连接逻辑
func (b *BaseAdapter) ConnectTo(serverAddr string) error {
	b.connMutex.Lock()
	defer b.connMutex.Unlock()

	if b.stream != nil {
		return fmt.Errorf("already connected")
	}

	if b.protocol == nil {
		return fmt.Errorf("protocol adapter not set")
	}

	conn, err := b.protocol.Dial(serverAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s server: %w", b.protocol.getConnectionType(), err)
	}

	b.SetAddr(serverAddr)

	b.streamMutex.Lock()
	b.stream = stream.NewStreamProcessor(conn, conn, b.Ctx())
	b.streamMutex.Unlock()

	return nil
}

// ListenFrom 通用监听逻辑
func (b *BaseAdapter) ListenFrom(listenAddr string) error {
	b.SetAddr(listenAddr)
	if b.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	if b.protocol == nil {
		return fmt.Errorf("protocol adapter not set")
	}

	// 适配器直接启动监听
	if err := b.protocol.Listen(b.Addr()); err != nil {
		return fmt.Errorf("failed to listen on %s: %w", b.protocol.getConnectionType(), err)
	}

	b.active = true
	go b.acceptLoop(b.protocol)
	return nil
}

// acceptLoop 通用接受连接循环
func (b *BaseAdapter) acceptLoop(adapter ProtocolAdapter) {
	for b.active {
		conn, err := adapter.Accept()
		if err != nil {
			if !b.IsClosed() {
				// 检查是否为可忽略的错误（如超时）
				if isIgnorableError(err) {
					continue
				}
				corelog.Errorf("%s accept error: %v", adapter.getConnectionType(), err)
			}
			return
		}

		if b.IsClosed() {
			corelog.Warnf("%s connection closed", adapter.getConnectionType())
			return
		}

		go b.handleConnection(adapter, conn)
	}
}

// isIgnorableError 检查是否为可忽略的错误
func isIgnorableError(err error) bool {
	// 检查是否为自定义超时错误
	if errors.IsProtocolTimeoutError(err) {
		return true
	}

	// 检查是否为网络超时错误
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	return false
}

// handleConnection 通用连接处理逻辑
func (b *BaseAdapter) handleConnection(adapter ProtocolAdapter, conn io.ReadWriteCloser) {
	shouldCloseConn := true // 默认关闭连接
	var streamConn *types.StreamConnection

	if persistentConn, ok := conn.(interface{ IsPersistent() bool }); ok && persistentConn.IsPersistent() {
		shouldCloseConn = false
	}

	defer func() {
		// 清理 SessionManager 中的连接（如果已创建）
		if streamConn != nil && b.session != nil {
			_ = b.session.CloseConnection(streamConn.ID)
		}

		// 关闭底层连接（如果不是持久连接）
		if shouldCloseConn {
			if closer, ok := conn.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		}
	}()

	corelog.Infof("%s adapter handling connection", adapter.getConnectionType())

	// Session是系统关键组件，必须存在
	if b.session == nil {
		corelog.Errorf("Session is required but not set for %s adapter", adapter.getConnectionType())
		return
	}

	// 初始化连接
	var err error
	streamConn, err = b.session.AcceptConnection(conn, conn)
	if err != nil {
		corelog.Errorf("Failed to initialize connection: %v", err)
		return
	}

	for {
		select {
		case <-b.Ctx().Done():
			return
		default:
		}

		// ✅ 设计改进：在每次循环开始时检查连接是否已切换到流模式
		// 如果已切换到流模式，readLoop 应该立即退出，因为：
		// 1. 流模式下数据是原始数据（如 MySQL 协议），不再是 Tunnox 协议包
		// 2. 流模式下的数据应该通过 net.Conn 直接转发，而不是通过 ReadPacket()
		// 3. 这样可以避免不必要的 ReadPacket 调用和解压缩错误
		if streamConn != nil && streamConn.Stream != nil {
			if reader, ok := streamConn.Stream.GetReader().(interface {
				IsStreamMode() bool
			}); ok && reader.IsStreamMode() {
				corelog.Infof("Connection %s is in stream mode, readLoop exiting (data will be forwarded directly via net.Conn)", streamConn.ID)
				shouldCloseConn = false
				streamConn = nil
				return
			}
		}

		pkt, _, err := streamConn.Stream.ReadPacket()
		if err != nil {
			isTimeoutError := false
			underlyingErr := err
			for underlyingErr != nil {
				if netErr, ok := underlyingErr.(interface {
					Timeout() bool
					Temporary() bool
				}); ok && netErr.Timeout() && netErr.Temporary() {
					isTimeoutError = true
					break
				}
				if unwrapper, ok := underlyingErr.(interface{ Unwrap() error }); ok {
					underlyingErr = unwrapper.Unwrap()
				} else {
					break
				}
			}
			if isTimeoutError {
				continue
			}
			if err != io.EOF {
				corelog.Errorf("Failed to read packet for connection %s: %v", streamConn.ID, err)
			}
			return
		}

		streamPacket := &types.StreamPacket{
			ConnectionID: streamConn.ID,
			Packet:       pkt,
			Timestamp:    time.Now(),
		}

		isTunnelOpenPacket := (pkt.PacketType & 0x3F) == packet.TunnelOpen

		if err := b.session.HandlePacket(streamPacket); err != nil {
			if isTunnelOpenPacket {
				errMsg := err.Error()
				if errMsg == "tunnel source connected, switching to stream mode" ||
					errMsg == "tunnel target connected, switching to stream mode" ||
					errMsg == "tunnel target connected via cross-server bridge, switching to stream mode" ||
					errMsg == "tunnel connected to existing bridge, switching to stream mode" {
					// 在设置为 nil 之前保存 ID 用于日志
					connID := streamConn.ID
					shouldCloseConn = false
					// 注意：对于隧道连接，streamConn 会被转移到隧道管理，不需要在这里关闭
					streamConn = nil
					corelog.Infof("Connection %s switched to stream mode, readLoop exiting", connID)
					return
				}
			}
			corelog.Errorf("Failed to handle packet for connection %s: %v", streamConn.ID, err)
		}
	}
}

// GetReader 获取读取器
func (b *BaseAdapter) GetReader() io.Reader {
	b.streamMutex.RLock()
	defer b.streamMutex.RUnlock()
	if b.stream != nil {
		return b.stream.GetReader()
	}
	return nil
}

// GetWriter 获取写入器
func (b *BaseAdapter) GetWriter() io.Writer {
	b.streamMutex.RLock()
	defer b.streamMutex.RUnlock()
	if b.stream != nil {
		return b.stream.GetWriter()
	}
	return nil
}

// Close 关闭适配器（实现Adapter接口）
func (b *BaseAdapter) Close() error {
	b.active = false
	result := b.Dispose.Close()
	if result.HasErrors() {
		return fmt.Errorf("dispose cleanup failed: %s", result.Error())
	}
	return nil
}

// onClose 通用资源清理
func (b *BaseAdapter) onClose() error {
	b.active = false

	b.streamMutex.Lock()
	if b.stream != nil {
		// 使用类型断言来调用CloseWithResult方法
		if streamProcessor, ok := b.stream.(interface{ CloseWithResult() *dispose.DisposeResult }); ok {
			result := streamProcessor.CloseWithResult()
			if result.HasErrors() {
				b.streamMutex.Unlock()
				return fmt.Errorf("stream processor cleanup failed: %v", result.Error())
			}
		} else {
			// 如果类型断言失败，使用普通的Close方法
			b.stream.Close()
		}
		b.stream = nil
	}
	b.streamMutex.Unlock()

	corelog.Infof("%s adapter closed", b.name)
	return nil
}
