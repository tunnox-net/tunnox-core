package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"tunnox-core/internal/client/mapping"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/stream"
)

// TunnelPoolConfig 隧道连接池配置
type TunnelPoolConfig struct {
	// MaxIdleConns 每个 mapping 的最大空闲连接数
	MaxIdleConns int
	// MaxConnsPerMapping 每个 mapping 的最大连接数
	MaxConnsPerMapping int
	// IdleTimeout 空闲连接超时时间
	IdleTimeout time.Duration
	// DialTimeout 建立隧道的超时时间
	DialTimeout time.Duration
	// HealthCheckOnGet 是否在获取连接时进行健康检查
	HealthCheckOnGet bool
	// Enabled 是否启用连接池
	Enabled bool
}

// DefaultTunnelPoolConfig 返回默认配置
func DefaultTunnelPoolConfig() *TunnelPoolConfig {
	return &TunnelPoolConfig{
		MaxIdleConns:       5,
		MaxConnsPerMapping: 20,
		IdleTimeout:        60 * time.Second,
		DialTimeout:        30 * time.Second,
		HealthCheckOnGet:   true,  // 默认启用健康检查
		Enabled:            false, // 默认禁用，因为隧道连接是有状态的，不能简单复用
	}
}

// TunnelPool 隧道连接池
// 为每个 mapping 维护一组可复用的隧道连接
type TunnelPool struct {
	client *TunnoxClient
	config *TunnelPoolConfig

	// pools 按 mappingID 分组的连接池
	pools   map[string]*MappingPool
	poolsMu sync.RWMutex

	// 统计信息
	totalCreated   atomic.Int64
	totalReused    atomic.Int64
	totalReleased  atomic.Int64
	totalDiscarded atomic.Int64 // 健康检查失败被丢弃的连接数

	ctx    context.Context
	cancel context.CancelFunc
}

// MappingPool 单个 mapping 的连接池
type MappingPool struct {
	mappingID string
	secretKey string

	// idle 空闲连接队列
	idle   []*PooledTunnelConn
	idleMu sync.Mutex
	idleCh chan struct{} // 用于通知有新的空闲连接

	// active 活跃连接计数
	active atomic.Int32

	// 配置
	maxIdle   int
	maxActive int

	// 生命周期
	closed atomic.Bool
}

// PooledTunnelConn 池化的隧道连接
type PooledTunnelConn struct {
	conn       net.Conn
	stream     stream.PackageStreamer
	tunnelID   string
	mappingID  string
	createdAt  time.Time
	lastUsedAt time.Time
	inUse      atomic.Bool
	pool       *MappingPool
}

// NewTunnelPool 创建隧道连接池
func NewTunnelPool(client *TunnoxClient, config *TunnelPoolConfig) *TunnelPool {
	if config == nil {
		config = DefaultTunnelPoolConfig()
	}

	ctx, cancel := context.WithCancel(client.Ctx())

	pool := &TunnelPool{
		client: client,
		config: config,
		pools:  make(map[string]*MappingPool),
		ctx:    ctx,
		cancel: cancel,
	}

	// 启动空闲连接清理
	go pool.cleanupLoop()

	return pool
}

// Get 获取一个隧道连接
// 优先从池中获取空闲连接，如果没有则创建新连接
func (p *TunnelPool) Get(mappingID, secretKey string) (*PooledTunnelConn, error) {
	if !p.config.Enabled {
		return p.createNewConn(mappingID, secretKey)
	}

	pool := p.getOrCreateMappingPool(mappingID, secretKey)

	// 尝试从池中获取健康的空闲连接
	if conn := p.getHealthyIdleConn(pool); conn != nil {
		p.totalReused.Add(1)
		corelog.Debugf("TunnelPool[%s]: reused idle connection, tunnelID=%s", mappingID, conn.tunnelID)
		return conn, nil
	}

	// 检查是否达到最大连接数
	if int(pool.active.Load()) >= pool.maxActive {
		// 等待空闲连接（带超时）
		select {
		case <-pool.idleCh:
			if conn := p.getHealthyIdleConn(pool); conn != nil {
				p.totalReused.Add(1)
				return conn, nil
			}
		case <-time.After(p.config.DialTimeout):
			return nil, fmt.Errorf("timeout waiting for available connection")
		case <-p.ctx.Done():
			return nil, p.ctx.Err()
		}
	}

	// 创建新连接
	return p.createNewConn(mappingID, secretKey)
}

