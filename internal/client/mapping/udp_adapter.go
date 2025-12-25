package mapping

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/config"
	corelog "tunnox-core/internal/core/log"
)

// UDPMappingAdapter UDP映射适配器
// UDP 是无连接协议，需要为每个源地址创建虚拟连接
type UDPMappingAdapter struct {
	listener net.PacketConn
	connChan chan *UDPVirtualConn
	sessions map[string]*UDPVirtualConn
	mu       sync.RWMutex
	closeCh  chan struct{}
	wg       sync.WaitGroup
}

// UDPVirtualConn UDP虚拟连接
// 为每个 UDP 源地址创建一个虚拟连接，实现 io.ReadWriteCloser
type UDPVirtualConn struct {
	listener   net.PacketConn
	remoteAddr net.Addr
	readChan   chan []byte
	writeChan  chan []byte
	closeCh    chan struct{}
	closeOnce  sync.Once
	lastActive time.Time
	mu         sync.RWMutex
}

const (
	udpReadChanSize  = 4096 // 增加缓冲区以支持高性能场景
	udpWriteChanSize = 4096 // 增加缓冲区以支持高性能场景
	udpSessionTTL    = 60 * time.Second
)

// NewUDPMappingAdapter 创建UDP映射适配器
func NewUDPMappingAdapter() *UDPMappingAdapter {
	return &UDPMappingAdapter{
		connChan: make(chan *UDPVirtualConn, 100),
		sessions: make(map[string]*UDPVirtualConn),
		closeCh:  make(chan struct{}),
	}
}

// StartListener 启动UDP监听
func (a *UDPMappingAdapter) StartListener(config config.MappingConfig) error {
	addr := fmt.Sprintf(":%d", config.LocalPort)
	listener, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	a.listener = listener
	corelog.Infof("UDPMappingAdapter: listening on %s", addr)

	// 启动数据接收循环
	a.wg.Add(2)
	go a.readLoop()
	go a.cleanupLoop()

	return nil
}

// readLoop 读取UDP数据包并分发到对应的虚拟连接
func (a *UDPMappingAdapter) readLoop() {
	defer a.wg.Done()

	buffer := make([]byte, 65536) // UDP最大包大小

	for {
		select {
		case <-a.closeCh:
			return
		default:
		}

		// 设置读超时，避免阻塞
		a.listener.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, remoteAddr, err := a.listener.ReadFrom(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if netErr, ok := err.(net.Error); ok && !netErr.Temporary() {
				corelog.Errorf("UDPMappingAdapter: read error: %v", err)
				return
			}
			continue
		}

		if n == 0 {
			continue
		}

		// 获取或创建会话
		addrKey := remoteAddr.String()
		a.mu.Lock()
		session, exists := a.sessions[addrKey]
		if !exists {
			// 创建新的虚拟连接
			session = &UDPVirtualConn{
				listener:   a.listener,
				remoteAddr: remoteAddr,
				readChan:   make(chan []byte, udpReadChanSize),
				writeChan:  make(chan []byte, udpWriteChanSize),
				closeCh:    make(chan struct{}),
				lastActive: time.Now(),
			}
			a.sessions[addrKey] = session

			// 启动写入循环
			go session.writeLoop()

			// 通知有新连接
			select {
			case a.connChan <- session:
				corelog.Debugf("UDPMappingAdapter: new session from %s", remoteAddr)
			default:
				corelog.Warnf("UDPMappingAdapter: connection channel full, dropping session from %s", remoteAddr)
				session.Close()
				delete(a.sessions, addrKey)
			}
		}
		a.mu.Unlock()

		// 发送数据到会话
		if session != nil {
			// 复制数据
			data := make([]byte, n)
			copy(data, buffer[:n])

			select {
			case session.readChan <- data:
				session.updateLastActive()
			default:
				corelog.Warnf("UDPMappingAdapter: read channel full for %s, dropping packet", remoteAddr)
			}
		}
	}
}

// cleanupLoop 清理过期的会话
func (a *UDPMappingAdapter) cleanupLoop() {
	defer a.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.closeCh:
			return
		case <-ticker.C:
			a.cleanupStaleSessions()
		}
	}
}

// cleanupStaleSessions 清理过期会话
func (a *UDPMappingAdapter) cleanupStaleSessions() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for addr, session := range a.sessions {
		if now.Sub(session.getLastActive()) > udpSessionTTL {
			corelog.Debugf("UDPMappingAdapter: cleaning up stale session %s", addr)
			session.Close()
			delete(a.sessions, addr)
		}
	}
}

// Accept 接受UDP虚拟连接
func (a *UDPMappingAdapter) Accept() (io.ReadWriteCloser, error) {
	select {
	case conn := <-a.connChan:
		return conn, nil
	case <-a.closeCh:
		return nil, fmt.Errorf("adapter closed")
	}
}

// PrepareConnection UDP不需要预处理
func (a *UDPMappingAdapter) PrepareConnection(conn io.ReadWriteCloser) error {
	return nil
}

// GetProtocol 获取协议名称
func (a *UDPMappingAdapter) GetProtocol() string {
	return "udp"
}

// Close 关闭资源
func (a *UDPMappingAdapter) Close() error {
	close(a.closeCh)

	// 关闭所有会话
	a.mu.Lock()
	for _, session := range a.sessions {
		session.Close()
	}
	a.sessions = make(map[string]*UDPVirtualConn)
	a.mu.Unlock()

	// 关闭监听器
	if a.listener != nil {
		a.listener.Close()
	}

	a.wg.Wait()
	return nil
}

// === UDPVirtualConn 实现 ===

// Read 从虚拟连接读取数据
func (c *UDPVirtualConn) Read(p []byte) (int, error) {
	select {
	case data := <-c.readChan:
		n := copy(p, data)
		return n, nil
	case <-c.closeCh:
		return 0, io.EOF
	}
}

// Write 向虚拟连接写入数据
func (c *UDPVirtualConn) Write(p []byte) (int, error) {
	// 复制数据
	data := make([]byte, len(p))
	copy(data, p)

	select {
	case c.writeChan <- data:
		c.updateLastActive()
		return len(p), nil
	case <-c.closeCh:
		return 0, io.ErrClosedPipe
	default:
		return 0, fmt.Errorf("write channel full")
	}
}

// writeLoop 写入循环，将数据发送到 UDP socket
func (c *UDPVirtualConn) writeLoop() {
	for {
		select {
		case data := <-c.writeChan:
			if _, err := c.listener.WriteTo(data, c.remoteAddr); err != nil {
				corelog.Errorf("UDPVirtualConn: write error to %s: %v", c.remoteAddr, err)
			}
		case <-c.closeCh:
			return
		}
	}
}

// Close 关闭虚拟连接
func (c *UDPVirtualConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closeCh)
	})
	return nil
}

// updateLastActive 更新最后活跃时间
func (c *UDPVirtualConn) updateLastActive() {
	c.mu.Lock()
	c.lastActive = time.Now()
	c.mu.Unlock()
}

// getLastActive 获取最后活跃时间
func (c *UDPVirtualConn) getLastActive() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastActive
}
