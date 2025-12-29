package client

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/stream"
)

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
