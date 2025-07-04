package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"tunnox-core/internal/cloud"
	"tunnox-core/internal/cloud/storage"
)

// Repository 数据访问层
type Repository struct {
	storage storage.Storage
}

// NewRepository 创建新的数据访问层
func NewRepository(storage storage.Storage) *Repository {
	return &Repository{
		storage: storage,
	}
}

// UserRepository 用户数据访问
type UserRepository struct {
	*Repository
}

// NewUserRepository 创建用户数据访问层
func NewUserRepository(repo *Repository) *UserRepository {
	return &UserRepository{Repository: repo}
}

// SaveUser 保存用户
func (r *UserRepository) SaveUser(ctx context.Context, user *cloud.User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("marshal user failed: %w", err)
	}

	key := fmt.Sprintf("user:%s", user.ID)
	return r.storage.Set(ctx, key, string(data), 0) // 用户数据不过期
}

// GetUser 获取用户
func (r *UserRepository) GetUser(ctx context.Context, userID string) (*cloud.User, error) {
	key := fmt.Sprintf("user:%s", userID)
	data, err := r.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	userData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid user data type")
	}

	var user cloud.User
	if err := json.Unmarshal([]byte(userData), &user); err != nil {
		return nil, fmt.Errorf("unmarshal user failed: %w", err)
	}

	return &user, nil
}

// DeleteUser 删除用户
func (r *UserRepository) DeleteUser(ctx context.Context, userID string) error {
	key := fmt.Sprintf("user:%s", userID)
	return r.storage.Delete(ctx, key)
}

// ListUsers 列出用户
func (r *UserRepository) ListUsers(ctx context.Context, userType cloud.UserType) ([]*cloud.User, error) {
	key := "users:list"
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*cloud.User{}, nil
	}

	var users []*cloud.User
	for _, item := range data {
		if userData, ok := item.(string); ok {
			var user cloud.User
			if err := json.Unmarshal([]byte(userData), &user); err != nil {
				continue
			}
			if userType == "" || user.Type == userType {
				users = append(users, &user)
			}
		}
	}

	return users, nil
}

// AddUserToList 添加用户到列表
func (r *UserRepository) AddUserToList(ctx context.Context, user *cloud.User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	key := "users:list"
	return r.storage.AppendToList(ctx, key, string(data))
}

// ClientRepository 客户端数据访问
type ClientRepository struct {
	*Repository
}

// NewClientRepository 创建客户端数据访问层
func NewClientRepository(repo *Repository) *ClientRepository {
	return &ClientRepository{Repository: repo}
}

// SaveClient 保存客户端
func (r *ClientRepository) SaveClient(ctx context.Context, client *cloud.Client) error {
	data, err := json.Marshal(client)
	if err != nil {
		return fmt.Errorf("marshal client failed: %w", err)
	}

	key := fmt.Sprintf("client:%s", client.ID)
	ttl := 24 * time.Hour // 客户端数据24小时过期
	return r.storage.Set(ctx, key, string(data), ttl)
}

// GetClient 获取客户端
func (r *ClientRepository) GetClient(ctx context.Context, clientID string) (*cloud.Client, error) {
	key := fmt.Sprintf("client:%s", clientID)
	data, err := r.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	clientData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid client data type")
	}

	var client cloud.Client
	if err := json.Unmarshal([]byte(clientData), &client); err != nil {
		return nil, fmt.Errorf("unmarshal client failed: %w", err)
	}

	return &client, nil
}

// DeleteClient 删除客户端
func (r *ClientRepository) DeleteClient(ctx context.Context, clientID string) error {
	key := fmt.Sprintf("client:%s", clientID)
	return r.storage.Delete(ctx, key)
}

// UpdateClientStatus 更新客户端状态
func (r *ClientRepository) UpdateClientStatus(ctx context.Context, clientID string, status cloud.ClientStatus, nodeID string) error {
	client, err := r.GetClient(ctx, clientID)
	if err != nil {
		return err
	}

	client.Status = status
	client.NodeID = nodeID
	now := time.Now()
	client.LastSeen = &now
	client.UpdatedAt = now

	return r.SaveClient(ctx, client)
}

// ListUserClients 列出用户的客户端
func (r *ClientRepository) ListUserClients(ctx context.Context, userID string) ([]*cloud.Client, error) {
	key := fmt.Sprintf("user_clients:%s", userID)
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*cloud.Client{}, nil
	}

	var clients []*cloud.Client
	for _, item := range data {
		if clientData, ok := item.(string); ok {
			var client cloud.Client
			if err := json.Unmarshal([]byte(clientData), &client); err != nil {
				continue
			}
			clients = append(clients, &client)
		}
	}

	return clients, nil
}

// AddClientToUser 添加客户端到用户
func (r *ClientRepository) AddClientToUser(ctx context.Context, userID string, client *cloud.Client) error {
	data, err := json.Marshal(client)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("user_clients:%s", userID)
	return r.storage.AppendToList(ctx, key, string(data))
}

// PortMappingRepository 端口映射数据访问
type PortMappingRepository struct {
	*Repository
}

// NewPortMappingRepository 创建端口映射数据访问层
func NewPortMappingRepository(repo *Repository) *PortMappingRepository {
	return &PortMappingRepository{Repository: repo}
}

// SavePortMapping 保存端口映射
func (r *PortMappingRepository) SavePortMapping(ctx context.Context, mapping *cloud.PortMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshal port mapping failed: %w", err)
	}

	key := fmt.Sprintf("mapping:%s", mapping.ID)
	ttl := 24 * time.Hour // 映射数据24小时过期
	return r.storage.Set(ctx, key, string(data), ttl)
}

