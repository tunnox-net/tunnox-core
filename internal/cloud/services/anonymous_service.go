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

// AnonymousServiceImpl 匿名用户服务实现
type AnonymousServiceImpl struct {
	*dispose.ResourceBase
	clientRepo  *repos.ClientRepository
	mappingRepo *repos.PortMappingRepo
	idManager   *idgen.IDManager
}

// NewAnonymousService 创建匿名用户服务
func NewAnonymousService(clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, parentCtx context.Context) AnonymousService {
	service := &AnonymousServiceImpl{
		ResourceBase: dispose.NewResourceBase("AnonymousService"),
		clientRepo:   clientRepo,
		mappingRepo:  mappingRepo,
		idManager:    idManager,
	}
	service.Initialize(parentCtx)
	return service
}

// GenerateAnonymousCredentials 生成匿名客户端凭据
func (s *AnonymousServiceImpl) GenerateAnonymousCredentials() (*models.Client, error) {
	// 生成客户端ID
	clientID, err := s.idManager.GenerateClientID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate client ID: %w", err)
	}

	// 生成认证码和密钥
	authCode, err := s.idManager.GenerateAuthCode()
	if err != nil {
		_ = s.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("failed to generate auth code: %w", err)
	}

	secretKey, err := s.idManager.GenerateSecretKey()
	if err != nil {
		_ = s.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("failed to generate secret key: %w", err)
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
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Config:    configs.ClientConfig{}, // 使用默认配置
	}

	// 保存到存储
	if err := s.clientRepo.CreateClient(client); err != nil {
		// 释放已生成的ID
		_ = s.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("failed to create anonymous client: %w", err)
	}

	utils.Infof("Generated anonymous client: %d", clientID)
	return client, nil
}

// GetAnonymousClient 获取匿名客户端
func (s *AnonymousServiceImpl) GetAnonymousClient(clientID int64) (*models.Client, error) {
	client, err := s.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return nil, fmt.Errorf("anonymous client %d not found: %w", clientID, err)
	}

	// 验证是否为匿名客户端
	if client.Type != models.ClientTypeAnonymous {
		return nil, fmt.Errorf("client %d is not anonymous", clientID)
	}

	return client, nil
}

// DeleteAnonymousClient 删除匿名客户端
func (s *AnonymousServiceImpl) DeleteAnonymousClient(clientID int64) error {
	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return fmt.Errorf("anonymous client %d not found: %w", clientID, err)
	}

	// 验证是否为匿名客户端
	if client.Type != models.ClientTypeAnonymous {
		return fmt.Errorf("client %d is not anonymous", clientID)
	}

	// 删除客户端
	if err := s.clientRepo.DeleteClient(utils.Int64ToString(clientID)); err != nil {
		return fmt.Errorf("failed to delete anonymous client %d: %w", clientID, err)
	}

	// 释放客户端ID
	if err := s.idManager.ReleaseClientID(clientID); err != nil {
		utils.Warnf("Failed to release anonymous client ID %d: %v", clientID, err)
	}

	utils.Infof("Deleted anonymous client: %d", clientID)
	return nil
}

// ListAnonymousClients 列出匿名客户端
func (s *AnonymousServiceImpl) ListAnonymousClients() ([]*models.Client, error) {
	clients, err := s.clientRepo.ListClients()
	if err != nil {
		return nil, fmt.Errorf("failed to list clients: %w", err)
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
func (s *AnonymousServiceImpl) CreateAnonymousMapping(sourceClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) {
	// 验证源客户端
	_, err := s.clientRepo.GetClient(utils.Int64ToString(sourceClientID))
	if err != nil {
		return nil, fmt.Errorf("source client %d not found: %w", sourceClientID, err)
	}

	// 验证目标客户端
	_, err = s.clientRepo.GetClient(utils.Int64ToString(targetClientID))
	if err != nil {
		return nil, fmt.Errorf("target client %d not found: %w", targetClientID, err)
	}

	// 生成映射ID
	mappingID, err := s.idManager.GeneratePortMappingID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate mapping ID: %w", err)
	}

	// 创建映射
	mapping := &models.PortMapping{
		ID:             mappingID,
		SourceClientID: sourceClientID,
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
		_ = s.idManager.ReleasePortMappingID(mappingID)
		return nil, fmt.Errorf("failed to create anonymous mapping: %w", err)
	}

	utils.Infof("Created anonymous mapping: %s between clients %d and %d", mappingID, sourceClientID, targetClientID)
	return mapping, nil
}

// GetAnonymousMappings 获取匿名映射
func (s *AnonymousServiceImpl) GetAnonymousMappings() ([]*models.PortMapping, error) {
	// 暂时返回空列表，因为PortMappingRepo没有按类型列表的方法
	// TODO: 实现按类型列表功能
	utils.Warnf("GetAnonymousMappings not implemented yet")
	return []*models.PortMapping{}, nil
}

// CleanupExpiredAnonymous 清理过期的匿名资源
func (s *AnonymousServiceImpl) CleanupExpiredAnonymous() error {
	// 获取所有匿名客户端
	anonymousClients, err := s.ListAnonymousClients()
	if err != nil {
		return fmt.Errorf("failed to list anonymous clients: %w", err)
	}

	now := time.Now()
	expiredCount := 0

	for _, client := range anonymousClients {
		// 检查是否过期（超过24小时未活动）
		if client.LastSeen != nil && now.Sub(*client.LastSeen) > 24*time.Hour {
			if err := s.DeleteAnonymousClient(client.ID); err != nil {
				utils.Warnf("Failed to delete expired anonymous client %d: %v", client.ID, err)
			} else {
				expiredCount++
			}
		}
	}

	utils.Infof("Cleaned up %d expired anonymous clients", expiredCount)
	return nil
}
