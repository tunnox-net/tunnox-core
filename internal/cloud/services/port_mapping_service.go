package services

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/utils"
)

// PortMappingServiceImpl 端口映射服务实现
type PortMappingServiceImpl struct {
	mappingRepo *repos.PortMappingRepo
	idManager   *generators.IDManager
	utils.Dispose
}

// NewPortMappingService 创建端口映射服务
func NewPortMappingService(mappingRepo *repos.PortMappingRepo, idManager *generators.IDManager, parentCtx context.Context) PortMappingService {
	service := &PortMappingServiceImpl{
		mappingRepo: mappingRepo,
		idManager:   idManager,
	}
	service.SetCtx(parentCtx, service.onClose)
	return service
}

// onClose 资源清理回调
func (s *PortMappingServiceImpl) onClose() error {
	utils.Infof("Port mapping service resources cleaned up")
	return nil
}

// CreatePortMapping 创建端口映射
func (s *PortMappingServiceImpl) CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error) {
	// 生成映射ID
	mappingID, err := s.idManager.GeneratePortMappingID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate port mapping ID: %w", err)
	}

	// 设置映射基本信息
	mapping.ID = mappingID
	mapping.Status = models.MappingStatusInactive
	mapping.CreatedAt = time.Now()
	mapping.UpdatedAt = time.Now()
	mapping.TrafficStats = stats.TrafficStats{
		LastUpdated: time.Now(),
	}

	// 如果配置为空，使用默认配置
	if mapping.Config == (configs.MappingConfig{}) {
		mapping.Config = configs.MappingConfig{
			EnableCompression: true,
			BandwidthLimit:    0, // 无限制
			MaxConnections:    100,
			Timeout:           30,
			RetryCount:        3,
			EnableLogging:     true,
		}
	}

	// 保存到存储
	if err := s.mappingRepo.CreatePortMapping(mapping); err != nil {
		// 释放已生成的ID
		_ = s.idManager.ReleasePortMappingID(mappingID)
		return nil, fmt.Errorf("failed to create port mapping: %w", err)
	}

	// 添加到用户映射列表
	if mapping.UserID != "" {
		if err := s.mappingRepo.AddMappingToUser(mapping.UserID, mapping); err != nil {
			utils.Warnf("Failed to add mapping to user list: %v", err)
		}
	}

	// 添加到源客户端映射列表
	if err := s.mappingRepo.AddMappingToClient(utils.Int64ToString(mapping.SourceClientID), mapping); err != nil {
		utils.Warnf("Failed to add mapping to source client list: %v", err)
	}

	utils.Infof("Created port mapping: %s for client: %d", mappingID, mapping.SourceClientID)
	return mapping, nil
}

// GetPortMapping 获取端口映射
func (s *PortMappingServiceImpl) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	mapping, err := s.mappingRepo.GetPortMapping(mappingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get port mapping %s: %w", mappingID, err)
	}
	return mapping, nil
}

// UpdatePortMapping 更新端口映射
func (s *PortMappingServiceImpl) UpdatePortMapping(mapping *models.PortMapping) error {
	mapping.UpdatedAt = time.Now()
	if err := s.mappingRepo.UpdatePortMapping(mapping); err != nil {
		return fmt.Errorf("failed to update port mapping %s: %w", mapping.ID, err)
	}
	utils.Infof("Updated port mapping: %s", mapping.ID)
	return nil
}

// DeletePortMapping 删除端口映射
func (s *PortMappingServiceImpl) DeletePortMapping(mappingID string) error {
	// 获取映射信息
	_, err := s.mappingRepo.GetPortMapping(mappingID)
	if err != nil {
		return fmt.Errorf("failed to get port mapping %s: %w", mappingID, err)
	}

	// 删除映射
	if err := s.mappingRepo.DeletePortMapping(mappingID); err != nil {
		return fmt.Errorf("failed to delete port mapping %s: %w", mappingID, err)
	}

	// 释放映射ID
	if err := s.idManager.ReleasePortMappingID(mappingID); err != nil {
		utils.Warnf("Failed to release port mapping ID %s: %v", mappingID, err)
	}

	utils.Infof("Deleted port mapping: %s", mappingID)
	return nil
}

// UpdatePortMappingStatus 更新端口映射状态
func (s *PortMappingServiceImpl) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	if err := s.mappingRepo.UpdatePortMappingStatus(mappingID, status); err != nil {
		return fmt.Errorf("failed to update port mapping status %s: %w", mappingID, err)
	}
	utils.Infof("Updated port mapping %s status to %s", mappingID, status)
	return nil
}

// UpdatePortMappingStats 更新端口映射统计信息
func (s *PortMappingServiceImpl) UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error {
	if err := s.mappingRepo.UpdatePortMappingStats(mappingID, stats); err != nil {
		return fmt.Errorf("failed to update port mapping stats %s: %w", mappingID, err)
	}
	utils.Debugf("Updated port mapping %s stats", mappingID)
	return nil
}

// GetUserPortMappings 获取用户的端口映射
func (s *PortMappingServiceImpl) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	mappings, err := s.mappingRepo.GetUserPortMappings(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user port mappings for %s: %w", userID, err)
	}
	return mappings, nil
}

// ListPortMappings 列出端口映射
func (s *PortMappingServiceImpl) ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error) {
	// 暂时返回空列表，因为PortMappingRepo没有按类型列表的方法
	// TODO: 实现按类型列表功能
	utils.Warnf("ListPortMappings by type not implemented yet")
	return []*models.PortMapping{}, nil
}

// SearchPortMappings 搜索端口映射
func (s *PortMappingServiceImpl) SearchPortMappings(keyword string) ([]*models.PortMapping, error) {
	// 暂时返回空列表，因为PortMappingRepo没有Search方法
	// TODO: 实现搜索功能
	utils.Warnf("SearchPortMappings not implemented yet")
	return []*models.PortMapping{}, nil
}
