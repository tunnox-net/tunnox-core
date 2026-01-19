package adapter

import (
	"io"
	"net"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
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
		return coreerrors.New(coreerrors.CodeAlreadyExists, "already connected")
	}

	if b.protocol == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "protocol adapter not set")
	}

	conn, err := b.protocol.Dial(serverAddr)
	if err != nil {
		return coreerrors.Wrapf(err, coreerrors.CodeConnectionError, "failed to connect to %s server", b.protocol.getConnectionType())
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
		return coreerrors.New(coreerrors.CodeInvalidParam, "address not set")
	}

	if b.protocol == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "protocol adapter not set")
	}

	// 适配器直接启动监听
	if err := b.protocol.Listen(b.Addr()); err != nil {
		return coreerrors.Wrapf(err, coreerrors.CodeConnectionError, "failed to listen on %s", b.protocol.getConnectionType())
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
	// 检查是否为超时错误码
	if coreerrors.IsCode(err, coreerrors.CodeTimeout) {
		return true
	}

	// 检查是否为网络超时错误
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	return false
}

// connectionState 保存连接处理过程中的状态
type connectionState struct {
	streamConn      *types.StreamConnection
	shouldCloseConn bool
}

// handleConnection 通用连接处理逻辑
func (b *BaseAdapter) handleConnection(adapter ProtocolAdapter, conn io.ReadWriteCloser) {
	state := &connectionState{shouldCloseConn: true}

	// 检查是否为持久连接
	if persistentConn, ok := conn.(interface{ IsPersistent() bool }); ok && persistentConn.IsPersistent() {
		state.shouldCloseConn = false
	}

	defer b.cleanupConnection(state, conn)

	corelog.Infof("%s adapter handling connection", adapter.getConnectionType())

	// 初始化连接
	if err := b.initializeConnection(adapter, conn, state); err != nil {
		return
	}

	// 进入读取循环
	b.connectionReadLoop(state)
}

// cleanupConnection 清理连接资源
func (b *BaseAdapter) cleanupConnection(state *connectionState, conn io.ReadWriteCloser) {
	connID := ""
	if state.streamConn != nil {
		connID = state.streamConn.ID
	}
	corelog.Debugf("cleanupConnection: connID=%s, streamConn=%v, shouldCloseConn=%v",
		connID, state.streamConn != nil, state.shouldCloseConn)

	// 清理 SessionManager 中的连接（如果已创建，忽略关闭错误，连接可能已关闭）
	if state.streamConn != nil && b.session != nil {
		corelog.Debugf("cleanupConnection: calling CloseConnection for connID=%s", connID)
		_ = b.session.CloseConnection(state.streamConn.ID)
	}

	// 关闭底层连接（如果不是持久连接，忽略关闭错误，连接可能已关闭）
	if state.shouldCloseConn {
		corelog.Debugf("cleanupConnection: closing underlying connection for connID=%s", connID)
		if closer, ok := conn.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	} else {
		corelog.Debugf("cleanupConnection: NOT closing underlying connection (shouldCloseConn=false)")
	}
}

// initializeConnection 初始化连接并验证 session
func (b *BaseAdapter) initializeConnection(adapter ProtocolAdapter, conn io.ReadWriteCloser, state *connectionState) error {
	// Session 是系统关键组件，必须存在
	if b.session == nil {
		corelog.Errorf("Session is required but not set for %s adapter", adapter.getConnectionType())
		return coreerrors.New(coreerrors.CodeNotConfigured, "session not set")
	}

	// 接受连接
	streamConn, err := b.session.AcceptConnection(conn, conn)
	if err != nil {
		corelog.Errorf("Failed to initialize connection: %v", err)
		return err
	}

	state.streamConn = streamConn
	return nil
}

// connectionReadLoop 连接读取循环
func (b *BaseAdapter) connectionReadLoop(state *connectionState) {
	for {
		select {
		case <-b.Ctx().Done():
			return
		default:
		}

		// 检查是否已切换到流模式
		if b.checkAndHandleStreamMode(state) {
			return
		}

		// 读取数据包
		pkt, shouldContinue, shouldReturn := b.readPacketWithTimeout(state)
		if shouldReturn {
			return
		}
		if shouldContinue {
			continue
		}

		// 处理数据包
		if b.handlePacketAndCheckModeSwitch(state, pkt) {
			return
		}
	}
}

