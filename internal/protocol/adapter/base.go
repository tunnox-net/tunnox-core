package adapter

import (
	"fmt"
	"io"
	"net"
	"sync"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/types"
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
	Close() error
	SetAddr(addr string)
	GetAddr() string
}

// ProtocolAdapterBase 基础适配器，提供通用的连接管理和流处理逻辑 (Renamed from BaseAdapter)
type ProtocolAdapterBase struct {
	dispose.Dispose
	name        string
	addr        string
	session     types.Session
	active      bool
	connMutex   sync.RWMutex
	stream      stream.PackageStreamer
	streamMutex sync.RWMutex
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

func (b *ProtocolAdapterBase) GetAddr() string  { return b.addr }
func (b *ProtocolAdapterBase) Name() string     { return b.name }
func (b *ProtocolAdapterBase) Addr() string     { return b.addr }
func (b *ProtocolAdapterBase) SetName(n string) { b.name = n }
func (b *ProtocolAdapterBase) SetAddr(a string) { b.addr = a }

// SetSession 设置会话
func (b *ProtocolAdapterBase) SetSession(session types.Session) {
	b.session = session
}

// GetSession 获取会话
func (b *ProtocolAdapterBase) GetSession() types.Session {
	return b.session
}

// ConnectTo 通用连接逻辑
func (b *ProtocolAdapterBase) ConnectTo(adapter ProtocolAdapter, serverAddr string) error {
	b.connMutex.Lock()
	defer b.connMutex.Unlock()

	if b.stream != nil {
		return fmt.Errorf("already connected")
	}

	conn, err := adapter.Dial(serverAddr)
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
func (b *ProtocolAdapterBase) ListenFrom(adapter ProtocolAdapter, listenAddr string) error {
	b.SetAddr(listenAddr)
	if b.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	// 适配器直接启动监听
	if err := adapter.Listen(b.Addr()); err != nil {
		return fmt.Errorf("failed to listen on %s: %w", adapter.getConnectionType(), err)
	}

	b.active = true
	go b.acceptLoop(adapter)
	return nil
}

// acceptLoop 通用接受连接循环
func (b *ProtocolAdapterBase) acceptLoop(adapter ProtocolAdapter) {
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
func (b *ProtocolAdapterBase) handleConnection(adapter ProtocolAdapter, conn io.ReadWriteCloser) {
	defer func() {
		if closer, ok := conn.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()

	utils.Infof("%s adapter handling connection", adapter.getConnectionType())

	// Session是系统关键组件，必须存在
	if b.session == nil {
		utils.Errorf("Session is required but not set for %s adapter", adapter.getConnectionType())
		return
	}

	// 初始化连接
	_, err := b.session.AcceptConnection(conn, conn)
	if err != nil {
		utils.Errorf("Failed to initialize connection: %v", err)
		return
	}
}

// GetReader 获取读取器
func (b *ProtocolAdapterBase) GetReader() io.Reader {
	b.streamMutex.RLock()
	defer b.streamMutex.RUnlock()
	if b.stream != nil {
		return b.stream.GetReader()
	}
	return nil
}

// GetWriter 获取写入器
func (b *ProtocolAdapterBase) GetWriter() io.Writer {
	b.streamMutex.RLock()
	defer b.streamMutex.RUnlock()
	if b.stream != nil {
		return b.stream.GetWriter()
	}
	return nil
}

// Close 关闭适配器（实现Adapter接口）
func (b *ProtocolAdapterBase) Close() error {
	b.active = false
	result := b.Dispose.Close()
	if result.HasErrors() {
		return fmt.Errorf("dispose cleanup failed: %s", result.Error())
	}
	return nil
}

// onClose 通用资源清理
func (b *ProtocolAdapterBase) onClose() error {
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
