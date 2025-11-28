package managers

import (
	"fmt"
	"time"

	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
)

// CreateClient 创建客户端
func (c *CloudControl) CreateClient(userID, clientName string) (*models.Client, error) {
	// 生成客户端ID，确保不重复
	var clientID int64
	for attempts := 0; attempts < constants.DefaultMaxAttempts; attempts++ {
		generatedID, err := c.idManager.GenerateClientID()
		if err != nil {
			return nil, fmt.Errorf("generate client ID failed: %w", err)
		}

		// 检查客户端是否已存在
		existingClient, err := c.clientRepo.GetClient(fmt.Sprintf("%d", generatedID))
		if err != nil {
			// 客户端不存在，可以使用这个ID
			clientID = generatedID
			break
		}

		if existingClient != nil {
			// 客户端已存在，释放ID并重试
			_ = c.idManager.ReleaseClientID(generatedID)
			continue
		}

		clientID = generatedID
		break
	}

	if clientID == 0 {
		return nil, fmt.Errorf("failed to generate unique client ID after %d attempts", constants.DefaultMaxAttempts)
	}

	authCode, err := c.idManager.GenerateAuthCode()
	if err != nil {
		return nil, c.handleErrorWithIDRelease(err, clientID, c.idManager.ReleaseClientID, "generate auth code failed")
	}

	secretKey, err := c.idManager.GenerateSecretKey()
	if err != nil {
		return nil, c.handleErrorWithIDRelease(err, clientID, c.idManager.ReleaseClientID, "generate secret key failed")
	}

	now := time.Now()
	client := &models.Client{
		ID:        clientID,
		UserID:    userID,
		Name:      clientName,
		AuthCode:  authCode,
		SecretKey: secretKey,
		Status:    models.ClientStatusOffline,
		Type:      models.ClientTypeRegistered,
		Config: configs.ClientConfig{
			EnableCompression: constants.DefaultEnableCompression,
			BandwidthLimit:    constants.DefaultClientBandwidthLimit,
			MaxConnections:    constants.DefaultClientMaxConnections,
			AllowedPorts:      constants.DefaultAllowedPorts,
			BlockedPorts:      constants.DefaultBlockedPorts,
			AutoReconnect:     constants.DefaultAutoReconnect,
			HeartbeatInterval: constants.DefaultHeartbeatInterval,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := c.clientRepo.CreateClient(client); err != nil {
		return nil, c.handleErrorWithIDRelease(err, clientID, c.idManager.ReleaseClientID, "save client failed")
	}

	if err := c.clientRepo.AddClientToUser(userID, client); err != nil {
		// 如果添加到用户失败，删除客户端并释放ID
		_ = c.clientRepo.DeleteClient(fmt.Sprintf("%d", clientID))
		return nil, c.handleErrorWithIDRelease(err, clientID, c.idManager.ReleaseClientID, "add client to user failed")
	}

	// 更新统计计数器
	if c.statsManager != nil && c.statsManager.GetCounter() != nil {
		_ = c.statsManager.GetCounter().IncrClient(1)
	}

	return client, nil
}

// TouchClient 更新客户端活动时间
func (c *CloudControl) TouchClient(clientID int64) {
	client, err := c.clientRepo.GetClient(fmt.Sprintf("%d", clientID))
	if (err == nil) && (client != nil) {
		client.UpdatedAt = time.Now()
		_ = c.clientRepo.UpdateClient(client)
		_ = c.clientRepo.TouchClient(fmt.Sprintf("%d", clientID))
	}
}

// GetClient 获取客户端
func (c *CloudControl) GetClient(clientID int64) (*models.Client, error) {
	return c.clientRepo.GetClient(fmt.Sprintf("%d", clientID))
}

// UpdateClient 更新客户端
func (c *CloudControl) UpdateClient(client *models.Client) error {
	client.UpdatedAt = time.Now()
	return c.clientRepo.UpdateClient(client)
}

// DeleteClient 删除客户端
func (c *CloudControl) DeleteClient(clientID int64) error {
	// 获取客户端信息，用于释放ID
	client, err := c.clientRepo.GetClient(fmt.Sprintf("%d", clientID))
	if err == nil && client != nil {
		// 释放客户端ID
		_ = c.idManager.ReleaseClientID(clientID)
	}
	return c.clientRepo.DeleteClient(fmt.Sprintf("%d", clientID))
}

// UpdateClientStatus 更新客户端状态
func (c *CloudControl) UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error {
	return c.clientRepo.UpdateClientStatus(fmt.Sprintf("%d", clientID), status, nodeID)
}

// ListClients 列出客户端
func (c *CloudControl) ListClients(userID string, clientType models.ClientType) ([]*models.Client, error) {
	var clients []*models.Client
	var err error

	if userID != "" {
		clients, err = c.clientRepo.ListUserClients(userID)
	} else {
		// 列出所有客户端（使用全局列表）
		clients, err = c.clientRepo.ListClients()
	}

	if err != nil {
		return nil, err
	}

	if clientType == "" {
		return clients, nil
	}

	var filtered []*models.Client
	for _, client := range clients {
		if client.Type == clientType {
			filtered = append(filtered, client)
		}
	}
	return filtered, nil
}

// ListUserClients 列出用户的客户端
func (c *CloudControl) ListUserClients(userID string) ([]*models.Client, error) {
	return c.clientRepo.ListUserClients(userID)
}

// GetClientPortMappings 获取客户端的端口映射
func (c *CloudControl) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	return c.mappingRepo.GetClientPortMappings(fmt.Sprintf("%d", clientID))
}

// ========== 客户端状态快速查询（新增，兼容CloudControlAPI接口） ==========

// GetClientNodeID 获取客户端所在节点ID（快速查询）
//
// 实现说明：旧版CloudControl没有分离的StateRepository，直接查Client
func (c *CloudControl) GetClientNodeID(clientID int64) (string, error) {
	client, err := c.clientRepo.GetClient(fmt.Sprintf("%d", clientID))
	if err != nil {
		return "", err
	}
	if client == nil || client.Status != models.ClientStatusOnline {
		return "", nil // 离线或不存在
	}
	return client.NodeID, nil
}

// IsClientOnNode 检查客户端是否在指定节点
func (c *CloudControl) IsClientOnNode(clientID int64, nodeID string) (bool, error) {
	client, err := c.clientRepo.GetClient(fmt.Sprintf("%d", clientID))
	if err != nil {
		return false, err
	}
	return client != nil && client.Status == models.ClientStatusOnline && client.NodeID == nodeID, nil
}

// GetNodeClients 获取节点的所有在线客户端
//
// 实现说明：旧版没有节点索引，遍历所有客户端过滤（性能较差）
func (c *CloudControl) GetNodeClients(nodeID string) ([]*models.Client, error) {
	allClients, err := c.clientRepo.ListClients()
	if err != nil {
		return nil, err
	}

	nodeClients := make([]*models.Client, 0)
	for _, client := range allClients {
		if client.Status == models.ClientStatusOnline && client.NodeID == nodeID {
			nodeClients = append(nodeClients, client)
		}
	}
	return nodeClients, nil
}

// MigrateClientMappings 迁移客户端的端口映射
func (c *CloudControl) MigrateClientMappings(fromClientID, toClientID int64) error {
	// 获取源客户端的所有映射
	mappings, err := c.mappingRepo.GetClientPortMappings(fmt.Sprintf("%d", fromClientID))
	if err != nil {
		return fmt.Errorf("failed to get mappings for client %d: %w", fromClientID, err)
	}

	if len(mappings) == 0 {
		return nil // 没有映射需要迁移
	}

	// 迁移每个映射
	for _, mapping := range mappings {
		// 更新映射的源客户端ID
		// ✅ 统一使用 ListenClientID
		mapping.ListenClientID = toClientID
		mapping.SourceClientID = toClientID // 向后兼容
		mapping.UpdatedAt = time.Now()

		// 保存更新后的映射
		if err := c.mappingRepo.UpdatePortMapping(mapping); err != nil {
			continue
		}

		// 添加到新客户端的映射列表
		_ = c.mappingRepo.AddMappingToClient(fmt.Sprintf("%d", toClientID), mapping)
	}

	return nil
}
