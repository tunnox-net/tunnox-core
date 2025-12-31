package types

import (
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
)

// CloudControl 云控制接口
// 定义 SessionManager 等组件需要的云控制功能
// 实现方：cloud/services/CloudControlAPI 或适配器
type CloudControl interface {
	// GetPortMapping 获取端口映射配置
	GetPortMapping(mappingID string) (*models.PortMapping, error)

	// UpdatePortMappingStats 更新端口映射统计信息
	UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error

	// GetClientPortMappings 获取客户端的所有端口映射
	GetClientPortMappings(clientID int64) ([]*models.PortMapping, error)
}

// BridgeManager 桥接管理器接口
// 定义跨服务器隧道转发功能
type BridgeManager interface {
	// ForwardToNode 转发数据到指定节点
	ForwardToNode(nodeID string, tunnelID string, data []byte) error

	// Subscribe 订阅跨节点消息
	Subscribe(topic string, handler func(data []byte) error) error

	// Publish 发布跨节点消息
	Publish(topic string, data []byte) error

	// GetNodeID 获取当前节点ID
	GetNodeID() string
}
