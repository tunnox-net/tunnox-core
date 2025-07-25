package services

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/utils"
)

// ClientServiceImpl 客户端服务实现
type ClientServiceImpl struct {
	*dispose.ResourceBase
	clientRepo  *repos.ClientRepository
	mappingRepo *repos.PortMappingRepo
	idManager   *idgen.IDManager
	statsMgr    *managers.StatsManager
}

// NewClientService 创建客户端服务
func NewClientService(clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, statsMgr *managers.StatsManager, parentCtx context.Context) ClientService {
	service := &ClientServiceImpl{
		ResourceBase: dispose.NewResourceBase("ClientService"),
		clientRepo:   clientRepo,
		mappingRepo:  mappingRepo,
		idManager:    idManager,
		statsMgr:     statsMgr,
	}
	service.Initialize(parentCtx)
	return service
}

// CreateClient 创建客户端
func (s *ClientServiceImpl) CreateClient(userID, clientName string) (*models.Client, error) {
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

	// 创建客户端
	client := &models.Client{
		ID:        clientID,
		UserID:    userID,
		Name:      clientName,
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    models.ClientStatusOffline,
		Type:      models.ClientTypeRegistered,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Config:    configs.ClientConfig{}, // 使用默认配置
	}

	// 保存到存储
	if err := s.clientRepo.CreateClient(client); err != nil {
		// 释放已生成的ID
		_ = s.idManager.ReleaseClientID(clientID)
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	// 添加到用户客户端列表
	if err := s.clientRepo.AddClientToUser(userID, client); err != nil {
		utils.Warnf("Failed to add client to user list: %v", err)
	}

	utils.Infof("Created client: %s (ID: %d) for user: %s", clientName, clientID, userID)
	return client, nil
}

// GetClient 获取客户端
func (s *ClientServiceImpl) GetClient(clientID int64) (*models.Client, error) {
	client, err := s.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return nil, fmt.Errorf("failed to get client %d: %w", clientID, err)
	}
	return client, nil
}

// TouchClient 更新客户端最后活动时间
func (s *ClientServiceImpl) TouchClient(clientID int64) {
	if err := s.clientRepo.TouchClient(utils.Int64ToString(clientID)); err != nil {
		utils.Warnf("Failed to touch client %d: %v", clientID, err)
	}
}

// UpdateClient 更新客户端
func (s *ClientServiceImpl) UpdateClient(client *models.Client) error {
	client.UpdatedAt = time.Now()
	if err := s.clientRepo.UpdateClient(client); err != nil {
		return fmt.Errorf("failed to update client %d: %w", client.ID, err)
	}
	utils.Infof("Updated client: %d", client.ID)
	return nil
}

// DeleteClient 删除客户端
func (s *ClientServiceImpl) DeleteClient(clientID int64) error {
	// 获取客户端信息
	client, err := s.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return fmt.Errorf("failed to get client %d: %w", clientID, err)
	}

	// 删除客户端
	if err := s.clientRepo.DeleteClient(utils.Int64ToString(clientID)); err != nil {
		return fmt.Errorf("failed to delete client %d: %w", clientID, err)
	}

	// 从用户客户端列表中移除
	if client.UserID != "" {
		if err := s.clientRepo.RemoveClientFromUser(client.UserID, client); err != nil {
			utils.Warnf("Failed to remove client from user list: %v", err)
		}
	}

	// 释放客户端ID
	if err := s.idManager.ReleaseClientID(clientID); err != nil {
		utils.Warnf("Failed to release client ID %d: %v", clientID, err)
	}

	utils.Infof("Deleted client: %d", clientID)
	return nil
}

// UpdateClientStatus 更新客户端状态
func (s *ClientServiceImpl) UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error {
	if err := s.clientRepo.UpdateClientStatus(utils.Int64ToString(clientID), status, nodeID); err != nil {
		return fmt.Errorf("failed to update client status %d: %w", clientID, err)
	}
	utils.Infof("Updated client %d status to %s", clientID, status)
	return nil
}

// ListClients 列出客户端
func (s *ClientServiceImpl) ListClients(userID string, clientType models.ClientType) ([]*models.Client, error) {
	if userID != "" {
		// 获取用户的所有客户端
		clients, err := s.clientRepo.ListUserClients(userID)
		if err != nil {
			return nil, fmt.Errorf("failed to list user clients for %s: %w", userID, err)
		}

		// 如果指定了类型，进行过滤
		if clientType != "" {
			filteredClients := make([]*models.Client, 0)
			for _, client := range clients {
				if client.Type == clientType {
					filteredClients = append(filteredClients, client)
				}
			}
			return filteredClients, nil
		}

		return clients, nil
	}

	// 获取所有客户端
	clients, err := s.clientRepo.ListClients()
	if err != nil {
		return nil, fmt.Errorf("failed to list all clients: %w", err)
	}

	// 如果指定了类型，进行过滤
	if clientType != "" {
		filteredClients := make([]*models.Client, 0)
		for _, client := range clients {
			if client.Type == clientType {
				filteredClients = append(filteredClients, client)
			}
		}
		return filteredClients, nil
	}

	return clients, nil
}

// ListUserClients 列出用户的所有客户端
func (s *ClientServiceImpl) ListUserClients(userID string) ([]*models.Client, error) {
	clients, err := s.clientRepo.ListUserClients(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user clients for %s: %w", userID, err)
	}
	return clients, nil
}

// GetClientPortMappings 获取客户端的端口映射
func (s *ClientServiceImpl) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	mappings, err := s.mappingRepo.GetClientPortMappings(utils.Int64ToString(clientID))
	if err != nil {
		return nil, fmt.Errorf("failed to get client port mappings for %d: %w", clientID, err)
	}
	return mappings, nil
}

// SearchClients 搜索客户端
func (s *ClientServiceImpl) SearchClients(keyword string) ([]*models.Client, error) {
	// 暂时返回空列表，因为ClientRepository没有Search方法
	// TODO: 实现搜索功能
	utils.Warnf("SearchClients not implemented yet")
	return []*models.Client{}, nil
}

// GetClientStats 获取客户端统计信息
func (s *ClientServiceImpl) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	if s.statsMgr == nil {
		return nil, fmt.Errorf("stats manager not available")
	}

	clientStats, err := s.statsMgr.GetClientStats(clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get client stats for %d: %w", clientID, err)
	}
	return clientStats, nil
}
