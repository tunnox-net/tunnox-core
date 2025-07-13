package repos

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	constants2 "tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/constants"

	"tunnox-core/internal/utils"
)

// Repository 数据访问层
type Repository struct {
	storage storages.Storage
	utils.Dispose
}

// NewRepository 创建新的数据访问层
func NewRepository(storage storages.Storage) *Repository {
	repo := &Repository{
		storage: storage,
	}
	repo.Dispose.SetCtx(context.Background(), nil)
	return repo
}

// GetStorage 获取底层存储实例
func (r *Repository) GetStorage() storages.Storage {
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
func (r *UserRepository) SaveUser(user *models.User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("marshal user failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUser, user.ID)
	utils.Infof("SaveUser: storing user %s with key %s, data type: %T, data length: %d", user.ID, key, string(data), len(string(data)))
	err = r.storage.Set(key, string(data), constants2.DefaultUserDataTTL)
	if err != nil {
		utils.Errorf("SaveUser: failed to store user %s: %v", user.ID, err)
		return err
	}
	utils.Infof("SaveUser: successfully stored user %s", user.ID)
	return nil
}

// CreateUser 创建新用户（仅创建，不允许覆盖）
func (r *UserRepository) CreateUser(user *models.User) error {
	utils.Infof("CreateUser: checking if user %s already exists", user.ID)
	// 检查用户是否已存在
	existingUser, err := r.GetUser(user.ID)
	if err == nil && existingUser != nil {
		utils.Errorf("CreateUser: user %s already exists", user.ID)
		return fmt.Errorf("user with ID %s already exists", user.ID)
	}
	// 如果err != nil，说明用户不存在，可以创建
	utils.Infof("CreateUser: user %s does not exist, creating new user", user.ID)

	return r.SaveUser(user)
}

// UpdateUser 更新用户（仅更新，不允许创建）
func (r *UserRepository) UpdateUser(user *models.User) error {
	// 检查用户是否存在
	existingUser, _ := r.GetUser(user.ID)
	if existingUser == nil {
		return fmt.Errorf("user with ID %s does not exist", user.ID)
	}

	return r.SaveUser(user)
}

// GetUser 获取用户
func (r *UserRepository) GetUser(userID string) (*models.User, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUser, userID)
	utils.Infof("GetUser: retrieving user %s with key %s", userID, key)
	data, err := r.storage.Get(key)
	if err != nil {
		utils.Errorf("GetUser: failed to get user %s: %v", userID, err)
		return nil, err
	}

	utils.Infof("GetUser: retrieved data for user %s, data type: %T, data value: %v", userID, data, data)
	userData, ok := data.(string)
	if !ok {
		utils.Errorf("GetUser: invalid user data type for user %s, expected string, got %T", userID, data)
		return nil, fmt.Errorf("invalid user data type")
	}

	var user models.User
	if err := json.Unmarshal([]byte(userData), &user); err != nil {
		utils.Errorf("GetUser: failed to unmarshal user %s: %v", userID, err)
		return nil, fmt.Errorf("unmarshal user failed: %w", err)
	}

	utils.Infof("GetUser: successfully retrieved user %s", userID)
	return &user, nil
}

// DeleteUser 删除用户
func (r *UserRepository) DeleteUser(userID string) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUser, userID)
	return r.storage.Delete(key)
}

