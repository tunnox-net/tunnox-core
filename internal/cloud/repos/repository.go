package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	constants2 "tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage"

	"tunnox-core/internal/utils"
)

// GenericRepository 泛型Repository接口
type GenericRepository[T any] interface {
	// 基础CRUD操作
	Save(entity T, keyPrefix string, ttl time.Duration) error
	Create(entity T, keyPrefix string, ttl time.Duration) error
	Update(entity T, keyPrefix string, ttl time.Duration) error
	Get(id string, keyPrefix string) (T, error)
	Delete(id string, keyPrefix string) error

	// 列表操作
	List(listKey string) ([]T, error)
	AddToList(entity T, listKey string) error
	RemoveFromList(entity T, listKey string) error
}

// GenericRepositoryImpl 泛型Repository实现
type GenericRepositoryImpl[T any] struct {
	*Repository
	getIDFunc func(T) (string, error)
}

// NewGenericRepository 创建泛型Repository
func NewGenericRepository[T any](repo *Repository, getIDFunc func(T) (string, error)) *GenericRepositoryImpl[T] {
	return &GenericRepositoryImpl[T]{
		Repository: repo,
		getIDFunc:  getIDFunc,
	}
}

// Save 保存实体（创建或更新）
func (r *GenericRepositoryImpl[T]) Save(entity T, keyPrefix string, ttl time.Duration) error {
	data, err := json.Marshal(entity)
	if err != nil {
		return fmt.Errorf("marshal entity failed: %w", err)
	}

	// 使用反射获取ID字段
	id, err := r.getEntityID(entity)
	if err != nil {
		return fmt.Errorf("get entity ID failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", keyPrefix, id)
	return r.storage.Set(key, string(data), ttl)
}

// Create 创建实体（仅创建，不允许覆盖）
func (r *GenericRepositoryImpl[T]) Create(entity T, keyPrefix string, ttl time.Duration) error {
	id, err := r.getEntityID(entity)
	if err != nil {
		return fmt.Errorf("get entity ID failed: %w", err)
	}

	// 检查实体是否已存在
	_, err = r.Get(id, keyPrefix)
	if err == nil {
		// 如果获取成功，说明实体已存在
		return fmt.Errorf("entity with ID %s already exists", id)
	}

	return r.Save(entity, keyPrefix, ttl)
}

// Update 更新实体（仅更新，不允许创建）
func (r *GenericRepositoryImpl[T]) Update(entity T, keyPrefix string, ttl time.Duration) error {
	id, err := r.getEntityID(entity)
	if err != nil {
		return fmt.Errorf("get entity ID failed: %w", err)
	}

	// 检查实体是否存在
	_, err = r.Get(id, keyPrefix)
	if err != nil {
		return fmt.Errorf("entity with ID %s does not exist", id)
	}

	return r.Save(entity, keyPrefix, ttl)
}

// Get 获取实体
func (r *GenericRepositoryImpl[T]) Get(id string, keyPrefix string) (T, error) {
	var entity T

	key := fmt.Sprintf("%s:%s", keyPrefix, id)
	data, err := r.storage.Get(key)
	if err != nil {
		return entity, err
	}

	entityData, ok := data.(string)
	if !ok {
		return entity, fmt.Errorf("invalid entity data type")
	}

	if err := json.Unmarshal([]byte(entityData), &entity); err != nil {
		return entity, fmt.Errorf("unmarshal entity failed: %w", err)
	}

	return entity, nil
}

// Delete 删除实体
func (r *GenericRepositoryImpl[T]) Delete(id string, keyPrefix string) error {
	key := fmt.Sprintf("%s:%s", keyPrefix, id)
	return r.storage.Delete(key)
}

// List 列出实体
func (r *GenericRepositoryImpl[T]) List(listKey string) ([]T, error) {
	data, err := r.storage.GetList(listKey)
	if err != nil {
		return []T{}, nil
	}

	var entities []T
	for _, item := range data {
		if entityData, ok := item.(string); ok {
			var entity T
			if err := json.Unmarshal([]byte(entityData), &entity); err != nil {
				continue
			}
			entities = append(entities, entity)
		}
	}

	return entities, nil
}

// AddToList 添加实体到列表
func (r *GenericRepositoryImpl[T]) AddToList(entity T, listKey string) error {
	data, err := json.Marshal(entity)
	if err != nil {
		return err
	}

	return r.storage.AppendToList(listKey, string(data))
}

// RemoveFromList 从列表移除实体
func (r *GenericRepositoryImpl[T]) RemoveFromList(entity T, listKey string) error {
	data, err := json.Marshal(entity)
	if err != nil {
		return err
	}

	return r.storage.RemoveFromList(listKey, string(data))
}

// getEntityID 获取实体ID
func (r *GenericRepositoryImpl[T]) getEntityID(entity T) (string, error) {
	if r.getIDFunc == nil {
		return "", fmt.Errorf("getIDFunc not set")
	}
	return r.getIDFunc(entity)
}

// Repository 数据访问层
type Repository struct {
	storage storage.Storage
	dispose.Dispose
}

// NewRepository 创建新的数据访问层
func NewRepository(storage storage.Storage) *Repository {
	repo := &Repository{
		storage: storage,
	}
	repo.Dispose.SetCtx(context.Background(), nil)
	return repo
}

// GetStorage 获取底层存储实例
func (r *Repository) GetStorage() storage.Storage {
	return r.storage
}

// UserRepository 用户数据访问
type UserRepository struct {
	*GenericRepositoryImpl[*models.User]
}

// NewUserRepository 创建用户数据访问层
func NewUserRepository(repo *Repository) *UserRepository {
	genericRepo := NewGenericRepository[*models.User](repo, func(user *models.User) (string, error) {
		return user.ID, nil
	})
	return &UserRepository{GenericRepositoryImpl: genericRepo}
}

// SaveUser 保存用户（创建或更新）
func (r *UserRepository) SaveUser(user *models.User) error {
	return r.Save(user, constants.KeyPrefixUser, constants2.DefaultUserDataTTL)
}

// CreateUser 创建新用户（仅创建，不允许覆盖）
func (r *UserRepository) CreateUser(user *models.User) error {
	return r.Create(user, constants.KeyPrefixUser, constants2.DefaultUserDataTTL)
}

// UpdateUser 更新用户（仅更新，不允许创建）
func (r *UserRepository) UpdateUser(user *models.User) error {
	return r.Update(user, constants.KeyPrefixUser, constants2.DefaultUserDataTTL)
}

// GetUser 获取用户
func (r *UserRepository) GetUser(userID string) (*models.User, error) {
	return r.Get(userID, constants.KeyPrefixUser)
}

// DeleteUser 删除用户
func (r *UserRepository) DeleteUser(userID string) error {
	return r.Delete(userID, constants.KeyPrefixUser)
}

// ListUsers 列出用户
func (r *UserRepository) ListUsers(userType models.UserType) ([]*models.User, error) {
	users, err := r.List(constants.KeyPrefixUserList)
	if err != nil {
		return []*models.User{}, nil
	}

	// 过滤用户类型
	if userType != "" {
		var filteredUsers []*models.User
		for _, user := range users {
			if user.Type == userType {
				filteredUsers = append(filteredUsers, user)
			}
		}
		return filteredUsers, nil
	}

	return users, nil
}

// AddUserToList 添加用户到列表
func (r *UserRepository) AddUserToList(user *models.User) error {
	return r.AddToList(user, constants.KeyPrefixUserList)
}

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
	return r.Create(client, constants.KeyPrefixClient, constants2.DefaultClientDataTTL)
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

	return r.storage.SetList(key, data, constants2.DefaultUserDataTTL)
}

