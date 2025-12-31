package anonymous

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/base"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils"
)

// Notifier avoids circular dependency with managers package
type Notifier interface {
	NotifyClientUpdate(clientID int64)
}

// Service 匿名服务实现
type Service struct {
	*dispose.ServiceBase
	baseService *base.Service
	clientRepo  *repos.ClientRepository
	configRepo  *repos.ClientConfigRepository // 新系统：用于 clientService.GetClient 读取
	mappingRepo *repos.PortMappingRepo
	idManager   *idgen.IDManager
	notifier    Notifier // 通知识别接口
}

// NewService 创建匿名服务
// configRepo: 新系统的客户端配置存储，确保 clientService.GetClient 能读取到匿名客户端
func NewService(clientRepo *repos.ClientRepository, configRepo *repos.ClientConfigRepository, mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, parentCtx context.Context) *Service {
	service := &Service{
		ServiceBase: dispose.NewService("AnonymousService", parentCtx),
		baseService: base.NewService(),
		clientRepo:  clientRepo,
		configRepo:  configRepo,
		mappingRepo: mappingRepo,
		idManager:   idManager,
	}
	return service
}

// GenerateAnonymousCredentials 生成匿名客户端凭据
func (s *Service) GenerateAnonymousCredentials() (*models.Client, error) {
	// 生成客户端ID
	clientID, err := s.idManager.GenerateClientID()
	if err != nil {
		return nil, s.baseService.WrapError(err, "generate client ID")
	}

	// 生成认证码和密钥
	authCode, err := s.idManager.GenerateAuthCode()
	if err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "generate auth code")
	}

	secretKey, err := s.idManager.GenerateSecretKey()
	if err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "generate secret key")
	}

	// 创建匿名客户端
	client := &models.Client{
		ID:        clientID,
		UserID:    "", // 匿名用户没有UserID
		Name:      fmt.Sprintf("Anonymous-%d", clientID),
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    models.ClientStatusOffline,
		Type:      models.ClientTypeAnonymous,
		Config:    configs.ClientConfig{}, // 使用默认配置
	}

	// 设置时间戳
	s.baseService.SetTimestamps(&client.CreatedAt, &client.UpdatedAt)

	// 保存到旧系统（兼容性）
	if err := s.clientRepo.CreateClient(client); err != nil {
		return nil, s.baseService.HandleErrorWithIDReleaseInt64(err, clientID, s.idManager.ReleaseClientID, "create anonymous client")
	}

	// 同时保存到新系统（确保 clientService.GetClient 能读取）
	if s.configRepo != nil {
		clientConfig := &models.ClientConfig{
			ID:        client.ID,
			UserID:    client.UserID,
			Name:      client.Name,
			AuthCode:  client.AuthCode,
			SecretKey: client.SecretKey,
			Type:      client.Type,
			Config:    client.Config,
			CreatedAt: client.CreatedAt,
			UpdatedAt: client.UpdatedAt,
		}
		if err := s.configRepo.SaveConfig(clientConfig); err != nil {
			corelog.Warnf("Failed to save anonymous client to new config system: %v", err)
		}
	}

	s.baseService.LogCreated("anonymous client", fmt.Sprintf("%d", clientID))
	return client, nil
}

// GetAnonymousClient 获取匿名客户端
func (s *Service) GetAnonymousClient(clientID int64) (*models.Client, error) {
	client, err := s.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeClientNotFound, "anonymous client %d not found", clientID)
	}

	// 验证是否为匿名客户端
	if client.Type != models.ClientTypeAnonymous {
		return nil, coreerrors.Newf(coreerrors.CodeInvalidRequest, "client %d is not anonymous", clientID)
	}

	return client, nil
}

// DeleteAnonymousClient 删除匿名客户端
func (s *Service) DeleteAnonymousClient(clientID int64) error {
	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "get anonymous client", clientID)
	}

	// 验证是否为匿名客户端
	if client.Type != models.ClientTypeAnonymous {
		return coreerrors.Newf(coreerrors.CodeInvalidRequest, "client %d is not anonymous", clientID)
	}

	// 删除客户端
	if err := s.clientRepo.DeleteClient(utils.Int64ToString(clientID)); err != nil {
		return s.baseService.WrapErrorWithInt64ID(err, "delete anonymous client", clientID)
	}

	// 释放客户端ID
	if err := s.idManager.ReleaseClientID(clientID); err != nil {
		s.baseService.LogWarning("release anonymous client ID", err, clientID)
	}

	s.baseService.LogDeleted("anonymous client", fmt.Sprintf("%d", clientID))
	return nil
}

