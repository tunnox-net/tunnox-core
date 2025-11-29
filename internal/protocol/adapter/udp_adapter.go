package adapter

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

const (
	udpBufferSize         = 65535
	udpControlBufferSize  = 4096 // 控制连接缓冲区
	udpReadTimeout        = 30 * time.Second
	udpControlReadTimeout = 20 * time.Millisecond
	udpTunnelReadTimeout  = 100 * time.Millisecond
	udpSessionTimeout     = 20 * time.Second  // 缩短为20秒（负载均衡器后面需要更积极清理）
	udpCleanupInterval    = 5 * time.Second   // 缩短为5秒（更频繁清理）
	udpMaxSessions        = 10000             // 最大会话数限制
)

// udpTimeoutError UDP 读取超时错误
// 临时错误，不应该导致连接关闭
type udpTimeoutError struct {
	msg string
}

func (e *udpTimeoutError) Error() string {
	return e.msg
}

func (e *udpTimeoutError) Timeout() bool {
	return true
}

func (e *udpTimeoutError) Temporary() bool {
	return true
}

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

	// 使用较小的缓冲区进行读取
	buf := make([]byte, min(len(p), udpControlBufferSize))
	n, _, err = u.conn.ReadFrom(buf)
	if n > 0 {
		copy(p, buf[:n])
	}
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
	addr        net.Addr
	lastActive  time.Time
	buffer      chan []byte
	conn        net.PacketConn
	sessionConn *UdpSessionConn
	isAccepted  bool
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.RWMutex
}

func newUdpSession(addr net.Addr, conn net.PacketConn, parentCtx context.Context) *udpSession {
	ctx, cancel := context.WithCancel(parentCtx)
	sess := &udpSession{
		addr:       addr,
		lastActive: time.Now(),
		buffer:     make(chan []byte, 100), // 缓冲100个数据包
		conn:       conn,
		isAccepted: false,
		ctx:        ctx,
		cancel:     cancel,
	}
	sess.sessionConn = &UdpSessionConn{
		session:    sess,
		addr:       addr.String(),
		adapter:    nil,
		readBuffer: nil,
		readPos:    0,
		closed:     false,
	}
	return sess
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

		// 设置较长的读取超时（1秒），避免频繁超时
		if err := u.conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
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
			session, isNew := u.getOrCreateSession(addr)
			session.updateActivity()

			select {
			case session.buffer <- data:
				if isNew {
					session.mu.Lock()
					session.isAccepted = false
					session.mu.Unlock()
				}
			default:
				utils.Warnf("UDP session buffer full for %s, dropping packet", addr)
			}
		}
	}
}

// getOrCreateSession 获取或创建 UDP 会话
func (u *UdpAdapter) getOrCreateSession(addr net.Addr) (*udpSession, bool) {
	addrStr := addr.String()

	u.sessLock.RLock()
	session, exists := u.sessions[addrStr]
	u.sessLock.RUnlock()

	if exists {
		return session, false // 返回已存在的会话，isNew=false
	}

	u.sessLock.Lock()
	defer u.sessLock.Unlock()

	// 双重检查
	if session, exists := u.sessions[addrStr]; exists {
		return session, false
	}

	// 检查会话数限制
	if len(u.sessions) >= udpMaxSessions {
		// 清理最旧的会话
		oldestSession := u.findOldestSessionLocked()
		if oldestSession != nil {
			utils.Warnf("UDP session limit reached (%d/%d), removing oldest session %s",
				len(u.sessions), udpMaxSessions, oldestSession.addr.String())
			if oldestSession.sessionConn != nil {
				oldestSession.sessionConn.Close()
			}
			delete(u.sessions, oldestSession.addr.String())
		} else {
			utils.Warnf("UDP session limit reached (%d/%d), cannot create new session",
				len(u.sessions), udpMaxSessions)
			return nil, false
		}
	}

	session = newUdpSession(addr, u.conn, u.ctx)
	session.sessionConn.adapter = u
	u.sessions[addrStr] = session

	return session, true
}

// findOldestSessionLocked 查找最旧的会话（需要在持有锁的情况下调用）
func (u *UdpAdapter) findOldestSessionLocked() *udpSession {
	var oldestSession *udpSession
	var oldestTime time.Time

	for _, session := range u.sessions {
		session.mu.RLock()
		lastActive := session.lastActive
		session.mu.RUnlock()
		if oldestSession == nil || lastActive.Before(oldestTime) {
			oldestSession = session
			oldestTime = lastActive
		}
	}

	return oldestSession
}

