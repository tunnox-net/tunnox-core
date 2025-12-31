package adapter

import (
	"context"
	"io"
	"net"

	"tunnox-core/internal/cloud/constants"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/protocol/session"
)

// TcpConn TCP连接包装器
type TcpConn struct {
	net.Conn
}

func (t *TcpConn) Close() error {
	return t.Conn.Close()
}

// TcpAdapter TCP协议适配器
// 只实现协议相关方法，其余继承 BaseAdapter

type TcpAdapter struct {
	BaseAdapter
	listener net.Listener
}

func NewTcpAdapter(parentCtx context.Context, session session.Session) *TcpAdapter {
	t := &TcpAdapter{}
	t.BaseAdapter = BaseAdapter{} // 初始化 BaseAdapter
	t.SetName("tcp")
	t.SetSession(session)
	t.SetCtx(parentCtx, t.onClose)
	t.SetProtocolAdapter(t) // 设置协议适配器引用
	return t
}

func (t *TcpAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	// 🚀 性能优化: 设置 TCP 参数
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)                              // 禁用 Nagle 算法，减少延迟
		tcpConn.SetReadBuffer(constants.TCPSocketBufferSize)  // 512KB 读缓冲区
		tcpConn.SetWriteBuffer(constants.TCPSocketBufferSize) // 512KB 写缓冲区
		tcpConn.SetKeepAlive(true)                            // 启用 KeepAlive
	}
	return &TcpConn{Conn: conn}, nil
}

func (t *TcpAdapter) Listen(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	t.listener = listener
	return nil
}

func (t *TcpAdapter) Accept() (io.ReadWriteCloser, error) {
	if t.listener == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "TCP listener not initialized")
	}
	conn, err := t.listener.Accept()
	if err != nil {
		return nil, err
	}
	// 🚀 性能优化: 设置 TCP 参数
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetNoDelay(true)                              // 禁用 Nagle 算法，减少延迟
		tcpConn.SetReadBuffer(constants.TCPSocketBufferSize)  // 512KB 读缓冲区
		tcpConn.SetWriteBuffer(constants.TCPSocketBufferSize) // 512KB 写缓冲区
		tcpConn.SetKeepAlive(true)                            // 启用 KeepAlive
	}
	return &TcpConn{Conn: conn}, nil
}

func (t *TcpAdapter) getConnectionType() string {
	return "TCP"
}

// onClose TCP 特定的资源清理
func (t *TcpAdapter) onClose() error {
	var err error
	if t.listener != nil {
		err = t.listener.Close()
		t.listener = nil
	}
	baseErr := t.BaseAdapter.onClose()
	if err != nil {
		return err
	}
	return baseErr
}
