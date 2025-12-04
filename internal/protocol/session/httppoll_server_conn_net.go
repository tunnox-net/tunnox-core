package session

import (
	"net"
	"time"
)

// Close 实现 io.Closer
func (c *ServerHTTPLongPollingConn) Close() error {
	return c.Dispose.CloseWithError()
}

// LocalAddr 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline 实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// httppollServerAddr 实现 net.Addr 接口
type httppollServerAddr struct {
	network string
	addr    string
}

func (a *httppollServerAddr) Network() string {
	return a.network
}

func (a *httppollServerAddr) String() string {
	return a.addr
}

