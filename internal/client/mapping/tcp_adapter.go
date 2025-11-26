package mapping

import (
	"fmt"
	"io"
	"net"

	"tunnox-core/internal/config"
	"tunnox-core/internal/utils"
)

// TCPMappingAdapter TCP映射适配器
// 只实现TCP协议特定的逻辑
type TCPMappingAdapter struct {
	listener net.Listener
}

// NewTCPMappingAdapter 创建TCP映射适配器
func NewTCPMappingAdapter() *TCPMappingAdapter {
	return &TCPMappingAdapter{}
}

// StartListener 启动TCP监听
func (a *TCPMappingAdapter) StartListener(config config.MappingConfig) error {
	addr := fmt.Sprintf(":%d", config.LocalPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	a.listener = listener
	utils.Debugf("TCPMappingAdapter: listening on %s", addr)
	return nil
}

// Accept 接受TCP连接
func (a *TCPMappingAdapter) Accept() (io.ReadWriteCloser, error) {
	if a.listener == nil {
		return nil, fmt.Errorf("TCP listener not initialized")
	}

	conn, err := a.listener.Accept()
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// PrepareConnection TCP不需要预处理
func (a *TCPMappingAdapter) PrepareConnection(conn io.ReadWriteCloser) error {
	// TCP直接返回nil，无需额外处理
	return nil
}

// GetProtocol 获取协议名称
func (a *TCPMappingAdapter) GetProtocol() string {
	return "tcp"
}

// Close 关闭资源
func (a *TCPMappingAdapter) Close() error {
	if a.listener != nil {
		return a.listener.Close()
	}
	return nil
}

