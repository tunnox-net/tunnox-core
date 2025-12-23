// Package websocket 提供 WebSocket 传输模块
// 用于客户端通过 WebSocket 方式连接服务器
package websocket

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

// WebSocketModule WebSocket 传输模块
type WebSocketModule struct {
	*dispose.ServiceBase

	config   *httpservice.WebSocketModuleConfig
	deps     *httpservice.ModuleDependencies
	upgrader websocket.Upgrader
	connChan chan *WebSocketServerConn
	session  session.Session
}

// NewWebSocketModule 创建 WebSocket 模块
func NewWebSocketModule(ctx context.Context, config *httpservice.WebSocketModuleConfig) *WebSocketModule {
	m := &WebSocketModule{
		ServiceBase: dispose.NewService("WebSocketModule", ctx),
		config:      config,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  constants.WebSocketBufferSize,
			WriteBufferSize: constants.WebSocketBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins
			},
		},
		connChan: make(chan *WebSocketServerConn, 100),
	}

	return m
}

// Name 返回模块名称
func (m *WebSocketModule) Name() string {
	return "WebSocket"
}

// SetDependencies 注入依赖
func (m *WebSocketModule) SetDependencies(deps *httpservice.ModuleDependencies) {
	m.deps = deps
}

// SetSession 设置会话管理器
func (m *WebSocketModule) SetSession(sess session.Session) {
	m.session = sess
}

// RegisterRoutes 注册路由
func (m *WebSocketModule) RegisterRoutes(router *mux.Router) {
	if !m.config.Enabled {
		corelog.Infof("WebSocketModule: disabled, skipping route registration")
		return
	}

	// 注册 /_tunnox 路由处理 WebSocket 升级
	router.HandleFunc("/_tunnox", m.handleWebSocket).Methods("GET")
	corelog.Infof("WebSocketModule: registered route /_tunnox")
}

// handleWebSocket 处理 WebSocket 升级请求
func (m *WebSocketModule) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 升级 HTTP 连接为 WebSocket
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		corelog.Errorf("WebSocketModule: upgrade failed: %v", err)
		return
	}

	// 包装 WebSocket 连接
	wsConn := &WebSocketServerConn{
		conn:           conn,
		remoteAddr:     r.RemoteAddr,
		closed:         make(chan struct{}),
		streamDataChan: make(chan []byte, 100), // 流模式数据通道
	}

	// 如果有会话管理器，处理连接
	if m.session != nil {
		go m.handleConnection(wsConn)
	} else {
		corelog.Warnf("WebSocketModule: session is nil, cannot handle connection")
		// 没有会话管理器，发送到通道等待处理
		select {
		case m.connChan <- wsConn:
		case <-m.Ctx().Done():
			wsConn.Close()
			return
		case <-time.After(5 * time.Second):
			corelog.Errorf("WebSocketModule: connection queue full, rejecting")
			wsConn.Close()
			return
		}
	}
}

// handleConnection 处理 WebSocket 连接的数据包循环
func (m *WebSocketModule) handleConnection(wsConn *WebSocketServerConn) {
	shouldCloseConn := true
	var streamConn *types.StreamConnection

	defer func() {
		// 清理 SessionManager 中的连接（如果已创建）
		if streamConn != nil && m.session != nil {
			_ = m.session.CloseConnection(streamConn.ID)
		}

		// 关闭底层连接
		if shouldCloseConn {
			wsConn.Close()
		}
	}()

	// 初始化连接
	var err error
	streamConn, err = m.session.AcceptConnection(wsConn, wsConn)
	if err != nil {
		corelog.Errorf("WebSocketModule: failed to accept connection: %v", err)
		return
	}

	// 设置连接ID（用于调试）
	wsConn.SetConnectionID(streamConn.ID)

	// 数据包处理循环
	for {
		select {
		case <-m.Ctx().Done():
			return
		default:
		}

		// 检查连接是否已切换到流模式
		if streamConn != nil && streamConn.Stream != nil {
			if reader, ok := streamConn.Stream.GetReader().(interface {
				IsStreamMode() bool
			}); ok && reader.IsStreamMode() {
				// 启动流模式读取器
				wsConn.SetStreamMode(true)
				wsConn.StartStreamModeReader()
				shouldCloseConn = false
				streamConn = nil
				return
			}
		}

		pkt, _, err := streamConn.Stream.ReadPacket()
		if err != nil {
			// 检查是否为超时错误
			if netErr, ok := err.(interface {
				Timeout() bool
				Temporary() bool
			}); ok && netErr.Timeout() && netErr.Temporary() {
				continue
			}
			if err != io.EOF {
				corelog.Errorf("WebSocketModule: failed to read packet for connection %s: %v", streamConn.ID, err)
			}
			return
		}

		streamPacket := &types.StreamPacket{
			ConnectionID: streamConn.ID,
			Packet:       pkt,
			Timestamp:    time.Now(),
		}

		isTunnelOpenPacket := (pkt.PacketType & 0x3F) == packet.TunnelOpen

		if err := m.session.HandlePacket(streamPacket); err != nil {
			if isTunnelOpenPacket {
				errMsg := err.Error()
				if errMsg == "tunnel source connected, switching to stream mode" ||
					errMsg == "tunnel target connected, switching to stream mode" ||
					errMsg == "tunnel target connected via cross-server bridge, switching to stream mode" ||
					errMsg == "tunnel target connected via cross-node forwarding, switching to stream mode" ||
					errMsg == "tunnel connected to existing bridge, switching to stream mode" {
					// 启动流模式读取器
					wsConn.SetStreamMode(true)
					wsConn.StartStreamModeReader()
					shouldCloseConn = false
					streamConn = nil
					return
				}
			}
			corelog.Errorf("WebSocketModule: failed to handle packet for connection %s: %v", streamConn.ID, err)
		}
	}
}

