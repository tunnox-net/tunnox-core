package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"tunnox-core/internal/utils"
)

// Repository 数据访问层
type Repository struct {
	storage Storage
	utils.Dispose
}

// NewRepository 创建新的数据访问层
func NewRepository(storage Storage) *Repository {
	repo := &Repository{
		storage: storage,
	}
	repo.Dispose.SetCtx(context.Background(), nil)
	return repo
}

// GetStorage 获取底层存储实例
func (r *Repository) GetStorage() Storage {
	return r.storage
}

// UserRepository 用户数据访问
type UserRepository struct {
	*Repository
}

// NewUserRepository 创建用户数据访问层
func NewUserRepository(repo *Repository) *UserRepository {
	return &UserRepository{Repository: repo}
}

// SaveUser 保存用户（创建或更新）
func (r *UserRepository) SaveUser(ctx context.Context, user *User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("marshal user failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixUser, user.ID)
	return r.storage.Set(ctx, key, string(data), DefaultUserDataTTL)
}

// CreateUser 创建新用户（仅创建，不允许覆盖）
func (r *UserRepository) CreateUser(ctx context.Context, user *User) error {
	// 检查用户是否已存在
	existingUser, err := r.GetUser(ctx, user.ID)
	if err == nil && existingUser != nil {
		return fmt.Errorf("user with ID %s already exists", user.ID)
	}

	return r.SaveUser(ctx, user)
}

// UpdateUser 更新用户（仅更新，不允许创建）
func (r *UserRepository) UpdateUser(ctx context.Context, user *User) error {
	// 检查用户是否存在
	existingUser, err := r.GetUser(ctx, user.ID)
	if err != nil || existingUser == nil {
		return fmt.Errorf("user with ID %s does not exist", user.ID)
	}

	return r.SaveUser(ctx, user)
}

// GetUser 获取用户
func (r *UserRepository) GetUser(ctx context.Context, userID string) (*User, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixUser, userID)
	data, err := r.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	userData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid user data type")
	}

	var user User
	if err := json.Unmarshal([]byte(userData), &user); err != nil {
		return nil, fmt.Errorf("unmarshal user failed: %w", err)
	}

	return &user, nil
}

// DeleteUser 删除用户
func (r *UserRepository) DeleteUser(ctx context.Context, userID string) error {
	key := fmt.Sprintf("%s:%s", KeyPrefixUser, userID)
	return r.storage.Delete(ctx, key)
}

