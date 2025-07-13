package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
)

// TcpConn TCP连接包装器
type TcpConn struct {
	net.Conn
}

func (t *TcpConn) Close() error {
	return t.Conn.Close()
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

// Dial 实现连接功能
func (t *TcpAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &TcpConn{Conn: conn}, nil
}

// Listen 实现监听功能
func (t *TcpAdapter) Listen(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	t.listener = listener
	return nil
}

// Accept 实现接受连接功能
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

// 重写 ConnectTo 和 ListenFrom 以使用 BaseAdapter 的通用逻辑
func (t *TcpAdapter) ConnectTo(serverAddr string) error {
	return t.BaseAdapter.ConnectTo(t, serverAddr)
}

func (t *TcpAdapter) ListenFrom(listenAddr string) error {
	return t.BaseAdapter.ListenFrom(t, listenAddr)
}

// onClose TCP 特定的资源清理
func (t *TcpAdapter) onClose() error {
	if t.listener != nil {
		_ = t.listener.Close()
		t.listener = nil
	}
	return t.BaseAdapter.onClose()
}
