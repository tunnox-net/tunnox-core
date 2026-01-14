// Package websocket 提供 WebSocket 传输模块
package websocket

import (
	"io"
	"net"
	"sync"
	"time"

	corelog "tunnox-core/internal/core/log"

	"github.com/gorilla/websocket"
)

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

// GetNetConn 返回底层 net.Conn（自身实现了 net.Conn 接口）
// 这允许 Bridge 正确关闭 WebSocket 连接
func (c *WebSocketServerConn) GetNetConn() net.Conn {
	return c
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
			c.Close()
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
