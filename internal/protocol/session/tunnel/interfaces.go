// Package tunnel 提供隧道桥接和路由功能
package tunnel

import (
	"net"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/stream"
)

// ============================================================================
// 隧道连接接口（从 session 包抽象）
// 注意：这个接口是 session.TunnelConnectionInterface 的子集
// 仅包含 Bridge 需要的方法，确保 session 包的实现可以满足这个接口
// ============================================================================

// TunnelConnectionInterface 隧道连接接口（Bridge 需要的最小接口）
type TunnelConnectionInterface interface {
	// 基础信息
	GetConnectionID() string // 连接标识
	GetClientID() int64      // 客户端ID
	GetMappingID() string    // 映射ID
	GetTunnelID() string     // 隧道ID

	// 流接口
	GetStream() stream.PackageStreamer // 获取流
	GetNetConn() net.Conn              // 获取底层连接

	// 生命周期
	Close() error   // 关闭连接
	IsClosed() bool // 检查是否已关闭
}

// ============================================================================
// 云控接口（用于流量统计）
// ============================================================================

// CloudControlAPI 云控API接口（用于流量统计）
type CloudControlAPI interface {
	GetPortMapping(mappingID string) (*models.PortMapping, error)
	UpdatePortMappingStats(mappingID string, stats interface{}) error
	GetClientPortMappings(clientID int64) ([]*models.PortMapping, error)
}

// ============================================================================
// 跨节点连接接口
// ============================================================================

// CrossNodeConnInterface 跨节点连接接口
type CrossNodeConnInterface interface {
	// GetNodeID 获取节点ID
	GetNodeID() string

	// GetReader 获取读取器
	GetReader() interface{}

	// GetWriter 获取写入器
	GetWriter() interface{}

	// Close 关闭连接
	Close() error

	// Release 释放连接（归还到池）
	Release()
}
