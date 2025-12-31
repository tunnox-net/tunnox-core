// Package adapter WebSocket 连接包装器
// 提供 WebSocket 连接的 io.ReadWriteCloser 和 net.Conn 接口实现
package adapter

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// =============================================================================
// WebSocket 连接包装器
// =============================================================================

// wsServerConn 服务端 WebSocket 连接包装器
type wsServerConn struct {
	conn       *websocket.Conn
	readBuf    []byte
	readMu     sync.Mutex
	writeMu    sync.Mutex
	closeOnce  sync.Once
	closed     chan struct{}
	remoteAddr string
}

// newWSServerConn 创建服务端连接包装器
func newWSServerConn(conn *websocket.Conn, remoteAddr string) *wsServerConn {
	c := &wsServerConn{
		conn:       conn,
		readBuf:    make([]byte, 0),
		closed:     make(chan struct{}),
		remoteAddr: remoteAddr,
	}

	// 设置 pong 处理器
	conn.SetPongHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(WebSocketPongTimeout))
		return nil
	})

	return c
}

// Read 实现 io.Reader
func (c *wsServerConn) Read(p []byte) (int, error) {
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
			return 0, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "websocket read failed")
		}
	}

	if messageType != websocket.BinaryMessage {
		return 0, coreerrors.Newf(coreerrors.CodeProtocolError,
			"unexpected websocket message type: %d", messageType)
	}

	// 复制数据到输出缓冲区
	n := copy(p, data)

	// 如果数据未完全读取，缓存剩余部分
	if n < len(data) {
		c.readBuf = append(c.readBuf, data[n:]...)
	}

	return n, nil
}

// Write 实现 io.Writer
func (c *wsServerConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	select {
	case <-c.closed:
		return 0, io.ErrClosedPipe
	default:
	}

	// 设置写超时
	c.conn.SetWriteDeadline(time.Now().Add(WebSocketWriteTimeout))

	// 发送二进制消息
	err := c.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "websocket write failed")
	}

	return len(p), nil
}

// Close 实现 io.Closer
func (c *wsServerConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)

		// 发送关闭消息
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		c.conn.SetWriteDeadline(time.Now().Add(time.Second))
		c.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))

		err = c.conn.Close()
		corelog.Debugf("WebSocket: server connection closed, remote=%s", c.remoteAddr)
	})
	return err
}

// LocalAddr 实现 net.Conn 接口
func (c *wsServerConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr 实现 net.Conn 接口
func (c *wsServerConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetDeadline 实现 net.Conn 接口
func (c *wsServerConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline 实现 net.Conn 接口
func (c *wsServerConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline 实现 net.Conn 接口
func (c *wsServerConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// =============================================================================
// 客户端 WebSocket 连接包装器
// =============================================================================

// wsClientConn 客户端 WebSocket 连接包装器
type wsClientConn struct {
	conn      *websocket.Conn
	readBuf   []byte
	readMu    sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once
	closed    chan struct{}
}

// newWSClientConn 创建客户端连接包装器
func newWSClientConn(conn *websocket.Conn) *wsClientConn {
	c := &wsClientConn{
		conn:    conn,
		readBuf: make([]byte, 0),
		closed:  make(chan struct{}),
	}

	// 清除默认超时
	conn.SetReadDeadline(time.Time{})
	conn.SetWriteDeadline(time.Time{})

	return c
}

// Read 实现 io.Reader
func (c *wsClientConn) Read(p []byte) (int, error) {
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
			return 0, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "websocket read failed")
		}
	}

	if messageType != websocket.BinaryMessage {
		return 0, coreerrors.Newf(coreerrors.CodeProtocolError,
			"unexpected websocket message type: %d", messageType)
	}

	// 复制数据到输出缓冲区
	n := copy(p, data)

	// 如果数据未完全读取，缓存剩余部分
	if n < len(data) {
		c.readBuf = append(c.readBuf, data[n:]...)
	}

	return n, nil
}

// Write 实现 io.Writer
func (c *wsClientConn) Write(p []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	select {
	case <-c.closed:
		return 0, io.ErrClosedPipe
	default:
	}

	// 发送二进制消息
	err := c.conn.WriteMessage(websocket.BinaryMessage, p)
	if err != nil {
		return 0, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "websocket write failed")
	}

	return len(p), nil
}

// Close 实现 io.Closer
func (c *wsClientConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)

		// 发送关闭消息
		closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		c.conn.SetWriteDeadline(time.Now().Add(time.Second))
		c.conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(time.Second))

		err = c.conn.Close()
		corelog.Debugf("WebSocket: client connection closed")
	})
	return err
}

// LocalAddr 实现 net.Conn 接口
func (c *wsClientConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr 实现 net.Conn 接口
func (c *wsClientConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetDeadline 实现 net.Conn 接口
func (c *wsClientConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline 实现 net.Conn 接口
func (c *wsClientConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline 实现 net.Conn 接口
func (c *wsClientConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
