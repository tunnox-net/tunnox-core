package client

import (
	"net"
	"time"

	corelog "tunnox-core/internal/core/log"
)

// KeepAliveConn 支持 KeepAlive 的连接接口
type KeepAliveConn interface {
	net.Conn
	SetKeepAlive(keepalive bool) error
	SetKeepAlivePeriod(d time.Duration) error
	SetNoDelay(noDelay bool) error
}

// SetKeepAliveIfSupported 如果连接支持 KeepAlive，则设置它
// 注意：这些是可选的 TCP 优化设置，失败不影响连接功能，仅记录 debug 日志
func SetKeepAliveIfSupported(conn net.Conn, keepalive bool) {
	keepAliveConn, ok := conn.(KeepAliveConn)
	if !ok {
		return
	}

	// KeepAlive 设置失败不影响连接功能，仅用于检测死连接
	if err := keepAliveConn.SetKeepAlive(keepalive); err != nil {
		// 某些连接类型（如 QUIC）可能不支持此选项，记录 debug 日志
		corelog.Debugf("SetKeepAlive failed (non-critical): %v", err)
	}

	// KeepAlivePeriod 设置失败同样不影响连接功能
	if err := keepAliveConn.SetKeepAlivePeriod(30 * time.Second); err != nil {
		// 某些平台可能不支持自定义周期，记录 debug 日志
		corelog.Debugf("SetKeepAlivePeriod failed (non-critical): %v", err)
	}

	// NoDelay 设置失败不影响数据传输，仅影响小包延迟
	if err := keepAliveConn.SetNoDelay(true); err != nil {
		// 某些连接类型可能不支持此选项，记录 debug 日志
		corelog.Debugf("SetNoDelay failed (non-critical): %v", err)
	}
}
