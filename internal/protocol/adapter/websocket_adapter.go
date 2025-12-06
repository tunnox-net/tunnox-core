package adapter

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"

	"github.com/gorilla/websocket"
)

// WebSocketAdapter handles WebSocket connections
type WebSocketAdapter struct {
	BaseAdapter
	server   *http.Server
	upgrader websocket.Upgrader
	connChan chan io.ReadWriteCloser
	mu       sync.Mutex
	closed   bool
}

// NewWebSocketAdapter creates a new WebSocket adapter
func NewWebSocketAdapter(parentCtx context.Context, sess session.Session) *WebSocketAdapter {
	adapter := &WebSocketAdapter{
		BaseAdapter: BaseAdapter{},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  64 * 1024,
			WriteBufferSize: 64 * 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for now
			},
		},
		connChan: make(chan io.ReadWriteCloser, 100),
	}

	adapter.SetName("websocket")
	adapter.SetSession(sess)
	adapter.SetCtx(parentCtx, adapter.onClose)

	return adapter
}

// handleWebSocket handles WebSocket upgrade and connection
func (a *WebSocketAdapter) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	utils.Debugf("WebSocket: upgrade request from %s", r.RemoteAddr)

	// Upgrade HTTP connection to WebSocket
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		utils.Errorf("WebSocket: upgrade failed: %v", err)
		return
	}

	utils.Infof("WebSocket: connection established from %s", r.RemoteAddr)

	// Wrap WebSocket connection
	wsConn := &WebSocketServerConn{
		conn:       conn,
		remoteAddr: r.RemoteAddr,
		closed:     make(chan struct{}),
	}

	// Send to accept channel
	select {
	case a.connChan <- wsConn:
		utils.Debugf("WebSocket: connection queued for acceptance")
	case <-a.Ctx().Done():
		wsConn.Close()
		return
	case <-time.After(5 * time.Second):
		utils.Errorf("WebSocket: connection queue full, rejecting")
		wsConn.Close()
		return
	}
}

// ListenFrom starts the WebSocket server on the given address
func (a *WebSocketAdapter) ListenFrom(addr string) error {
	utils.Infof("WebSocket adapter starting on %s", addr)

	// 设置地址到 BaseAdapter
	a.SetAddr(addr)

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/_tunnox", a.handleWebSocket)

	a.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			utils.Errorf("WebSocket server error: %v", err)
		}
	}()

	// Start handling connections
	go a.handleConnections()

	utils.Infof("WebSocket adapter started on %s/_tunnox", addr)
	return nil
}

// handleConnections processes incoming WebSocket connections
func (a *WebSocketAdapter) handleConnections() {
	for {
		conn, err := a.Accept()
		if err != nil {
			select {
			case <-a.Ctx().Done():
				return
			default:
				utils.Errorf("WebSocket: accept error: %v", err)
				continue
			}
		}

		go a.handleConnection(a, conn)
	}
}

// Dial is not supported for WebSocket adapter (server-side only)
func (a *WebSocketAdapter) Dial(address string) (io.ReadWriteCloser, error) {
	return nil, fmt.Errorf("dial not supported for WebSocket adapter")
}

// Listen is not used for WebSocket adapter (uses HTTP server instead)
func (a *WebSocketAdapter) Listen(address string) error {
	return fmt.Errorf("listen not supported for WebSocket adapter")
}

// getConnectionType returns the connection type for this adapter
func (a *WebSocketAdapter) getConnectionType() string {
	return "websocket"
}

// Accept accepts a new WebSocket connection
func (a *WebSocketAdapter) Accept() (io.ReadWriteCloser, error) {
	select {
	case conn := <-a.connChan:
		return conn, nil
	case <-a.Ctx().Done():
		return nil, fmt.Errorf("websocket adapter closed")
	}
}

// onClose handles cleanup when the adapter is closed
func (a *WebSocketAdapter) onClose() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()

	utils.Infof("WebSocket adapter closing")

	// Shutdown HTTP server
	if a.server != nil {
		// 使用 WebSocketAdapter 的 context 作为父 context，确保能接收退出信号
		ctx, cancel := context.WithTimeout(a.Ctx(), 5*time.Second)
		defer cancel()

		if err := a.server.Shutdown(ctx); err != nil {
			utils.Errorf("WebSocket server shutdown error: %v", err)
		}
	}

	close(a.connChan)

	return nil
}

// WebSocketServerConn wraps a WebSocket connection for server side
type WebSocketServerConn struct {
	conn       *websocket.Conn
	remoteAddr string
	readBuf    []byte
	readMu     sync.Mutex
	writeMu    sync.Mutex
	closeOnce  sync.Once
	closed     chan struct{}
}

// Read implements io.Reader
func (c *WebSocketServerConn) Read(p []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	select {
	case <-c.closed:
		return 0, io.EOF
	default:
	}

	// If we have buffered data, return it first
	if len(c.readBuf) > 0 {
		n := copy(p, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	// Read next WebSocket message
	messageType, data, err := c.conn.ReadMessage()
	if err != nil {
		select {
		case <-c.closed:
			return 0, io.EOF
		default:
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return 0, io.EOF
			}
			return 0, fmt.Errorf("websocket read failed: %w", err)
		}
	}

	if messageType != websocket.BinaryMessage {
		return 0, fmt.Errorf("unexpected websocket message type: %d", messageType)
	}

	// Copy data to output buffer
	n := copy(p, data)

	// If we couldn't fit all data, buffer the rest
	if n < len(data) {
		c.readBuf = append(c.readBuf, data[n:]...)
	}

	return n, nil
}

// Write implements io.Writer
func (c *WebSocketServerConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	select {
	case <-c.closed:
		return 0, io.ErrClosedPipe
	default:
	}

	// Send as binary message
	err := c.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, fmt.Errorf("websocket write failed: %w", err)
	}

	return len(p), nil
}

// Close implements io.Closer
func (c *WebSocketServerConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)

		// Send close message
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		c.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))

		err = c.conn.Close()
		utils.Debugf("WebSocket: server connection closed")
	})
	return err
}

// LocalAddr implements net.Conn
func (c *WebSocketServerConn) LocalAddr() net.Addr {
	return &wsAddr{addr: "websocket-server"}
}

// RemoteAddr implements net.Conn
func (c *WebSocketServerConn) RemoteAddr() net.Addr {
	return &wsAddr{addr: c.remoteAddr}
}

// SetDeadline implements net.Conn
func (c *WebSocketServerConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline implements net.Conn
func (c *WebSocketServerConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn
func (c *WebSocketServerConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// wsAddr implements net.Addr for WebSocket connections
type wsAddr struct {
	addr string
}

func (a *wsAddr) Network() string {
	return "websocket"
}

func (a *wsAddr) String() string {
	return a.addr
}
