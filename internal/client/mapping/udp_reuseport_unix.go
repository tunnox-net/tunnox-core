//go:build !windows

package mapping

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// createReusePortListener 创建支持 SO_REUSEPORT 的 UDP listener
// Unix/Linux/macOS: 使用 SO_REUSEPORT 允许多个 socket 绑定同一端口
//
// 限制: 当前只支持 IPv4 (0.0.0.0)，如需 IPv6 支持请扩展此函数
func createReusePortListener(port int) (net.PacketConn, error) {
	// 创建 socket
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, unix.IPPROTO_UDP)
	if err != nil {
		return nil, fmt.Errorf("failed to create socket: %w", err)
	}

	// 设置 SO_REUSEADDR
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to set SO_REUSEADDR: %w", err)
	}

	// 设置 SO_REUSEPORT - 允许多个进程/线程绑定同一端口
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to set SO_REUSEPORT: %w", err)
	}

	// 设置 OS 级别缓冲区
	_ = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_RCVBUF, udpSocketBufferSize)
	_ = unix.SetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_SNDBUF, udpSocketBufferSize)

	// 绑定到指定端口
	addr := &unix.SockaddrInet4{Port: port}
	if err := unix.Bind(fd, addr); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to bind to port %d: %w", port, err)
	}

	// 设置非阻塞模式
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("failed to set non-blocking: %w", err)
	}

	// 将 fd 转换为 *net.UDPConn
	// 使用 os.NewFile 包装 fd，然后用 net.FilePacketConn 转换
	file := os.NewFile(uintptr(fd), fmt.Sprintf("udp:%d", port))
	conn, err := net.FilePacketConn(file)
	file.Close() // FilePacketConn 会 dup fd，所以这里可以关闭

	if err != nil {
		return nil, fmt.Errorf("failed to create PacketConn from fd: %w", err)
	}

	return conn, nil
}

// supportsReusePort 检查当前平台是否支持 SO_REUSEPORT
func supportsReusePort() bool {
	// Unix/Linux/macOS 都支持 SO_REUSEPORT
	return true
}
