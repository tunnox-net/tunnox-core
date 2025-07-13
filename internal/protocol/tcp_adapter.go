package protocol

import (
	"context"
	"net"
	"tunnox-core/internal/stream"
)

// TcpConn TCP连接包装器
type TcpConn struct {
	net.Conn
}

func (t *TcpConn) Close() error {
	return t.Conn.Close()
}

// TcpListener TCP监听器包装器
type TcpListener struct {
	net.Listener
}

func (t *TcpListener) Accept() (ProtocolConn, error) {
	conn, err := t.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &TcpConn{Conn: conn}, nil
}

// TcpDialer TCP连接器
type TcpDialer struct{}

func (t *TcpDialer) Dial(addr string) (ProtocolConn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &TcpConn{Conn: conn}, nil
}

type TcpAdapter struct {
	BaseAdapter
	listener net.Listener
}

func NewTcpAdapter(parentCtx context.Context, session Session) *TcpAdapter {
	t := &TcpAdapter{}
	t.SetName("tcp")
	t.SetSession(session)
	t.SetCtx(parentCtx, t.onClose)
	return t
}

// 实现 ProtocolAdapter 接口
func (t *TcpAdapter) createDialer() ProtocolDialer {
	return &TcpDialer{}
}

func (t *TcpAdapter) createListener(addr string) (ProtocolListener, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	t.listener = listener
	return &TcpListener{Listener: listener}, nil
}

func (t *TcpAdapter) handleProtocolSpecific(conn ProtocolConn) error {
	// TCP 特定的 echo 处理
	ctx, cancel := context.WithCancel(t.Ctx())
	defer cancel()
	ps := stream.NewStreamProcessor(conn, conn, ctx)
	defer ps.Close()

	buf := make([]byte, 1024)
	for {
		n, err := ps.GetReader().Read(buf)
		if err != nil {
			break
		}
		if n > 0 {
			if _, err := ps.GetWriter().Write(buf[:n]); err != nil {
				break
			}
		}
	}
	return nil
}

func (t *TcpAdapter) getConnectionType() string {
	return "TCP"
}

// 重写 ConnectTo 和 ListenFrom 以使用 BaseAdapter 的通用逻辑
func (t *TcpAdapter) ConnectTo(serverAddr string) error {
	return t.BaseAdapter.ConnectTo(t, serverAddr)
}

func (t *TcpAdapter) ListenFrom(listenAddr string) error {
	return t.BaseAdapter.ListenFrom(t, listenAddr)
}

// onClose TCP 特定的资源清理
func (t *TcpAdapter) onClose() {
	if t.listener != nil {
		_ = t.listener.Close()
		t.listener = nil
	}
	t.BaseAdapter.onClose()
}
