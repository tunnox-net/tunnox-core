package repos

import (
	"encoding/json"
	"fmt"
	"time"

	constants2 "tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage"
)

// 编译时接口断言，确保 ClientRepository 实现了 IClientRepository 接口
var _ IClientRepository = (*ClientRepository)(nil)

// ClientRepository 客户端数据访问
type ClientRepository struct {
	*GenericRepositoryImpl[*models.Client]
}

// NewClientRepository 创建客户端数据访问层
func NewClientRepository(repo *Repository) *ClientRepository {
	genericRepo := NewGenericRepository[*models.Client](repo, func(client *models.Client) (string, error) {
		return fmt.Sprintf("%d", client.ID), nil
	})
	return &ClientRepository{GenericRepositoryImpl: genericRepo}
}

// SaveClient 保存客户端（创建或更新）
func (r *ClientRepository) SaveClient(client *models.Client) error {
	if err := r.Save(client, constants.KeyPrefixClient, constants2.DefaultClientDataTTL); err != nil {
		return err
	}
	// 将客户端添加到全局客户端列表中
	return r.AddClientToList(client)
}

// CreateClient 创建新客户端（仅创建，不允许覆盖）
func (r *ClientRepository) CreateClient(client *models.Client) error {
	if err := r.Create(client, constants.KeyPrefixClient, constants2.DefaultClientDataTTL); err != nil {
		return err
	}
	// 将客户端添加到全局客户端列表中
	return r.AddClientToList(client)
}

// UpdateClient 更新客户端（仅更新，不允许创建）
func (r *ClientRepository) UpdateClient(client *models.Client) error {
	return r.Update(client, constants.KeyPrefixClient, constants2.DefaultClientDataTTL)
}

// GetClient 获取客户端
func (r *ClientRepository) GetClient(clientID string) (*models.Client, error) {
	return r.Get(clientID, constants.KeyPrefixClient)
}

// DeleteClient 删除客户端
func (r *ClientRepository) DeleteClient(clientID string) error {
	return r.Delete(clientID, constants.KeyPrefixClient)
}

// UpdateClientStatus 更新客户端状态
func (r *ClientRepository) UpdateClientStatus(clientID string, status models.ClientStatus, nodeID string) error {
	client, err := r.GetClient(clientID)
	if err != nil {
		return err
	}

	client.Status = status
	client.NodeID = nodeID
	client.UpdatedAt = time.Now()

	return r.SaveClient(client)
}

// ListUserClients 列出用户的所有客户端
func (r *ClientRepository) ListUserClients(userID string) ([]*models.Client, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserClients, userID)
	return r.List(key)
}

// AddClientToUser 添加客户端到用户
func (r *ClientRepository) AddClientToUser(userID string, client *models.Client) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserClients, userID)
	return r.AddToList(client, key)
}

// RemoveClientFromUser 从用户移除客户端
func (r *ClientRepository) RemoveClientFromUser(userID string, client *models.Client) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserClients, userID)
	return r.RemoveFromList(client, key)
}

// ListClients 列出所有客户端
func (r *ClientRepository) ListClients() ([]*models.Client, error) {
	return r.List(constants.KeyPrefixClientList)
}

// ListAllClients 列出所有客户端（ListClients的别名）
func (r *ClientRepository) ListAllClients() ([]*models.Client, error) {
	return r.ListClients()
}

// AddClientToList 添加客户端到全局客户端列表
func (r *ClientRepository) AddClientToList(client *models.Client) error {
	return r.AddToList(client, constants.KeyPrefixClientList)
}

// saveUserClients 保存用户客户端列表
func (r *ClientRepository) saveUserClients(userID string, clients []*models.Client) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserClients, userID)
	var data []interface{}

	for _, client := range clients {
		clientData, err := json.Marshal(client)
		if err != nil {
			return err
		}
		data = append(data, string(clientData))
	}

	listStore, ok := r.storage.(storage.ListStore)
	if !ok {
		return coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support list operations")
	}
	return listStore.SetList(key, data, constants2.DefaultUserDataTTL)
}

// TouchClient 刷新客户端的LastSeen和延长过期时间
func (r *ClientRepository) TouchClient(clientID string) error {
	client, err := r.GetClient(clientID)
	if err != nil {
		return err
	}
	now := time.Now()
	client.LastSeen = &now
	client.UpdatedAt = now
	if err := r.SaveClient(client); err != nil {
		return err
	}
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixClient, clientID)
	return r.storage.SetExpiration(key, constants2.DefaultClientDataTTL)
}