// ListUsers 列出用户
func (r *UserRepository) ListUsers(userType models.UserType) ([]*models.User, error) {
	key := constants.KeyPrefixUserList
	data, err := r.storage.GetList(key)
	if err != nil {
		return []*models.User{}, nil
	}

	var users []*models.User
	for _, item := range data {
		if userData, ok := item.(string); ok {
			var user models.User
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
func (r *UserRepository) AddUserToList(user *models.User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return err
	}

	key := constants.KeyPrefixUserList
	return r.storage.AppendToList(key, string(data))
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
func (r *ClientRepository) SaveClient(client *models.Client) error {
	data, err := json.Marshal(client)
	if err != nil {
		return fmt.Errorf("marshal client failed: %w", err)
	}

	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClient, client.ID)
	err = r.storage.Set(key, string(data), constants2.DefaultClientDataTTL)
	if err != nil {
		return err
	}

	// 将客户端添加到全局客户端列表中
	return r.AddClientToList(client)
}

// CreateClient 创建新客户端（仅创建，不允许覆盖）
func (r *ClientRepository) CreateClient(client *models.Client) error {
	// 检查客户端是否已存在
	existingClient, err := r.GetClient(fmt.Sprintf("%d", client.ID))
	if err == nil && existingClient != nil {
		return fmt.Errorf("client with ID %d already exists", client.ID)
	}

	return r.SaveClient(client)
}

// UpdateClient 更新客户端（仅更新，不允许创建）
func (r *ClientRepository) UpdateClient(client *models.Client) error {
	// 检查客户端是否存在
	existingClient, _ := r.GetClient(fmt.Sprintf("%d", client.ID))
	if existingClient == nil {
		return fmt.Errorf("client with ID %d does not exist", client.ID)
	}

	return r.SaveClient(client)
}

// GetClient 获取客户端
func (r *ClientRepository) GetClient(clientID string) (*models.Client, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixClient, clientID)
	data, err := r.storage.Get(key)
	if err != nil {
		return nil, err
	}

	clientData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid client data type")
	}

	var client models.Client
	if err := json.Unmarshal([]byte(clientData), &client); err != nil {
		return nil, fmt.Errorf("unmarshal client failed: %w", err)
	}

	return &client, nil
}

// DeleteClient 删除客户端
func (r *ClientRepository) DeleteClient(clientID string) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixClient, clientID)
	return r.storage.Delete(key)
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
	data, err := r.storage.GetList(key)
	if err != nil {
		return []*models.Client{}, nil
	}

	var clients []*models.Client
	for _, item := range data {
		if clientData, ok := item.(string); ok {
			var client models.Client
			if err := json.Unmarshal([]byte(clientData), &client); err != nil {
				continue
			}
			clients = append(clients, &client)
		}
	}

	return clients, nil
}

// AddClientToUser 添加客户端到用户
func (r *ClientRepository) AddClientToUser(userID string, client *models.Client) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserClients, userID)
	data, err := json.Marshal(client)
	if err != nil {
		return err
	}

	return r.storage.AppendToList(key, string(data))
}

// RemoveClientFromUser 从用户移除客户端
func (r *ClientRepository) RemoveClientFromUser(userID string, client *models.Client) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserClients, userID)
	data, err := json.Marshal(client)
	if err != nil {
		return err
	}

	return r.storage.RemoveFromList(key, string(data))
}

// ListClients 列出所有客户端
func (r *ClientRepository) ListClients() ([]*models.Client, error) {
	key := constants.KeyPrefixClientList
	data, err := r.storage.GetList(key)
	if err != nil {
		return []*models.Client{}, nil
	}

	var clients []*models.Client
	for _, item := range data {
		if clientData, ok := item.(string); ok {
			var client models.Client
			if err := json.Unmarshal([]byte(clientData), &client); err != nil {
				continue
			}
			clients = append(clients, &client)
		}
	}

	return clients, nil
}

// AddClientToList 添加客户端到全局客户端列表
func (r *ClientRepository) AddClientToList(client *models.Client) error {
	key := constants.KeyPrefixClientList
	data, err := json.Marshal(client)
	if err != nil {
		return err
	}

	return r.storage.AppendToList(key, string(data))
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
	*Repository
}

// NewPortMappingRepo 创建端口映射数据访问层
func NewPortMappingRepo(repo *Repository) *PortMappingRepo {
	return &PortMappingRepo{Repository: repo}
}

// SavePortMapping 保存端口映射（创建或更新）
func (r *PortMappingRepo) SavePortMapping(mapping *models.PortMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("marshal port mapping failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", constants.KeyPrefixPortMapping, mapping.ID)
	return r.storage.Set(key, string(data), constants2.DefaultMappingDataTTL)
}