// Start 启动模块
func (m *WebSocketModule) Start() error {
	if !m.config.Enabled {
		corelog.Infof("WebSocketModule: disabled")
		return nil
	}
	corelog.Infof("WebSocketModule: started")
	return nil
}

// Stop 停止模块
func (m *WebSocketModule) Stop() error {
	close(m.connChan)
	corelog.Infof("WebSocketModule: stopped")
	return nil
}

// GetConnChan 获取连接通道（供外部处理连接）
func (m *WebSocketModule) GetConnChan() <-chan *WebSocketServerConn {
	return m.connChan
}

// WebSocketServerConn 包装 WebSocket 连接
type WebSocketServerConn struct {
	conn       *websocket.Conn
	remoteAddr string
	readBuf    []byte
	readMu     sync.Mutex
	writeMu    sync.Mutex
	closeOnce  sync.Once
	closed     chan struct{}

	// 流模式支持（用于跨节点隧道）
	streamMode         bool
	streamModeMu       sync.RWMutex
	streamDataChan     chan []byte // 流模式数据通道
	streamDataChanOnce sync.Once   // 确保 streamDataChan 只关闭一次
	connectionID       string      // 连接ID（用于调试）
}

// Read 实现 io.Reader
func (c *WebSocketServerConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	select {
	case <-c.closed:
		return 0, io.EOF
	default:
	}

	// 如果有缓冲数据，先返回
	if len(c.readBuf) > 0 {
		n := copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	// 检查是否在流模式
	c.streamModeMu.RLock()
	isStreamMode := c.streamMode
	c.streamModeMu.RUnlock()

	if isStreamMode {
		// 流模式：从数据通道读取
		// 注意：这里不应该设置超时，因为 io.Copy 需要阻塞等待数据
		// 如果 channel 关闭或连接关闭，会返回 EOF
		select {
		case <-c.closed:
			return 0, io.EOF
		case data, ok := <-c.streamDataChan:
			if !ok {
				// channel 已关闭（由 StartStreamModeReader 关闭）
				return 0, io.EOF
			}
			n := copy(p, data)
			if n < len(data) {
				c.readBuf = append(c.readBuf, data[n:]...)
			}
			return n, nil
		}
	}

	// 非流模式：直接读取 WebSocket 消息
	messageType, data, err := c.conn.ReadMessage()
	if err != nil {
		select {
		case <-c.closed:
			return 0, io.EOF
		default:
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return 0, io.EOF
			}
			return 0, err
		}
	}

	if messageType != websocket.BinaryMessage {
		return 0, nil // 忽略非二进制消息
	}

	n := copy(p, data)
	if n < len(data) {
		c.readBuf = append(c.readBuf, data[n:]...)
	}

	return n, nil
}

// Write 实现 io.Writer
func (c *WebSocketServerConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	select {
	case <-c.closed:
		return 0, io.ErrClosedPipe
	default:
	}

	err := c.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, err
	}

	return len(p), nil
}

// Close 实现 io.Closer
func (c *WebSocketServerConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		c.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))
		err = c.conn.Close()
	})
	return err
}

// LocalAddr 实现 net.Conn
func (c *WebSocketServerConn) LocalAddr() net.Addr {
	return &wsAddr{addr: "websocket-server"}
}

// RemoteAddr 实现 net.Conn
func (c *WebSocketServerConn) RemoteAddr() net.Addr {
	return &wsAddr{addr: c.remoteAddr}
}