// checkAndHandleStreamMode 检查连接是否已切换到流模式
// 如果已切换到流模式，readLoop 应该立即退出，因为：
// 1. 流模式下数据是原始数据（如 MySQL 协议），不再是 Tunnox 协议包
// 2. 流模式下的数据应该通过 net.Conn 直接转发，而不是通过 ReadPacket()
// 3. 这样可以避免不必要的 ReadPacket 调用和解压缩错误
func (b *BaseAdapter) checkAndHandleStreamMode(state *connectionState) bool {
	if state.streamConn == nil || state.streamConn.Stream == nil {
		return false
	}

	reader, ok := state.streamConn.Stream.GetReader().(interface {
		IsStreamMode() bool
	})
	if !ok || !reader.IsStreamMode() {
		return false
	}

	corelog.Infof("Connection %s is in stream mode, readLoop exiting (data will be forwarded directly via net.Conn)", state.streamConn.ID)
	state.shouldCloseConn = false
	state.streamConn = nil
	return true
}

// readPacketWithTimeout 读取数据包并处理超时错误
// 返回值: (packet, shouldContinue, shouldReturn)
func (b *BaseAdapter) readPacketWithTimeout(state *connectionState) (*packet.TransferPacket, bool, bool) {
	pkt, _, err := state.streamConn.Stream.ReadPacket()
	if err != nil {
		if b.isTimeoutError(err) {
			return nil, true, false // shouldContinue
		}
		if err != io.EOF {
			corelog.Errorf("Failed to read packet for connection %s: %v", state.streamConn.ID, err)
		}
		return nil, false, true // shouldReturn
	}
	return pkt, false, false
}

// isTimeoutError 检查错误是否为超时错误
func (b *BaseAdapter) isTimeoutError(err error) bool {
	underlyingErr := err
	for underlyingErr != nil {
		if netErr, ok := underlyingErr.(interface {
			Timeout() bool
			Temporary() bool
		}); ok && netErr.Timeout() && netErr.Temporary() {
			return true
		}
		if unwrapper, ok := underlyingErr.(interface{ Unwrap() error }); ok {
			underlyingErr = unwrapper.Unwrap()
		} else {
			break
		}
	}
	return false
}

// handlePacketAndCheckModeSwitch 处理数据包并检查是否需要切换模式
// 返回 true 表示需要退出循环
func (b *BaseAdapter) handlePacketAndCheckModeSwitch(state *connectionState, pkt *packet.TransferPacket) bool {
	streamPacket := &types.StreamPacket{
		ConnectionID: state.streamConn.ID,
		Packet:       pkt,
		Timestamp:    time.Now(),
	}

	isTunnelOpenPacket := (pkt.PacketType & 0x3F) == packet.TunnelOpen

	if err := b.session.HandlePacket(streamPacket); err != nil {
		if isTunnelOpenPacket && b.isTunnelModeSwitch(err) {
			connID := state.streamConn.ID
			corelog.Infof("Connection %s: detected TunnelModeSwitch, setting shouldCloseConn=false, streamConn=nil", connID)
			state.shouldCloseConn = false
			// 注意：对于隧道连接，streamConn 会被转移到隧道管理，不需要在这里关闭
			state.streamConn = nil
			corelog.Infof("Connection %s switched to stream mode, readLoop exiting", connID)
			return true
		}
		corelog.Errorf("Failed to handle packet for connection %s: %v", state.streamConn.ID, err)
	}
	return false
}

// isTunnelModeSwitch 检查错误是否表示隧道模式切换
func (b *BaseAdapter) isTunnelModeSwitch(err error) bool {
	return coreerrors.IsCode(err, coreerrors.CodeTunnelModeSwitch)
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
		return coreerrors.Newf(coreerrors.CodeCleanupError, "dispose cleanup failed: %s", result.Error())
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
				return coreerrors.Newf(coreerrors.CodeCleanupError, "stream processor cleanup failed: %v", result.Error())
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