// CreatePortMapping 创建新端口映射（仅创建，不允许覆盖）
func (r *PortMappingRepo) CreatePortMapping(mapping *models.PortMapping) error {
	// 检查端口映射是否已存在
	existingMapping, err := r.GetPortMapping(mapping.ID)
	if err == nil && existingMapping != nil {
		return fmt.Errorf("port mapping with ID %s already exists", mapping.ID)
	}

	return r.SavePortMapping(mapping)
}

// UpdatePortMapping 更新端口映射（仅更新，不允许创建）
func (r *PortMappingRepo) UpdatePortMapping(mapping *models.PortMapping) error {
	// 检查端口映射是否存在
	existingMapping, _ := r.GetPortMapping(mapping.ID)
	if existingMapping == nil {
		return fmt.Errorf("port mapping with ID %s does not exist", mapping.ID)
	}

	return r.SavePortMapping(mapping)
}

// GetPortMapping 获取端口映射
func (r *PortMappingRepo) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixPortMapping, mappingID)
	data, err := r.storage.Get(key)
	if err != nil {
		return nil, err
	}

	mappingData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid mapping data type")
	}

	var mapping models.PortMapping
	if err := json.Unmarshal([]byte(mappingData), &mapping); err != nil {
		return nil, fmt.Errorf("unmarshal port mapping failed: %w", err)
	}

	return &mapping, nil
}

// DeletePortMapping 删除端口映射
func (r *PortMappingRepo) DeletePortMapping(mappingID string) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixPortMapping, mappingID)
	return r.storage.Delete(key)
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
	data, err := r.storage.GetList(key)
	if err != nil {
		return []*models.PortMapping{}, nil
	}

	var mappings []*models.PortMapping
	for _, item := range data {
		if mappingData, ok := item.(string); ok {
			var mapping models.PortMapping
			if err := json.Unmarshal([]byte(mappingData), &mapping); err != nil {
				continue
			}
			mappings = append(mappings, &mapping)
		}
	}

	return mappings, nil
}

// GetClientPortMappings 列出客户端的端口映射
func (r *PortMappingRepo) GetClientPortMappings(clientID string) ([]*models.PortMapping, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, clientID)
	data, err := r.storage.GetList(key)
	if err != nil {
		return []*models.PortMapping{}, nil
	}

	var mappings []*models.PortMapping
	for _, item := range data {
		if mappingData, ok := item.(string); ok {
			var mapping models.PortMapping
			if err := json.Unmarshal([]byte(mappingData), &mapping); err != nil {
				continue
			}
			mappings = append(mappings, &mapping)
		}
	}

	return mappings, nil
}

// AddMappingToUser 添加映射到用户
func (r *PortMappingRepo) AddMappingToUser(userID string, mapping *models.PortMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", constants.KeyPrefixUserMappings, userID)
	return r.storage.AppendToList(key, string(data))
}

// AddMappingToClient 添加映射到客户端
func (r *PortMappingRepo) AddMappingToClient(clientID string, mapping *models.PortMapping) error {
	data, err := json.Marshal(mapping)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", constants.KeyPrefixClientMappings, clientID)
	return r.storage.AppendToList(key, string(data))
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
func (r *NodeRepository) SaveNode(node *models.Node) error {
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshal node failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", constants.KeyPrefixNode, node.ID)
	return r.storage.Set(key, string(data), constants2.DefaultNodeDataTTL)
}

// CreateNode 创建新节点（仅创建，不允许覆盖）
func (r *NodeRepository) CreateNode(node *models.Node) error {
	// 检查节点是否已存在
	existingNode, err := r.GetNode(node.ID)
	if err == nil && existingNode != nil {
		return fmt.Errorf("node with ID %s already exists", node.ID)
	}

	return r.SaveNode(node)
}

// UpdateNode 更新节点（仅更新，不允许创建）
func (r *NodeRepository) UpdateNode(node *models.Node) error {
	// 检查节点是否存在
	existingNode, _ := r.GetNode(node.ID)
	if existingNode == nil {
		return fmt.Errorf("node with ID %s does not exist", node.ID)
	}

	return r.SaveNode(node)
}

// GetNode 获取节点
func (r *NodeRepository) GetNode(nodeID string) (*models.Node, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixNode, nodeID)
	data, err := r.storage.Get(key)
	if err != nil {
		return nil, err
	}

	nodeData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid node data type")
	}

	var node models.Node
	if err := json.Unmarshal([]byte(nodeData), &node); err != nil {
		return nil, fmt.Errorf("unmarshal node failed: %w", err)
	}

	return &node, nil
}

