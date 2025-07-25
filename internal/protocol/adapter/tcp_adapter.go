package adapter

import (
	"context"
	"fmt"
	"io"
	"net"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
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
	return t
}

func (t *TcpAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("TCP listener not initialized")
	}
	conn, err := t.listener.Accept()
	if err != nil {
		return nil, err
	}
	return &TcpConn{Conn: conn}, nil
}

func (t *TcpAdapter) getConnectionType() string {
	return "TCP"
}

// ListenFrom 重写BaseAdapter的ListenFrom方法
func (t *TcpAdapter) ListenFrom(listenAddr string) error {
	t.SetAddr(listenAddr)
	if t.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	utils.Infof("TcpAdapter.ListenFrom called for adapter: %s, type: %T", t.Name(), t)

	// 直接使用自身作为ProtocolAdapter
	if err := t.Listen(t.Addr()); err != nil {
		return fmt.Errorf("failed to listen on %s: %w", t.getConnectionType(), err)
	}

	t.active = true
	go t.acceptLoop(t)
	return nil
}

// ConnectTo 重写BaseAdapter的ConnectTo方法
func (t *TcpAdapter) ConnectTo(serverAddr string) error {
	t.connMutex.Lock()
	defer t.connMutex.Unlock()

	if t.stream != nil {
		return fmt.Errorf("already connected")
	}

	// 直接使用自身作为ProtocolAdapter
	conn, err := t.Dial(serverAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s server: %w", t.getConnectionType(), err)
	}

	t.SetAddr(serverAddr)

	t.streamMutex.Lock()
	t.stream = stream.NewStreamProcessor(conn, conn, t.Ctx())
	t.streamMutex.Unlock()

	return nil
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
