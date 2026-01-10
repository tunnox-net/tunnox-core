package services

import (
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/security"
)

// BrokerAware 消息代理感知接口
// 实现此接口的服务可以接收 MessageBroker 实例用于发布事件
type BrokerAware interface {
	SetBroker(b broker.MessageBroker)
}

type WebhookNotifierAware interface {
	SetWebhookNotifier(n WebhookNotifier)
}

type WebhookNotifier interface {
	DispatchClientOnline(clientID int64, userID, ipAddress, nodeID string)
	DispatchClientOffline(clientID int64, userID string)
}

// ClientNotifier 客户端通知接口
// 用于在服务层避免循环依赖，与 managers.ClientNotifier 接口兼容
// 注意：anonymous.Notifier 使用相同的方法签名，满足 Go 的隐式接口实现
type ClientNotifier interface {
	NotifyClientUpdate(clientID int64)
}

// ClientKicker 客户端踢出接口
// 用于在凭据重置后踢掉当前连接
// 注意：与 client.ClientKicker 使用相同的方法签名，满足 Go 的隐式接口实现
type ClientKicker interface {
	KickClient(clientID int64, reason, message string) error
}

// anonymousNotifierAdapter 用于适配 services.ClientNotifier 到 anonymous.Notifier
type anonymousNotifierAdapter struct {
	notifier ClientNotifier
}

func (a *anonymousNotifierAdapter) NotifyClientUpdate(clientID int64) {
	a.notifier.NotifyClientUpdate(clientID)
}

// UserService 用户管理服务
type UserService interface {
	CreateUser(username, email string, platformUserID int64) (*models.User, error) // platformUserID: Platform 用户 ID（BIGINT，用于双向关联）
	GetUser(userID string) (*models.User, error)
	UpdateUser(user *models.User) error
	DeleteUser(userID string) error
	ListUsers(userType models.UserType) ([]*models.User, error)
	SearchUsers(keyword string) ([]*models.User, error)
	GetUserStats(userID string) (*stats.UserStats, error)
}

// ClientService 客户端管理服务
type ClientService interface {
	CreateClient(userID, clientName string) (*models.Client, error)
	GetClient(clientID int64) (*models.Client, error)
	GetClientConfig(clientID int64) (*models.ClientConfig, error) // 获取客户端配置（用于挑战-响应认证）
	TouchClient(clientID int64)
	UpdateClient(client *models.Client) error
	DeleteClient(clientID int64) error
	UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error
	ConnectClient(clientID int64, nodeID, connID, ipAddress, protocol, version string) error    // 客户端连接（更新完整运行时状态）
	DisconnectClient(clientID int64) error                                                     // 客户端断开连接
	EnsureClientOnline(clientID int64, nodeID, connID, ipAddress, protocol, version string) error // 确保客户端在线状态存在（心跳时调用，状态丢失时重建）
	ListClients(userID string, clientType models.ClientType) ([]*models.Client, error)
	ListUserClients(userID string) ([]*models.Client, error)
	GetClientPortMappings(clientID int64) ([]*models.PortMapping, error)
	SearchClients(keyword string) ([]*models.Client, error)
	GetClientStats(clientID int64) (*stats.ClientStats, error)

	// 凭据管理（SecretKey V3）
	SetSecretKeyManager(mgr *security.SecretKeyManager)                                     // 设置 SecretKey 管理器
	ResetSecretKey(clientID int64, kicker interface{ KickClient(int64, string, string) error }) (newSecretKey string, err error) // 重置 SecretKey
	MigrateToEncrypted(clientID int64) error                                                // 迁移到加密存储
	VerifySecretKey(clientID int64, secretKey string) (bool, error)                         // 验证 SecretKey
}

// PortMappingService 端口映射服务
type PortMappingService interface {
	CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error)
	GetPortMapping(mappingID string) (*models.PortMapping, error)
	GetPortMappingByDomain(fullDomain string) (*models.PortMapping, error) // 通过域名查找 HTTP 映射
	UpdatePortMapping(mapping *models.PortMapping) error
	DeletePortMapping(mappingID string) error
	CleanupOrphanedMappingIndexes(mappingID, userID string, mappingData map[string]interface{}) error // 清理孤立的映射索引
	UpdatePortMappingStatus(mappingID string, status models.MappingStatus) error
	UpdatePortMappingStats(mappingID string, stats *stats.TrafficStats) error
	GetUserPortMappings(userID string) ([]*models.PortMapping, error)
	ListPortMappings(mappingType models.MappingType) ([]*models.PortMapping, error)
	SearchPortMappings(keyword string) ([]*models.PortMapping, error)
}

// NodeService 节点管理服务
type NodeService interface {
	NodeRegister(req *models.NodeRegisterRequest) (*models.NodeRegisterResponse, error)
	NodeUnregister(req *models.NodeUnregisterRequest) error
	NodeHeartbeat(req *models.NodeHeartbeatRequest) (*models.NodeHeartbeatResponse, error)
	GetNodeServiceInfo(nodeID string) (*models.NodeServiceInfo, error)
	GetAllNodeServiceInfo() ([]*models.NodeServiceInfo, error)
}

// AuthService 认证服务
type AuthService interface {
	Authenticate(req *models.AuthRequest) (*models.AuthResponse, error)
	ValidateToken(token string) (*models.AuthResponse, error)
	GenerateJWTToken(clientID int64) (*JWTTokenInfo, error)
	RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error)
	ValidateJWTToken(token string) (*JWTTokenInfo, error)
	RevokeJWTToken(token string) error
}

// AnonymousService 匿名用户服务
type AnonymousService interface {
	GenerateAnonymousCredentials() (*models.Client, error)
	GetAnonymousClient(clientID int64) (*models.Client, error)
	DeleteAnonymousClient(clientID int64) error
	ListAnonymousClients() ([]*models.Client, error)
	CreateAnonymousMapping(listenClientID, targetClientID int64, protocol models.Protocol, sourcePort, targetPort int) (*models.PortMapping, error) // ✅ 统一命名：listenClientID
	GetAnonymousMappings() ([]*models.PortMapping, error)
	CleanupExpiredAnonymous() error
	SetNotifier(notifier ClientNotifier)
	SetSecretKeyManager(mgr *security.SecretKeyManager) // SecretKey 管理器（用于加密存储凭据）
}

// ConnectionService 连接管理服务
type ConnectionService interface {
	RegisterConnection(mappingID string, connInfo *models.ConnectionInfo) error
	UnregisterConnection(connID string) error
	GetConnections(mappingID string) ([]*models.ConnectionInfo, error)
	GetClientConnections(clientID int64) ([]*models.ConnectionInfo, error)
	UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error
}

// StatsService 统计服务
type StatsService interface {
	GetSystemStats() (*stats.SystemStats, error)
	GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error)
	GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error)
}

// 注意: JWTTokenInfo 定义在 auth 子包中，通过 auth_facade.go 重新导出
// StatsProvider, JWTProvider 等接口类型定义在 base_service.go 中
// 这里保留了服务接口定义，避免循环导入
