package adapter

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

const (
	// UDP 相关常量
	udpBufferSize      = 65535            // UDP 最大包大小
	udpReadTimeout     = 1 * time.Second  // UDP 读取超时
	udpSessionTimeout  = 30 * time.Second // UDP 会话超时
	udpCleanupInterval = 10 * time.Second // 清理过期会话的间隔
)

// UdpConn UDP连接包装器，用于客户端连接
type UdpConn struct {
	conn net.PacketConn
	addr net.Addr
	mu   sync.RWMutex
}

func (u *UdpConn) Read(p []byte) (n int, err error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if u.conn == nil {
		return 0, fmt.Errorf("connection closed")
	}

	n, _, err = u.conn.ReadFrom(p)
	return n, err
}

func (u *UdpConn) Write(p []byte) (n int, err error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if u.conn == nil {
		return 0, fmt.Errorf("connection closed")
	}

	if u.addr == nil {
		return 0, fmt.Errorf("no target address set")
	}

	return u.conn.WriteTo(p, u.addr)
}

func (u *UdpConn) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.conn != nil {
		err := u.conn.Close()
		u.conn = nil
		return err
	}
	return nil
}

// UdpAdapter UDP协议适配器
// 只实现协议相关方法，其余继承 BaseAdapter
type UdpAdapter struct {
	BaseAdapter
	conn     net.PacketConn
	sessions map[string]*udpSession
	sessLock sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// udpSession UDP会话，用于管理来自同一客户端的多个数据包
type udpSession struct {
	addr       net.Addr
	lastActive time.Time
	buffer     chan []byte
	conn       net.PacketConn
	mu         sync.RWMutex
}

func newUdpSession(addr net.Addr, conn net.PacketConn) *udpSession {
	return &udpSession{
		addr:       addr,
		lastActive: time.Now(),
		buffer:     make(chan []byte, 100), // 缓冲100个数据包
		conn:       conn,
	}
}

func (s *udpSession) updateActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActive = time.Now()
}

func (s *udpSession) isExpired() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.lastActive) > udpSessionTimeout
}

func NewUdpAdapter(parentCtx context.Context, session session.Session) *UdpAdapter {
	ctx, cancel := context.WithCancel(parentCtx)
	adapter := &UdpAdapter{
		sessions: make(map[string]*udpSession),
		ctx:      ctx,
		cancel:   cancel,
	}
	adapter.BaseAdapter = BaseAdapter{} // 初始化 BaseAdapter
	adapter.SetName("udp")
	adapter.SetSession(session)
	adapter.SetCtx(parentCtx, adapter.onClose)
	adapter.SetProtocolAdapter(adapter) // 设置协议适配器引用
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

	// 设置合理的缓冲区大小
	if err := conn.SetReadBuffer(udpBufferSize); err != nil {
		utils.Warnf("Failed to set UDP read buffer: %v", err)
	}
	if err := conn.SetWriteBuffer(udpBufferSize); err != nil {
		utils.Warnf("Failed to set UDP write buffer: %v", err)
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

	// 设置合理的缓冲区大小
	if err := conn.SetReadBuffer(udpBufferSize); err != nil {
		utils.Warnf("Failed to set UDP read buffer: %v", err)
	}
	if err := conn.SetWriteBuffer(udpBufferSize); err != nil {
		utils.Warnf("Failed to set UDP write buffer: %v", err)
	}

	u.conn = conn

	// 启动会话清理 goroutine
	go u.cleanupSessions()

	// 启动数据包接收 goroutine
	go u.receivePackets()

	utils.Infof("UDP adapter listening on %s", addr)
	return nil
}

// receivePackets 接收并分发 UDP 数据包到对应的会话
func (u *UdpAdapter) receivePackets() {
	buffer := make([]byte, udpBufferSize)

	for {
		select {
		case <-u.ctx.Done():
			return
		default:
		}

		// 设置读取超时以便能够响应 ctx.Done()
		if err := u.conn.SetReadDeadline(time.Now().Add(udpReadTimeout)); err != nil {
			if !u.IsClosed() {
				utils.Errorf("Failed to set read deadline: %v", err)
			}
			return
		}

		n, addr, err := u.conn.ReadFrom(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // 超时是正常的，继续循环
			}
			if !u.IsClosed() {
				utils.Errorf("UDP read error: %v", err)
			}
			return
		}

		if n > 0 {
			// 复制数据，因为 buffer 会被重用
			data := make([]byte, n)
			copy(data, buffer[:n])

			// 获取或创建会话
			session := u.getOrCreateSession(addr)
			session.updateActivity()

			// 将数据放入会话缓冲区
			select {
			case session.buffer <- data:
			default:
				utils.Warnf("UDP session buffer full for %s, dropping packet", addr)
			}
		}
	}
}

