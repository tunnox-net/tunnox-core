package adapter

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
	"tunnox-core/internal/protocol/session"
	udpprotocol "tunnox-core/internal/protocol/udp"
)

// UdpConn UDP连接包装器
type UdpConn struct {
	*udpprotocol.Transport
}

func (u *UdpConn) Close() error {
	return u.Transport.Close()
}

// UdpAdapter UDP协议适配器
// 只实现协议相关方法，其余继承 BaseAdapter
type UdpAdapter struct {
	BaseAdapter
	listener   *net.UDPConn
	remoteAddr *net.UDPAddr // 用于客户端连接
}

// NewUdpAdapter 创建 UDP 适配器
func NewUdpAdapter(parentCtx context.Context, session session.Session) *UdpAdapter {
	u := &UdpAdapter{}
	u.BaseAdapter = BaseAdapter{} // 初始化 BaseAdapter
	u.SetName("udp")
	u.SetSession(session)
	u.SetCtx(parentCtx, u.onClose)
	u.SetProtocolAdapter(u) // 设置协议适配器引用

	return u
}

// Dial 建立 UDP 连接
func (u *UdpAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial UDP: %w", err)
	}

	// 生成 SessionID（简化实现，使用随机数）
	sessionID := u.generateSessionID()

	// 创建 Transport
	transport := udpprotocol.NewTransport(conn, udpAddr, sessionID, u.Ctx())

	return &UdpConn{Transport: transport}, nil
}

// Listen 启动 UDP 监听
func (u *UdpAdapter) Listen(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	listener, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen UDP: %w", err)
	}

	u.listener = listener
	return nil
}

// Accept 接受 UDP 连接
// 注意：UDP 是无连接的，这里每次 Accept 返回一个新的 Transport
// 实际使用中，需要根据数据报的源地址来区分不同的会话
// 简化实现：读取第一个数据报来确定远程地址和 SessionID
func (u *UdpAdapter) Accept() (io.ReadWriteCloser, error) {
	if u.listener == nil {
		return nil, fmt.Errorf("UDP listener not initialized")
	}

	// UDP 是无连接的，需要从第一个数据报中获取源地址
	// 设置读取超时，避免永久阻塞
	if err := u.listener.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, fmt.Errorf("failed to set read deadline: %w", err)
	}

	buf := make([]byte, udpprotocol.MaxUDPPayloadSize)
	n, remoteAddr, err := u.listener.ReadFromUDP(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read from UDP: %w", err)
	}

	// 解析头部获取 SessionID
	if n < udpprotocol.HeaderLength() {
		return nil, fmt.Errorf("packet too small: %d bytes", n)
	}

	header, _, err := udpprotocol.DecodeHeader(buf[:n])
	if err != nil {
		return nil, fmt.Errorf("failed to decode header: %w", err)
	}

	// 为这个会话创建独立的 UDPConn
	// 注意：每个会话需要独立的 Transport，但共享同一个 listener
	// 这里创建一个新的 UDPConn，但它会使用同一个 listener
	// 实际实现中，Transport 的 receiver 会从 listener 读取数据
	// 需要根据 remoteAddr 和 SessionID 来过滤数据报
	
	// 创建新的 Transport，传递第一个数据报
	transport := udpprotocol.NewTransport(u.listener, remoteAddr, header.SessionID, u.Ctx(), buf[:n])

	return &UdpConn{Transport: transport}, nil
}

// getConnectionType 返回连接类型
func (u *UdpAdapter) getConnectionType() string {
	return "UDP"
}

// generateSessionID 生成随机的 SessionID
func (u *UdpAdapter) generateSessionID() uint32 {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// 如果随机数生成失败，使用时间戳
		return uint32(time.Now().Unix())
	}
	return binary.BigEndian.Uint32(buf[:])
}

// onClose UDP 特定的资源清理
func (u *UdpAdapter) onClose() error {
	var err error
	if u.listener != nil {
		err = u.listener.Close()
		u.listener = nil
	}
	baseErr := u.BaseAdapter.onClose()
	if err != nil {
		return err
	}
	return baseErr
}

