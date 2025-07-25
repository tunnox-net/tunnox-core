package adapter

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
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
// 只实现协议相关方法，其余继承 BaseAdapter

type UdpAdapter struct {
	BaseAdapter
	conn net.PacketConn
}

func NewUdpAdapter(parentCtx context.Context, session session.Session) *UdpAdapter {
	adapter := &UdpAdapter{}
	adapter.BaseAdapter = BaseAdapter{} // 初始化 BaseAdapter
	adapter.SetName("udp")
	adapter.SetSession(session)
	adapter.SetCtx(parentCtx, adapter.onClose)
	return adapter
}

func (u *UdpAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to UDP server: %w", err)
	}
	return &UdpConn{conn: conn, addr: udpAddr}, nil
}

func (u *UdpAdapter) Listen(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}
	u.conn = conn
	return nil
}

func (u *UdpAdapter) Accept() (io.ReadWriteCloser, error) {
	if u.conn == nil {
		return nil, fmt.Errorf("UDP listener not initialized")
	}
	err := u.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}
	buffer := make([]byte, 1024)
	n, addr, err := u.conn.ReadFrom(buffer)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, &TimeoutError{Protocol: "UDP packet"}
		}
		return nil, err
	}
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

// ListenFrom 重写BaseAdapter的ListenFrom方法
func (u *UdpAdapter) ListenFrom(listenAddr string) error {
	u.SetAddr(listenAddr)
	if u.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	utils.Infof("UdpAdapter.ListenFrom called for adapter: %s, type: %T", u.Name(), u)

	// 直接使用自身作为ProtocolAdapter
	if err := u.Listen(u.Addr()); err != nil {
		return fmt.Errorf("failed to listen on %s: %w", u.getConnectionType(), err)
	}

	u.active = true
	go u.acceptLoop(u)
	return nil
}

// ConnectTo 重写BaseAdapter的ConnectTo方法
func (u *UdpAdapter) ConnectTo(serverAddr string) error {
	u.connMutex.Lock()
	defer u.connMutex.Unlock()

	if u.stream != nil {
		return fmt.Errorf("already connected")
	}

	// 直接使用自身作为ProtocolAdapter
	conn, err := u.Dial(serverAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s server: %w", u.getConnectionType(), err)
	}

	u.SetAddr(serverAddr)

	u.streamMutex.Lock()
	u.stream = stream.NewStreamProcessor(conn, conn, u.Ctx())
	u.streamMutex.Unlock()

	return nil
}

// onClose UDP 特定的资源清理
func (u *UdpAdapter) onClose() error {
	var err error
	if u.conn != nil {
		err = u.conn.Close()
		u.conn = nil
	}
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
