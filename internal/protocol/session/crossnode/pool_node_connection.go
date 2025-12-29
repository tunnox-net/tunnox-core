// Package crossnode 提供跨节点通信功能
package crossnode

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// ============================================================================
// NodeConnectionPool - 单节点连接池
// ============================================================================

// NodeConnectionPool 单节点连接池
type NodeConnectionPool struct {
	nodeID   string
	nodeAddr string
	config   PoolConfig

	conns     chan *Conn // 可用连接
	active    int32      // 活跃连接数（包括使用中和空闲的）
	inUse     int32      // 使用中的连接数
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
	config PoolConfig,
	totalCreated *int64,
) *NodeConnectionPool {
	return &NodeConnectionPool{
		nodeID:       nodeID,
		nodeAddr:     nodeAddr,
		config:       config,
		conns:        make(chan *Conn, config.MaxConns),
		parentCtx:    parentCtx,
		totalCreated: totalCreated,
	}
}

// Get 获取连接
func (p *NodeConnectionPool) Get(ctx context.Context) (*Conn, error) {
	// 支持多次重试，从池中获取健康的连接
	maxRetries := 3
	for retry := 0; retry < maxRetries; retry++ {
		// 先尝试从池中获取
		select {
		case conn := <-p.conns:
			if conn != nil {
				// 完整的健康检查
				if conn.IsHealthy() {
					conn.MarkInUse()
					atomic.AddInt32(&p.inUse, 1)
					corelog.Debugf("NodeConnectionPool[%s]: reused connection from pool", p.nodeID)
					return conn, nil
				}
				// 连接不健康，关闭并继续重试
				corelog.Debugf("NodeConnectionPool[%s]: connection unhealthy, closing (retry %d/%d)",
					p.nodeID, retry+1, maxRetries)
				conn.Close()
				atomic.AddInt32(&p.active, -1)
				continue
			}
		default:
			// 池中没有可用连接，跳出重试循环
			break
		}
	}

	// 检查是否可以创建新连接
	if atomic.LoadInt32(&p.active) >= int32(p.config.MaxConns) {
		// 等待可用连接
		select {
		case conn := <-p.conns:
			if conn != nil {
				// 等待时获取的连接也要做健康检查
				if conn.IsHealthy() {
					conn.MarkInUse()
					atomic.AddInt32(&p.inUse, 1)
					return conn, nil
				}
				conn.Close()
				atomic.AddInt32(&p.active, -1)
			}
			// 连接不健康，递归重试
			return p.Get(ctx)
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(p.config.DialTimeout):
			return nil, coreerrors.New(coreerrors.CodeTimeout, "timeout waiting for connection")
		}
	}

	// 创建新连接
	corelog.Debugf("NodeConnectionPool[%s]: creating new connection (active=%d)", p.nodeID, atomic.LoadInt32(&p.active))
	return p.createConnection(ctx)
}

// createConnection 创建新连接
func (p *NodeConnectionPool) createConnection(ctx context.Context) (*Conn, error) {
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

	// 创建 Conn
	conn := NewConn(p.parentCtx, p.nodeID, tcpConn, p)

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
func (p *NodeConnectionPool) Put(conn *Conn) {
	if conn == nil {
		return
	}

	atomic.AddInt32(&p.inUse, -1)
	conn.MarkIdle()

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
func (p *NodeConnectionPool) Remove(conn *Conn) {
	if conn == nil {
		return
	}
	if conn.IsInUse() {
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
			if now.Sub(conn.GetLastUsed()) > idleTimeout && atomic.LoadInt32(&p.active) > int32(minConns) {
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
