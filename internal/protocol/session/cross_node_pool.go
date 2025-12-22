// Package session 提供会话管理功能
package session

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// CrossNodePoolConfig 跨节点连接池配置
type CrossNodePoolConfig struct {
	MinConns    int           `json:"min_conns"`    // 每节点最小连接数，默认 2
	MaxConns    int           `json:"max_conns"`    // 每节点最大连接数，默认 10
	IdleTimeout time.Duration `json:"idle_timeout"` // 空闲连接超时，默认 5 分钟
	DialTimeout time.Duration `json:"dial_timeout"` // 建立连接超时，默认 5 秒
}

// DefaultCrossNodePoolConfig 返回默认配置
func DefaultCrossNodePoolConfig() CrossNodePoolConfig {
	return CrossNodePoolConfig{
		MinConns:    2,
		MaxConns:    10,
		IdleTimeout: 5 * time.Minute,
		DialTimeout: 5 * time.Second,
	}
}

// CrossNodePool 跨节点连接池
// 管理到其他节点的 TCP 连接，支持连接复用和池化
type CrossNodePool struct {
	*dispose.ServiceBase

	storage   storage.Storage
	nodeID    string // 当前节点 ID
	pools     map[string]*NodeConnectionPool
	poolsLock sync.RWMutex
	config    CrossNodePoolConfig

	// 统计信息
	totalGets    int64
	totalPuts    int64
	totalCreated int64
	totalClosed  int64
}

// NewCrossNodePool 创建跨节点连接池
func NewCrossNodePool(
	parentCtx context.Context,
	storage storage.Storage,
	nodeID string,
	config CrossNodePoolConfig,
) *CrossNodePool {
	pool := &CrossNodePool{
		ServiceBase: dispose.NewService("CrossNodePool", parentCtx),
		storage:     storage,
		nodeID:      nodeID,
		pools:       make(map[string]*NodeConnectionPool),
		config:      config,
	}

	// 启动空闲连接清理
	go pool.startIdleCleanup()

	// 添加清理处理器
	pool.AddCleanHandler(func() error {
		return pool.closeAllPools()
	})

	corelog.Infof("CrossNodePool: initialized for node %s (minConns=%d, maxConns=%d)",
		nodeID, config.MinConns, config.MaxConns)

	return pool
}

// Get 获取到目标节点的连接
func (p *CrossNodePool) Get(ctx context.Context, targetNodeID string) (*CrossNodeConn, error) {
	if p.IsClosed() {
		return nil, coreerrors.New(coreerrors.CodeUnavailable, "pool is closed")
	}

	if targetNodeID == p.nodeID {
		return nil, coreerrors.New(coreerrors.CodeInvalidRequest, "cannot connect to self")
	}

	atomic.AddInt64(&p.totalGets, 1)

	// 获取或创建节点连接池
	nodePool, err := p.getOrCreateNodePool(targetNodeID)
	if err != nil {
		return nil, err
	}

	// 从节点池获取连接
	return nodePool.Get(ctx)
}

// Put 归还连接到池
func (p *CrossNodePool) Put(conn *CrossNodeConn) {
	if conn == nil {
		return
	}

	atomic.AddInt64(&p.totalPuts, 1)

	// 如果连接已损坏，直接关闭
	if conn.IsBroken() {
		p.CloseConn(conn)
		return
	}

	// 归还到节点池
	p.poolsLock.RLock()
	nodePool, exists := p.pools[conn.nodeID]
	p.poolsLock.RUnlock()

	if exists {
		nodePool.Put(conn)
	} else {
		// 节点池不存在，直接关闭连接
		conn.Close()
	}
}

// CloseConn 关闭连接（不归还，直接销毁）
func (p *CrossNodePool) CloseConn(conn *CrossNodeConn) {
	if conn == nil {
		return
	}

	atomic.AddInt64(&p.totalClosed, 1)

	p.poolsLock.RLock()
	nodePool, exists := p.pools[conn.nodeID]
	p.poolsLock.RUnlock()

	if exists {
		nodePool.Remove(conn)
	}

	conn.Close()
}

