package cloud

import (
	"context"
	"time"
)

// CloudControlAPI 云控平台接口
type CloudControlAPI interface {
	// 节点管理
	NodeRegister(ctx context.Context, req *NodeRegisterRequest) (*NodeRegisterResponse, error)
	NodeUnregister(ctx context.Context, req *NodeUnregisterRequest) error
	NodeHeartbeat(ctx context.Context, req *NodeHeartbeatRequest) (*NodeHeartbeatResponse, error)

	// 用户认证
	Authenticate(ctx context.Context, req *AuthRequest) (*AuthResponse, error)
	ValidateToken(ctx context.Context, token string) (*AuthResponse, error)

	// 用户管理
	CreateUser(ctx context.Context, username, email string) (*User, error) // 创建用户，服务端分配用户ID
	GetUser(ctx context.Context, userID string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	DeleteUser(ctx context.Context, userID string) error
	ListUsers(ctx context.Context, userType UserType) ([]*User, error)

	// 客户端管理
	CreateClient(ctx context.Context, userID string, clientName string) (*Client, error) // 为指定用户创建客户端
	GetClient(ctx context.Context, clientID string) (*Client, error)
	UpdateClient(ctx context.Context, client *Client) error
	DeleteClient(ctx context.Context, clientID string) error
	UpdateClientStatus(ctx context.Context, clientID string, status ClientStatus, nodeID string) error
	ListClients(ctx context.Context, userID string, clientType ClientType) ([]*Client, error)
	GetUserClients(ctx context.Context, userID string) ([]*Client, error)               // 获取用户下的所有客户端
	GetClientPortMappings(ctx context.Context, clientID string) ([]*PortMapping, error) // 获取客户端下的所有端口映射

	// 端口映射管理
	CreatePortMapping(ctx context.Context, mapping *PortMapping) (*PortMapping, error)
	GetPortMappings(ctx context.Context, userID string) ([]*PortMapping, error)
	GetPortMapping(ctx context.Context, mappingID string) (*PortMapping, error)
	UpdatePortMapping(ctx context.Context, mapping *PortMapping) error
	DeletePortMapping(ctx context.Context, mappingID string) error
	UpdatePortMappingStatus(ctx context.Context, mappingID string, status MappingStatus) error
	UpdatePortMappingStats(ctx context.Context, mappingID string, stats *TrafficStats) error
	ListPortMappings(ctx context.Context, mappingType MappingType) ([]*PortMapping, error)

	// 匿名用户管理
	GenerateAnonymousCredentials(ctx context.Context) (*Client, error) // 生成匿名客户端凭据
	GetAnonymousClient(ctx context.Context, clientID string) (*Client, error)
	DeleteAnonymousClient(ctx context.Context, clientID string) error
	ListAnonymousClients(ctx context.Context) ([]*Client, error)
	CreateAnonymousMapping(ctx context.Context, sourceClientID, targetClientID string, protocol Protocol, sourcePort, targetPort int) (*PortMapping, error)
	GetAnonymousMappings(ctx context.Context) ([]*PortMapping, error)
	CleanupExpiredAnonymous(ctx context.Context) error

	// 节点服务信息
	GetNodeServiceInfo(ctx context.Context, nodeID string) (*NodeServiceInfo, error)
	GetAllNodeServiceInfo(ctx context.Context) ([]*NodeServiceInfo, error)

	// 统计和监控接口
	GetUserStats(ctx context.Context, userID string) (*UserStats, error)                      // 获取用户统计信息
	GetClientStats(ctx context.Context, clientID string) (*ClientStats, error)                // 获取客户端统计信息
	GetSystemStats(ctx context.Context) (*SystemStats, error)                                 // 获取系统整体统计
	GetTrafficStats(ctx context.Context, timeRange string) ([]*TrafficDataPoint, error)       // 获取流量统计图表数据
	GetConnectionStats(ctx context.Context, timeRange string) ([]*ConnectionDataPoint, error) // 获取连接数统计图表数据

	// 连接管理接口
	RegisterConnection(ctx context.Context, mappingId string, connInfo *ConnectionInfo) error
	UnregisterConnection(ctx context.Context, connId string) error
	GetConnections(ctx context.Context, mappingId string) ([]*ConnectionInfo, error)
	GetClientConnections(ctx context.Context, clientId string) ([]*ConnectionInfo, error)
	UpdateConnectionStats(ctx context.Context, connId string, bytesSent, bytesReceived int64) error

	// JWT Token管理接口
	GenerateJWTToken(ctx context.Context, clientId string) (*JWTTokenInfo, error)
	RefreshJWTToken(ctx context.Context, refreshToken string) (*JWTTokenInfo, error)
	ValidateJWTToken(ctx context.Context, token string) (*JWTTokenInfo, error)
	RevokeJWTToken(ctx context.Context, token string) error

	// 搜索和过滤接口
	SearchUsers(ctx context.Context, keyword string) ([]*User, error)               // 搜索用户
	SearchClients(ctx context.Context, keyword string) ([]*Client, error)           // 搜索客户端
	SearchPortMappings(ctx context.Context, keyword string) ([]*PortMapping, error) // 搜索端口映射

	Close() error
}

// CloudControlConfig 云控配置
type CloudControlConfig struct {
	APIEndpoint string        `json:"api_endpoint"`
	APIKey      string        `json:"api_key,omitempty"`
	APISecret   string        `json:"api_secret,omitempty"`
	Timeout     time.Duration `json:"timeout"`
	NodeID      string        `json:"node_id,omitempty"`
	NodeName    string        `json:"node_name,omitempty"`
	UseBuiltIn  bool          `json:"use_built_in"`

	// JWT配置
	JWTSecretKey      string        `json:"jwt_secret_key"`     // JWT签名密钥
	JWTExpiration     time.Duration `json:"jwt_expiration"`     // JWT过期时间
	RefreshExpiration time.Duration `json:"refresh_expiration"` // 刷新Token过期时间
	JWTIssuer         string        `json:"jwt_issuer"`         // JWT签发者
}

// DefaultConfig 返回默认配置
func DefaultConfig() *CloudControlConfig {
	return &CloudControlConfig{
		APIEndpoint:       "http://localhost:8080",
		Timeout:           30 * time.Second,
		UseBuiltIn:        true,
		JWTSecretKey:      "your-secret-key",
		JWTExpiration:     DefaultDataTTL,
		RefreshExpiration: 7 * DefaultDataTTL, // 7天
		JWTIssuer:         "tunnox",
	}
}
