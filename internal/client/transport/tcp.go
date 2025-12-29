// Package transport TCP 传输层实现
// TCP 是基础协议，始终编译
package transport

import (
	"context"
	"net"
	"time"
)

func init() {
	RegisterProtocol("tcp", 30, DialTCP) // 优先级 30（中等）
}

// DialTCP 建立 TCP 连接
func DialTCP(ctx context.Context, address string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}

	// 设置 KeepAlive
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	return conn, nil
}