// cleanupExpiredSessionsLocked 清理过期的会话（需要在持有锁的情况下调用）
func (u *UdpAdapter) cleanupExpiredSessionsLocked() int {
	var expiredAddrs []string
	for addr, session := range u.sessions {
		if session.isExpired() {
			expiredAddrs = append(expiredAddrs, addr)
		}
	}

	for _, addr := range expiredAddrs {
		if session, exists := u.sessions[addr]; exists {
			// 关闭会话连接
			if session.sessionConn != nil {
				session.sessionConn.Close()
			}
			// 安全关闭 channel（避免重复关闭）
			select {
			case <-session.buffer:
			default:
			}
			close(session.buffer)
			delete(u.sessions, addr)
		}
	}

	return len(expiredAddrs)
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
			expiredCount := u.cleanupExpiredSessionsLocked()
			u.sessLock.Unlock()
			if expiredCount > 0 {
				utils.Debugf("UDP adapter: cleaned up %d expired sessions", expiredCount)
			}
		}
	}
}

// Accept 接受UDP连接
func (u *UdpAdapter) Accept() (io.ReadWriteCloser, error) {
	if u.conn == nil {
		return nil, fmt.Errorf("UDP listener not initialized")
	}

	// 直接检查会话，不使用 ticker 轮询
	for {
		select {
		case <-u.ctx.Done():
			return nil, fmt.Errorf("adapter closed")
		default:
		}

		// 查找有数据且未被Accept的会话
		u.sessLock.RLock()
		var readySession *udpSession
		for _, session := range u.sessions {
			session.mu.RLock()
			hasData := len(session.buffer) > 0
			notAccepted := !session.isAccepted
			session.mu.RUnlock()

			if hasData && notAccepted {
				readySession = session
				break
			}
		}
		u.sessLock.RUnlock()

		if readySession != nil {
			// 标记为已Accept
			readySession.mu.Lock()
			readySession.isAccepted = true
			readySession.mu.Unlock()

			return readySession.sessionConn, nil
		}

		// 短暂休眠避免 CPU 占用过高
		time.Sleep(5 * time.Millisecond)
	}
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
	sessionCount := len(u.sessions)
	for addr, session := range u.sessions {
		// 取消会话上下文
		if session.cancel != nil {
			session.cancel()
		}
		// 关闭会话连接
		if session.sessionConn != nil {
			session.sessionConn.Close()
		}
		// 安全关闭 channel（避免重复关闭）
		select {
		case <-session.buffer:
		default:
		}
		close(session.buffer)
		delete(u.sessions, addr)
	}
	u.sessions = make(map[string]*udpSession)
	u.sessLock.Unlock()

	if sessionCount > 0 {
		utils.Debugf("UDP adapter: closed %d sessions", sessionCount)
	}

	baseErr := u.BaseAdapter.onClose()
	if err != nil {
		return err
	}
	return baseErr
}

// UdpSessionConn UDP会话连接，用于处理特定客户端的数据流
// 持久连接，不应该在单次packet处理后关闭
type UdpSessionConn struct {
	session       *udpSession
	addr          string
	adapter       *UdpAdapter
	readBuffer    []byte
	readPos       int
	closed        bool
	isControlConn bool
	mu            sync.Mutex
}

// GetAddr 获取远程地址字符串（用于包装成net.Conn）
func (u *UdpSessionConn) GetAddr() string {
	return u.addr
}

// IsPersistent 标记这是一个持久连接，不应该在handleConnection后关闭
func (u *UdpSessionConn) IsPersistent() bool {
	return true
}

// SetControlConnection 设置连接类型
func (u *UdpSessionConn) SetControlConnection(isControl bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.isControlConn = isControl
}

// Read 实现io.Reader接口
func (u *UdpSessionConn) Read(p []byte) (n int, err error) {
	u.mu.Lock()
	if u.closed {
		u.mu.Unlock()
		return 0, io.EOF
	}

	if u.readBuffer != nil && u.readPos < len(u.readBuffer) {
		n = copy(p, u.readBuffer[u.readPos:])
		u.readPos += n
		if u.readPos >= len(u.readBuffer) {
			u.readBuffer = nil
			u.readPos = 0
		}
		u.mu.Unlock()
		return n, nil
	}
	u.mu.Unlock()

	timeout := udpTunnelReadTimeout
	if u.isControlConn {
		timeout = udpControlReadTimeout
	}

	select {
	case data, ok := <-u.session.buffer:
		if !ok {
			return 0, io.EOF
		}
		u.mu.Lock()
		n = copy(p, data)
		if n < len(data) {
			u.readBuffer = data
			u.readPos = n
		}
		u.mu.Unlock()
		return n, nil
	case <-u.session.ctx.Done():
		return 0, io.EOF
	case <-time.After(timeout):
		if u.isControlConn {
			return 0, &udpTimeoutError{msg: "udp read timeout, will retry"}
		}
		return 0, &udpTimeoutError{msg: "udp read timeout, connection still alive"}
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
	// 取消会话上下文，停止相关 goroutine
	if u.session != nil && u.session.cancel != nil {
		u.session.cancel()
	}
	return nil
}