// ListUsers 列出用户
func (r *UserRepository) ListUsers(ctx context.Context, userType UserType) ([]*User, error) {
	key := KeyPrefixUserList
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*User{}, nil
	}

	var users []*User
	for _, item := range data {
		if userData, ok := item.(string); ok {
			var user User
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
func (r *UserRepository) AddUserToList(ctx context.Context, user *User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	key := KeyPrefixUserList
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

// SaveClient 保存客户端（创建或更新）
func (r *ClientRepository) SaveClient(ctx context.Context, client *Client) error {
	data, err := json.Marshal(client)
	if err != nil {
		return fmt.Errorf("marshal client failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixClient, client.ID)
	return r.storage.Set(ctx, key, string(data), DefaultClientDataTTL)
}

// CreateClient 创建新客户端（仅创建，不允许覆盖）
func (r *ClientRepository) CreateClient(ctx context.Context, client *Client) error {
	// 检查客户端是否已存在
	existingClient, err := r.GetClient(ctx, client.ID)
	if err == nil && existingClient != nil {
		return fmt.Errorf("client with ID %s already exists", client.ID)
	}

	return r.SaveClient(ctx, client)
}

// UpdateClient 更新客户端（仅更新，不允许创建）
func (r *ClientRepository) UpdateClient(ctx context.Context, client *Client) error {
	// 检查客户端是否存在
	existingClient, err := r.GetClient(ctx, client.ID)
	if err != nil || existingClient == nil {
		return fmt.Errorf("client with ID %s does not exist", client.ID)
	}

	return r.SaveClient(ctx, client)
}

// GetClient 获取客户端
func (r *ClientRepository) GetClient(ctx context.Context, clientID string) (*Client, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixClient, clientID)
	data, err := r.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	clientData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid client data type")
	}

	var client Client
	if err := json.Unmarshal([]byte(clientData), &client); err != nil {
		return nil, fmt.Errorf("unmarshal client failed: %w", err)
	}

	return &client, nil
}

// DeleteClient 删除客户端
func (r *ClientRepository) DeleteClient(ctx context.Context, clientID string) error {
	key := fmt.Sprintf("%s:%s", KeyPrefixClient, clientID)
	return r.storage.Delete(ctx, key)
}

// UpdateClientStatus 更新客户端状态
func (r *ClientRepository) UpdateClientStatus(ctx context.Context, clientID string, status ClientStatus, nodeID string) error {
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
func (r *ClientRepository) ListUserClients(ctx context.Context, userID string) ([]*Client, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixUserClients, userID)
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*Client{}, nil
	}

	var clients []*Client
	for _, item := range data {
		if clientData, ok := item.(string); ok {
			var client Client
			if err := json.Unmarshal([]byte(clientData), &client); err != nil {
				continue
			}
			clients = append(clients, &client)
		}
	}

	return clients, nil
}

// AddClientToUser 添加客户端到用户
func (r *ClientRepository) AddClientToUser(ctx context.Context, userID string, client *Client) error {
	data, err := json.Marshal(client)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixUserClients, userID)
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

// SavePortMapping 保存端口映射（创建或更新）
func (r *PortMappingRepository) SavePortMapping(ctx context.Context, mapping *PortMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshal port mapping failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixPortMapping, mapping.ID)
	return r.storage.Set(ctx, key, string(data), DefaultMappingDataTTL)
}

// CreatePortMapping 创建新端口映射（仅创建，不允许覆盖）
func (r *PortMappingRepository) CreatePortMapping(ctx context.Context, mapping *PortMapping) error {
	// 检查端口映射是否已存在
	existingMapping, err := r.GetPortMapping(ctx, mapping.ID)
	if err == nil && existingMapping != nil {
		return fmt.Errorf("port mapping with ID %s already exists", mapping.ID)
	}

	return r.SavePortMapping(ctx, mapping)
}

// UpdatePortMapping 更新端口映射（仅更新，不允许创建）
func (r *PortMappingRepository) UpdatePortMapping(ctx context.Context, mapping *PortMapping) error {
	// 检查端口映射是否存在
	existingMapping, err := r.GetPortMapping(ctx, mapping.ID)
	if err != nil || existingMapping == nil {
		return fmt.Errorf("port mapping with ID %s does not exist", mapping.ID)
	}

	return r.SavePortMapping(ctx, mapping)
}

// GetPortMapping 获取端口映射
func (r *PortMappingRepository) GetPortMapping(ctx context.Context, mappingID string) (*PortMapping, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixPortMapping, mappingID)
	data, err := r.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	mappingData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid mapping data type")
	}

	var mapping PortMapping
	if err := json.Unmarshal([]byte(mappingData), &mapping); err != nil {
		return nil, fmt.Errorf("unmarshal port mapping failed: %w", err)
	}

	return &mapping, nil
}

// DeletePortMapping 删除端口映射
func (r *PortMappingRepository) DeletePortMapping(ctx context.Context, mappingID string) error {
	key := fmt.Sprintf("%s:%s", KeyPrefixPortMapping, mappingID)
	return r.storage.Delete(ctx, key)
}

// UpdatePortMappingStatus 更新端口映射状态
func (r *PortMappingRepository) UpdatePortMappingStatus(ctx context.Context, mappingID string, status MappingStatus) error {
	mapping, err := r.GetPortMapping(ctx, mappingID)
	if err != nil {
		return err
	}

	mapping.Status = status
	mapping.UpdatedAt = time.Now()

	return r.SavePortMapping(ctx, mapping)
}

// UpdatePortMappingStats 更新端口映射统计
func (r *PortMappingRepository) UpdatePortMappingStats(ctx context.Context, mappingID string, stats *TrafficStats) error {
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
func (r *PortMappingRepository) ListUserMappings(ctx context.Context, userID string) ([]*PortMapping, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixUserMappings, userID)
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*PortMapping{}, nil
	}

	var mappings []*PortMapping
	for _, item := range data {
		if mappingData, ok := item.(string); ok {
			var mapping PortMapping
			if err := json.Unmarshal([]byte(mappingData), &mapping); err != nil {
				continue
			}
			mappings = append(mappings, &mapping)
		}
	}

	return mappings, nil
}

