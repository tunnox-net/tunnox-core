package mapping

import (
	"io"

	"tunnox-core/internal/config"
)

// MappingAdapter 映射协议适配器接口
// 不同协议（TCP/UDP/SOCKS5）需要实现此接口
type MappingAdapter interface {
	// StartListener 启动监听
	// 协议特定的监听逻辑，例如：
	// - TCP: net.Listen("tcp", addr)
	// - UDP: net.ListenPacket("udp", addr)
	// - SOCKS5: net.Listen("tcp", addr) + SOCKS5 server
	StartListener(config config.MappingConfig) error

	// Accept 接受连接
	// 返回一个可读写的连接对象
	// 对于无连接协议（UDP），返回虚拟连接
	Accept() (io.ReadWriteCloser, error)

	// PrepareConnection 连接预处理（可选）
	// 某些协议需要在数据传输前进行握手
	// 例如：SOCKS5需要处理握手和认证
	// TCP协议可以直接返回nil
	PrepareConnection(conn io.ReadWriteCloser) error

	// GetProtocol 获取协议名称
	// 返回 "tcp", "udp", "socks5" 等
	GetProtocol() string

	// Close 关闭资源
	// 关闭监听器、清理会话等
	Close() error
}

