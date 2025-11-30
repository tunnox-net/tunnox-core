package client

import (
	"net"
	"time"
)

// KeepAliveConn 支持 KeepAlive 的连接接口
type KeepAliveConn interface {
	net.Conn
	SetKeepAlive(keepalive bool) error
	SetKeepAlivePeriod(d time.Duration) error
	SetNoDelay(noDelay bool) error
}

// SetKeepAliveIfSupported 如果连接支持 KeepAlive，则设置它
func SetKeepAliveIfSupported(conn net.Conn, keepalive bool) {
	if keepAliveConn, ok := conn.(KeepAliveConn); ok {
		_ = keepAliveConn.SetKeepAlive(keepalive)
		_ = keepAliveConn.SetKeepAlivePeriod(30 * time.Second)
		_ = keepAliveConn.SetNoDelay(true)
	}
}
