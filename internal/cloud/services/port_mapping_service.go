package services

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/utils"
)

// portMappingService 端口映射服务实现
type portMappingService struct {
	*dispose.ServiceBase
	baseService  *BaseService
	mappingRepo  *repos.PortMappingRepo
	idManager    *idgen.IDManager
	statsCounter *stats.StatsCounter
}

// NewPortMappingService 创建端口映射服务
func NewPortMappingService(mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, statsCounter *stats.StatsCounter, parentCtx context.Context) PortMappingService {
	service := &portMappingService{
		ServiceBase:  dispose.NewService("PortMappingService", parentCtx),
		baseService:  NewBaseService(),
		mappingRepo:  mappingRepo,
		idManager:    idManager,
		statsCounter: statsCounter,
	}
	return service
}

// CreatePortMapping 创建端口映射
func (s *portMappingService) CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error) {
	// 生成映射ID
	mappingID, err := s.idManager.GeneratePortMappingID()
	if err != nil {
		return nil, s.baseService.WrapError(err, "generate port mapping ID")
	}

	// 设置映射基本信息
	mapping.ID = mappingID
	mapping.Status = models.MappingStatusInactive
	s.baseService.SetTimestamps(&mapping.CreatedAt, &mapping.UpdatedAt)
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
		return nil, s.baseService.HandleErrorWithIDReleaseString(err, mappingID, s.idManager.ReleasePortMappingID, "create port mapping")
	}

	// 添加到用户映射列表
	if mapping.UserID != "" {
		if err := s.mappingRepo.AddMappingToUser(mapping.UserID, mapping); err != nil {
			s.baseService.LogWarning("add mapping to user list", err)
		}
	}

	// 添加到源客户端映射列表
	if err := s.mappingRepo.AddMappingToClient(utils.Int64ToString(mapping.SourceClientID), mapping); err != nil {
		s.baseService.LogWarning("add mapping to source client list", err)
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		if err := s.statsCounter.IncrMapping(1); err != nil {
			s.baseService.LogWarning("update mapping stats counter", err, mappingID)
		}
	}

	s.baseService.LogCreated("port mapping", fmt.Sprintf("%s for client: %d", mappingID, mapping.SourceClientID))
	return mapping, nil
}

// GetPortMapping 获取端口映射
func (s *portMappingService) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	mapping, err := s.mappingRepo.GetPortMapping(mappingID)
	if err != nil {
		return nil, s.baseService.WrapErrorWithID(err, "get port mapping", mappingID)
	}
	return mapping, nil
}

// UpdatePortMapping 更新端口映射
func (s *portMappingService) UpdatePortMapping(mapping *models.PortMapping) error {
	s.baseService.SetUpdatedTimestamp(&mapping.UpdatedAt)
	if err := s.mappingRepo.UpdatePortMapping(mapping); err != nil {
		return s.baseService.WrapErrorWithID(err, "update port mapping", mapping.ID)
	}
	s.baseService.LogUpdated("port mapping", mapping.ID)
	return nil
}

// DeletePortMapping 删除端口映射
func (s *portMappingService) DeletePortMapping(mappingID string) error {
	// 获取映射信息
	_, err := s.mappingRepo.GetPortMapping(mappingID)
	if err != nil {
		return s.baseService.WrapErrorWithID(err, "get port mapping", mappingID)
	}

	// 删除映射
	if err := s.mappingRepo.DeletePortMapping(mappingID); err != nil {
		return s.baseService.WrapErrorWithID(err, "delete port mapping", mappingID)
	}

	// 释放映射ID
	if err := s.idManager.ReleasePortMappingID(mappingID); err != nil {
		s.baseService.LogWarning("release port mapping ID", err, mappingID)
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		if err := s.statsCounter.IncrMapping(-1); err != nil {
			s.baseService.LogWarning("update mapping stats counter", err, mappingID)
		}
	}

	s.baseService.LogDeleted("port mapping", mappingID)
	return nil
}

// UpdatePortMappingStatus 更新端口映射状态
func (s *portMappingService) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	if err := s.mappingRepo.UpdatePortMappingStatus(mappingID, status); err != nil {
		return s.baseService.WrapErrorWithID(err, "update port mapping status", mappingID)
	}
	s.baseService.LogUpdated("port mapping", fmt.Sprintf("%s status to %s", mappingID, status))
	return nil
}

// UpdatePortMappingStats 更新端口映射统计信息
func (s *portMappingService) UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error {
	if err := s.mappingRepo.UpdatePortMappingStats(mappingID, stats); err != nil {
		return s.baseService.WrapErrorWithID(err, "update port mapping stats", mappingID)
	}
	return nil
}

// GetUserPortMappings 获取用户的端口映射
func (s *portMappingService) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	mappings, err := s.mappingRepo.GetUserPortMappings(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user port mappings for %s: %w", userID, err)
	}
	return mappings, nil
}

// ListPortMappings 列出端口映射
func (s *portMappingService) ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error) {
	// 暂时返回空列表，因为PortMappingRepo没有按类型列表的方法
	// 这里预留：可根据类型过滤端口映射
	utils.Warnf("ListPortMappings by type not implemented yet")
	return []*models.PortMapping{}, nil
}

// SearchPortMappings 搜索端口映射
func (s *portMappingService) SearchPortMappings(keyword string) ([]*models.PortMapping, error) {
	// 暂时返回空列表，因为PortMappingRepo没有Search方法
	// 这里预留：可扩展搜索功能
	utils.Warnf("SearchPortMappings not implemented yet")
	return []*models.PortMapping{}, nil
}
