package repos

import (
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
)

// =============================================================================
// Repository 接口定义
// =============================================================================
//
// 本文件定义了所有 Repository 的接口，用于：
// - 依赖注入：Service 层依赖接口而非具体实现
// - Mock 测试：测试时可以注入 Mock 实现
// - 解耦合：降低模块间的耦合度
//
// =============================================================================

// IClientRepository 客户端数据访问接口
//
// 管理客户端的 CRUD 操作和列表查询
type IClientRepository interface {
	// SaveClient 保存客户端（创建或更新）
	SaveClient(client *models.Client) error

	// CreateClient 创建新客户端（仅创建，不允许覆盖）
	CreateClient(client *models.Client) error

	// UpdateClient 更新客户端（仅更新，不允许创建）
	UpdateClient(client *models.Client) error

	// GetClient 获取客户端
	GetClient(clientID string) (*models.Client, error)

	// DeleteClient 删除客户端
	DeleteClient(clientID string) error

	// UpdateClientStatus 更新客户端状态
	UpdateClientStatus(clientID string, status models.ClientStatus, nodeID string) error

	// ListUserClients 列出用户的所有客户端
	ListUserClients(userID string) ([]*models.Client, error)

	// AddClientToUser 添加客户端到用户
	AddClientToUser(userID string, client *models.Client) error

	// RemoveClientFromUser 从用户移除客户端
	RemoveClientFromUser(userID string, client *models.Client) error

	// ListClients 列出所有客户端
	ListClients() ([]*models.Client, error)

	// ListAllClients 列出所有客户端（ListClients的别名）
	ListAllClients() ([]*models.Client, error)

	// AddClientToList 添加客户端到全局客户端列表
	AddClientToList(client *models.Client) error

	// TouchClient 刷新客户端的LastSeen和延长过期时间
	TouchClient(clientID string) error
}

// IPortMappingRepository 端口映射数据访问接口
//
// 管理端口映射的 CRUD 操作和关联查询
type IPortMappingRepository interface {
	// SavePortMapping 保存端口映射（创建或更新）
	SavePortMapping(mapping *models.PortMapping) error

	// CreatePortMapping 创建新端口映射（仅创建，不允许覆盖）
	CreatePortMapping(mapping *models.PortMapping) error

	// UpdatePortMapping 更新端口映射（仅更新，不允许创建）
	UpdatePortMapping(mapping *models.PortMapping) error

	// GetPortMapping 获取端口映射
	GetPortMapping(mappingID string) (*models.PortMapping, error)

	// GetPortMappingByDomain 通过域名查找 HTTP 映射
	GetPortMappingByDomain(fullDomain string) (*models.PortMapping, error)

	// DeletePortMapping 删除端口映射
	DeletePortMapping(mappingID string) error

	// UpdatePortMappingStatus 更新端口映射状态
	UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error

	// UpdatePortMappingStats 更新端口映射统计
	UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error

	// GetUserPortMappings 列出用户的端口映射
	GetUserPortMappings(userID string) ([]*models.PortMapping, error)

	// GetClientPortMappings 列出客户端的端口映射
	GetClientPortMappings(clientID string) ([]*models.PortMapping, error)

	// AddMappingToUser 添加映射到用户
	AddMappingToUser(userID string, mapping *models.PortMapping) error

	// AddMappingToClient 添加映射到客户端
	AddMappingToClient(clientID string, mapping *models.PortMapping) error

	// ListAllMappings 列出所有端口映射
	ListAllMappings() ([]*models.PortMapping, error)

	// AddMappingToList 添加映射到全局映射列表
	AddMappingToList(mapping *models.PortMapping) error
}

// IConnectionRepository 连接数据访问接口
//
// 管理隧道连接的 CRUD 操作和关联查询
type IConnectionRepository interface {
	// SaveConnection 保存连接信息（创建或更新）
	SaveConnection(connInfo *models.ConnectionInfo) error

	// CreateConnection 创建新连接（仅创建，不允许覆盖）
	CreateConnection(connInfo *models.ConnectionInfo) error

	// UpdateConnection 更新连接（仅更新，不允许创建）
	UpdateConnection(connInfo *models.ConnectionInfo) error

	// GetConnection 获取连接信息
	GetConnection(connID string) (*models.ConnectionInfo, error)

	// DeleteConnection 删除连接
	DeleteConnection(connID string) error

	// ListMappingConns 列出映射的连接
	ListMappingConns(mappingID string) ([]*models.ConnectionInfo, error)

	// ListClientConns 列出客户端的连接
	ListClientConns(clientID int64) ([]*models.ConnectionInfo, error)

	// AddConnectionToMapping 添加连接到映射列表
	AddConnectionToMapping(mappingID string, connInfo *models.ConnectionInfo) error

	// AddConnectionToClient 添加连接到客户端
	AddConnectionToClient(clientID int64, connInfo *models.ConnectionInfo) error

	// RemoveConnectionFromMapping 从映射连接列表中移除连接
	RemoveConnectionFromMapping(mappingID string, connInfo *models.ConnectionInfo) error

	// RemoveConnectionFromClient 从客户端连接列表中移除连接
	RemoveConnectionFromClient(clientID int64, connInfo *models.ConnectionInfo) error

	// UpdateStats 更新连接统计
	UpdateStats(connID string, bytesSent, bytesReceived int64) error
}

