package services

import (
	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
)

// BrokerAware 消息代理感知接口
// 实现此接口的服务可以接收 MessageBroker 实例用于发布事件
type BrokerAware interface {
	SetBroker(b broker.MessageBroker)
}

// ClientNotifier 客户端通知接口
// 用于在服务层避免循环依赖，与 managers.ClientNotifier 接口兼容
// 注意：anonymous.Notifier 使用相同的方法签名，满足 Go 的隐式接口实现
type ClientNotifier interface {
	NotifyClientUpdate(clientID int64)
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
	CreateUser(username, email string) (*models.User, error)
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
	TouchClient(clientID int64)
	UpdateClient(client *models.Client) error
	DeleteClient(clientID int64) error
	UpdateClientStatus(clientID int64, status models.ClientStatus, nodeID string) error
	ListClients(userID string, clientType models.ClientType) ([]*models.Client, error)
	ListUserClients(userID string) ([]*models.Client, error)
	GetClientPortMappings(clientID int64) ([]*models.PortMapping, error)
	SearchClients(keyword string) ([]*models.Client, error)
	GetClientStats(clientID int64) (*stats.ClientStats, error)
}

// PortMappingService 端口映射服务
type PortMappingService interface {
	CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error)
	GetPortMapping(mappingID string) (*models.PortMapping, error)
	GetPortMappingByDomain(fullDomain string) (*models.PortMapping, error) // 通过域名查找 HTTP 映射
	UpdatePortMapping(mapping *models.PortMapping) error
	DeletePortMapping(mappingID string) error
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
