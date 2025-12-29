package services

import (
	"context"
	"time"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/stats"
)

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
	SetNotifier(notifier interface{}) // 使用 interface{} 避免循环依赖，具体实现转换
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

// JWTTokenInfo JWT令牌信息
type JWTTokenInfo struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
	ClientID     int64     `json:"client_id"`
}

// StatsProvider 统计数据提供者接口
// 由 managers.StatsManager 实现，供 Service 层使用
type StatsProvider interface {
	// GetCounter 获取统计计数器
	GetCounter() *stats.StatsCounter
	// GetUserStats 获取用户统计信息
	GetUserStats(userID string) (*stats.UserStats, error)
	// GetClientStats 获取客户端统计信息
	GetClientStats(clientID int64) (*stats.ClientStats, error)
}

// JWTProvider JWT令牌提供者接口
// 由 managers.JWTManager 实现，供 Service 层使用
// 注意：返回类型使用 interface{} 以避免循环依赖，实际实现会返回具体类型
type JWTProvider interface {
	// GenerateTokenPair 生成Token对（访问Token + 刷新Token）
	// 返回 *managers.JWTTokenInfo
	GenerateTokenPair(ctx context.Context, client *models.Client) (JWTTokenResult, error)
	// ValidateAccessToken 验证访问Token
	// 返回 *managers.JWTClaims
	ValidateAccessToken(ctx context.Context, tokenString string) (JWTClaimsResult, error)
	// ValidateRefreshToken 验证刷新Token
	// 返回 *managers.RefreshTokenClaims
	ValidateRefreshToken(ctx context.Context, refreshTokenString string) (RefreshTokenClaimsResult, error)
	// RefreshAccessToken 使用刷新Token生成新的访问Token
	// 返回 *managers.JWTTokenInfo
	RefreshAccessToken(ctx context.Context, refreshTokenString string, client *models.Client) (JWTTokenResult, error)
	// RevokeToken 撤销Token
	RevokeToken(ctx context.Context, tokenID string) error
}

// JWTTokenResult JWT令牌生成结果接口
type JWTTokenResult interface {
	GetToken() string
	GetRefreshToken() string
	GetExpiresAt() time.Time
	GetClientId() int64
	GetTokenID() string
}

// JWTClaimsResult JWT声明结果接口
type JWTClaimsResult interface {
	GetClientID() int64
	GetUserID() string
	GetClientType() string
	GetNodeID() string
}

// RefreshTokenClaimsResult 刷新Token声明结果接口
type RefreshTokenClaimsResult interface {
	GetClientID() int64
	GetTokenID() string
}

// ManagerFactories 管理器工厂函数集合
// 用于解决 services 和 managers 之间的循环依赖
type ManagerFactories struct {
	// NewJWTProvider 创建 JWT 提供者的工厂函数
	NewJWTProvider func(config interface{}, storage interface{}, parentCtx context.Context) JWTProvider
	// NewStatsProvider 创建统计提供者的工厂函数
	NewStatsProvider func(userRepo, clientRepo, mappingRepo, nodeRepo interface{}, storage interface{}, parentCtx context.Context) StatsProvider
}