// ListAnonymousClients 列出匿名客户端
func (s *Service) ListAnonymousClients() ([]*models.Client, error) {
	clients, err := s.clientRepo.ListClients()
	if err != nil {
		return nil, s.baseService.WrapError(err, "list clients")
	}

	// 过滤匿名客户端
	anonymousClients := make([]*models.Client, 0)
	for _, client := range clients {
		if client.Type == models.ClientTypeAnonymous {
			anonymousClients = append(anonymousClients, client)
		}
	}

	return anonymousClients, nil
}

// CreateAnonymousMapping 创建匿名映射
func (s *Service) CreateAnonymousMapping(listenClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) {
	// 验证监听客户端
	_, err := s.clientRepo.GetClient(utils.Int64ToString(listenClientID))
	if err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeClientNotFound, "listen client %d not found", listenClientID)
	}

	// 验证目标客户端
	_, err = s.clientRepo.GetClient(utils.Int64ToString(targetClientID))
	if err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeClientNotFound, "target client %d not found", targetClientID)
	}

	// 生成映射ID
	mappingID, err := s.idManager.GeneratePortMappingID()
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to generate mapping ID")
	}

	// 创建映射
	mapping := &models.PortMapping{
		ID:             mappingID,
		ListenClientID: listenClientID,
		TargetClientID: targetClientID,
		Protocol:       protocol,
		SourcePort:     sourcePort,
		TargetPort:     targetPort,
		Status:         models.MappingStatusInactive,
		Type:           models.MappingTypeAnonymous,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Config:         configs.MappingConfig{}, // 使用默认配置
	}

	mapping.TrafficStats = stats.TrafficStats{
		LastUpdated: time.Now(),
	}

	if err := s.mappingRepo.CreatePortMapping(mapping); err != nil {
		// 回滚：释放已分配的ID（忽略释放错误，主流程已失败）
		_ = s.idManager.ReleasePortMappingID(mappingID)
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to create anonymous mapping")
	}

	corelog.Infof("Created anonymous mapping: %s between clients %d and %d", mappingID, listenClientID, targetClientID)

	// 通知监听客户端更新配置
	if s.notifier != nil {
		corelog.Infof("Notifying client %d of mapping update", listenClientID)
		s.notifier.NotifyClientUpdate(listenClientID)
	}

	return mapping, nil
}

// GetAnonymousMappings 获取匿名映射
func (s *Service) GetAnonymousMappings() ([]*models.PortMapping, error) {
	// 暂时返回空列表，因为PortMappingRepo没有按类型列表的方法
	// 这里预留：支持按类型筛选匿名服务
	corelog.Warnf("GetAnonymousMappings not implemented yet")
	return []*models.PortMapping{}, nil
}

// CleanupExpiredAnonymous 清理过期的匿名资源
func (s *Service) CleanupExpiredAnonymous() error {
	// 获取所有匿名客户端
	anonymousClients, err := s.ListAnonymousClients()
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to list anonymous clients")
	}

	now := time.Now()
	expiredCount := 0

	for _, client := range anonymousClients {
		// 检查是否过期（超过24小时未活动）
		if client.LastSeen != nil && now.Sub(*client.LastSeen) > 24*time.Hour {
			if err := s.DeleteAnonymousClient(client.ID); err != nil {
				corelog.Warnf("Failed to delete expired anonymous client %d: %v", client.ID, err)
			} else {
				expiredCount++
			}
		}
	}

	corelog.Infof("Cleaned up %d expired anonymous clients", expiredCount)
	return nil
}

// SetNotifier 设置通知器
// notifier 实现 Notifier 接口（与 services.ClientNotifier 兼容）
func (s *Service) SetNotifier(notifier Notifier) {
	s.notifier = notifier
	corelog.Infof("AnonymousService: Notifier set successfully")
}