// PortMappingRepo 端口映射数据访问
type PortMappingRepo struct {
	*GenericRepositoryImpl[*models.PortMapping]
}

// NewPortMappingRepo 创建端口映射数据访问层
func NewPortMappingRepo(repo *Repository) *PortMappingRepo {
	genericRepo := NewGenericRepository[*models.PortMapping](repo, func(mapping *models.PortMapping) (string, error) {
		return mapping.ID, nil
	})
	return &PortMappingRepo{GenericRepositoryImpl: genericRepo}
}

// SavePortMapping 保存端口映射（创建或更新）
func (r *PortMappingRepo) SavePortMapping(mapping *models.PortMapping) error {
	return r.Save(mapping, constants.KeyPrefixPortMapping, constants2.DefaultMappingDataTTL)
}

// CreatePortMapping 创建新端口映射（仅创建，不允许覆盖）
func (r *PortMappingRepo) CreatePortMapping(mapping *models.PortMapping) error {
	return r.Create(mapping, constants.KeyPrefixPortMapping, constants2.DefaultMappingDataTTL)
}

// UpdatePortMapping 更新端口映射（仅更新，不允许创建）
func (r *PortMappingRepo) UpdatePortMapping(mapping *models.PortMapping) error {
	return r.Update(mapping, constants.KeyPrefixPortMapping, constants2.DefaultMappingDataTTL)
}

// GetPortMapping 获取端口映射
func (r *PortMappingRepo) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	return r.Get(mappingID, constants.KeyPrefixPortMapping)
}