// GetPortMapping 获取端口映射
func (r *PortMappingRepository) GetPortMapping(ctx context.Context, mappingID string) (*cloud.PortMapping, error) {
	key := fmt.Sprintf("mapping:%s", mappingID)
	data, err := r.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	mappingData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid mapping data type")
	}

	var mapping cloud.PortMapping
	if err := json.Unmarshal([]byte(mappingData), &mapping); err != nil {
		return nil, fmt.Errorf("unmarshal port mapping failed: %w", err)
	}

	return &mapping, nil
}

// DeletePortMapping 删除端口映射
func (r *PortMappingRepository) DeletePortMapping(ctx context.Context, mappingID string) error {
	key := fmt.Sprintf("mapping:%s", mappingID)
	return r.storage.Delete(ctx, key)
}

// UpdatePortMappingStatus 更新端口映射状态
func (r *PortMappingRepository) UpdatePortMappingStatus(ctx context.Context, mappingID string, status cloud.MappingStatus) error {
	mapping, err := r.GetPortMapping(ctx, mappingID)
	if err != nil {
		return err
	}

	mapping.Status = status
	mapping.UpdatedAt = time.Now()

	return r.SavePortMapping(ctx, mapping)
}

// UpdatePortMappingStats 更新端口映射统计
func (r *PortMappingRepository) UpdatePortMappingStats(ctx context.Context, mappingID string, stats *cloud.TrafficStats) error {
	mapping, err := r.GetPortMapping(ctx, mappingID)
	if err != nil {
		return err
	}

	mapping.TrafficStats = *stats
	mapping.UpdatedAt = time.Now()
	now := time.Now()
	mapping.LastActive = &now

	return r.SavePortMapping(ctx, mapping)
}

// ListUserMappings 列出用户的端口映射
func (r *PortMappingRepository) ListUserMappings(ctx context.Context, userID string) ([]*cloud.PortMapping, error) {
	key := fmt.Sprintf("user_mappings:%s", userID)
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*cloud.PortMapping{}, nil
	}

	var mappings []*cloud.PortMapping
	for _, item := range data {
		if mappingData, ok := item.(string); ok {
			var mapping cloud.PortMapping
			if err := json.Unmarshal([]byte(mappingData), &mapping); err != nil {
				continue
			}
			mappings = append(mappings, &mapping)
		}
	}

	return mappings, nil
}

// ListClientMappings 列出客户端的端口映射
func (r *PortMappingRepository) ListClientMappings(ctx context.Context, clientID string) ([]*cloud.PortMapping, error) {
	key := fmt.Sprintf("client_mappings:%s", clientID)
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*cloud.PortMapping{}, nil
	}

	var mappings []*cloud.PortMapping
	for _, item := range data {
		if mappingData, ok := item.(string); ok {
			var mapping cloud.PortMapping
			if err := json.Unmarshal([]byte(mappingData), &mapping); err != nil {
				continue
			}
			mappings = append(mappings, &mapping)
		}
	}

	return mappings, nil
}

// AddMappingToUser 添加映射到用户
func (r *PortMappingRepository) AddMappingToUser(ctx context.Context, userID string, mapping *cloud.PortMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("user_mappings:%s", userID)
	return r.storage.AppendToList(ctx, key, string(data))
}

// AddMappingToClient 添加映射到客户端
func (r *PortMappingRepository) AddMappingToClient(ctx context.Context, clientID string, mapping *cloud.PortMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("client_mappings:%s", clientID)
	return r.storage.AppendToList(ctx, key, string(data))
}

// NodeRepository 节点数据访问
type NodeRepository struct {
	*Repository
}

// NewNodeRepository 创建节点数据访问层
func NewNodeRepository(repo *Repository) *NodeRepository {
	return &NodeRepository{Repository: repo}
}

// SaveNode 保存节点
func (r *NodeRepository) SaveNode(ctx context.Context, node *cloud.Node) error {
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshal node failed: %w", err)
	}

	key := fmt.Sprintf("node:%s", node.ID)
	ttl := 1 * time.Hour // 节点数据1小时过期
	return r.storage.Set(ctx, key, string(data), ttl)
}

// GetNode 获取节点
func (r *NodeRepository) GetNode(ctx context.Context, nodeID string) (*cloud.Node, error) {
	key := fmt.Sprintf("node:%s", nodeID)
	data, err := r.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	nodeData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid node data type")
	}

	var node cloud.Node
	if err := json.Unmarshal([]byte(nodeData), &node); err != nil {
		return nil, fmt.Errorf("unmarshal node failed: %w", err)
	}

	return &node, nil
}

// DeleteNode 删除节点
func (r *NodeRepository) DeleteNode(ctx context.Context, nodeID string) error {
	key := fmt.Sprintf("node:%s", nodeID)
	return r.storage.Delete(ctx, key)
}

// ListNodes 列出所有节点
func (r *NodeRepository) ListNodes(ctx context.Context) ([]*cloud.Node, error) {
	key := "nodes:list"
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*cloud.Node{}, nil
	}

	var nodes []*cloud.Node
	for _, item := range data {
		if nodeData, ok := item.(string); ok {
			var node cloud.Node
			if err := json.Unmarshal([]byte(nodeData), &node); err != nil {
				continue
			}
			nodes = append(nodes, &node)
		}
	}

	return nodes, nil
}

// AddNodeToList 添加节点到列表
func (r *NodeRepository) AddNodeToList(ctx context.Context, node *cloud.Node) error {
	data, err := json.Marshal(node)
	if err != nil {
		return err
	}

	key := "nodes:list"
	return r.storage.AppendToList(ctx, key, string(data))
}
