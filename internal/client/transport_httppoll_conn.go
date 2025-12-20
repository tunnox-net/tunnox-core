package client

import (
	"fmt"
	"net"
	"time"
	corelog "tunnox-core/internal/core/log"
)

func (c *HTTPLongPollingConn) Close() error {
	return c.Dispose.CloseWithError()
}

// LocalAddr 实现 net.Conn 接口
func (c *HTTPLongPollingConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr 实现 net.Conn 接口
func (c *HTTPLongPollingConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline 实现 net.Conn 接口
func (c *HTTPLongPollingConn) SetDeadline(t time.Time) error {
	// HTTP 长轮询不支持设置 deadline
	return nil
}

// SetReadDeadline 实现 net.Conn 接口
func (c *HTTPLongPollingConn) SetReadDeadline(t time.Time) error {
	// HTTP 长轮询不支持设置 read deadline
	return nil
}

// SetWriteDeadline 实现 net.Conn 接口
func (c *HTTPLongPollingConn) SetWriteDeadline(t time.Time) error {
	// HTTP 长轮询不支持设置 write deadline
	return nil
}

// SetStreamMode 切换到流模式（隧道建立后调用）
// 流模式下，直接转发原始数据，不再解析数据包格式
func (c *HTTPLongPollingConn) SetStreamMode(streamMode bool) {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	oldMode := c.streamMode
	c.streamMode = streamMode
	corelog.Infof("HTTP long polling: [SetStreamMode] switching stream mode from %v to %v, clientID=%d, mappingID=%s",
		oldMode, streamMode, c.clientID, c.mappingID)
}

// httppollAddr 实现 net.Addr 接口
type httppollAddr struct {
	network string
	addr    string
}

func (a *httppollAddr) Network() string {
	return a.network
}

func (a *httppollAddr) String() string {
	return a.addr
}

// generateRequestID 生成请求 ID
func generateRequestID() string {
	// 使用时间戳和随机数生成唯一请求ID
	nanos := time.Now().UnixNano()
	// 使用简单的哈希值而不是 binary.BigEndian.Uint64（避免索引越界）
	hash := uint64(0)
	for _, b := range []byte("request") {
		hash = hash*31 + uint64(b)
	}
	return fmt.Sprintf("%d-%d", nanos, hash)
}
