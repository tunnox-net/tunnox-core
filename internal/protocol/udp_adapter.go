package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
)

// UdpConn UDP连接包装器
type UdpConn struct {
	conn net.PacketConn
	addr net.Addr
}

func (u *UdpConn) Read(p []byte) (n int, err error) {
	n, _, err = u.conn.ReadFrom(p)
	return n, err
}

func (u *UdpConn) Write(p []byte) (n int, err error) {
	if u.addr == nil {
		return 0, fmt.Errorf("no target address set")
	}
	return u.conn.WriteTo(p, u.addr)
}

func (u *UdpConn) Close() error {
	return u.conn.Close()
}

// UdpAdapter UDP协议适配器
type UdpAdapter struct {
	BaseAdapter
	conn net.PacketConn
}

// NewUdpAdapter 创建新的UDP适配器
func NewUdpAdapter(parentCtx context.Context, session Session) *UdpAdapter {
	adapter := &UdpAdapter{}
	adapter.SetName("udp")
	adapter.SetSession(session)
	adapter.SetCtx(parentCtx, adapter.onClose)
	return adapter
}

// Dial 实现连接功能
func (u *UdpAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	// 解析服务器地址
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// 创建UDP连接
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to UDP server: %w", err)
	}

	return &UdpConn{conn: conn, addr: udpAddr}, nil
}

// Listen 实现监听功能
func (u *UdpAdapter) Listen(addr string) error {
	// 解析监听地址
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// 创建UDP监听器
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	u.conn = conn
	return nil
}

// Accept 实现接受连接功能
func (u *UdpAdapter) Accept() (io.ReadWriteCloser, error) {
	if u.conn == nil {
		return nil, fmt.Errorf("UDP listener not initialized")
	}

	// UDP 是面向数据包的，这里应该阻塞等待数据包
	// 设置超时避免无限阻塞
	err := u.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}

	buffer := make([]byte, 1024)
	n, addr, err := u.conn.ReadFrom(buffer)
	if err != nil {
		// 如果是超时错误，返回自定义超时错误
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, &TimeoutError{Protocol: "UDP packet"}
		}
		return nil, err
	}

	// 创建一个虚拟连接来处理这个数据包
	return &UdpVirtualConn{
		data: buffer[:n],
		addr: addr,
		conn: u.conn,
		pos:  0,
	}, nil
}

func (u *UdpAdapter) getConnectionType() string {
	return "UDP"
}

// 重写 ConnectTo 和 ListenFrom 以使用 BaseAdapter 的通用逻辑
func (u *UdpAdapter) ConnectTo(serverAddr string) error {
	return u.BaseAdapter.ConnectTo(u, serverAddr)
}

func (u *UdpAdapter) ListenFrom(listenAddr string) error {
	return u.BaseAdapter.ListenFrom(u, listenAddr)
}

// onClose UDP 特定的资源清理
func (u *UdpAdapter) onClose() error {
	var err error
	if u.conn != nil {
		err = u.conn.Close()
		u.conn = nil
	}

	// 调用基类的清理方法
	baseErr := u.BaseAdapter.onClose()
	if err != nil {
		return err
	}
	return baseErr
}

// UdpVirtualConn UDP虚拟连接，用于单次数据包处理
type UdpVirtualConn struct {
	data []byte
	addr net.Addr
	conn net.PacketConn
	pos  int
}

// Read 实现io.Reader接口
func (u *UdpVirtualConn) Read(p []byte) (n int, err error) {
	if u.pos >= len(u.data) {
		return 0, io.EOF
	}
	copyLen := len(u.data) - u.pos
	if copyLen > len(p) {
		copyLen = len(p)
	}
	copy(p, u.data[u.pos:u.pos+copyLen])
	u.pos += copyLen
	return copyLen, nil
}

// Write 实现io.Writer接口
func (u *UdpVirtualConn) Write(p []byte) (n int, err error) {
	return u.conn.WriteTo(p, u.addr)
}

// Close 实现io.Closer接口
func (u *UdpVirtualConn) Close() error {
	return nil
}
