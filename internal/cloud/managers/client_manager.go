package managers

import (
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
)

// CreateClient 创建客户端
// 注意：此方法委托给 ClientService 处理，遵循 Manager -> Service -> Repository 架构
func (c *CloudControl) CreateClient(userID, clientName string) (*models.Client, error) {
	if c.clientService == nil {
		return nil, fmt.Errorf("clientService not initialized")
	}
	return c.clientService.CreateClient(userID, clientName)
}

// TouchClient 更新客户端活动时间
func (c *CloudControl) TouchClient(clientID int64) {
	if c.clientService != nil {
		c.clientService.TouchClient(clientID)
	}
}

// GetClient 获取客户端
func (c *CloudControl) GetClient(clientID int64) (*models.Client, error) {
	if c.clientService == nil {
		return nil, fmt.Errorf("clientService not initialized")
	}
	return c.clientService.GetClient(clientID)
}

// UpdateClient 更新客户端
func (c *CloudControl) UpdateClient(client *models.Client) error {
	if c.clientService == nil {
		return fmt.Errorf("clientService not initialized")
	}
	client.UpdatedAt = time.Now()
	return c.clientService.UpdateClient(client)
}

// DeleteClient 删除客户端
func (c *CloudControl) DeleteClient(clientID int64) error {
	if c.clientService == nil {
		return fmt.Errorf("clientService not initialized")
	}
	return c.clientService.DeleteClient(clientID)
}

// UpdateClientStatus 更新客户端状态
func (c *CloudControl) UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error {
	if c.clientService == nil {
		return fmt.Errorf("clientService not initialized")
	}
	return c.clientService.UpdateClientStatus(clientID, status, nodeID)
}

// ListClients 列出客户端
func (c *CloudControl) ListClients(userID string, clientType models.ClientType) ([]*models.Client, error) {
	if c.clientService == nil {
		return nil, fmt.Errorf("clientService not initialized")
	}
	return c.clientService.ListClients(userID, clientType)
}

// ListUserClients 列出用户的客户端
func (c *CloudControl) ListUserClients(userID string) ([]*models.Client, error) {
	if c.clientService == nil {
		return nil, fmt.Errorf("clientService not initialized")
	}
	return c.clientService.ListUserClients(userID)
}

// GetClientPortMappings 获取客户端的端口映射
func (c *CloudControl) GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) {
	if c.clientService == nil {
		return nil, fmt.Errorf("clientService not initialized")
	}
	return c.clientService.GetClientPortMappings(clientID)
}

// ========== 客户端状态快速查询（新增，兼容CloudControlAPI接口） ==========

// GetClientNodeID 获取客户端所在节点ID（快速查询）
func (c *CloudControl) GetClientNodeID(clientID int64) (string, error) {
	if c.clientService == nil {
		return "", fmt.Errorf("clientService not initialized")
	}
	client, err := c.clientService.GetClient(clientID)
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
	if c.clientService == nil {
		return false, fmt.Errorf("clientService not initialized")
	}
	client, err := c.clientService.GetClient(clientID)
	if err != nil {
		return false, err
	}
	return client != nil && client.Status == models.ClientStatusOnline && client.NodeID == nodeID, nil
}

// GetNodeClients 获取节点的所有在线客户端
func (c *CloudControl) GetNodeClients(nodeID string) ([]*models.Client, error) {
	if c.clientService == nil {
		return nil, fmt.Errorf("clientService not initialized")
	}
	// 通过 ClientService 列出所有客户端并过滤
	allClients, err := c.clientService.ListClients("", "")
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
	if c.clientService == nil || c.portMappingService == nil {
		return fmt.Errorf("services not initialized")
	}

	// 获取源客户端的所有映射
	mappings, err := c.clientService.GetClientPortMappings(fromClientID)
	if err != nil {
		return fmt.Errorf("failed to get mappings for client %d: %w", fromClientID, err)
	}

	if len(mappings) == 0 {
		return nil // 没有映射需要迁移
	}

	// 迁移每个映射
	for _, mapping := range mappings {
		// 更新映射的源客户端ID
		mapping.ListenClientID = toClientID
		mapping.UpdatedAt = time.Now()

		// 保存更新后的映射
		if err := c.portMappingService.UpdatePortMapping(mapping); err != nil {
			continue
		}
	}

	return nil
}
