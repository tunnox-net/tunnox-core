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
	"tunnox-core/internal/httpservice"
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
	corelog.Debugf("WebSocketModule: upgrade request from %s", r.RemoteAddr)

	// 升级 HTTP 连接为 WebSocket
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		corelog.Errorf("WebSocketModule: upgrade failed: %v", err)
		return
	}

	corelog.Infof("WebSocketModule: connection established from %s", r.RemoteAddr)

	// 包装 WebSocket 连接
	wsConn := &WebSocketServerConn{
		conn:       conn,
		remoteAddr: r.RemoteAddr,
		closed:     make(chan struct{}),
	}

	// 如果有会话管理器，处理连接
	if m.session != nil {
		go func() {
			_, err := m.session.AcceptConnection(wsConn, wsConn)
			if err != nil {
				corelog.Errorf("WebSocketModule: failed to accept connection: %v", err)
				wsConn.Close()
			}
		}()
	} else {
		// 没有会话管理器，发送到通道等待处理
		select {
		case m.connChan <- wsConn:
			corelog.Debugf("WebSocketModule: connection queued for acceptance")
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

	// 读取下一个 WebSocket 消息
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
