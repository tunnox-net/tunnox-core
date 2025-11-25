package bridge

import (
	"context"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/utils"
)

// BridgeConnectionPool 跨节点桥接连接池（管理多个节点的连接）
type BridgeConnectionPool struct {
	*dispose.ManagerBase
	nodePools        map[string]*NodeConnectionPool
	nodePoolsMu      sync.RWMutex
	config           *PoolConfig
	metricsCollector *MetricsCollector
}

// PoolConfig 连接池配置
type PoolConfig struct {
	MinConnsPerNode     int32         // 每个节点的最小连接数
	MaxConnsPerNode     int32         // 每个节点的最大连接数
	MaxIdleTime         time.Duration // 连接最大空闲时间
	MaxStreamsPerConn   int32         // 每个连接的最大流数量
	DialTimeout         time.Duration // 拨号超时时间
	HealthCheckInterval time.Duration // 健康检查间隔
}

// DefaultPoolConfig 返回默认配置
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MinConnsPerNode:     2,
		MaxConnsPerNode:     10,
		MaxIdleTime:         5 * time.Minute,
		MaxStreamsPerConn:   100,
		DialTimeout:         10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}
}

// NewBridgeConnectionPool 创建连接池
func NewBridgeConnectionPool(parentCtx context.Context, config *PoolConfig) *BridgeConnectionPool {
	if config == nil {
		config = DefaultPoolConfig()
	}

	pool := &BridgeConnectionPool{
		ManagerBase:      dispose.NewManager("BridgeConnectionPool", parentCtx),
		nodePools:        make(map[string]*NodeConnectionPool),
		config:           config,
		metricsCollector: NewMetricsCollector(),
	}

	// 启动健康检查和指标收集
	go pool.healthCheckLoop()
	go pool.metricsCollectionLoop()

	utils.Infof("BridgeConnectionPool: initialized with config: min=%d, max=%d, max_streams=%d",
		config.MinConnsPerNode, config.MaxConnsPerNode, config.MaxStreamsPerConn)
	return pool
}

// GetOrCreateNodePool 获取或创建节点连接池
func (p *BridgeConnectionPool) getOrCreateNodePool(nodeID, nodeAddr string) (*NodeConnectionPool, error) {
	p.nodePoolsMu.RLock()
	if pool, exists := p.nodePools[nodeID]; exists {
		p.nodePoolsMu.RUnlock()
		return pool, nil
	}
	p.nodePoolsMu.RUnlock()

	// 创建新的节点池
	p.nodePoolsMu.Lock()
	defer p.nodePoolsMu.Unlock()

	// 双重检查
	if pool, exists := p.nodePools[nodeID]; exists {
		return pool, nil
	}

	poolConfig := &NodePoolConfig{
		MinConns:          p.config.MinConnsPerNode,
		MaxConns:          p.config.MaxConnsPerNode,
		MaxIdleTime:       p.config.MaxIdleTime,
		MaxStreamsPerConn: p.config.MaxStreamsPerConn,
		DialTimeout:       p.config.DialTimeout,
	}

	nodePool, err := NewNodeConnectionPool(p.Ctx(), nodeID, nodeAddr, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create node pool for %s: %w", nodeID, err)
	}

	p.nodePools[nodeID] = nodePool
	utils.Infof("BridgeConnectionPool: created node pool for %s at %s", nodeID, nodeAddr)
	return nodePool, nil
}

// CreateSession 创建转发会话
func (p *BridgeConnectionPool) CreateSession(ctx context.Context, targetNodeID, targetNodeAddr string, metadata *SessionMetadata) (*ForwardSession, error) {
	nodePool, err := p.getOrCreateNodePool(targetNodeID, targetNodeAddr)
	if err != nil {
		p.metricsCollector.RecordError(targetNodeID, "create_pool_failed")
		return nil, err
	}

	session, err := nodePool.GetOrCreateSession(ctx, metadata)
	if err != nil {
		p.metricsCollector.RecordError(targetNodeID, "create_session_failed")
		return nil, err
	}

	p.metricsCollector.RecordSessionCreated(targetNodeID)
	utils.Infof("BridgeConnectionPool: created session %s to node %s", session.StreamID(), targetNodeID)
	return session, nil
}

// GetNodePool 获取指定节点的连接池
func (p *BridgeConnectionPool) GetNodePool(nodeID string) (*NodeConnectionPool, bool) {
	p.nodePoolsMu.RLock()
	defer p.nodePoolsMu.RUnlock()
	pool, exists := p.nodePools[nodeID]
	return pool, exists
}

// GetAllStats 获取所有节点的统计信息
func (p *BridgeConnectionPool) GetAllStats() map[string]PoolStats {
	p.nodePoolsMu.RLock()
	defer p.nodePoolsMu.RUnlock()

	stats := make(map[string]PoolStats)
	for nodeID, pool := range p.nodePools {
		stats[nodeID] = pool.GetStats()
	}
	return stats
}

// healthCheckLoop 健康检查循环
func (p *BridgeConnectionPool) healthCheckLoop() {
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.performHealthCheck()
		case <-p.Ctx().Done():
			utils.Infof("BridgeConnectionPool: health check loop stopped")
			return
		}
	}
}

// performHealthCheck 执行健康检查
func (p *BridgeConnectionPool) performHealthCheck() {
	p.nodePoolsMu.RLock()
	pools := make([]*NodeConnectionPool, 0, len(p.nodePools))
	for _, pool := range p.nodePools {
		pools = append(pools, pool)
	}
	p.nodePoolsMu.RUnlock()

	for _, pool := range pools {
		stats := pool.GetStats()
		utils.Debugf("NodePool[%s]: conns=%d, streams=%d",
			stats.NodeID, stats.TotalConns, stats.ActiveStreams)

		p.metricsCollector.UpdatePoolStats(stats.NodeID, stats.TotalConns, stats.ActiveStreams)
	}
}

// metricsCollectionLoop 指标收集循环
func (p *BridgeConnectionPool) metricsCollectionLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := p.metricsCollector.GetMetrics()
			utils.Debugf("BridgeConnectionPool metrics: %+v", metrics)
		case <-p.Ctx().Done():
			utils.Infof("BridgeConnectionPool: metrics collection loop stopped")
			return
		}
	}
}

// RemoveNodePool 移除节点连接池
func (p *BridgeConnectionPool) RemoveNodePool(nodeID string) error {
	p.nodePoolsMu.Lock()
	defer p.nodePoolsMu.Unlock()

	pool, exists := p.nodePools[nodeID]
	if !exists {
		return fmt.Errorf("node pool not found: %s", nodeID)
	}

	if err := pool.Close(); err != nil {
		return fmt.Errorf("failed to close node pool: %w", err)
	}

	delete(p.nodePools, nodeID)
	utils.Infof("BridgeConnectionPool: removed node pool for %s", nodeID)
	return nil
}

// Close 关闭连接池
func (p *BridgeConnectionPool) Close() error {
	p.nodePoolsMu.Lock()

	// 关闭所有节点池
	for nodeID, pool := range p.nodePools {
		if err := pool.Close(); err != nil {
			utils.Errorf("BridgeConnectionPool: failed to close pool for node %s: %v", nodeID, err)
		}
	}

	p.nodePools = make(map[string]*NodeConnectionPool)
	p.nodePoolsMu.Unlock()

	utils.Infof("BridgeConnectionPool: closed all node pools")

	// 调用基类 Close
	return p.ManagerBase.Close()
}

// GetMetrics 获取连接池指标
func (p *BridgeConnectionPool) GetMetrics() *PoolMetrics {
	return p.metricsCollector.GetMetrics()
}
