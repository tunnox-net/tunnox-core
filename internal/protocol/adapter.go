package protocol

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// TimeoutError 超时错误类型
type TimeoutError struct {
	Protocol string
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("timeout waiting for %s", e.Protocol)
}

// IsTimeoutError 检查是否为超时错误
func IsTimeoutError(err error) bool {
	_, ok := err.(*TimeoutError)
	return ok
}

// Adapter 协议适配器统一接口
type Adapter interface {
	ConnectTo(serverAddr string) error
	ListenFrom(serverAddr string) error
	Name() string
	GetReader() io.Reader
	GetWriter() io.Writer
	Close()
	SetAddr(addr string)
	GetAddr() string
}

// BaseAdapter 基础适配器，提供通用的连接管理和流处理逻辑
type BaseAdapter struct {
	utils.Dispose
	name        string
	addr        string
	session     Session
	active      bool
	connMutex   sync.RWMutex
	stream      stream.PackageStreamer
	streamMutex sync.RWMutex
}

// 协议特定的连接类型接口
type ProtocolConn interface {
	io.Reader
	io.Writer
	Close() error
}

// 协议特定的监听器接口
type ProtocolListener interface {
	Accept() (ProtocolConn, error)
	Close() error
}

// 协议特定的连接器接口
type ProtocolDialer interface {
	Dial(addr string) (ProtocolConn, error)
}

// 协议特定的监听器创建器接口
type ProtocolListenerCreator interface {
	Listen(addr string) (ProtocolListener, error)
}

// 协议适配器接口，子类需要实现
type ProtocolAdapter interface {
	Adapter
	// 协议特定的方法
	createDialer() ProtocolDialer
	createListener(addr string) (ProtocolListener, error)
	handleProtocolSpecific(conn ProtocolConn) error
	getConnectionType() string
}

func (b *BaseAdapter) GetAddr() string  { return b.addr }
func (b *BaseAdapter) Name() string     { return b.name }
func (b *BaseAdapter) Addr() string     { return b.addr }
func (b *BaseAdapter) SetName(n string) { b.name = n }
func (b *BaseAdapter) SetAddr(a string) { b.addr = a }

// SetSession 设置会话
func (b *BaseAdapter) SetSession(session Session) {
	b.session = session
}

// GetSession 获取会话
func (b *BaseAdapter) GetSession() Session {
	return b.session
}

// ConnectTo 通用连接逻辑
func (b *BaseAdapter) ConnectTo(adapter ProtocolAdapter, serverAddr string) error {
	b.connMutex.Lock()
	defer b.connMutex.Unlock()

	if b.stream != nil {
		return fmt.Errorf("already connected")
	}

	dialer := adapter.createDialer()
	conn, err := dialer.Dial(serverAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s server: %w", adapter.getConnectionType(), err)
	}

	b.SetAddr(serverAddr)

	b.streamMutex.Lock()
	b.stream = stream.NewStreamProcessor(conn, conn, b.Ctx())
	b.streamMutex.Unlock()

	return nil
}

// ListenFrom 通用监听逻辑
func (b *BaseAdapter) ListenFrom(adapter ProtocolAdapter, listenAddr string) error {
	b.SetAddr(listenAddr)
	if b.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	listener, err := adapter.createListener(b.Addr())
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", adapter.getConnectionType(), err)
	}

	b.active = true
	go b.acceptLoop(adapter, listener)
	return nil
}

// acceptLoop 通用接受连接循环
func (b *BaseAdapter) acceptLoop(adapter ProtocolAdapter, listener ProtocolListener) {
	for b.active {
		conn, err := listener.Accept()
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
	if IsTimeoutError(err) {
		return true
	}

	// 检查是否为网络超时错误
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	return false
}

// handleConnection 通用连接处理逻辑
func (b *BaseAdapter) handleConnection(adapter ProtocolAdapter, conn ProtocolConn) {
	defer func() {
		if closer, ok := conn.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()

	utils.Infof("%s adapter handling connection", adapter.getConnectionType())

	// 使用 Session 接口处理连接
	if b.session != nil {
		// 初始化连接
		connInfo, err := b.session.InitConnection(conn, conn)
		if err != nil {
			utils.Errorf("Failed to initialize connection: %v", err)
			return
		}
		defer func(session Session, connectionId string) {
			err := session.CloseConnection(connectionId)
			if err != nil {
				utils.Errorf("Failed to close connection: %v", err)
			}
		}(b.session, connInfo.ID)

		// 处理数据流
		for {
			packet, _, err := connInfo.Stream.ReadPacket()
			if err != nil {
				if err == io.EOF {
					utils.Infof("Connection closed by peer: %s", connInfo.ID)
				} else {
					utils.Errorf("Failed to read packet: %v", err)
				}
				break
			}

			utils.Debugf("Received packet type: %v", packet.PacketType)

			// 包装成 StreamPacket
			connPacket := &StreamPacket{
				ConnectionID: connInfo.ID,
				Packet:       packet,
				Timestamp:    time.Now(),
			}

			// 处理数据包
			if err := b.session.HandlePacket(connPacket); err != nil {
				utils.Errorf("Failed to handle packet: %v", err)
				break
			}
		}
	} else {
		// 如果没有session，使用协议特定的处理
		if err := adapter.handleProtocolSpecific(conn); err != nil {
			utils.Errorf("Protocol specific handling failed: %v", err)
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
func (b *BaseAdapter) Close() {
	b.active = false
	b.Dispose.Close()
}

// onClose 通用资源清理
func (b *BaseAdapter) onClose() {
	b.active = false

	b.streamMutex.Lock()
	if b.stream != nil {
		b.stream.Close()
		b.stream = nil
	}
	b.streamMutex.Unlock()

	utils.Infof("%s adapter closed", b.name)
}
