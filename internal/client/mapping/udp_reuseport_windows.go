//go:build windows

package mapping

import (
	"fmt"
	"net"
)

// createReusePortListener Windows 回退实现
// Windows 不支持 SO_REUSEPORT，使用标准的 net.ListenPacket
// 这意味着 Windows 上只能使用单个 listener
func createReusePortListener(port int) (net.PacketConn, error) {
	addr := fmt.Sprintf(":%d", port)
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	// 设置 OS 级别缓冲区
	if udpConn, ok := conn.(*net.UDPConn); ok {
		_ = udpConn.SetReadBuffer(udpSocketBufferSize)
		_ = udpConn.SetWriteBuffer(udpSocketBufferSize)
	}

	return conn, nil
}

// supportsReusePort 检查当前平台是否支持 SO_REUSEPORT
func supportsReusePort() bool {
	// Windows 不支持 SO_REUSEPORT
	return false
}
