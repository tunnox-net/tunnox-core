package managers

import (
	coreerrors "tunnox-core/internal/core/errors"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
)

// CreatePortMapping 创建端口映射
// 注意：此方法委托给 PortMappingService 处理，遵循 Manager -> Service -> Repository 架构
func (c *CloudControl) CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error) {
	if c.portMappingService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.CreatePortMapping(mapping)
}

// GetPortMapping 获取端口映射
func (c *CloudControl) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	if c.portMappingService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.GetPortMapping(mappingID)
}

// GetPortMappingByDomain 通过域名查找 HTTP 映射
func (c *CloudControl) GetPortMappingByDomain(fullDomain string) (*models.PortMapping, error) {
	if c.portMappingService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.GetPortMappingByDomain(fullDomain)
}

// UpdatePortMapping 更新端口映射
func (c *CloudControl) UpdatePortMapping(mapping *models.PortMapping) error {
	if c.portMappingService == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.UpdatePortMapping(mapping)
}

// DeletePortMapping 删除端口映射
func (c *CloudControl) DeletePortMapping(mappingID string) error {
	if c.portMappingService == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.DeletePortMapping(mappingID)
}

// CleanupOrphanedMappingIndexes 清理孤立的映射索引
// 当主数据不存在但索引中仍有残留时调用
func (c *CloudControl) CleanupOrphanedMappingIndexes(mappingID, userID string, mappingData map[string]interface{}) error {
	if c.portMappingService == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.CleanupOrphanedMappingIndexes(mappingID, userID, mappingData)
}

// UpdatePortMappingStatus 更新端口映射状态
func (c *CloudControl) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	if c.portMappingService == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.UpdatePortMappingStatus(mappingID, status)
}

// UpdatePortMappingStats 更新端口映射统计
func (c *CloudControl) UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error {
	if c.portMappingService == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.UpdatePortMappingStats(mappingID, stats)
}

// ListPortMappings 列出端口映射
func (c *CloudControl) ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error) {
	if c.portMappingService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.ListPortMappings(mappingType)
}

// GetUserPortMappings 获取用户的端口映射
func (c *CloudControl) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	if c.portMappingService == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "portMappingService not initialized")
	}
	return c.portMappingService.GetUserPortMappings(userID)
}
