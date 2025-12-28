package mapping

import (
	"fmt"
	"io"
	"net"
	"time"

	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/config"
	corelog "tunnox-core/internal/core/log"
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
	// 明确使用 IPv4 地址，避免 IPv6 双栈可能的问题
	addr := fmt.Sprintf("127.0.0.1:%d", config.LocalPort)
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	a.listener = listener
	corelog.Debugf("TCPMappingAdapter: listening on %s", addr)
	return nil
}

// Accept 接受TCP连接
func (a *TCPMappingAdapter) Accept() (io.ReadWriteCloser, error) {
	if a.listener == nil {
		corelog.Errorf("TCPMappingAdapter: listener is nil!")
		return nil, fmt.Errorf("TCP listener not initialized")
	}

	addr := a.listener.Addr()
	corelog.Debugf("TCPMappingAdapter: calling listener.Accept() on %v", addr)

	// 设置 Accept 超时（5秒），用于诊断和避免永久阻塞
	if tcpListener, ok := a.listener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Now().Add(5 * time.Second))
	}

	conn, err := a.listener.Accept()

	// 清除超时设置
	if tcpListener, ok := a.listener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Time{})
	}

	if err != nil {
		// 检查是否是超时错误
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			// 超时是正常的，只在 debug 级别记录
			corelog.Debugf("TCPMappingAdapter: Accept() timeout on %v, will retry", addr)
			return nil, err
		}
		corelog.Debugf("TCPMappingAdapter: listener.Accept() returned error: %v", err)
		return nil, err
	}
	corelog.Debugf("TCPMappingAdapter: accepted connection from %v", conn.RemoteAddr())

	// 🚀 性能优化: 设置 TCP 参数
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)                              // 禁用 Nagle 算法
		tcpConn.SetReadBuffer(constants.TCPSocketBufferSize)  // 512KB 读缓冲区
		tcpConn.SetWriteBuffer(constants.TCPSocketBufferSize) // 512KB 写缓冲区
		tcpConn.SetKeepAlive(true)                            // 启用 KeepAlive
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
