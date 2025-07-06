package cloud

import (
	"time"
)

// CloudControlAPI 云控平台接口
type CloudControlAPI interface {
	// 节点管理
	NodeRegister(req *NodeRegisterRequest) (*NodeRegisterResponse, error)
	NodeUnregister(req *NodeUnregisterRequest) error
	NodeHeartbeat(req *NodeHeartbeatRequest) (*NodeHeartbeatResponse, error)

	// 用户认证
	Authenticate(req *AuthRequest) (*AuthResponse, error)
	ValidateToken(token string) (*AuthResponse, error)

	// 用户管理
	CreateUser(username, email string) (*User, error) // 创建用户，服务端分配用户ID
	GetUser(userID string) (*User, error)
	UpdateUser(user *User) error
	DeleteUser(userID string) error
	ListUsers(userType UserType) ([]*User, error)

	// 客户端管理
	CreateClient(userID, clientName string) (*Client, error) // 为指定用户创建客户端
	GetClient(clientID int64) (*Client, error)
	TouchClient(clientID int64)
	UpdateClient(client *Client) error
	DeleteClient(clientID int64) error
	UpdateClientStatus(clientID int64, status ClientStatus, nodeID string) error
	ListClients(userID string, clientType ClientType) ([]*Client, error)
	ListUserClients(userID string) ([]*Client, error)             // 获取用户下的所有客户端
	GetClientPortMappings(clientID int64) ([]*PortMapping, error) // 获取客户端下的所有端口映射

	// 端口映射管理
	CreatePortMapping(mapping *PortMapping) (*PortMapping, error)
	GetUserPortMappings(userID string) ([]*PortMapping, error)
	GetPortMapping(mappingID string) (*PortMapping, error)
	UpdatePortMapping(mapping *PortMapping) error
	DeletePortMapping(mappingID string) error
	UpdatePortMappingStatus(mappingID string, status MappingStatus) error
	UpdatePortMappingStats(mappingID string, stats *TrafficStats) error
	ListPortMappings(mappingType MappingType) ([]*PortMapping, error)

	// 匿名用户管理
	GenerateAnonymousCredentials() (*Client, error) // 生成匿名客户端凭据
	GetAnonymousClient(clientID int64) (*Client, error)
	DeleteAnonymousClient(clientID int64) error
	ListAnonymousClients() ([]*Client, error)
	CreateAnonymousMapping(sourceClientID, targetClientID int64, protocol Protocol, sourcePort, targetPort int) (*PortMapping, error)
	GetAnonymousMappings() ([]*PortMapping, error)
	CleanupExpiredAnonymous() error

	// 节点服务信息
	GetNodeServiceInfo(nodeID string) (*NodeServiceInfo, error)
	GetAllNodeServiceInfo() ([]*NodeServiceInfo, error)

	// 统计和监控接口
	GetUserStats(userID string) (*UserStats, error)                      // 获取用户统计信息
	GetClientStats(clientID int64) (*ClientStats, error)                 // 获取客户端统计信息
	GetSystemStats() (*SystemStats, error)                               // 获取系统整体统计
	GetTrafficStats(timeRange string) ([]*TrafficDataPoint, error)       // 获取流量统计图表数据
	GetConnectionStats(timeRange string) ([]*ConnectionDataPoint, error) // 获取连接数统计图表数据

	// 连接管理接口
	RegisterConnection(mappingID string, connInfo *ConnectionInfo) error
	UnregisterConnection(connID string) error
	GetConnections(mappingID string) ([]*ConnectionInfo, error)
	GetClientConnections(clientID int64) ([]*ConnectionInfo, error)
	UpdateConnectionStats(connID string, bytesSent, bytesReceived int64) error

	// JWT Token管理接口
	GenerateJWTToken(clientID int64) (*JWTTokenInfo, error)
	RefreshJWTToken(refreshToken string) (*JWTTokenInfo, error)
	ValidateJWTToken(token string) (*JWTTokenInfo, error)
	RevokeJWTToken(token string) error

	// 搜索和过滤接口
	SearchUsers(keyword string) ([]*User, error)               // 搜索用户
	SearchClients(keyword string) ([]*Client, error)           // 搜索客户端
	SearchPortMappings(keyword string) ([]*PortMapping, error) // 搜索端口映射

	Close() error
}

// ControlConfig 云控配置
type ControlConfig struct {
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
func DefaultConfig() *ControlConfig {
	return &ControlConfig{
		APIEndpoint:       "http://localhost:8080",
		Timeout:           30 * time.Second,
		UseBuiltIn:        true,
		JWTSecretKey:      "your-secret-key",
		JWTExpiration:     DefaultDataTTL,
		RefreshExpiration: 7 * DefaultDataTTL, // 7天
		JWTIssuer:         "tunnox",
	}
}