// ListClientMappings 列出客户端的端口映射
func (r *PortMappingRepository) ListClientMappings(ctx context.Context, clientID string) ([]*PortMapping, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixClientMappings, clientID)
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*PortMapping{}, nil
	}

	var mappings []*PortMapping
	for _, item := range data {
		if mappingData, ok := item.(string); ok {
			var mapping PortMapping
			if err := json.Unmarshal([]byte(mappingData), &mapping); err != nil {
				continue
			}
			mappings = append(mappings, &mapping)
		}
	}

	return mappings, nil
}

// AddMappingToUser 添加映射到用户
func (r *PortMappingRepository) AddMappingToUser(ctx context.Context, userID string, mapping *PortMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixUserMappings, userID)
	return r.storage.AppendToList(ctx, key, string(data))
}

// AddMappingToClient 添加映射到客户端
func (r *PortMappingRepository) AddMappingToClient(ctx context.Context, clientID string, mapping *PortMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixClientMappings, clientID)
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

// SaveNode 保存节点（创建或更新）
func (r *NodeRepository) SaveNode(ctx context.Context, node *Node) error {
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshal node failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixNode, node.ID)
	return r.storage.Set(ctx, key, string(data), DefaultNodeDataTTL)
}

// CreateNode 创建新节点（仅创建，不允许覆盖）
func (r *NodeRepository) CreateNode(ctx context.Context, node *Node) error {
	// 检查节点是否已存在
	existingNode, err := r.GetNode(ctx, node.ID)
	if err == nil && existingNode != nil {
		return fmt.Errorf("node with ID %s already exists", node.ID)
	}

	return r.SaveNode(ctx, node)
}

// UpdateNode 更新节点（仅更新，不允许创建）
func (r *NodeRepository) UpdateNode(ctx context.Context, node *Node) error {
	// 检查节点是否存在
	existingNode, err := r.GetNode(ctx, node.ID)
	if err != nil || existingNode == nil {
		return fmt.Errorf("node with ID %s does not exist", node.ID)
	}

	return r.SaveNode(ctx, node)
}

// GetNode 获取节点
func (r *NodeRepository) GetNode(ctx context.Context, nodeID string) (*Node, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixNode, nodeID)
	data, err := r.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	nodeData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid node data type")
	}

	var node Node
	if err := json.Unmarshal([]byte(nodeData), &node); err != nil {
		return nil, fmt.Errorf("unmarshal node failed: %w", err)
	}

	return &node, nil
}

// DeleteNode 删除节点
func (r *NodeRepository) DeleteNode(ctx context.Context, nodeID string) error {
	key := fmt.Sprintf("%s:%s", KeyPrefixNode, nodeID)
	return r.storage.Delete(ctx, key)
}

// ListNodes 列出所有节点
func (r *NodeRepository) ListNodes(ctx context.Context) ([]*Node, error) {
	key := KeyPrefixNodeList
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*Node{}, nil
	}

	var nodes []*Node
	for _, item := range data {
		if nodeData, ok := item.(string); ok {
			var node Node
			if err := json.Unmarshal([]byte(nodeData), &node); err != nil {
				continue
			}
			nodes = append(nodes, &node)
		}
	}

	return nodes, nil
}

// AddNodeToList 添加节点到列表
func (r *NodeRepository) AddNodeToList(ctx context.Context, node *Node) error {
	data, err := json.Marshal(node)
	if err != nil {
		return err
	}

	key := KeyPrefixNodeList
	return r.storage.AppendToList(ctx, key, string(data))
}

// ConnectionRepository 连接数据访问
type ConnectionRepository struct {
	*Repository
	utils.Dispose
}

// NewConnectionRepository 创建连接数据访问层
func NewConnectionRepository(repo *Repository) *ConnectionRepository {
	cr := &ConnectionRepository{Repository: repo}
	cr.Dispose.SetCtx(context.Background(), cr.onClose)
	return cr
}

func (cr *ConnectionRepository) onClose() {
	if cr.Repository != nil {
		cr.Repository.Dispose.Close()
	}
}

