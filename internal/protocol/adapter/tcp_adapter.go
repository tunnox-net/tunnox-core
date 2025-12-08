package adapter

import (
	"context"
	"io"
	"net"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/errors"
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
	t := &TcpAdapter{
		BaseAdapter: BaseAdapter{
			ResourceBase: dispose.NewResourceBase("TcpAdapter"),
		},
	}
	t.Initialize(parentCtx)
	t.AddCleanHandler(t.onClose)
	t.SetName("tcp")
	t.SetSession(session)
	t.SetProtocolAdapter(t) // 设置协议适配器引用
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
		return nil, errors.New(errors.ErrorTypePermanent, "TCP listener not initialized")
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
