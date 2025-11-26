package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"tunnox-core/internal/utils"
)

// websocketStreamConn wraps a WebSocket connection to implement net.Conn interface
// for use with StreamProcessor
type websocketStreamConn struct {
	conn      *websocket.Conn
	readBuf   []byte
	readMu    sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once
	closed    chan struct{}
	localAddr  net.Addr
	remoteAddr net.Addr
}

// newWebSocketStreamConn creates a new WebSocket stream connection
func newWebSocketStreamConn(wsURL string) (*websocketStreamConn, error) {
	utils.Debugf("WebSocket: connecting to %s", wsURL)
	
	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
		ReadBufferSize:   64 * 1024,
		WriteBufferSize:  64 * 1024,
	}
	
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}
	
	utils.Infof("WebSocket: connected to %s", wsURL)
	
	// Set read/write deadlines to prevent hanging
	conn.SetReadDeadline(time.Time{})
	conn.SetWriteDeadline(time.Time{})
	
	wsc := &websocketStreamConn{
		conn:       conn,
		readBuf:    make([]byte, 0),
		closed:     make(chan struct{}),
		localAddr:  &wsAddr{addr: "websocket-local"},
		remoteAddr: &wsAddr{addr: wsURL},
	}
	
	return wsc, nil
}

// Read implements io.Reader
func (c *websocketStreamConn) Read(p []byte) (int, error) {
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
func (c *websocketStreamConn) Write(p []byte) (int, error) {
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
func (c *websocketStreamConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		
		// Send close message
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		c.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))
		
		err = c.conn.Close()
		utils.Debugf("WebSocket: connection closed")
	})
	return err
}

// LocalAddr implements net.Conn
func (c *websocketStreamConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr implements net.Conn
func (c *websocketStreamConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline implements net.Conn
func (c *websocketStreamConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline implements net.Conn
func (c *websocketStreamConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn
func (c *websocketStreamConn) SetWriteDeadline(t time.Time) error {
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

// dialWebSocket creates a WebSocket connection to the server
func dialWebSocket(ctx context.Context, address, path string) (net.Conn, error) {
	wsURL := fmt.Sprintf("ws://%s%s", address, path)
	return newWebSocketStreamConn(wsURL)
}

