package core

import (
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
)

// CloudControlAPI 定义 CloudControl 接口
// 统一返回 *models.PortMapping，不使用 interface{}
type CloudControlAPI interface {
	GetPortMapping(mappingID string) (*models.PortMapping, error)
	UpdatePortMappingStats(mappingID string, trafficStats *stats.TrafficStats) error
	GetClientPortMappings(clientID int64) ([]*models.PortMapping, error)
	TouchClient(clientID int64)            // 刷新客户端状态 TTL（心跳时调用）
	DisconnectClient(clientID int64) error // 断开客户端连接（触发 webhook 通知）
}

// CloudControlAdapter 适配器，将 BuiltinCloudControl 转换为 CloudControlAPI 接口
type CloudControlAdapter struct {
	cc *managers.BuiltinCloudControl
}

// NewCloudControlAdapter 创建适配器
func NewCloudControlAdapter(cc *managers.BuiltinCloudControl) CloudControlAPI {
	return &CloudControlAdapter{cc: cc}
}

// GetPortMapping 实现 CloudControlAPI 接口
func (a *CloudControlAdapter) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	return a.cc.GetPortMapping(mappingID)
}

// UpdatePortMappingStats 更新端口映射统计
func (a *CloudControlAdapter) UpdatePortMappingStats(mappingID string, trafficStats *stats.TrafficStats) error {
	return a.cc.UpdatePortMappingStats(mappingID, trafficStats)
}

// GetClientPortMappings 获取客户端的所有端口映射
func (a *CloudControlAdapter) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	return a.cc.GetClientPortMappings(clientID)
}

// TouchClient 刷新客户端状态 TTL
func (a *CloudControlAdapter) TouchClient(clientID int64) {
	a.cc.TouchClient(clientID)
}

// DisconnectClient 断开客户端连接（触发 webhook 通知）
func (a *CloudControlAdapter) DisconnectClient(clientID int64) error {
	return a.cc.DisconnectClient(clientID)
}
