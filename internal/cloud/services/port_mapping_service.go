package services

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	cloudutils "tunnox-core/internal/cloud/utils"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils/random"
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
	// ✅ 如果状态未设置，默认为 Inactive；否则保留传入的状态
	if mapping.Status == "" {
		mapping.Status = models.MappingStatusInactive
	}
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

	// 双向索引：同时添加到 ListenClientID 和 TargetClientID 的索引
	// 确保 GetClientPortMappings 能查询到该客户端作为 ListenClient 或 TargetClient 的所有映射
	if mapping.ListenClientID > 0 {
		clientKey := random.Int64ToString(mapping.ListenClientID)
		if err := s.mappingRepo.AddMappingToClient(clientKey, mapping); err != nil {
			s.baseService.LogWarning("add mapping to listen client list", err)
		} else {
			corelog.Infof("PortMappingService: added mapping %s to listen client %s index", mapping.ID, clientKey)
		}
	}

	if mapping.TargetClientID > 0 {
		clientKey := random.Int64ToString(mapping.TargetClientID)
		if err := s.mappingRepo.AddMappingToClient(clientKey, mapping); err != nil {
			s.baseService.LogWarning("add mapping to target client list", err)
		} else {
			corelog.Infof("PortMappingService: added mapping %s to target client %s index", mapping.ID, clientKey)
		}
	}

	// 更新统计计数器
	if s.statsCounter != nil {
		if err := s.statsCounter.IncrMapping(1); err != nil {
			s.baseService.LogWarning("update mapping stats counter", err, mappingID)
		}
	}

	s.baseService.LogCreated("port mapping", fmt.Sprintf("%s for client: %d", mappingID, mapping.ListenClientID))
	return mapping, nil
}

// ParseListenAddress 解析监听地址
//
// 格式：0.0.0.0:7788 或 127.0.0.1:9999
// 返回：主机地址、端口、错误
func (s *portMappingService) ParseListenAddress(addr string) (string, int, error) {
	return cloudutils.ParseListenAddress(addr)
}

// ParseTargetAddress 解析目标地址
//
// 格式：tcp://10.51.22.69:3306 或 udp://192.168.1.1:53
// 返回：主机地址、端口、协议、错误
func (s *portMappingService) ParseTargetAddress(addr string) (string, int, string, error) {
	return cloudutils.ParseTargetAddress(addr)
}

// GetPortMapping 获取端口映射
func (s *portMappingService) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	mapping, err := s.mappingRepo.GetPortMapping(mappingID)
	if err != nil {
		return nil, s.baseService.WrapErrorWithID(err, "get port mapping", mappingID)
	}
	return mapping, nil
}

// GetPortMappingByDomain 通过域名查找 HTTP 映射
func (s *portMappingService) GetPortMappingByDomain(fullDomain string) (*models.PortMapping, error) {
	mapping, err := s.mappingRepo.GetPortMappingByDomain(fullDomain)
	if err != nil {
		return nil, s.baseService.WrapError(err, "get port mapping by domain")
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
// 即使主数据不存在，也会尝试清理索引（处理数据不一致情况）
func (s *portMappingService) DeletePortMapping(mappingID string) error {
	// 删除映射（repo 层会处理主数据不存在但索引存在的情况）
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
		return nil, coreerrors.Wrapf(err, coreerrors.CodeStorageError, "failed to get user port mappings for %s", userID)
	}
	return mappings, nil
}

// ListPortMappings 列出端口映射
func (s *portMappingService) ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error) {
	// 获取所有映射
	allMappings, err := s.mappingRepo.ListAllMappings()
	if err != nil {
		return nil, s.baseService.WrapError(err, "list all port mappings")
	}

	// 如果未指定类型，返回所有映射
	if mappingType == "" {
		return allMappings, nil
	}

	// 按类型过滤
	var filtered []*models.PortMapping
	for _, m := range allMappings {
		if m.Type == mappingType {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}

// SearchPortMappings 搜索端口映射
func (s *portMappingService) SearchPortMappings(keyword string) ([]*models.PortMapping, error) {
	// 暂时返回空列表，因为PortMappingRepo没有Search方法
	// 这里预留：可扩展搜索功能
	corelog.Warnf("SearchPortMappings not implemented yet")
	return []*models.PortMapping{}, nil
}
