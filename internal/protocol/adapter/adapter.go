package adapter

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
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
				utils.Errorf("%s accept error: %v", adapter.getConnectionType(), err)
			}
			return
		}

		if b.IsClosed() {
			utils.Warnf("%s connection closed", adapter.getConnectionType())
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
	
	// 检查是否为持久连接（如UDP会话连接）
	if persistentConn, ok := conn.(interface{ IsPersistent() bool }); ok && persistentConn.IsPersistent() {
		shouldCloseConn = false
		utils.Debugf("%s adapter: persistent connection detected, will not close after handling", adapter.getConnectionType())
	}
	
	defer func() {
		// ✅ 只有非隧道连接且非持久连接才在此处关闭
		// 隧道连接交给TunnelBridge管理生命周期
		// 持久连接（如UDP会话）由会话管理器管理生命周期
		if shouldCloseConn {
			if closer, ok := conn.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		}
	}()

	utils.Infof("%s adapter handling connection", adapter.getConnectionType())

	// Session是系统关键组件，必须存在
	if b.session == nil {
		utils.Errorf("Session is required but not set for %s adapter", adapter.getConnectionType())
		return
	}

	// 初始化连接
	streamConn, err := b.session.AcceptConnection(conn, conn)
	if err != nil {
		utils.Errorf("Failed to initialize connection: %v", err)
		return
	}

	// 启动包处理循环
	utils.Debugf("Starting packet processing loop for connection %s", streamConn.ID)
	for {
		select {
		case <-b.Ctx().Done():
			utils.Debugf("Context cancelled, closing connection %s", streamConn.ID)
			return
		default:
		}

		// 读取并处理数据包
		utils.Debugf("Server: waiting for packet on connection %s", streamConn.ID)
		pkt, _, err := streamConn.Stream.ReadPacket()
		if err != nil {
			if err != io.EOF {
				utils.Errorf("Failed to read packet for connection %s: %v", streamConn.ID, err)
			} else {
				utils.Debugf("Connection %s closed by peer", streamConn.ID)
			}
			return
		}
		utils.Infof("Server: received packet, type=%d on connection %s", pkt.PacketType, streamConn.ID)

		// 填充 ConnectionID 并处理
		streamPacket := &types.StreamPacket{
			ConnectionID: streamConn.ID,
			Packet:       pkt,
			Timestamp:    time.Now(),
		}

		// 处理数据包
		if err := b.session.HandlePacket(streamPacket); err != nil {
			// ✅ 检查是否为隧道切换标记
			if err.Error() == "tunnel source connected, switching to stream mode" || 
			   err.Error() == "tunnel target connected, switching to stream mode" {
				utils.Infof("Connection %s switched to tunnel stream mode, not closing", streamConn.ID)
				shouldCloseConn = false // ✅ 不关闭隧道连接
				return
			}
			utils.Errorf("Failed to handle packet for connection %s: %v", streamConn.ID, err)
			// 继续处理下一个包，不要直接返回
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

	utils.Infof("%s adapter closed", b.name)
	return nil
}
