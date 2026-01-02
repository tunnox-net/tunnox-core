package managers

import (
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
)

// CloudControlAPI 云控平台接口
type CloudControlAPI interface {
	// 节点管理
	NodeRegister(req *models.NodeRegisterRequest) (*models.NodeRegisterResponse, error)
	NodeUnregister(req *models.NodeUnregisterRequest) error
	NodeHeartbeat(req *models.NodeHeartbeatRequest) (*models.NodeHeartbeatResponse, error)

	// 用户认证
	Authenticate(req *models.AuthRequest) (*models.AuthResponse, error)
	ValidateToken(token string) (*models.AuthResponse, error)

	// 用户管理
	CreateUser(username, email string) (*models.User, error) // 创建用户，服务端分配用户ID
	GetUser(userID string) (*models.User, error)
	UpdateUser(user *models.User) error
	DeleteUser(userID string) error
	ListUsers(userType models.UserType) ([]*models.User, error)

	// 客户端管理
	CreateClient(userID, clientName string) (*models.Client, error) // 为指定用户创建客户端
	GetClient(clientID int64) (*models.Client, error)
	TouchClient(clientID int64)
	UpdateClient(client *models.Client) error
	DeleteClient(clientID int64) error
	UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error
	ConnectClient(clientID int64, nodeID, connID, ipAddress, protocol, version string) error // 客户端连接（更新完整运行时状态）
	DisconnectClient(clientID int64) error                                                   // 客户端断开连接
	ListClients(userID string, clientType models.ClientType) ([]*models.Client, error)
	ListUserClients(userID string) ([]*models.Client, error)             // 获取用户下的所有客户端
	GetClientPortMappings(clientID int64) ([]*models.PortMapping, error) // 获取客户端下的所有端口映射

	// 客户端状态查询（快速接口，仅查Redis）
	GetClientNodeID(clientID int64) (string, error)             // 获取客户端所在节点ID
	IsClientOnNode(clientID int64, nodeID string) (bool, error) // 检查客户端是否在指定节点
	GetNodeClients(nodeID string) ([]*models.Client, error)     // 获取节点的所有在线客户端

	// 端口映射管理
	CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error)
	GetUserPortMappings(userID string) ([]*models.PortMapping, error)
	GetPortMapping(mappingID string) (*models.PortMapping, error)
	GetPortMappingByDomain(fullDomain string) (*models.PortMapping, error) // 通过域名查找 HTTP 映射
	UpdatePortMapping(mapping *models.PortMapping) error
	DeletePortMapping(mappingID string) error
	UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error
	UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error
	ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error)
	MigrateClientMappings(fromClientID, toClientID int64) error

	// 匿名用户管理
	GenerateAnonymousCredentials() (*models.Client, error) // 生成匿名客户端凭据
	GetAnonymousClient(clientID int64) (*models.Client, error)
	DeleteAnonymousClient(clientID int64) error
	ListAnonymousClients() ([]*models.Client, error)
	CreateAnonymousMapping(listenClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) // ✅ 统一命名：listenClientID
	GetAnonymousMappings() ([]*models.PortMapping, error)
	CleanupExpiredAnonymous() error

	// 节点服务信息
	GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error)
	GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error)

	// 统计和监控接口
	GetUserStats(userID string) (*stats.UserStats, error)                      // 获取用户统计信息
	GetClientStats(clientID int64) (*stats.ClientStats, error)                 // 获取客户端统计信息
	GetSystemStats() (*stats.SystemStats, error)                               // 获取系统整体统计
	GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error)       // 获取流量统计图表数据
	GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error) // 获取连接数统计图表数据

	// 连接管理接口
	RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error
	UnregisterConnection(connID string) error
	GetConnections(mappingID string) ([]*models.ConnectionInfo, error)
	GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error)
	UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error

	// JWT Token管理接口
	GenerateJWTToken(clientID int64) (*JWTTokenInfo, error)
	RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error)
	ValidateJWTToken(token string) (*JWTTokenInfo, error)
	RevokeJWTToken(token string) error

	// 搜索和过滤接口
	SearchUsers(keyword string) ([]*models.User, error)               // 搜索用户
	SearchClients(keyword string) ([]*models.Client, error)           // 搜索客户端
	SearchPortMappings(keyword string) ([]*models.PortMapping, error) // 搜索端口映射

	Close() error
}

// ClientNotifier 客户端通知接口
type ClientNotifier interface {
	NotifyClientUpdate(clientID int64)
}

// NotifierAware 支持注入通知器的接口
type NotifierAware interface {
	SetNotifier(notifier ClientNotifier)
}

// ControlConfig 是 configs.ControlConfig 的别名，保持向后兼容
type ControlConfig = configs.ControlConfig

// DefaultConfig 返回默认配置
func DefaultConfig() *ControlConfig {
	return configs.DefaultControlConfig()
}