// DeleteNode 删除节点
func (r *NodeRepository) DeleteNode(nodeID string) error {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixNode, nodeID)
	return r.storage.Delete(key)
}

// ListNodes 列出所有节点
func (r *NodeRepository) ListNodes() ([]*models.Node, error) {
	key := constants.KeyPrefixNodeList
	data, err := r.storage.GetList(key)
	if err != nil {
		return []*models.Node{}, nil
	}

	var nodes []*models.Node
	for _, item := range data {
		if nodeData, ok := item.(string); ok {
			var node models.Node
			if err := json.Unmarshal([]byte(nodeData), &node); err != nil {
				continue
			}
			nodes = append(nodes, &node)
		}
	}

	return nodes, nil
}

// AddNodeToList 添加节点到列表
func (r *NodeRepository) AddNodeToList(node *models.Node) error {
	data, err := json.Marshal(node)
	if err != nil {
		return err
	}

	key := constants.KeyPrefixNodeList
	return r.storage.AppendToList(key, string(data))
}

// ConnectionRepo 连接数据访问
type ConnectionRepo struct {
	*Repository
	utils.Dispose
}

// NewConnectionRepo 创建连接数据访问层
func NewConnectionRepo(repo *Repository) *ConnectionRepo {
	cr := &ConnectionRepo{Repository: repo}
	cr.Dispose.SetCtx(context.Background(), cr.onClose)
	return cr
}

func (cr *ConnectionRepo) onClose() error {
	if cr.Repository != nil {
		return cr.Repository.Dispose.Close()
	}
	return nil
}

// SaveConnection 保存连接信息（创建或更新）
func (r *ConnectionRepo) SaveConnection(connInfo *models.ConnectionInfo) error {
	data, err := json.Marshal(connInfo)
	if err != nil {
		return fmt.Errorf("marshal connection failed: %w", err)
	}

	key := fmt.Sprintf("%s:%s", constants.KeyPrefixConnection, connInfo.ConnID)
	if err := r.storage.Set(key, string(data), constants2.DefaultConnectionTTL); err != nil {
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
	// 检查连接是否已存在
	existingConn, err := r.GetConnection(connInfo.ConnID)
	if err == nil && existingConn != nil {
		return fmt.Errorf("connection with ID %s already exists", connInfo.ConnID)
	}

	return r.SaveConnection(connInfo)
}

// UpdateConnection 更新连接（仅更新，不允许创建）
func (r *ConnectionRepo) UpdateConnection(connInfo *models.ConnectionInfo) error {
	// 检查连接是否存在
	existingConn, _ := r.GetConnection(connInfo.ConnID)
	if existingConn == nil {
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
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixConnection, connID)
	data, err := r.storage.Get(key)
	if err != nil {
		return nil, err
	}

	connData, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid connection data type")
	}

	var connInfo models.ConnectionInfo
	if err := json.Unmarshal([]byte(connData), &connInfo); err != nil {
		return nil, fmt.Errorf("unmarshal connection failed: %w", err)
	}

	return &connInfo, nil
}

// DeleteConnection 删除连接
func (r *ConnectionRepo) DeleteConnection(connID string) error {
	// 先获取连接信息，以便从相关列表中移除
	connInfo, err := r.GetConnection(connID)
	if err != nil {
		// 如果连接不存在，直接返回
		return err
	}

	// 从映射连接列表中移除
	if connInfo.MappingID != "" {
		if err := r.RemoveConnectionFromMapping(connInfo.MappingID, connID); err != nil {
			// 记录错误但不中断删除过程
			utils.Warnf("Failed to remove connection %s from mapping %s: %v", connID, connInfo.MappingID, err)
		}
	}

	// 从客户端连接列表中移除
	if err := r.RemoveConnectionFromClient(connInfo.ClientID, connID); err != nil {
		// 记录错误但不中断删除过程
		utils.Warnf("Failed to remove connection %s from client %d: %v", connID, connInfo.ClientID, err)
	}

	// 删除主连接记录
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixConnection, connID)
	return r.storage.Delete(key)
}

