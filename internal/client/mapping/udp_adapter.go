package mapping

import (
	"fmt"
	"io"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/ipv4"
	"tunnox-core/internal/config"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// ============================================================================
// UDP 性能优化常量
// ============================================================================

const (
	// Channel 容量：从 4096 扩大到 65536，支持高吞吐场景
	udpReadChanSize  = 65536
	udpWriteChanSize = 65536

	// 会话超时
	udpSessionTTL = 60 * time.Second

	// 读超时：从 1s 降低到 100ms，提升响应速度
	udpReadTimeout = 100 * time.Millisecond

	// OS 缓冲区大小：4MB，减少内核层丢包
	udpSocketBufferSize = 4 * 1024 * 1024

	// 背压等待时间：channel 满时短暂等待
	udpBackpressureTimeout = 5 * time.Millisecond

	// 默认 reader 数量：使用 SO_REUSEPORT 时的并行 socket 数
	defaultUDPReaderCount = 4
)

// ============================================================================
// 内存池：复用缓冲区，减少 GC 压力
// ============================================================================

var udpBufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 65536)
	},
}

// getBuffer 从池中获取缓冲区
func getBuffer() []byte {
	return udpBufferPool.Get().([]byte)
}

// putBuffer 归还缓冲区到池中
func putBuffer(buf []byte) {
	if cap(buf) >= 65536 {
		udpBufferPool.Put(buf[:cap(buf)])
	}
}

// ============================================================================
// UDPMappingAdapter - 支持 SO_REUSEPORT 多 socket 并行
// ============================================================================

// UDPMappingAdapter UDP映射适配器
// 支持 SO_REUSEPORT 多 socket 并行读取，提升高吞吐场景下的性能
type UDPMappingAdapter struct {
	listeners   []net.PacketConn   // 多个 listener (SO_REUSEPORT)
	connChan    chan *UDPVirtualConn
	sessions    sync.Map           // map[string]*UDPVirtualConn，无锁并发访问
	closeCh     chan struct{}
	wg          sync.WaitGroup
	readerCount int                // reader 数量
	port        int                // 监听端口
}

// UDPVirtualConn UDP虚拟连接
// 为每个 UDP 源地址创建一个虚拟连接，实现 io.ReadWriteCloser
// 使用 atomic 操作替代锁，提升并发性能
type UDPVirtualConn struct {
	listener     net.PacketConn
	remoteAddr   net.Addr
	readChan     chan *udpPacket // 使用结构体以支持内存池
	writeChan    chan []byte
	closeCh      chan struct{}
	closeOnce    sync.Once
	lastActive   atomic.Int64    // Unix 纳秒时间戳，使用 atomic 无锁访问
	readDeadline atomic.Int64    // Unix 纳秒时间戳
}

// udpPacket UDP 数据包（支持内存池）
type udpPacket struct {
	data   []byte // 实际数据（切片）
	buffer []byte // 原始缓冲区（用于归还池）
}

// NewUDPMappingAdapter 创建UDP映射适配器
func NewUDPMappingAdapter() *UDPMappingAdapter {
	readerCount := defaultUDPReaderCount
	// 如果平台不支持 SO_REUSEPORT，只使用单个 reader
	if !supportsReusePort() {
		readerCount = 1
	}

	return &UDPMappingAdapter{
		connChan:    make(chan *UDPVirtualConn, 100),
		closeCh:     make(chan struct{}),
		readerCount: readerCount,
	}
}

// StartListener 启动UDP监听
// 使用 SO_REUSEPORT 创建多个 socket 并行读取
func (a *UDPMappingAdapter) StartListener(cfg config.MappingConfig) error {
	a.port = cfg.LocalPort

	// 创建多个 listener
	a.listeners = make([]net.PacketConn, 0, a.readerCount)

	if supportsReusePort() && a.readerCount > 1 {
		// 使用 SO_REUSEPORT 创建多个 socket
		for i := 0; i < a.readerCount; i++ {
			listener, err := createReusePortListener(a.port)
			if err != nil {
				// 清理已创建的 listener
				for _, l := range a.listeners {
					l.Close()
				}
				return coreerrors.Wrapf(err, coreerrors.CodeNetworkError, "failed to create SO_REUSEPORT listener %d", i)
			}
			a.listeners = append(a.listeners, listener)
		}
		corelog.Infof("UDPMappingAdapter: created %d SO_REUSEPORT listeners on port %d", a.readerCount, a.port)
	} else {
		// 回退到单个 listener
		addr := fmt.Sprintf(":%d", a.port)
		listener, err := net.ListenPacket("udp", addr)
		if err != nil {
			return coreerrors.Wrapf(err, coreerrors.CodeNetworkError, "failed to listen on %s", addr)
		}

		// 设置 OS 级别缓冲区
		if udpConn, ok := listener.(*net.UDPConn); ok {
			_ = udpConn.SetReadBuffer(udpSocketBufferSize)
			_ = udpConn.SetWriteBuffer(udpSocketBufferSize)
			corelog.Infof("UDPMappingAdapter: OS buffer set to %d bytes", udpSocketBufferSize)
		}

		a.listeners = append(a.listeners, listener)
		corelog.Infof("UDPMappingAdapter: listening on %s (single socket mode)", addr)
	}

	// 为每个 listener 启动 readLoop
	for i, listener := range a.listeners {
		a.wg.Add(1)
		go a.readLoop(i, listener)
	}

	// 启动清理循环
	a.wg.Add(1)
	go a.cleanupLoop()

	return nil
}