// SaveConnection 保存连接信息（创建或更新）
func (r *ConnectionRepository) SaveConnection(ctx context.Context, connInfo *ConnectionInfo) error {
	data, err := json.Marshal(connInfo)
	if err != nil {
		return fmt.Errorf("marshal connection failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixConnection, connInfo.ConnId)
	return r.storage.Set(ctx, key, string(data), DefaultConnectionTTL)
}

// CreateConnection 创建新连接（仅创建，不允许覆盖）
func (r *ConnectionRepository) CreateConnection(ctx context.Context, connInfo *ConnectionInfo) error {
	// 检查连接是否已存在
	existingConn, err := r.GetConnection(ctx, connInfo.ConnId)
	if err == nil && existingConn != nil {
		return fmt.Errorf("connection with ID %s already exists", connInfo.ConnId)
	}

	return r.SaveConnection(ctx, connInfo)
}

// UpdateConnection 更新连接（仅更新，不允许创建）
func (r *ConnectionRepository) UpdateConnection(ctx context.Context, connInfo *ConnectionInfo) error {
	// 检查连接是否存在
	existingConn, err := r.GetConnection(ctx, connInfo.ConnId)
	if err != nil || existingConn == nil {
		return fmt.Errorf("connection with ID %s does not exist", connInfo.ConnId)
	}

	return r.SaveConnection(ctx, connInfo)
}

// GetConnection 获取连接信息
func (r *ConnectionRepository) GetConnection(ctx context.Context, connID string) (*ConnectionInfo, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixConnection, connID)
	data, err := r.storage.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	connData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid connection data type")
	}

	var connInfo ConnectionInfo
	if err := json.Unmarshal([]byte(connData), &connInfo); err != nil {
		return nil, fmt.Errorf("unmarshal connection failed: %w", err)
	}

	return &connInfo, nil
}

// DeleteConnection 删除连接
func (r *ConnectionRepository) DeleteConnection(ctx context.Context, connID string) error {
	key := fmt.Sprintf("%s:%s", KeyPrefixConnection, connID)
	return r.storage.Delete(ctx, key)
}

// ListMappingConnections 列出映射的连接
func (r *ConnectionRepository) ListMappingConnections(ctx context.Context, mappingID string) ([]*ConnectionInfo, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixMappingConnections, mappingID)
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*ConnectionInfo{}, nil
	}

	var connections []*ConnectionInfo
	for _, item := range data {
		if connData, ok := item.(string); ok {
			var connInfo ConnectionInfo
			if err := json.Unmarshal([]byte(connData), &connInfo); err != nil {
				continue
			}
			connections = append(connections, &connInfo)
		}
	}

	return connections, nil
}

// ListClientConnections 列出客户端的连接
func (r *ConnectionRepository) ListClientConnections(ctx context.Context, clientID string) ([]*ConnectionInfo, error) {
	key := fmt.Sprintf("%s:%s", KeyPrefixClientConnections, clientID)
	data, err := r.storage.GetList(ctx, key)
	if err != nil {
		return []*ConnectionInfo{}, nil
	}

	var connections []*ConnectionInfo
	for _, item := range data {
		if connData, ok := item.(string); ok {
			var connInfo ConnectionInfo
			if err := json.Unmarshal([]byte(connData), &connInfo); err != nil {
				continue
			}
			connections = append(connections, &connInfo)
		}
	}

	return connections, nil
}

// AddConnectionToMapping 添加连接到映射列表
func (r *ConnectionRepository) AddConnectionToMapping(ctx context.Context, mappingID string, connInfo *ConnectionInfo) error {
	data, err := json.Marshal(connInfo)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixMappingConnections, mappingID)
	return r.storage.AppendToList(ctx, key, string(data))
}

// AddConnectionToClient 添加连接到客户端列表
func (r *ConnectionRepository) AddConnectionToClient(ctx context.Context, clientID string, connInfo *ConnectionInfo) error {
	data, err := json.Marshal(connInfo)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", KeyPrefixClientConnections, clientID)
	return r.storage.AppendToList(ctx, key, string(data))
}

// UpdateConnectionStats 更新连接统计
func (r *ConnectionRepository) UpdateConnectionStats(ctx context.Context, connID string, bytesSent, bytesReceived int64) error {
	connInfo, err := r.GetConnection(ctx, connID)
	if err != nil {
		return err
	}

	connInfo.BytesSent = bytesSent
	connInfo.BytesReceived = bytesReceived
	connInfo.LastActivity = time.Now()
	connInfo.UpdatedAt = time.Now()

	return r.UpdateConnection(ctx, connInfo)
}
