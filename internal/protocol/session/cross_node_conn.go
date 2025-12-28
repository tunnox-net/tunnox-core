// Package session 提供会话管理功能
package session

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
)

// CrossNodeConn 跨节点连接
// 封装到其他节点的 TCP 连接，支持零拷贝和连接池复用
type CrossNodeConn struct {
	*dispose.ServiceBase

	nodeID    string
	tcpConn   *net.TCPConn
	pool      *NodeConnectionPool
	createdAt time.Time
	lastUsed  time.Time
	inUse     bool
	broken    bool
	mu        sync.Mutex
}

// NewCrossNodeConn 创建跨节点连接
func NewCrossNodeConn(
	parentCtx context.Context,
	nodeID string,
	tcpConn *net.TCPConn,
	pool *NodeConnectionPool,
) *CrossNodeConn {
	conn := &CrossNodeConn{
		ServiceBase: dispose.NewService("CrossNodeConn", parentCtx),
		nodeID:      nodeID,
		tcpConn:     tcpConn,
		pool:        pool,
		createdAt:   time.Now(),
		lastUsed:    time.Now(),
		inUse:       true,
	}

	// 添加清理处理器
	conn.AddCleanHandler(func() error {
		return conn.closeInternal()
	})

	return conn
}

// GetTCPConn 获取底层 TCP 连接（用于零拷贝 splice）
func (c *CrossNodeConn) GetTCPConn() *net.TCPConn {
	return c.tcpConn
}

// NodeID 返回目标节点 ID
func (c *CrossNodeConn) NodeID() string {
	return c.nodeID
}

// Read 实现 io.Reader
func (c *CrossNodeConn) Read(p []byte) (n int, err error) {
	if c.tcpConn == nil {
		return 0, io.EOF
	}
	n, err = c.tcpConn.Read(p)
	if err != nil {
		c.MarkBroken()
	}
	return
}

// Write 实现 io.Writer
func (c *CrossNodeConn) Write(p []byte) (n int, err error) {
	if c.tcpConn == nil {
		return 0, io.ErrClosedPipe
	}
	n, err = c.tcpConn.Write(p)
	if err != nil {
		c.MarkBroken()
	}
	return
}

// Release 归还连接到池
func (c *CrossNodeConn) Release() {
	c.mu.Lock()
	if !c.inUse {
		c.mu.Unlock()
		return
	}
	c.inUse = false
	c.lastUsed = time.Now()
	c.mu.Unlock()

	if c.pool != nil && !c.broken {
		c.pool.Put(c)
	} else {
		c.Close()
	}
}

// MarkBroken 标记连接为损坏
func (c *CrossNodeConn) MarkBroken() {
	c.mu.Lock()
	c.broken = true
	c.mu.Unlock()
}

// IsBroken 检查连接是否损坏
func (c *CrossNodeConn) IsBroken() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.broken
}

// IsHealthy 检查连接健康状态（用于连接池健康检查）
// 🔥 新增：检查TCP连接是否真的可用
func (c *CrossNodeConn) IsHealthy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. 检查是否已标记为broken
	if c.broken {
		return false
	}

	// 2. 检查TCP连接是否存在
	if c.tcpConn == nil {
		return false
	}

	// 3. 检查连接是否超过最大空闲时间（5分钟）
	maxIdleTime := 5 * time.Minute
	if time.Since(c.lastUsed) > maxIdleTime {
		corelog.Debugf("CrossNodeConn[%s]: connection idle for %v, marking as unhealthy",
			c.nodeID, time.Since(c.lastUsed))
		return false
	}

	// 4. 尝试设置读超时来检测连接是否可用
	// 设置一个很短的超时，尝试读取0字节
	oldDeadline := time.Time{}
	c.tcpConn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	defer c.tcpConn.SetReadDeadline(oldDeadline)

	// 尝试从连接读取（应该超时或返回0）
	one := make([]byte, 1)
	_, err := c.tcpConn.Read(one)
	if err != nil {
		// 检查是否是超时错误（正常情况）
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return true // 超时说明连接正常，只是没有数据
		}
		// 其他错误说明连接已断开
		corelog.Debugf("CrossNodeConn[%s]: health check failed: %v", c.nodeID, err)
		return false
	}

	// 如果读到了数据，这不正常（应该没有数据可读）
	// 但也说明连接是通的，标记为健康但记录警告
	corelog.Warnf("CrossNodeConn[%s]: unexpected data during health check", c.nodeID)
	return true
}

// markInUse 标记为使用中
func (c *CrossNodeConn) markInUse() {
	c.mu.Lock()
	c.inUse = true
	c.lastUsed = time.Now()
	c.mu.Unlock()
}

// markIdle 标记为空闲
func (c *CrossNodeConn) markIdle() {
	c.mu.Lock()
	c.inUse = false
	c.lastUsed = time.Now()
	c.mu.Unlock()
}

// closeInternal 内部关闭方法
func (c *CrossNodeConn) closeInternal() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.tcpConn != nil {
		err := c.tcpConn.Close()
		c.tcpConn = nil
		corelog.Debugf("CrossNodeConn[%s]: closed connection", c.nodeID)
		return err
	}
	return nil
}

// SetDeadline 设置读写超时
func (c *CrossNodeConn) SetDeadline(t time.Time) error {
	if c.tcpConn == nil {
		return io.ErrClosedPipe
	}
	return c.tcpConn.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (c *CrossNodeConn) SetReadDeadline(t time.Time) error {
	if c.tcpConn == nil {
		return io.ErrClosedPipe
	}
	return c.tcpConn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (c *CrossNodeConn) SetWriteDeadline(t time.Time) error {
	if c.tcpConn == nil {
		return io.ErrClosedPipe
	}
	return c.tcpConn.SetWriteDeadline(t)
}

// LocalAddr 返回本地地址
func (c *CrossNodeConn) LocalAddr() net.Addr {
	if c.tcpConn == nil {
		return nil
	}
	return c.tcpConn.LocalAddr()
}

// RemoteAddr 返回远程地址
func (c *CrossNodeConn) RemoteAddr() net.Addr {
	if c.tcpConn == nil {
		return nil
	}
	return c.tcpConn.RemoteAddr()
}