// getOrCreateNodePool 获取或创建节点连接池
func (p *CrossNodePool) getOrCreateNodePool(nodeID string) (*NodeConnectionPool, error) {
	// 先尝试读取
	p.poolsLock.RLock()
	nodePool, exists := p.pools[nodeID]
	p.poolsLock.RUnlock()

	if exists {
		return nodePool, nil
	}

	// 需要创建新的节点池
	p.poolsLock.Lock()
	defer p.poolsLock.Unlock()

	// 双重检查
	if nodePool, exists = p.pools[nodeID]; exists {
		return nodePool, nil
	}

	// 获取节点地址
	nodeAddr, err := p.getNodeAddress(nodeID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to get node address")
	}

	// 创建节点连接池
	nodePool = NewNodeConnectionPool(
		p.Ctx(),
		nodeID,
		nodeAddr,
		p.config,
		&p.totalCreated,
	)

	p.pools[nodeID] = nodePool
	corelog.Infof("CrossNodePool: created pool for node %s at %s", nodeID, nodeAddr)

	return nodePool, nil
}

// getNodeAddress 获取节点地址
func (p *CrossNodePool) getNodeAddress(nodeID string) (string, error) {
	if p.storage == nil {
		// 默认使用节点 ID 作为主机名，跨节点 TCP 端口为 50052
		return fmt.Sprintf("%s:50052", nodeID), nil
	}

	key := fmt.Sprintf("tunnox:node:%s:addr", nodeID)
	value, err := p.storage.Get(key)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			// 默认使用节点 ID 作为主机名
			return fmt.Sprintf("%s:50052", nodeID), nil
		}
		return "", err
	}

	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return fmt.Sprintf("%s:50052", nodeID), nil
	}
}

// startIdleCleanup 启动空闲连接清理
func (p *CrossNodePool) startIdleCleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-p.Ctx().Done():
			return
		case <-ticker.C:
			p.cleanupIdleConnections()
		}
	}
}

// cleanupIdleConnections 清理空闲连接
func (p *CrossNodePool) cleanupIdleConnections() {
	p.poolsLock.RLock()
	pools := make([]*NodeConnectionPool, 0, len(p.pools))
	for _, pool := range p.pools {
		pools = append(pools, pool)
	}
	p.poolsLock.RUnlock()

	for _, pool := range pools {
		pool.CleanupIdle(p.config.IdleTimeout, p.config.MinConns)
	}
}

// closeAllPools 关闭所有节点池
func (p *CrossNodePool) closeAllPools() error {
	p.poolsLock.Lock()
	defer p.poolsLock.Unlock()

	for nodeID, pool := range p.pools {
		pool.CloseAll()
		delete(p.pools, nodeID)
	}

	corelog.Infof("CrossNodePool: closed all pools (gets=%d, puts=%d, created=%d, closed=%d)",
		p.totalGets, p.totalPuts, p.totalCreated, p.totalClosed)

	return nil
}

// Stats 返回连接池统计信息
func (p *CrossNodePool) Stats() map[string]int64 {
	return map[string]int64{
		"total_gets":    atomic.LoadInt64(&p.totalGets),
		"total_puts":    atomic.LoadInt64(&p.totalPuts),
		"total_created": atomic.LoadInt64(&p.totalCreated),
		"total_closed":  atomic.LoadInt64(&p.totalClosed),
	}
}

// ============================================================================
// NodeConnectionPool - 单节点连接池
// ============================================================================

// NodeConnectionPool 单节点连接池
type NodeConnectionPool struct {
	nodeID   string
	nodeAddr string
	config   CrossNodePoolConfig

	conns     chan *CrossNodeConn // 可用连接
	active    int32               // 活跃连接数（包括使用中和空闲的）
	inUse     int32               // 使用中的连接数
	mu        sync.Mutex
	closed    bool
	parentCtx context.Context

	// 统计指针（指向父池的统计）
	totalCreated *int64
}

// NewNodeConnectionPool 创建单节点连接池
func NewNodeConnectionPool(
	parentCtx context.Context,
	nodeID string,
	nodeAddr string,
	config CrossNodePoolConfig,
	totalCreated *int64,
) *NodeConnectionPool {
	return &NodeConnectionPool{
		nodeID:       nodeID,
		nodeAddr:     nodeAddr,
		config:       config,
		conns:        make(chan *CrossNodeConn, config.MaxConns),
		parentCtx:    parentCtx,
		totalCreated: totalCreated,
	}
}