// getHealthyIdleConn 从池中获取健康的空闲连接
// 会循环检查直到找到健康连接或池为空
func (p *TunnelPool) getHealthyIdleConn(pool *MappingPool) *PooledTunnelConn {
	for {
		conn := pool.getIdle()
		if conn == nil {
			return nil
		}

		// 如果未启用健康检查，直接返回
		if !p.config.HealthCheckOnGet {
			return conn
		}

		// 健康检查
		if p.isConnHealthy(conn) {
			return conn
		}

		// 连接不健康，关闭并继续尝试下一个
		corelog.Debugf("TunnelPool[%s]: discarding unhealthy connection, tunnelID=%s", conn.mappingID, conn.tunnelID)
		p.totalDiscarded.Add(1)
		p.closeConn(conn)
	}
}

// GetInterface 获取一个隧道连接（接口方法）
func (p *TunnelPool) GetInterface(mappingID, secretKey string) (mapping.PooledTunnelConnInterface, error) {
	return p.Get(mappingID, secretKey)
}

// Put 归还连接到池中
func (p *TunnelPool) Put(conn *PooledTunnelConn) {
	if conn == nil || conn.pool == nil {
		return
	}

	conn.inUse.Store(false)
	conn.lastUsedAt = time.Now()

	pool := conn.pool

	// 检查连接是否仍然有效
	if pool.closed.Load() || !p.isConnValid(conn) {
		p.closeConn(conn)
		return
	}

	pool.idleMu.Lock()
	if len(pool.idle) >= pool.maxIdle {
		// 池已满，关闭连接
		pool.idleMu.Unlock()
		p.closeConn(conn)
		return
	}

	pool.idle = append(pool.idle, conn)
	pool.idleMu.Unlock()

	// 通知等待者
	select {
	case pool.idleCh <- struct{}{}:
	default:
	}

	p.totalReleased.Add(1)
	corelog.Debugf("TunnelPool[%s]: returned connection to pool, tunnelID=%s, idle=%d",
		conn.mappingID, conn.tunnelID, len(pool.idle))
}

// Close 关闭连接（不归还到池）
func (p *TunnelPool) Close(conn *PooledTunnelConn) {
	if conn == nil {
		return
	}
	p.closeConn(conn)
}

// Shutdown 关闭连接池
func (p *TunnelPool) Shutdown() {
	p.cancel()

	p.poolsMu.Lock()
	defer p.poolsMu.Unlock()

	for _, pool := range p.pools {
		pool.close()
	}
	p.pools = make(map[string]*MappingPool)

	corelog.Infof("TunnelPool: shutdown complete, created=%d, reused=%d, released=%d, discarded=%d",
		p.totalCreated.Load(), p.totalReused.Load(), p.totalReleased.Load(), p.totalDiscarded.Load())
}

// Stats 返回统计信息
func (p *TunnelPool) Stats() map[string]interface{} {
	p.poolsMu.RLock()
	defer p.poolsMu.RUnlock()

	poolStats := make(map[string]interface{})
	for mappingID, pool := range p.pools {
		pool.idleMu.Lock()
		poolStats[mappingID] = map[string]interface{}{
			"idle":   len(pool.idle),
			"active": pool.active.Load(),
		}
		pool.idleMu.Unlock()
	}

	return map[string]interface{}{
		"enabled":        p.config.Enabled,
		"totalCreated":   p.totalCreated.Load(),
		"totalReused":    p.totalReused.Load(),
		"totalReleased":  p.totalReleased.Load(),
		"totalDiscarded": p.totalDiscarded.Load(),
		"pools":          poolStats,
	}
}

