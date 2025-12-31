//go:build !no_websocket

// Package transport WebSocket 传输层实现
// 提供 WebSocket 协议的网络连接封装
// 使用 -tags no_websocket 可以排除此协议以减小二进制体积
package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"tunnox-core/internal/cloud/constants"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"

	"github.com/gorilla/websocket"
)

func init() {
	RegisterProtocol("websocket", 10, DialWebSocket) // 优先级 10（最高）
}

// WebSocketStreamConn wraps a WebSocket connection to implement net.Conn interface
// for use with StreamProcessor
type WebSocketStreamConn struct {
	conn       *websocket.Conn
	readBuf    []byte
	readMu     sync.Mutex
	writeMu    sync.Mutex
	closeOnce  sync.Once
	closed     chan struct{}
	localAddr  net.Addr
	remoteAddr net.Addr
}

// NewWebSocketStreamConn creates a new WebSocket stream connection
func NewWebSocketStreamConn(wsURL string) (*WebSocketStreamConn, error) {
	corelog.Debugf("WebSocket: connecting to %s", wsURL)

	dialer := websocket.Dialer{
		HandshakeTimeout: 20 * time.Second,
		ReadBufferSize:   constants.WebSocketBufferSize,
		WriteBufferSize:  constants.WebSocketBufferSize,
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "websocket dial failed")
	}

	corelog.Infof("WebSocket: connected to %s", wsURL)

	// Set read/write deadlines to prevent hanging
	conn.SetReadDeadline(time.Time{})
	conn.SetWriteDeadline(time.Time{})

	wsc := &WebSocketStreamConn{
		conn:       conn,
		readBuf:    make([]byte, 0),
		closed:     make(chan struct{}),
		localAddr:  &wsAddr{addr: "websocket-local"},
		remoteAddr: &wsAddr{addr: wsURL},
	}

	return wsc, nil
}

// Read implements io.Reader
func (c *WebSocketStreamConn) Read(p []byte) (int, error) {
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
			return 0, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "websocket read failed")
		}
	}

	if messageType != websocket.BinaryMessage {
		return 0, coreerrors.Newf(coreerrors.CodeProtocolError, "unexpected websocket message type: %d", messageType)
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
func (c *WebSocketStreamConn) Write(p []byte) (int, error) {
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
		return 0, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "websocket write failed")
	}

	return len(p), nil
}

// Close implements io.Closer
func (c *WebSocketStreamConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)

		// Send close message
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		c.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))

		err = c.conn.Close()
		corelog.Debugf("WebSocket: connection closed")
	})
	return err
}

// LocalAddr implements net.Conn
func (c *WebSocketStreamConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr implements net.Conn
func (c *WebSocketStreamConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline implements net.Conn
func (c *WebSocketStreamConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline implements net.Conn
func (c *WebSocketStreamConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn
func (c *WebSocketStreamConn) SetWriteDeadline(t time.Time) error {
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

// NormalizeWebSocketURL 规范化 WebSocket URL，支持多种格式：
// - https://gw.tunnox.net/_tunnox -> wss://gw.tunnox.net/_tunnox
// - http://gw.tunnox.net/_tunnox -> ws://gw.tunnox.net/_tunnox
// - ws://gw.tunnox.net/_tunnox -> ws://gw.tunnox.net/_tunnox
// - wss://gw.tunnox.net/_tunnox -> wss://gw.tunnox.net/_tunnox
// - ws://gw.tunnox.net -> ws://gw.tunnox.net/_tunnox (添加默认路径)
func NormalizeWebSocketURL(address string) (string, error) {
	// 如果地址已经包含协议，直接解析
	if strings.HasPrefix(address, "ws://") || strings.HasPrefix(address, "wss://") ||
		strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		parsedURL, err := url.Parse(address)
		if err != nil {
			return "", coreerrors.Wrap(err, coreerrors.CodeInvalidParam, "invalid URL format")
		}

		// 转换 HTTP/HTTPS 为 WS/WSS
		scheme := strings.ToLower(parsedURL.Scheme)
		if scheme == "http" {
			scheme = "ws"
		} else if scheme == "https" {
			scheme = "wss"
		}

		// 如果没有路径或路径为空，使用默认路径
		path := parsedURL.Path
		if path == "" {
			path = "/_tunnox"
		}

		// 重建 URL
		wsURL := fmt.Sprintf("%s://%s%s", scheme, parsedURL.Host, path)
		if parsedURL.RawQuery != "" {
			wsURL += "?" + parsedURL.RawQuery
		}
		return wsURL, nil
	}

	// 如果地址不包含协议，假设是 host:port 格式
	// 检查是否包含路径
	if strings.Contains(address, "/") {
		// 包含路径，添加 ws:// 前缀
		return fmt.Sprintf("ws://%s", address), nil
	}

	// 不包含路径，添加默认路径
	return fmt.Sprintf("ws://%s/_tunnox", address), nil
}

// DialWebSocket creates a WebSocket connection to the server
// address 可以是多种格式：
// - https://gw.tunnox.net/_tunnox
// - ws://gw.tunnox.net/_tunnox
// - wss://gw.tunnox.net/_tunnox
// - ws://gw.tunnox.net (会自动添加 /_tunnox 路径)
// - gw.tunnox.net:8080 (会自动添加 ws:// 前缀和 /_tunnox 路径)
func DialWebSocket(ctx context.Context, address string) (net.Conn, error) {
	wsURL, err := NormalizeWebSocketURL(address)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to normalize WebSocket URL")
	}
	return NewWebSocketStreamConn(wsURL)
}