// readLoop 读取UDP数据包并分发到对应的虚拟连接
// 每个 listener 独立运行一个 readLoop
func (a *UDPMappingAdapter) readLoop(readerID int, listener net.PacketConn) {
	defer a.wg.Done()

	// 检测是否支持批量读取 (Linux + *net.UDPConn)
	udpConn, isUDP := listener.(*net.UDPConn)
	if isUDP && runtime.GOOS == "linux" {
		a.readLoopBatch(readerID, udpConn, listener)
	} else {
		a.readLoopSingle(readerID, listener)
	}
}

// readLoopBatch 批量读取模式 (Linux recvmmsg)
func (a *UDPMappingAdapter) readLoopBatch(readerID int, udpConn *net.UDPConn, listener net.PacketConn) {
	const batchSize = 32

	pktConn := ipv4.NewPacketConn(udpConn)
	messages := make([]ipv4.Message, batchSize)
	buffers := make([][]byte, batchSize)

	// 预分配缓冲区
	for i := range buffers {
		buffers[i] = make([]byte, 65536)
		messages[i].Buffers = [][]byte{buffers[i]}
	}

	for {
		select {
		case <-a.closeCh:
			return
		default:
		}

		// 设置读超时
		udpConn.SetReadDeadline(time.Now().Add(udpReadTimeout))

		// 批量读取
		n, err := pktConn.ReadBatch(messages, 0)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// 非超时错误，记录并退出
			corelog.Errorf("UDPMappingAdapter[%d]: batch read error: %v", readerID, err)
			return
		}

		// 批量处理读取到的包
		for i := 0; i < n; i++ {
			msg := &messages[i]
			if msg.N == 0 {
				continue
			}

			// 从池中获取缓冲区并复制数据
			buffer := getBuffer()
			copy(buffer, buffers[i][:msg.N])

			// 处理单个包
			a.processPacket(buffer, msg.N, msg.Addr, listener)
		}
	}
}

// readLoopSingle 单包读取模式 (macOS/Windows)
func (a *UDPMappingAdapter) readLoopSingle(readerID int, listener net.PacketConn) {
	// 使用可复用的 timer，避免 time.After 的 GC 压力
	backpressureTimer := time.NewTimer(udpBackpressureTimeout)
	backpressureTimer.Stop()
	defer backpressureTimer.Stop()

	for {
		select {
		case <-a.closeCh:
			return
		default:
		}

		// 设置读超时
		listener.SetReadDeadline(time.Now().Add(udpReadTimeout))

		// 从内存池获取缓冲区
		buffer := getBuffer()
		n, remoteAddr, err := listener.ReadFrom(buffer)
		if err != nil {
			putBuffer(buffer) // 归还缓冲区
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// 非超时错误，记录并退出
			corelog.Errorf("UDPMappingAdapter[%d]: read error: %v", readerID, err)
			return
		}

		if n == 0 {
			putBuffer(buffer)
			continue
		}

		a.processPacketWithBackpressure(buffer, n, remoteAddr, listener, backpressureTimer)
	}
}

// processPacket 处理单个 UDP 包（无背压等待，用于批量模式）
func (a *UDPMappingAdapter) processPacket(buffer []byte, n int, remoteAddr net.Addr, listener net.PacketConn) {
	addrKey := remoteAddr.String()

	// 使用 sync.Map 无锁查询
	sessionI, exists := a.sessions.Load(addrKey)
	var session *UDPVirtualConn
	if exists {
		session = sessionI.(*UDPVirtualConn)
	} else {
		// 需要创建新会话
		session = a.getOrCreateSession(addrKey, remoteAddr, listener)
	}

	if session == nil {
		putBuffer(buffer)
		return
	}

	pkt := &udpPacket{
		data:   buffer[:n],
		buffer: buffer,
	}

	// 非阻塞发送
	select {
	case session.readChan <- pkt:
		session.updateLastActive()
	default:
		putBuffer(buffer)
	}
}