// getOrCreateMappingPool 获取或创建 mapping 连接池
func (p *TunnelPool) getOrCreateMappingPool(mappingID, secretKey string) *MappingPool {
	p.poolsMu.RLock()
	pool, exists := p.pools[mappingID]
	p.poolsMu.RUnlock()

	if exists {
		return pool
	}

	p.poolsMu.Lock()
	defer p.poolsMu.Unlock()

	// 双重检查
	if pool, exists = p.pools[mappingID]; exists {
		return pool
	}

	pool = &MappingPool{
		mappingID: mappingID,
		secretKey: secretKey,
		idle:      make([]*PooledTunnelConn, 0, p.config.MaxIdleConns),
		idleCh:    make(chan struct{}, 1),
		maxIdle:   p.config.MaxIdleConns,
		maxActive: p.config.MaxConnsPerMapping,
	}
	p.pools[mappingID] = pool

	return pool
}

// createNewConn 创建新的隧道连接
func (p *TunnelPool) createNewConn(mappingID, secretKey string) (*PooledTunnelConn, error) {
	pool := p.getOrCreateMappingPool(mappingID, secretKey)

	// 增加活跃计数
	pool.active.Add(1)

	// 生成 tunnelID
	tunnelID := fmt.Sprintf("tcp-pool-%s-%d", mappingID, time.Now().UnixNano())

	// 建立隧道连接
	conn, tunnelStream, err := p.client.dialTunnel(tunnelID, mappingID, secretKey)
	if err != nil {
		pool.active.Add(-1)
		return nil, fmt.Errorf("failed to dial tunnel: %w", err)
	}

	pooledConn := &PooledTunnelConn{
		conn:       conn,
		stream:     tunnelStream,
		tunnelID:   tunnelID,
		mappingID:  mappingID,
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
		pool:       pool,
	}
	pooledConn.inUse.Store(true)

	p.totalCreated.Add(1)
	corelog.Debugf("TunnelPool[%s]: created new connection, tunnelID=%s, active=%d",
		mappingID, tunnelID, pool.active.Load())

	return pooledConn, nil
}

// closeConn 关闭连接
func (p *TunnelPool) closeConn(conn *PooledTunnelConn) {
	if conn == nil {
		return
	}

	if conn.pool != nil {
		conn.pool.active.Add(-1)
	}

	if conn.stream != nil {
		conn.stream.Close()
	}
	if conn.conn != nil {
		conn.conn.Close()
	}

	corelog.Debugf("TunnelPool[%s]: closed connection, tunnelID=%s", conn.mappingID, conn.tunnelID)
}

// isConnValid 检查连接是否有效（快速检查，用于归还时）
func (p *TunnelPool) isConnValid(conn *PooledTunnelConn) bool {
	if conn.conn == nil {
		return false
	}

	// 检查空闲时间
	if time.Since(conn.lastUsedAt) > p.config.IdleTimeout {
		return false
	}

	return true
}

