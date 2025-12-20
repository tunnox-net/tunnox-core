package bridge

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// NodeConnectionPool 单节点的连接池
type NodeConnectionPool struct {
	*dispose.ManagerBase
	targetNodeID      string
	targetAddr        string
	connections       []MultiplexedConn
	connsMu           sync.RWMutex
	minConns          int32
	maxConns          int32
	maxIdleTime       time.Duration
	maxStreamsPerConn int32
	dialOptions       []grpc.DialOption
}

// NodePoolConfig 节点连接池配置
type NodePoolConfig struct {
	MinConns          int32
	MaxConns          int32
	MaxIdleTime       time.Duration
	MaxStreamsPerConn int32
	DialTimeout       time.Duration
}

// NewNodeConnectionPool 创建节点连接池
func NewNodeConnectionPool(parentCtx context.Context, targetNodeID, targetAddr string, config *NodePoolConfig) (*NodeConnectionPool, error) {
	if config == nil {
		config = &NodePoolConfig{
			MinConns:          2,
			MaxConns:          10,
			MaxIdleTime:       5 * time.Minute,
			MaxStreamsPerConn: 100,
			DialTimeout:       10 * time.Second,
		}
	}

	pool := &NodeConnectionPool{
		ManagerBase:       dispose.NewManager(fmt.Sprintf("NodeConnectionPool-%s", targetNodeID), parentCtx),
		targetNodeID:      targetNodeID,
		targetAddr:        targetAddr,
		connections:       make([]MultiplexedConn, 0, config.MaxConns),
		minConns:          config.MinConns,
		maxConns:          config.MaxConns,
		maxIdleTime:       config.MaxIdleTime,
		maxStreamsPerConn: config.MaxStreamsPerConn,
		dialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
		},
	}

	// 初始化最小连接数
	if err := pool.initializeMinConnections(); err != nil {
		return nil, fmt.Errorf("failed to initialize min connections: %w", err)
	}

	// 启动清理协程
	go pool.cleanupLoop()

	corelog.Infof("NodeConnectionPool: created pool for node %s (min:%d, max:%d, max_streams:%d)",
		targetNodeID, config.MinConns, config.MaxConns, config.MaxStreamsPerConn)
	return pool, nil
}

// initializeMinConnections 初始化最小连接数
func (p *NodeConnectionPool) initializeMinConnections() error {
	for i := int32(0); i < p.minConns; i++ {
		if _, err := p.createConnection(); err != nil {
			return fmt.Errorf("failed to create initial connection %d: %w", i, err)
		}
	}
	return nil
}

// createConnection 创建新的连接
func (p *NodeConnectionPool) createConnection() (MultiplexedConn, error) {
	dialCtx, cancel := context.WithTimeout(p.Ctx(), 10*time.Second)
	defer cancel()

	grpcConn, err := grpc.DialContext(dialCtx, p.targetAddr, p.dialOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial node %s: %w", p.targetNodeID, err)
	}

	mc, err := NewMultiplexedConn(p.Ctx(), p.targetNodeID, grpcConn, p.maxStreamsPerConn)
	if err != nil {
		grpcConn.Close()
		return nil, fmt.Errorf("failed to create multiplexed connection: %w", err)
	}

	p.connsMu.Lock()
	p.connections = append(p.connections, mc)
	p.connsMu.Unlock()

	corelog.Infof("NodeConnectionPool: created new connection to node %s (total: %d)",
		p.targetNodeID, len(p.connections))
	return mc, nil
}

// GetOrCreateSession 获取或创建会话
func (p *NodeConnectionPool) GetOrCreateSession(ctx context.Context, metadata *SessionMetadata) (*ForwardSession, error) {
	p.connsMu.RLock()

	// 优先从现有连接中查找可用的
	for _, conn := range p.connections {
		if conn.CanAcceptStream() {
			p.connsMu.RUnlock()
			session := NewForwardSession(ctx, conn, metadata)
			if session != nil {
				corelog.Debugf("NodeConnectionPool: reused connection for new session %s", session.StreamID())
				return session, nil
			}
		}
	}

	currentCount := int32(len(p.connections))
	p.connsMu.RUnlock()

	// 如果所有连接都满了，且未达到最大连接数，创建新连接
	if currentCount < p.maxConns {
		conn, err := p.createConnection()
		if err != nil {
			return nil, fmt.Errorf("failed to create new connection: %w", err)
		}

		session := NewForwardSession(ctx, conn, metadata)
		if session != nil {
			corelog.Infof("NodeConnectionPool: created new connection and session %s", session.StreamID())
			return session, nil
		}
	}

	return nil, fmt.Errorf("no available connection for node %s (all connections at max capacity)", p.targetNodeID)
}

// cleanupLoop 清理空闲连接
func (p *NodeConnectionPool) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanupIdleConnections()
		case <-p.Ctx().Done():
			corelog.Infof("NodeConnectionPool: cleanup loop stopped for node %s", p.targetNodeID)
			return
		}
	}
}

// cleanupIdleConnections 清理空闲连接
func (p *NodeConnectionPool) cleanupIdleConnections() {
	p.connsMu.Lock()
	defer p.connsMu.Unlock()

	activeConns := make([]MultiplexedConn, 0, len(p.connections))
	closedCount := 0

	for _, conn := range p.connections {
		// 保留最小连接数
		if int32(len(activeConns)) < p.minConns {
			activeConns = append(activeConns, conn)
			continue
		}

		// 关闭空闲连接
		if conn.IsIdle(p.maxIdleTime) {
			conn.Close()
			closedCount++
			corelog.Debugf("NodeConnectionPool: closed idle connection to node %s", p.targetNodeID)
		} else {
			activeConns = append(activeConns, conn)
		}
	}

	p.connections = activeConns

	if closedCount > 0 {
		corelog.Infof("NodeConnectionPool: cleaned up %d idle connections for node %s (remaining: %d)",
			closedCount, p.targetNodeID, len(p.connections))
	}
}

// GetStats 获取连接池统计信息
func (p *NodeConnectionPool) GetStats() PoolStats {
	p.connsMu.RLock()
	defer p.connsMu.RUnlock()

	totalStreams := int32(0)
	for _, conn := range p.connections {
		totalStreams += conn.GetActiveStreams()
	}

	return PoolStats{
		NodeID:            p.targetNodeID,
		TotalConns:        int32(len(p.connections)),
		ActiveStreams:     totalStreams,
		MaxConns:          p.maxConns,
		MaxStreamsPerConn: p.maxStreamsPerConn,
	}
}

// Close 关闭连接池
func (p *NodeConnectionPool) Close() error {
	p.connsMu.Lock()

	// 关闭所有连接
	for _, conn := range p.connections {
		if err := conn.Close(); err != nil {
			corelog.Warnf("NodeConnectionPool: failed to close connection: %v", err)
		}
	}

	p.connections = nil
	p.connsMu.Unlock()

	corelog.Infof("NodeConnectionPool: closed pool for node %s", p.targetNodeID)

	// 调用基类 Close
	return p.ManagerBase.Close()
}

// PoolStats 连接池统计信息
type PoolStats struct {
	NodeID            string
	TotalConns        int32
	ActiveStreams     int32
	MaxConns          int32
	MaxStreamsPerConn int32
}
