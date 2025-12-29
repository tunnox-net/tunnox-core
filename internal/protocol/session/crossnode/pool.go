// Package crossnode 提供跨节点通信功能
package crossnode

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// PoolConfig 跨节点连接池配置
type PoolConfig struct {
	MinConns    int           `json:"min_conns"`    // 每节点最小连接数，默认 2
	MaxConns    int           `json:"max_conns"`    // 每节点最大连接数，默认 10
	IdleTimeout time.Duration `json:"idle_timeout"` // 空闲连接超时，默认 5 分钟
	DialTimeout time.Duration `json:"dial_timeout"` // 建立连接超时，默认 5 秒
}

// DefaultPoolConfig 返回默认配置
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MinConns:    5,                // 保持热连接减少延迟
		MaxConns:    100,              // 支持高并发（每个隧道复用连接）
		IdleTimeout: 10 * time.Minute, // 减少频繁重建
		DialTimeout: 5 * time.Second,
	}
}

// Pool 跨节点连接池
// 管理到其他节点的 TCP 连接，支持连接复用和池化
type Pool struct {
	*dispose.ServiceBase

	storage   storage.Storage
	nodeID    string // 当前节点 ID
	pools     map[string]*NodeConnectionPool
	poolsLock sync.RWMutex
	config    PoolConfig

	// 统计信息
	totalGets    int64
	totalPuts    int64
	totalCreated int64
	totalClosed  int64
}

// NewPool 创建跨节点连接池
func NewPool(
	parentCtx context.Context,
	storage storage.Storage,
	nodeID string,
	config PoolConfig,
) *Pool {
	pool := &Pool{
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
func (p *Pool) Get(ctx context.Context, targetNodeID string) (*Conn, error) {
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
func (p *Pool) Put(conn *Conn) {
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
func (p *Pool) CloseConn(conn *Conn) {
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
func (p *Pool) getOrCreateNodePool(nodeID string) (*NodeConnectionPool, error) {
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
func (p *Pool) getNodeAddress(nodeID string) (string, error) {
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
func (p *Pool) startIdleCleanup() {
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
func (p *Pool) cleanupIdleConnections() {
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
func (p *Pool) closeAllPools() error {
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
func (p *Pool) Stats() map[string]int64 {
	return map[string]int64{
		"total_gets":    atomic.LoadInt64(&p.totalGets),
		"total_puts":    atomic.LoadInt64(&p.totalPuts),
		"total_created": atomic.LoadInt64(&p.totalCreated),
		"total_closed":  atomic.LoadInt64(&p.totalClosed),
	}
}
