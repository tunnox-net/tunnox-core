package managers

import (
	"fmt"
	"time"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/utils"
)

// AnonymousManager 匿名用户管理服务
type AnonymousManager struct {
	clientRepo  *repos.ClientRepository
	mappingRepo *repos.PortMappingRepo
	idManager   *generators.IDManager
	utils.Dispose
}

// NewAnonymousManager 创建匿名用户管理服务
func NewAnonymousManager(clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, idManager *generators.IDManager) *AnonymousManager {
	manager := &AnonymousManager{
		clientRepo:  clientRepo,
		mappingRepo: mappingRepo,
		idManager:   idManager,
	}
	manager.SetCtx(nil, manager.onClose)
	return manager
}

// onClose 资源清理回调
func (am *AnonymousManager) onClose() error {
	utils.Infof("Anonymous manager resources cleaned up")
	// 清理匿名客户端缓存和临时数据
	// 这里可以添加清理匿名资源的逻辑
	return nil
}

// GenerateAnonymousCredentials 生成匿名客户端凭据
func (am *AnonymousManager) GenerateAnonymousCredentials() (*models.Client, error) {
	// 生成客户端ID，确保不重复
	var clientID int64
	for attempts := 0; attempts < constants.DefaultMaxAttempts; attempts++ {
		generatedID, err := am.idManager.GenerateClientID()
		if err != nil {
			return nil, fmt.Errorf("generate client ID failed: %w", err)
		}
		// 检查客户端是否已存在
		existingClient, err := am.clientRepo.GetClient(utils.Int64ToString(generatedID))
		if err != nil {
			clientID = generatedID
			break
		}
		if existingClient != nil {
			_ = am.idManager.ReleaseClientID(generatedID)
			continue
		}
		clientID = generatedID
		break
	}
	if clientID == 0 {
		return nil, fmt.Errorf("failed to generate unique client ID after %d attempts", constants.DefaultMaxAttempts)
	}

	authCode, err := am.idManager.GenerateAuthCode()
	if err != nil {
		_ = am.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("generate auth code failed: %w", err)
	}
	secretKey, err := am.idManager.GenerateSecretKey()
	if err != nil {
		_ = am.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("generate secret key failed: %w", err)
	}
	now := time.Now()
	client := &models.Client{
		ID:        clientID,
		UserID:    "",
		Name:      fmt.Sprintf("Anonymous-%s", authCode),
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    models.ClientStatusOffline,
		Type:      models.ClientTypeAnonymous,
		Config: configs.ClientConfig{
			EnableCompression: constants.DefaultEnableCompression,
			BandwidthLimit:    constants.DefaultAnonymousBandwidthLimit,
			MaxConnections:    constants.DefaultAnonymousMaxConnections,
			AllowedPorts:      constants.DefaultAllowedPorts,
			BlockedPorts:      constants.DefaultBlockedPorts,
			AutoReconnect:     constants.DefaultAutoReconnect,
			HeartbeatInterval: constants.DefaultHeartbeatInterval,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := am.clientRepo.CreateClient(client); err != nil {
		_ = am.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("save anonymous client failed: %w", err)
	}
	if err := am.clientRepo.AddClientToUser("", client); err != nil {
		_ = am.clientRepo.DeleteClient(utils.Int64ToString(clientID))
		_ = am.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("add anonymous client to list failed: %w", err)
	}
	return client, nil
}

// GetAnonymousClient 获取匿名客户端
func (am *AnonymousManager) GetAnonymousClient(clientID int64) (*models.Client, error) {
	client, err := am.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return nil, err
	}
	if client.Type != models.ClientTypeAnonymous {
		return nil, fmt.Errorf("client is not anonymous")
	}
	return client, nil
}

// ListAnonymousClients 列出所有匿名客户端
func (am *AnonymousManager) ListAnonymousClients() ([]*models.Client, error) {
	return am.clientRepo.ListUserClients("")
}

// DeleteAnonymousClient 删除匿名客户端
func (am *AnonymousManager) DeleteAnonymousClient(clientID int64) error {
	// 获取客户端信息，用于释放ID
	client, err := am.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err == nil && client != nil {
		// 释放客户端ID
		_ = am.idManager.ReleaseClientID(clientID)
	}
	return am.clientRepo.DeleteClient(utils.Int64ToString(clientID))
}

// CreateAnonymousMapping 创建匿名端口映射
func (am *AnonymousManager) CreateAnonymousMapping(sourceClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) {
	// 生成端口映射ID，确保不重复
	var mappingID string
	for attempts := 0; attempts < constants.DefaultMaxAttempts; attempts++ {
		generatedID, err := am.idManager.GeneratePortMappingID()
		if err != nil {
			return nil, fmt.Errorf("generate mapping ID failed: %w", err)
		}

		// 检查端口映射是否已存在
		existingMapping, err := am.mappingRepo.GetPortMapping(generatedID)
		if err != nil {
			// 端口映射不存在，可以使用这个ID
			mappingID = generatedID
			break
		}

		if existingMapping != nil {
			// 端口映射已存在，释放ID并重试
			_ = am.idManager.ReleasePortMappingID(generatedID)
			continue
		}

		mappingID = generatedID
		break
	}

	if mappingID == "" {
		return nil, fmt.Errorf("failed to generate unique mapping ID after %d attempts", constants.DefaultMaxAttempts)
	}

	now := time.Now()
	mapping := &models.PortMapping{
		ID:             mappingID,
		UserID:         "",
		SourceClientID: sourceClientID,
		TargetClientID: targetClientID,
		Protocol:       protocol,
		SourcePort:     sourcePort,
		TargetPort:     targetPort,
		Status:         models.MappingStatusActive,
		Type:           models.MappingTypeAnonymous,
		CreatedAt:      now,
		UpdatedAt:      now,
		TrafficStats:   stats.TrafficStats{},
	}

	if err := am.mappingRepo.CreatePortMapping(mapping); err != nil {
		// 如果保存失败，释放ID
		_ = am.idManager.ReleasePortMappingID(mappingID)
		return nil, fmt.Errorf("save anonymous mapping failed: %w", err)
	}

	if err := am.mappingRepo.AddMappingToUser("", mapping); err != nil {
		// 如果添加到匿名列表失败，删除映射并释放ID
		_ = am.mappingRepo.DeletePortMapping(mappingID)
		_ = am.idManager.ReleasePortMappingID(mappingID)
		return nil, fmt.Errorf("add anonymous mapping to list failed: %w", err)
	}

	return mapping, nil
}

// GetAnonymousMappings 获取所有匿名端口映射
func (am *AnonymousManager) GetAnonymousMappings() ([]*models.PortMapping, error) {
	return am.mappingRepo.GetUserPortMappings("")
}

// CleanupExpiredAnonymous 清理过期的匿名数据
func (am *AnonymousManager) CleanupExpiredAnonymous() error {
	// 这里可以实现清理逻辑
	return nil
}