// SetDeadline 实现 net.Conn
func (c *WebSocketServerConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline 实现 net.Conn
func (c *WebSocketServerConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline 实现 net.Conn
func (c *WebSocketServerConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// wsAddr 实现 net.Addr
type wsAddr struct {
	addr string
}

func (a *wsAddr) Network() string {
	return "websocket"
}

func (a *wsAddr) String() string {
	return a.addr
}

// ============================================================================
// 流模式支持（用于跨节点隧道数据转发）
// ============================================================================

// SetStreamMode 设置流模式
func (c *WebSocketServerConn) SetStreamMode(streamMode bool) {
	c.streamModeMu.Lock()
	defer c.streamModeMu.Unlock()
	c.streamMode = streamMode
}

// IsStreamMode 检查是否在流模式
func (c *WebSocketServerConn) IsStreamMode() bool {
	c.streamModeMu.RLock()
	defer c.streamModeMu.RUnlock()
	return c.streamMode
}

// SetConnectionID 设置连接ID（用于调试）
func (c *WebSocketServerConn) SetConnectionID(connID string) {
	c.connectionID = connID
}

// GetConnectionID 获取连接ID
func (c *WebSocketServerConn) GetConnectionID() string {
	return c.connectionID
}

// ReadAvailable 读取可用数据（不等待完整长度）- 实现 StreamDataForwarder 接口
func (c *WebSocketServerConn) ReadAvailable(maxLength int) ([]byte, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	select {
	case <-c.closed:
		return nil, io.EOF
	default:
	}

	// 如果有缓冲数据，先返回
	if len(c.readBuf) > 0 {
		readLen := len(c.readBuf)
		if readLen > maxLength {
			readLen = maxLength
		}
		data := make([]byte, readLen)
		copy(data, c.readBuf[:readLen])
		c.readBuf = c.readBuf[readLen:]
		return data, nil
	}

	// 检查是否在流模式
	c.streamModeMu.RLock()
	isStreamMode := c.streamMode
	c.streamModeMu.RUnlock()

	if isStreamMode {
		// 流模式：从数据通道读取（带超时）
		select {
		case <-c.closed:
			return nil, io.EOF
		case data, ok := <-c.streamDataChan:
			if !ok {
				return nil, io.EOF
			}
			readLen := len(data)
			if readLen > maxLength {
				readLen = maxLength
				// 将多余的数据放回缓冲区
				c.readBuf = append(c.readBuf, data[readLen:]...)
			}
			return data[:readLen], nil
		case <-time.After(5 * time.Second):
			// 超时，返回空数据（不是错误）
			return nil, nil
		}
	}

	// 非流模式：直接读取 WebSocket 消息
	messageType, data, err := c.conn.ReadMessage()
	if err != nil {
		select {
		case <-c.closed:
			return nil, io.EOF
		default:
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil, io.EOF
			}
			return nil, err
		}
	}

	if messageType != websocket.BinaryMessage {
		return nil, nil // 忽略非二进制消息
	}

	readLen := len(data)
	if readLen > maxLength {
		readLen = maxLength
		// 将多余的数据放回缓冲区
		c.readBuf = append(c.readBuf, data[readLen:]...)
	}
	return data[:readLen], nil
}

// ReadExact 读取指定长度的数据 - 实现 StreamDataForwarder 接口
func (c *WebSocketServerConn) ReadExact(length int) ([]byte, error) {
	result := make([]byte, 0, length)

	for len(result) < length {
		data, err := c.ReadAvailable(length - len(result))
		if err != nil {
			if len(result) > 0 {
				return result, err
			}
			return nil, err
		}
		if len(data) == 0 {
			// 超时，继续等待
			continue
		}
		result = append(result, data...)
	}

	return result, nil
}

// WriteExact 写入数据 - 实现 StreamDataForwarder 接口
func (c *WebSocketServerConn) WriteExact(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	select {
	case <-c.closed:
		return io.ErrClosedPipe
	default:
	}

	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// PushStreamData 推送数据到流模式通道（由 WebSocket 消息读取循环调用）
func (c *WebSocketServerConn) PushStreamData(data []byte) error {
	select {
	case <-c.closed:
		return io.ErrClosedPipe
	case c.streamDataChan <- data:
		return nil
	default:
		// 通道满了，阻塞等待
		select {
		case <-c.closed:
			return io.ErrClosedPipe
		case c.streamDataChan <- data:
			return nil
		}
	}
}

// StartStreamModeReader 启动流模式读取器（在切换到流模式后调用）
// 这个方法会持续读取 WebSocket 消息并推送到数据通道
func (c *WebSocketServerConn) StartStreamModeReader() {
	go func() {
		corelog.Debugf("WebSocket[%s]: StartStreamModeReader started", c.connectionID)
		defer func() {
			corelog.Debugf("WebSocket[%s]: StartStreamModeReader exited", c.connectionID)
			// 关闭数据通道，让 Read() 返回 EOF（只关闭一次）
			c.streamDataChanOnce.Do(func() {
				close(c.streamDataChan)
			})
		}()

		for {
			select {
			case <-c.closed:
				return
			default:
			}

			// 读取 WebSocket 消息
			messageType, data, err := c.conn.ReadMessage()
			if err != nil {
				select {
				case <-c.closed:
					return
				default:
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						corelog.Debugf("WebSocket[%s]: connection closed normally", c.connectionID)
						return
					}
					corelog.Errorf("WebSocket[%s]: read error in stream mode: %v", c.connectionID, err)
					return
				}
			}

			if messageType != websocket.BinaryMessage {
				corelog.Debugf("WebSocket[%s]: ignoring non-binary message type: %d", c.connectionID, messageType)
				continue // 忽略非二进制消息
			}

			corelog.Debugf("WebSocket[%s]: read %d bytes in stream mode", c.connectionID, len(data))

			// 推送到数据通道
			if err := c.PushStreamData(data); err != nil {
				corelog.Errorf("WebSocket[%s]: failed to push data: %v", c.connectionID, err)
				return
			}
		}
	}()
}