// processPacketWithBackpressure 处理单个 UDP 包（带背压等待）
func (a *UDPMappingAdapter) processPacketWithBackpressure(buffer []byte, n int, remoteAddr net.Addr, listener net.PacketConn, backpressureTimer *time.Timer) {
	addrKey := remoteAddr.String()

	// 使用 sync.Map 无锁查询
	sessionI, exists := a.sessions.Load(addrKey)
	var session *UDPVirtualConn
	if exists {
		session = sessionI.(*UDPVirtualConn)
	} else {
		session = a.getOrCreateSession(addrKey, remoteAddr, listener)
	}

	if session == nil {
		putBuffer(buffer)
		return
	}

	pkt := &udpPacket{
		data:   buffer[:n],
		buffer: buffer,
	}

	// 尝试发送，带背压等待
	select {
	case session.readChan <- pkt:
		session.updateLastActive()
	default:
		backpressureTimer.Reset(udpBackpressureTimeout)
		select {
		case session.readChan <- pkt:
			backpressureTimer.Stop()
			session.updateLastActive()
		case <-backpressureTimer.C:
			putBuffer(buffer)
			corelog.Warnf("UDPMappingAdapter: read channel full for %s, dropping packet", remoteAddr)
		case <-a.closeCh:
			putBuffer(buffer)
		}
	}
}

// getOrCreateSession 获取或创建会话（使用 LoadOrStore 原子操作）
func (a *UDPMappingAdapter) getOrCreateSession(addrKey string, remoteAddr net.Addr, listener net.PacketConn) *UDPVirtualConn {
	// 创建新会话
	session := &UDPVirtualConn{
		listener:   listener,
		remoteAddr: remoteAddr,
		readChan:   make(chan *udpPacket, udpReadChanSize),
		writeChan:  make(chan []byte, udpWriteChanSize),
		closeCh:    make(chan struct{}),
	}
	session.lastActive.Store(time.Now().UnixNano())

	// 使用 LoadOrStore 确保原子性
	actualI, loaded := a.sessions.LoadOrStore(addrKey, session)
	if loaded {
		// 已存在，返回现有会话
		return actualI.(*UDPVirtualConn)
	}

	// 新建会话成功，启动 writeLoop
	go session.writeLoop()

	// 推送到 connChan
	select {
	case a.connChan <- session:
		corelog.Debugf("UDPMappingAdapter: new session from %s", remoteAddr)
		return session
	default:
		// connChan 满，清理会话
		corelog.Warnf("UDPMappingAdapter: connection channel full, dropping session from %s", remoteAddr)
		session.Close()
		a.sessions.Delete(addrKey)
		return nil
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
	now := time.Now().UnixNano()
	ttlNanos := udpSessionTTL.Nanoseconds()

	a.sessions.Range(func(key, value interface{}) bool {
		session := value.(*UDPVirtualConn)
		lastActive := session.lastActive.Load()
		if now-lastActive > ttlNanos {
			addr := key.(string)
			corelog.Debugf("UDPMappingAdapter: cleaning up stale session %s", addr)
			session.Close()
			a.sessions.Delete(key)
		}
		return true
	})
}

// Accept 接受UDP虚拟连接
func (a *UDPMappingAdapter) Accept() (io.ReadWriteCloser, error) {
	select {
	case conn := <-a.connChan:
		return conn, nil
	case <-a.closeCh:
		return nil, coreerrors.New(coreerrors.CodeResourceClosed, "adapter closed")
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
	a.sessions.Range(func(key, value interface{}) bool {
		session := value.(*UDPVirtualConn)
		session.Close()
		a.sessions.Delete(key)
		return true
	})

	// 关闭所有监听器
	for _, listener := range a.listeners {
		listener.Close()
	}

	a.wg.Wait()
	return nil
}

// === UDPVirtualConn 实现 ===

// SetReadDeadline 设置读超时时间（使用 atomic 无锁访问）
func (c *UDPVirtualConn) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		c.readDeadline.Store(0)
	} else {
		c.readDeadline.Store(t.UnixNano())
	}
	return nil
}

// Read 从虚拟连接读取数据（支持超时）
func (c *UDPVirtualConn) Read(p []byte) (int, error) {
	deadlineNanos := c.readDeadline.Load()

	// 如果设置了 deadline，使用带超时的 select
	if deadlineNanos != 0 {
		now := time.Now().UnixNano()
		if now >= deadlineNanos {
			// 已超时，返回超时错误
			return 0, &net.OpError{Op: "read", Err: timeoutError{}}
		}

		timeout := time.Duration(deadlineNanos - now)
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case pkt := <-c.readChan:
			n := copy(p, pkt.data)
			putBuffer(pkt.buffer)
			return n, nil
		case <-c.closeCh:
			return 0, io.EOF
		case <-timer.C:
			return 0, &net.OpError{Op: "read", Err: timeoutError{}}
		}
	}

	// 无 deadline，使用阻塞读取
	select {
	case pkt := <-c.readChan:
		n := copy(p, pkt.data)
		putBuffer(pkt.buffer)
		return n, nil
	case <-c.closeCh:
		return 0, io.EOF
	}
}

// timeoutError 实现 net.Error 接口的超时错误
type timeoutError struct{}

func (e timeoutError) Error() string { return "i/o timeout" }
func (e timeoutError) Timeout() bool { return true }

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
		return 0, coreerrors.New(coreerrors.CodeResourceExhausted, "write channel full")
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

// updateLastActive 更新最后活跃时间（使用 atomic 无锁访问）
func (c *UDPVirtualConn) updateLastActive() {
	c.lastActive.Store(time.Now().UnixNano())
}