// INodeRepository 节点数据访问接口
//
// 管理集群节点的 CRUD 操作
type INodeRepository interface {
	// SaveNode 保存节点（创建或更新）
	SaveNode(node *models.Node) error

	// CreateNode 创建新节点（仅创建，不允许覆盖）
	CreateNode(node *models.Node) error

	// UpdateNode 更新节点（仅更新，不允许创建）
	UpdateNode(node *models.Node) error

	// GetNode 获取节点
	GetNode(nodeID string) (*models.Node, error)

	// DeleteNode 删除节点
	DeleteNode(nodeID string) error

	// ListNodes 列出所有节点
	ListNodes() ([]*models.Node, error)

	// AddNodeToList 添加节点到列表
	AddNodeToList(node *models.Node) error
}

// IConnectionCodeRepository 连接码数据访问接口
//
// 管理隧道连接码的 CRUD 操作和查询
type IConnectionCodeRepository interface {
	// Create 创建连接码
	Create(code *models.TunnelConnectionCode) error

	// GetByCode 按连接码查询
	GetByCode(code string) (*models.TunnelConnectionCode, error)

	// GetByID 按ID查询
	GetByID(id string) (*models.TunnelConnectionCode, error)

	// ListByTargetClient 查询TargetClient的所有连接码
	ListByTargetClient(targetClientID int64) ([]*models.TunnelConnectionCode, error)

	// Update 更新连接码
	Update(code *models.TunnelConnectionCode) error

	// Delete 删除连接码
	Delete(id string) error

	// CountByTargetClient 统计TargetClient的连接码数量
	CountByTargetClient(targetClientID int64) (int, error)

	// CountActiveByTargetClient 统计TargetClient的活跃连接码数量
	CountActiveByTargetClient(targetClientID int64) (int, error)
}

// IUserRepository 用户数据访问接口
//
// 管理用户的 CRUD 操作
type IUserRepository interface {
	// SaveUser 保存用户（创建或更新）
	SaveUser(user *models.User) error

	// CreateUser 创建新用户（仅创建，不允许覆盖）
	CreateUser(user *models.User) error

	// UpdateUser 更新用户（仅更新，不允许创建）
	UpdateUser(user *models.User) error

	// GetUser 获取用户
	GetUser(userID string) (*models.User, error)

	// DeleteUser 删除用户
	DeleteUser(userID string) error

	// ListUsers 列出用户（按类型过滤）
	ListUsers(userType models.UserType) ([]*models.User, error)

	// AddUserToList 添加用户到列表
	AddUserToList(user *models.User) error

	// ListAllUsers 列出所有用户（不过滤类型）
	ListAllUsers() ([]*models.User, error)
}

// IClientTokenRepository 客户端Token数据访问接口
//
// 管理客户端JWT Token的缓存操作
type IClientTokenRepository interface {
	// GetToken 获取Token
	GetToken(clientID int64) (*models.ClientToken, error)

	// SetToken 设置Token
	SetToken(token *models.ClientToken) error

	// DeleteToken 删除Token
	DeleteToken(clientID int64) error

	// TokenExists 检查Token是否存在且有效
	TokenExists(clientID int64) (bool, error)

	// RefreshToken 刷新Token（延长TTL）
	RefreshToken(token *models.ClientToken) error
}

// IClientStateRepository 客户端状态数据访问接口
//
// 管理客户端运行时状态的缓存操作
type IClientStateRepository interface {
	// GetState 获取客户端状态
	GetState(clientID int64) (*models.ClientRuntimeState, error)

	// SetState 设置客户端状态
	SetState(state *models.ClientRuntimeState) error

	// DeleteState 删除客户端状态
	DeleteState(clientID int64) error

	// TouchState 更新客户端心跳时间
	TouchState(clientID int64) error

	// GetNodeClients 获取指定节点的所有在线客户端ID列表
	GetNodeClients(nodeID string) ([]int64, error)

	// AddToNodeClients 将客户端添加到节点的客户端列表
	AddToNodeClients(nodeID string, clientID int64) error

	// RemoveFromNodeClients 从节点的客户端列表中移除客户端
	RemoveFromNodeClients(nodeID string, clientID int64) error
}

// IClientConfigRepository 客户端配置数据访问接口
//
// 管理客户端持久化配置的 CRUD 操作
type IClientConfigRepository interface {
	// GetConfig 获取客户端配置
	GetConfig(clientID int64) (*models.ClientConfig, error)

	// SaveConfig 保存客户端配置（创建或更新）
	SaveConfig(config *models.ClientConfig) error

	// CreateConfig 创建新的客户端配置（仅创建，不允许覆盖）
	CreateConfig(config *models.ClientConfig) error

	// UpdateConfig 更新客户端配置（仅更新，不允许创建）
	UpdateConfig(config *models.ClientConfig) error

	// DeleteConfig 删除客户端配置
	DeleteConfig(clientID int64) error

	// ListConfigs 列出所有客户端配置
	ListConfigs() ([]*models.ClientConfig, error)

	// ListUserConfigs 列出用户的所有客户端配置
	ListUserConfigs(userID string) ([]*models.ClientConfig, error)

	// AddConfigToList 将配置添加到全局列表
	AddConfigToList(config *models.ClientConfig) error

	// ExistsConfig 检查配置是否存在
	ExistsConfig(clientID int64) (bool, error)
}