// getOrCreateSession 获取或创建 UDP 会话
func (u *UdpAdapter) getOrCreateSession(addr net.Addr) *udpSession {
	addrStr := addr.String()

	u.sessLock.RLock()
	session, exists := u.sessions[addrStr]
	u.sessLock.RUnlock()

	if exists {
		return session
	}

	u.sessLock.Lock()
	defer u.sessLock.Unlock()

	// 双重检查
	if session, exists := u.sessions[addrStr]; exists {
		return session
	}

	session = newUdpSession(addr, u.conn)
	u.sessions[addrStr] = session

	utils.Infof("Created new UDP session for %s", addr)
	return session
}

// cleanupSessions 定期清理过期的 UDP 会话
func (u *UdpAdapter) cleanupSessions() {
	ticker := time.NewTicker(udpCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-u.ctx.Done():
			return
		case <-ticker.C:
			u.sessLock.Lock()
			for addr, session := range u.sessions {
				if session.isExpired() {
					close(session.buffer)
					delete(u.sessions, addr)
					utils.Infof("Cleaned up expired UDP session for %s", addr)
				}
			}
			u.sessLock.Unlock()
		}
	}
}

func (u *UdpAdapter) Accept() (io.ReadWriteCloser, error) {
	if u.conn == nil {
		return nil, fmt.Errorf("UDP listener not initialized")
	}

	// 等待有可用的会话
	timeout := time.NewTimer(udpReadTimeout)
	defer timeout.Stop()

	select {
	case <-u.ctx.Done():
		return nil, fmt.Errorf("adapter closed")
	case <-timeout.C:
		return nil, errors.NewProtocolTimeoutError("UDP packet")
	default:
	}

	// 查找有数据的会话
	u.sessLock.RLock()
	var readySession *udpSession
	var readyAddr string
	for addr, session := range u.sessions {
		if len(session.buffer) > 0 {
			readySession = session
			readyAddr = addr
			break
		}
	}
	u.sessLock.RUnlock()

	if readySession == nil {
		return nil, errors.NewProtocolTimeoutError("UDP packet")
	}

	// 创建虚拟连接
	return &UdpSessionConn{
		session: readySession,
		addr:    readyAddr,
		adapter: u,
	}, nil
}

func (u *UdpAdapter) getConnectionType() string {
	return "UDP"
}

// onClose UDP 特定的资源清理
func (u *UdpAdapter) onClose() error {
	// 取消上下文，停止所有 goroutine
	if u.cancel != nil {
		u.cancel()
	}

	var err error
	if u.conn != nil {
		err = u.conn.Close()
		u.conn = nil
	}

	// 清理所有会话
	u.sessLock.Lock()
	for addr, session := range u.sessions {
		close(session.buffer)
		delete(u.sessions, addr)
	}
	u.sessLock.Unlock()

	baseErr := u.BaseAdapter.onClose()
	if err != nil {
		return err
	}
	return baseErr
}

// UdpSessionConn UDP会话连接，用于处理特定客户端的数据流
type UdpSessionConn struct {
	session    *udpSession
	addr       string
	adapter    *UdpAdapter
	readBuffer []byte
	readPos    int
	closed     bool
	mu         sync.Mutex
}

// Read 实现io.Reader接口
func (u *UdpSessionConn) Read(p []byte) (n int, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.closed {
		return 0, io.EOF
	}

	// 如果有缓冲数据，先读取缓冲数据
	if u.readBuffer != nil && u.readPos < len(u.readBuffer) {
		n = copy(p, u.readBuffer[u.readPos:])
		u.readPos += n
		if u.readPos >= len(u.readBuffer) {
			u.readBuffer = nil
			u.readPos = 0
		}
		return n, nil
	}

	// 从会话缓冲区读取新数据
	select {
	case data, ok := <-u.session.buffer:
		if !ok {
			return 0, io.EOF
		}
		n = copy(p, data)
		if n < len(data) {
			// 数据太大，保存剩余部分
			u.readBuffer = data
			u.readPos = n
		}
		return n, nil
	case <-time.After(udpReadTimeout):
		return 0, io.EOF
	}
}

// Write 实现io.Writer接口
func (u *UdpSessionConn) Write(p []byte) (n int, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.closed {
		return 0, fmt.Errorf("connection closed")
	}

	u.session.updateActivity()
	return u.session.conn.WriteTo(p, u.session.addr)
}

// Close 实现io.Closer接口
func (u *UdpSessionConn) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.closed {
		return nil
	}

	u.closed = true
	// 不删除会话，因为可能还有其他数据包到达
	return nil
}
