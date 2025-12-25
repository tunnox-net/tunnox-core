package mapping

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/stream"
)

// PooledTunnelConnInterface 池化隧道连接接口
type PooledTunnelConnInterface interface {
	GetReader() io.Reader
	GetWriter() io.Writer
	GetConn() net.Conn
	GetStream() stream.PackageStreamer
	TunnelID() string
}

// ClientInterface 客户端接口（解耦TunnoxClient）
// 通过接口依赖，而不是具体类型，提高可测试性
type ClientInterface interface {
	// DialTunnel 建立隧道连接
	DialTunnel(tunnelID, mappingID, secretKey string) (net.Conn, stream.PackageStreamer, error)

	// DialTunnelPooled 从连接池获取隧道连接（如果启用了连接池）
	// 返回 nil 表示不使用连接池，应该使用 DialTunnel
	DialTunnelPooled(mappingID, secretKey string) (PooledTunnelConnInterface, error)

	// ReturnTunnelToPool 归还连接到池中
	ReturnTunnelToPool(conn PooledTunnelConnInterface)

	// CloseTunnelFromPool 关闭连接（不归还到池）
	CloseTunnelFromPool(conn PooledTunnelConnInterface)

	// IsTunnelPoolEnabled 是否启用了连接池
	IsTunnelPoolEnabled() bool

	// GetContext 获取上下文
	GetContext() context.Context

	// CheckMappingQuota 检查映射配额（连接数、流量等）
	CheckMappingQuota(mappingID string) error

	// TrackTraffic 上报流量统计
	TrackTraffic(mappingID string, bytesSent, bytesReceived int64) error

	// GetUserQuota 获取用户配额信息
	GetUserQuota() (*models.UserQuota, error)

	// GetServerProtocol 获取服务器协议（用于选择拷贝策略）
	GetServerProtocol() string
}

// TrafficStats 流量统计（本地缓存）
type TrafficStats struct {
	BytesSent       atomic.Int64 // 发送字节数
	BytesReceived   atomic.Int64 // 接收字节数
	ConnectionCount atomic.Int64 // 总连接数
	LastReportTime  time.Time    // 上次上报时间
	mu              sync.RWMutex
}

// Reset 重置统计数据
func (t *TrafficStats) Reset() {
	t.BytesSent.Store(0)
	t.BytesReceived.Store(0)
	t.mu.Lock()
	t.LastReportTime = time.Now()
	t.mu.Unlock()
}

// GetStats 获取统计数据快照
func (t *TrafficStats) GetStats() (sent, received, count int64) {
	return t.BytesSent.Load(), t.BytesReceived.Load(), t.ConnectionCount.Load()
}