// Get 获取连接
func (p *NodeConnectionPool) Get(ctx context.Context) (*CrossNodeConn, error) {
	// 先尝试从池中获取
	select {
	case conn := <-p.conns:
		if conn != nil && !conn.IsBroken() {
			conn.markInUse()
			atomic.AddInt32(&p.inUse, 1)
			return conn, nil
		}
		// 连接已损坏，关闭并继续
		if conn != nil {
			conn.Close()
			atomic.AddInt32(&p.active, -1)
		}
	default:
		// 池中没有可用连接
	}

	// 检查是否可以创建新连接
	if atomic.LoadInt32(&p.active) >= int32(p.config.MaxConns) {
		// 等待可用连接
		select {
		case conn := <-p.conns:
			if conn != nil && !conn.IsBroken() {
				conn.markInUse()
				atomic.AddInt32(&p.inUse, 1)
				return conn, nil
			}
			if conn != nil {
				conn.Close()
				atomic.AddInt32(&p.active, -1)
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(p.config.DialTimeout):
			return nil, coreerrors.New(coreerrors.CodeTimeout, "timeout waiting for connection")
		}
	}

	// 创建新连接
	return p.createConnection(ctx)
}

// createConnection 创建新连接
func (p *NodeConnectionPool) createConnection(ctx context.Context) (*CrossNodeConn, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, coreerrors.New(coreerrors.CodeUnavailable, "pool is closed")
	}
	p.mu.Unlock()

	// 建立 TCP 连接
	dialCtx, cancel := context.WithTimeout(ctx, p.config.DialTimeout)
	defer cancel()

	var d net.Dialer
	netConn, err := d.DialContext(dialCtx, "tcp", p.nodeAddr)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to dial node")
	}

	// 转换为 TCPConn（用于零拷贝）
	tcpConn, ok := netConn.(*net.TCPConn)
	if !ok {
		netConn.Close()
		return nil, coreerrors.New(coreerrors.CodeNetworkError, "connection is not TCP")
	}

	// 创建 CrossNodeConn
	conn := NewCrossNodeConn(p.parentCtx, p.nodeID, tcpConn, p)

	atomic.AddInt32(&p.active, 1)
	atomic.AddInt32(&p.inUse, 1)
	if p.totalCreated != nil {
		atomic.AddInt64(p.totalCreated, 1)
	}

	corelog.Debugf("NodeConnectionPool[%s]: created new connection to %s (active=%d)",
		p.nodeID, p.nodeAddr, atomic.LoadInt32(&p.active))

	return conn, nil
}

// Put 归还连接
func (p *NodeConnectionPool) Put(conn *CrossNodeConn) {
	if conn == nil {
		return
	}

	atomic.AddInt32(&p.inUse, -1)
	conn.markIdle()

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		conn.Close()
		atomic.AddInt32(&p.active, -1)
		return
	}
	p.mu.Unlock()

	// 尝试放回池中
	select {
	case p.conns <- conn:
		// 成功放回
	default:
		// 池已满，关闭连接
		conn.Close()
		atomic.AddInt32(&p.active, -1)
	}
}

// Remove 从池中移除连接（不归还）
func (p *NodeConnectionPool) Remove(conn *CrossNodeConn) {
	if conn == nil {
		return
	}
	if conn.inUse {
		atomic.AddInt32(&p.inUse, -1)
	}
	atomic.AddInt32(&p.active, -1)
}

// CleanupIdle 清理空闲连接
func (p *NodeConnectionPool) CleanupIdle(idleTimeout time.Duration, minConns int) {
	now := time.Now()
	cleaned := 0

	for {
		select {
		case conn := <-p.conns:
			if conn == nil {
				continue
			}

			// 检查是否超时且超过最小连接数
			if now.Sub(conn.lastUsed) > idleTimeout && atomic.LoadInt32(&p.active) > int32(minConns) {
				conn.Close()
				atomic.AddInt32(&p.active, -1)
				cleaned++
			} else {
				// 放回池中
				select {
				case p.conns <- conn:
				default:
					conn.Close()
					atomic.AddInt32(&p.active, -1)
				}
				return // 遇到未超时的连接，停止清理
			}
		default:
			// 池中没有更多连接
			if cleaned > 0 {
				corelog.Debugf("NodeConnectionPool[%s]: cleaned %d idle connections", p.nodeID, cleaned)
			}
			return
		}
	}
}

// CloseAll 关闭所有连接
func (p *NodeConnectionPool) CloseAll() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()

	// 关闭池中的所有连接
	close(p.conns)
	for conn := range p.conns {
		if conn != nil {
			conn.Close()
		}
	}

	corelog.Infof("NodeConnectionPool[%s]: closed all connections (active=%d, inUse=%d)",
		p.nodeID, atomic.LoadInt32(&p.active), atomic.LoadInt32(&p.inUse))
}