// DeletePortMapping 删除端口映射
func (r *PortMappingRepo) DeletePortMapping(mappingID string) error {
	return r.Delete(mappingID, constants.KeyPrefixPortMapping)
}

// UpdatePortMappingStatus 更新端口映射状态
func (r *PortMappingRepo) UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error {
	mapping, err := r.GetPortMapping(mappingID)
	if err != nil {
		return err
	}

	mapping.Status = status
	mapping.UpdatedAt = time.Now()

	return r.UpdatePortMapping(mapping)
}

// UpdatePortMappingStats 更新端口映射统计
func (r *PortMappingRepo) UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error {
	mapping, err := r.GetPortMapping(mappingID)
	if err != nil {
		return err
	}

	if stats != nil {
		mapping.TrafficStats = *stats
	}
	mapping.UpdatedAt = time.Now()

	return r.UpdatePortMapping(mapping)
}

// GetUserPortMappings 列出用户的端口映射
func (r *PortMappingRepo) GetUserPortMappings(userID string) ([]*models.PortMapping, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserMappings, userID)
	return r.List(key)
}

// GetClientPortMappings 列出客户端的端口映射
func (r *PortMappingRepo) GetClientPortMappings(clientID string) ([]*models.PortMapping, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, clientID)
	return r.List(key)
}

// AddMappingToUser 添加映射到用户
func (r *PortMappingRepo) AddMappingToUser(userID string, mapping *models.PortMapping) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserMappings, userID)
	return r.AddToList(mapping, key)
}

// AddMappingToClient 添加映射到客户端
func (r *PortMappingRepo) AddMappingToClient(clientID string, mapping *models.PortMapping) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, clientID)
	return r.AddToList(mapping, key)
}

// NodeRepository 节点数据访问
type NodeRepository struct {
	*GenericRepositoryImpl[*models.Node]
}

// NewNodeRepository 创建节点数据访问层
func NewNodeRepository(repo *Repository) *NodeRepository {
	genericRepo := NewGenericRepository[*models.Node](repo, func(node *models.Node) (string, error) {
		return node.ID, nil
	})
	return &NodeRepository{GenericRepositoryImpl: genericRepo}
}

// SaveNode 保存节点（创建或更新）
func (r *NodeRepository) SaveNode(node *models.Node) error {
	return r.Save(node, constants.KeyPrefixNode, constants2.DefaultNodeDataTTL)
}

// CreateNode 创建新节点（仅创建，不允许覆盖）
func (r *NodeRepository) CreateNode(node *models.Node) error {
	return r.Create(node, constants.KeyPrefixNode, constants2.DefaultNodeDataTTL)
}

// UpdateNode 更新节点（仅更新，不允许创建）
func (r *NodeRepository) UpdateNode(node *models.Node) error {
	return r.Update(node, constants.KeyPrefixNode, constants2.DefaultNodeDataTTL)
}

// GetNode 获取节点
func (r *NodeRepository) GetNode(nodeID string) (*models.Node, error) {
	return r.Get(nodeID, constants.KeyPrefixNode)
}

// DeleteNode 删除节点
func (r *NodeRepository) DeleteNode(nodeID string) error {
	return r.Delete(nodeID, constants.KeyPrefixNode)
}

// ListNodes 列出所有节点
func (r *NodeRepository) ListNodes() ([]*models.Node, error) {
	return r.List(constants.KeyPrefixNodeList)
}

// AddNodeToList 添加节点到列表
func (r *NodeRepository) AddNodeToList(node *models.Node) error {
	return r.AddToList(node, constants.KeyPrefixNodeList)
}

// ConnectionRepo 连接数据访问
type ConnectionRepo struct {
	*GenericRepositoryImpl[*models.ConnectionInfo]
	dispose.Dispose
}

// NewConnectionRepo 创建连接数据访问层
func NewConnectionRepo(repo *Repository) *ConnectionRepo {
	genericRepo := NewGenericRepository[*models.ConnectionInfo](repo, func(connInfo *models.ConnectionInfo) (string, error) {
		return connInfo.ConnID, nil
	})
	cr := &ConnectionRepo{GenericRepositoryImpl: genericRepo}
	cr.Dispose.SetCtx(context.Background(), cr.onClose)
	return cr
}

// getEntityID 获取连接ID
func (r *ConnectionRepo) getEntityID(connInfo *models.ConnectionInfo) (string, error) {
	return connInfo.ConnID, nil
}

func (cr *ConnectionRepo) onClose() error {
	if cr.GenericRepositoryImpl != nil {
		return cr.GenericRepositoryImpl.Repository.Dispose.Close()
	}
	return nil
}