// isConnHealthy 检查连接是否健康（用于取出时）
// 通过设置短超时的读取操作来检测连接是否已断开
func (p *TunnelPool) isConnHealthy(conn *PooledTunnelConn) bool {
	if conn.conn == nil {
		return false
	}

	// 1. 检查空闲时间
	if time.Since(conn.lastUsedAt) > p.config.IdleTimeout {
		corelog.Debugf("TunnelPool[%s]: connection idle timeout, tunnelID=%s, idleTime=%v",
			conn.mappingID, conn.tunnelID, time.Since(conn.lastUsedAt))
		return false
	}

	// 2. 检测底层连接是否仍然有效
	// 使用非阻塞方式检测：设置极短的读取超时，尝试读取
	// 如果连接已断开，会立即返回 EOF 或错误
	// 如果连接正常但没有数据，会返回超时错误（这是正常的）
	if tcpConn, ok := conn.conn.(*net.TCPConn); ok {
		// 设置 1ms 读取超时
		tcpConn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
		defer tcpConn.SetReadDeadline(time.Time{}) // 重置为无超时

		buf := make([]byte, 1)
		_, err := tcpConn.Read(buf)

		if err != nil {
			// 超时错误是正常的（没有数据可读，但连接有效）
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				return true
			}
			// 其他错误说明连接已断开
			corelog.Debugf("TunnelPool[%s]: connection health check failed, tunnelID=%s, err=%v",
				conn.mappingID, conn.tunnelID, err)
			return false
		}

		// 如果读到了数据，说明有意外的数据到达，连接状态异常
		corelog.Warnf("TunnelPool[%s]: unexpected data received during health check, tunnelID=%s",
			conn.mappingID, conn.tunnelID)
		return false
	}

	// 对于非 TCP 连接（如 QUIC wrapper），尝试通过 SyscallConn 检查
	if syscallConn, ok := conn.conn.(interface {
		SyscallConn() (interface{ Control(func(fd uintptr)) error }, error)
	}); ok {
		rawConn, err := syscallConn.SyscallConn()
		if err == nil {
			var connErr error
			rawConn.Control(func(fd uintptr) {
				// 这里可以使用 syscall 检查连接状态
				// 但为了跨平台兼容性，我们只依赖上面的读取检查
			})
			if connErr != nil {
				return false
			}
		}
	}

	// 默认认为有效（保守策略，实际使用时会发现问题）
	return true
}

// cleanupLoop 定期清理空闲连接
func (p *TunnelPool) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanupIdleConns()
		case <-p.ctx.Done():
			return
		}
	}
}

// cleanupIdleConns 清理过期的空闲连接
func (p *TunnelPool) cleanupIdleConns() {
	p.poolsMu.RLock()
	pools := make([]*MappingPool, 0, len(p.pools))
	for _, pool := range p.pools {
		pools = append(pools, pool)
	}
	p.poolsMu.RUnlock()

	now := time.Now()
	for _, pool := range pools {
		pool.idleMu.Lock()
		var remaining []*PooledTunnelConn
		for _, conn := range pool.idle {
			if now.Sub(conn.lastUsedAt) > p.config.IdleTimeout {
				p.closeConn(conn)
			} else {
				remaining = append(remaining, conn)
			}
		}
		pool.idle = remaining
		pool.idleMu.Unlock()
	}
}

// MappingPool methods

// getIdle 从池中获取空闲连接
func (mp *MappingPool) getIdle() *PooledTunnelConn {
	mp.idleMu.Lock()
	defer mp.idleMu.Unlock()

	for len(mp.idle) > 0 {
		// 从末尾取（LIFO，最近使用的连接更可能有效）
		n := len(mp.idle) - 1
		conn := mp.idle[n]
		mp.idle = mp.idle[:n]

		if conn.inUse.CompareAndSwap(false, true) {
			conn.lastUsedAt = time.Now()
			return conn
		}
	}

	return nil
}

// close 关闭 mapping 池
func (mp *MappingPool) close() {
	mp.closed.Store(true)

	mp.idleMu.Lock()
	defer mp.idleMu.Unlock()

	for _, conn := range mp.idle {
		if conn.stream != nil {
			conn.stream.Close()
		}
		if conn.conn != nil {
			conn.conn.Close()
		}
	}
	mp.idle = nil
}

// PooledTunnelConn methods

// GetReader 获取读取器
func (c *PooledTunnelConn) GetReader() io.Reader {
	if c.stream != nil {
		if reader := c.stream.GetReader(); reader != nil {
			return reader
		}
	}
	return c.conn
}

// GetWriter 获取写入器
func (c *PooledTunnelConn) GetWriter() io.Writer {
	if c.stream != nil {
		if writer := c.stream.GetWriter(); writer != nil {
			return writer
		}
	}
	return c.conn
}

// GetConn 获取底层连接
func (c *PooledTunnelConn) GetConn() net.Conn {
	return c.conn
}

// GetStream 获取流处理器
func (c *PooledTunnelConn) GetStream() stream.PackageStreamer {
	return c.stream
}

// TunnelID 获取隧道ID
func (c *PooledTunnelConn) TunnelID() string {
	return c.tunnelID
}
