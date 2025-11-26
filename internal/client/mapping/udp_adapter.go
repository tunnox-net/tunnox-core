package mapping

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/config"
	"tunnox-core/internal/utils"
)

const (
	udpSessionTimeout  = 30 * time.Second
	udpCleanupInterval = 10 * time.Second
	udpMaxPacketSize   = 65535
)

// UDPMappingAdapter UDP映射适配器
// UDP是无连接协议，需要会话管理
type UDPMappingAdapter struct {
	conn     *net.UDPConn
	sessions map[string]*udpVirtualConn
	sessLock sync.RWMutex
	connChan chan io.ReadWriteCloser
	ctx      context.Context
	cancel   context.CancelFunc
}

// udpVirtualConn UDP虚拟连接
// 将UDP会话包装成io.ReadWriteCloser接口
type udpVirtualConn struct {
	userAddr   *net.UDPAddr
	udpConn    *net.UDPConn
	recvChan   chan []byte
	ctx        context.Context
	cancel     context.CancelFunc
	lastActive time.Time
	mu         sync.RWMutex
}

func newUDPVirtualConn(userAddr *net.UDPAddr, udpConn *net.UDPConn, ctx context.Context) *udpVirtualConn {
	virtualCtx, virtualCancel := context.WithCancel(ctx)
	return &udpVirtualConn{
		userAddr:   userAddr,
		udpConn:    udpConn,
		recvChan:   make(chan []byte, 100),
		ctx:        virtualCtx,
		cancel:     virtualCancel,
		lastActive: time.Now(),
	}
}

func (c *udpVirtualConn) Read(p []byte) (n int, err error) {
	select {
	case <-c.ctx.Done():
		return 0, io.EOF
	case data := <-c.recvChan:
		if len(data) > len(p) {
			return 0, fmt.Errorf("buffer too small")
		}
		copy(p, data)
		c.updateActivity()
		return len(data), nil
	}
}

func (c *udpVirtualConn) Write(p []byte) (n int, err error) {
	// 写入数据通过UDP发送回用户
	// UDP需要加上长度前缀
	packet := make([]byte, 4+len(p))
	binary.BigEndian.PutUint32(packet[0:4], uint32(len(p)))
	copy(packet[4:], p)

	_, err = c.udpConn.WriteToUDP(packet, c.userAddr)
	if err != nil {
		return 0, err
	}
	c.updateActivity()
	return len(p), nil
}

func (c *udpVirtualConn) Close() error {
	c.cancel()
	close(c.recvChan)
	return nil
}

func (c *udpVirtualConn) updateActivity() {
	c.mu.Lock()
	c.lastActive = time.Now()
	c.mu.Unlock()
}

func (c *udpVirtualConn) isExpired() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Since(c.lastActive) > udpSessionTimeout
}

// NewUDPMappingAdapter 创建UDP映射适配器
func NewUDPMappingAdapter() *UDPMappingAdapter {
	ctx, cancel := context.WithCancel(context.Background())
	return &UDPMappingAdapter{
		sessions: make(map[string]*udpVirtualConn),
		connChan: make(chan io.ReadWriteCloser, 100),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// StartListener 启动UDP监听
func (a *UDPMappingAdapter) StartListener(config config.MappingConfig) error {
	addr := fmt.Sprintf(":%d", config.LocalPort)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	a.conn = conn
	utils.Debugf("UDPMappingAdapter: listening on %s", addr)

	// 启动接收循环
	go a.receiveLoop()

	// 启动会话清理循环
	go a.cleanupLoop()

	return nil
}

// receiveLoop 接收UDP数据包
func (a *UDPMappingAdapter) receiveLoop() {
	buffer := make([]byte, udpMaxPacketSize)

	for {
		select {
		case <-a.ctx.Done():
			return
		default:
		}

		a.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, userAddr, err := a.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if a.ctx.Err() != nil {
				return
			}
			utils.Errorf("UDPMappingAdapter: failed to read packet: %v", err)
			continue
		}

		if n == 0 {
			continue
		}

		// 复制数据
		data := make([]byte, n)
		copy(data, buffer[:n])

		// 处理数据包
		a.handlePacket(userAddr, data)
	}
}

// handlePacket 处理UDP数据包
func (a *UDPMappingAdapter) handlePacket(userAddr *net.UDPAddr, data []byte) {
	addrKey := userAddr.String()

	// 获取或创建会话
	a.sessLock.RLock()
	session := a.sessions[addrKey]
	a.sessLock.RUnlock()

	if session == nil {
		// 创建新会话
		session = newUDPVirtualConn(userAddr, a.conn, a.ctx)

		a.sessLock.Lock()
		a.sessions[addrKey] = session
		a.sessLock.Unlock()

		// 推送到Accept通道
		select {
		case a.connChan <- session:
			utils.Debugf("UDPMappingAdapter: new session for %s", userAddr)
		case <-a.ctx.Done():
			return
		default:
			utils.Warnf("UDPMappingAdapter: connection channel full, dropping new session")
			session.Close()
			return
		}
	}

	// 发送数据到会话
	select {
	case session.recvChan <- data:
		session.updateActivity()
	case <-session.ctx.Done():
		// 会话已关闭，删除
		a.removeSession(addrKey)
	default:
		utils.Warnf("UDPMappingAdapter: session receive channel full for %s", addrKey)
	}
}

// removeSession 删除会话
func (a *UDPMappingAdapter) removeSession(addrKey string) {
	a.sessLock.Lock()
	defer a.sessLock.Unlock()

	if session, exists := a.sessions[addrKey]; exists {
		session.Close()
		delete(a.sessions, addrKey)
		utils.Debugf("UDPMappingAdapter: removed session %s", addrKey)
	}
}

// cleanupLoop 定期清理超时会话
func (a *UDPMappingAdapter) cleanupLoop() {
	ticker := time.NewTicker(udpCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.cleanupExpiredSessions()
		}
	}
}

// cleanupExpiredSessions 清理超时会话
func (a *UDPMappingAdapter) cleanupExpiredSessions() {
	a.sessLock.Lock()
	defer a.sessLock.Unlock()

	var expiredKeys []string
	for key, session := range a.sessions {
		if session.isExpired() {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		if session, exists := a.sessions[key]; exists {
			session.Close()
			delete(a.sessions, key)
			utils.Debugf("UDPMappingAdapter: cleaned up expired session %s", key)
		}
	}

	if len(expiredKeys) > 0 {
		utils.Debugf("UDPMappingAdapter: cleaned up %d expired sessions", len(expiredKeys))
	}
}

// Accept 接受UDP虚拟连接
func (a *UDPMappingAdapter) Accept() (io.ReadWriteCloser, error) {
	select {
	case <-a.ctx.Done():
		return nil, io.EOF
	case conn := <-a.connChan:
		return conn, nil
	}
}

// PrepareConnection UDP不需要预处理
func (a *UDPMappingAdapter) PrepareConnection(conn io.ReadWriteCloser) error {
	// UDP虚拟连接已经在Accept中创建好了，不需要额外处理
	return nil
}

// GetProtocol 获取协议名称
func (a *UDPMappingAdapter) GetProtocol() string {
	return "udp"
}

// Close 关闭资源
func (a *UDPMappingAdapter) Close() error {
	a.cancel()

	// 关闭所有会话
	a.sessLock.Lock()
	for _, session := range a.sessions {
		session.Close()
	}
	a.sessions = make(map[string]*udpVirtualConn)
	a.sessLock.Unlock()

	// 关闭UDP连接
	if a.conn != nil {
		return a.conn.Close()
	}

	return nil
}