// ListConnections 列出映射的连接
func (r *ConnectionRepo) ListConnections(mappingID string) ([]*models.ConnectionInfo, error) {
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixMappingConnections, mappingID)
	data, err := r.storage.GetList(key)
	if err != nil {
		return []*models.ConnectionInfo{}, nil
	}

	var connections []*models.ConnectionInfo
	for _, item := range data {
		if connData, ok := item.(string); ok {
			var connInfo models.ConnectionInfo
			if err := json.Unmarshal([]byte(connData), &connInfo); err != nil {
				continue
			}
			connections = append(connections, &connInfo)
		}
	}

	return connections, nil
}

// ListClientConns 列出客户端的所有连接
func (r *ConnectionRepo) ListClientConns(clientID int64) ([]*models.ConnectionInfo, error) {
	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClientConnections, clientID)
	data, err := r.storage.GetList(key)
	if err != nil {
		return []*models.ConnectionInfo{}, nil
	}

	var conns []*models.ConnectionInfo
	for _, item := range data {
		if connData, ok := item.(string); ok {
			var conn models.ConnectionInfo
			if err := json.Unmarshal([]byte(connData), &conn); err != nil {
				continue
			}
			conns = append(conns, &conn)
		}
	}

	return conns, nil
}

// AddConnectionToMapping 添加连接到映射列表
func (r *ConnectionRepo) AddConnectionToMapping(mappingID string, connInfo *models.ConnectionInfo) error {
	data, err := json.Marshal(connInfo)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", constants.KeyPrefixMappingConnections, mappingID)

	// 使用存储层的原子列表操作
	return r.storage.AppendToList(key, string(data))
}

// AddConnectionToClient 添加连接到客户端
func (r *ConnectionRepo) AddConnectionToClient(clientID int64, connInfo *models.ConnectionInfo) error {
	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClientConnections, clientID)
	data, err := json.Marshal(connInfo)
	if err != nil {
		return err
	}
	return r.storage.AppendToList(key, string(data))
}

// RemoveConnectionFromMapping 从映射连接列表中移除连接
func (r *ConnectionRepo) RemoveConnectionFromMapping(mappingID string, connID string) error {
	// 获取映射的所有连接
	connections, err := r.ListConnections(mappingID)
	if err != nil {
		return err
	}

	// 找到要删除的连接并序列化
	var targetConnData string
	for _, conn := range connections {
		if conn.ConnID == connID {
			data, err := json.Marshal(conn)
			if err != nil {
				return err
			}
			targetConnData = string(data)
			break
		}
	}

	if targetConnData == "" {
		// 连接不在列表中，不需要删除
		return nil
	}

	// 从列表中移除
	key := fmt.Sprintf("%s:%s", constants.KeyPrefixMappingConnections, mappingID)
	return r.storage.RemoveFromList(key, targetConnData)
}

// RemoveConnectionFromClient 从客户端连接列表中移除连接
func (r *ConnectionRepo) RemoveConnectionFromClient(clientID int64, connID string) error {
	// 获取客户端的所有连接
	connections, err := r.ListClientConns(clientID)
	if err != nil {
		return err
	}

	// 找到要删除的连接并序列化
	var targetConnData string
	for _, conn := range connections {
		if conn.ConnID == connID {
			data, err := json.Marshal(conn)
			if err != nil {
				return err
			}
			targetConnData = string(data)
			break
		}
	}

	if targetConnData == "" {
		// 连接不在列表中，不需要删除
		return nil
	}

	// 从列表中移除
	key := fmt.Sprintf("%s:%d", constants.KeyPrefixClientConnections, clientID)
	return r.storage.RemoveFromList(key, targetConnData)
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
