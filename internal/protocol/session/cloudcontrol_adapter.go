package session

import (
	"fmt"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	cloudstats "tunnox-core/internal/cloud/stats"
)

// CloudControlAdapter 适配器，将 BuiltinCloudControl 转换为 SessionManager 所需的接口
type CloudControlAdapter struct {
	cc *managers.BuiltinCloudControl
}

// NewCloudControlAdapter 创建适配器
func NewCloudControlAdapter(cc *managers.BuiltinCloudControl) CloudControlAPI {
	return &CloudControlAdapter{cc: cc}
}

// GetPortMapping 实现 CloudControlAPI 接口
// ✅ 统一返回 *models.PortMapping，不再使用 interface{}
func (a *CloudControlAdapter) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	return a.cc.GetPortMapping(mappingID)
}

// UpdatePortMappingStats 更新端口映射统计
func (a *CloudControlAdapter) UpdatePortMappingStats(mappingID string, stats interface{}) error {
	trafficStats, ok := stats.(*cloudstats.TrafficStats)
	if !ok {
		return fmt.Errorf("invalid stats type: expected *cloudstats.TrafficStats, got %T", stats)
	}
	return a.cc.UpdatePortMappingStats(mappingID, trafficStats)
}