// SaveConnection 保存连接信息（创建或更新）
func (r *ConnectionRepo) SaveConnection(connInfo *models.ConnectionInfo) error {
	if err := r.Save(connInfo, constants.KeyPrefixConnection, constants2.DefaultConnectionTTL); err != nil {
		return fmt.Errorf("save connection failed: %w", err)
	}
	// 添加到映射和客户端的连接列表
	if err := r.AddConnectionToMapping(connInfo.MappingID, connInfo); err != nil {
		return fmt.Errorf("add connection to mapping failed: %w", err)
	}
	if err := r.AddConnectionToClient(connInfo.ClientID, connInfo); err != nil {
		return fmt.Errorf("add connection to client failed: %w", err)
	}
	return nil
}

// CreateConnection 创建新连接（仅创建，不允许覆盖）
func (r *ConnectionRepo) CreateConnection(connInfo *models.ConnectionInfo) error {
	return r.Create(connInfo, constants.KeyPrefixConnection, constants2.DefaultConnectionTTL)
}

// UpdateConnection 更新连接（仅更新，不允许创建）
func (r *ConnectionRepo) UpdateConnection(connInfo *models.ConnectionInfo) error {
	// 检查连接是否存在
	_, err := r.GetConnection(connInfo.ConnID)
	if err != nil {
		return fmt.Errorf("connection with ID %s does not exist", connInfo.ConnID)
	}

	// 只更新主连接记录，不重新添加到列表中
	data, err := json.Marshal(connInfo)
	if err != nil {
		return fmt.Errorf("marshal connection failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", constants.KeyPrefixConnection, connInfo.ConnID)
	if err := r.storage.Set(key, string(data), constants2.DefaultConnectionTTL); err != nil {
		return fmt.Errorf("update connection failed: %w", err)
	}

	return nil
}

// GetConnection 获取连接信息
func (r *ConnectionRepo) GetConnection(connID string) (*models.ConnectionInfo, error) {
	return r.Get(connID, constants.KeyPrefixConnection)
}

// DeleteConnection 删除连接
func (r *ConnectionRepo) DeleteConnection(connID string) error {
	// 获取连接信息以便从列表中移除
	connInfo, err := r.GetConnection(connID)
	if err != nil {
		return err
	}

	// 从映射和客户端列表中移除
	if err := r.RemoveConnectionFromMapping(connInfo.MappingID, connInfo); err != nil {
		utils.Warnf("Failed to remove connection from mapping list: %v", err)
	}
	if err := r.RemoveConnectionFromClient(connInfo.ClientID, connInfo); err != nil {
		utils.Warnf("Failed to remove connection from client list: %v", err)
	}

	// 删除主连接记录
	return r.Delete(connID, constants.KeyPrefixConnection)
}

// ListMappingConns 列出映射的连接
func (r *ConnectionRepo) ListMappingConns(mappingID string) ([]*models.ConnectionInfo, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixMappingConnections, mappingID)
	return r.List(key)
}

// ListClientConns 列出客户端的连接
func (r *ConnectionRepo) ListClientConns(clientID int64) ([]*models.ConnectionInfo, error) {
	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClientConnections, clientID)
	return r.List(key)
}

// AddConnectionToMapping 添加连接到映射列表
func (r *ConnectionRepo) AddConnectionToMapping(mappingID string, connInfo *models.ConnectionInfo) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixMappingConnections, mappingID)
	return r.AddToList(connInfo, key)
}

// AddConnectionToClient 添加连接到客户端
func (r *ConnectionRepo) AddConnectionToClient(clientID int64, connInfo *models.ConnectionInfo) error {
	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClientConnections, clientID)
	return r.AddToList(connInfo, key)
}

// RemoveConnectionFromMapping 从映射连接列表中移除连接
func (r *ConnectionRepo) RemoveConnectionFromMapping(mappingID string, connInfo *models.ConnectionInfo) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixMappingConnections, mappingID)
	return r.RemoveFromList(connInfo, key)
}

// RemoveConnectionFromClient 从客户端连接列表中移除连接
func (r *ConnectionRepo) RemoveConnectionFromClient(clientID int64, connInfo *models.ConnectionInfo) error {
	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClientConnections, clientID)
	return r.RemoveFromList(connInfo, key)
}

// UpdateStats 更新连接统计
func (r *ConnectionRepo) UpdateStats(connID string, bytesSent, bytesReceived int64) error {
	connInfo, err := r.GetConnection(connID)
	if err != nil {
		return err
	}

	connInfo.BytesSent = bytesSent
	connInfo.BytesReceived = bytesReceived
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()

	return r.UpdateConnection(connInfo)
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
