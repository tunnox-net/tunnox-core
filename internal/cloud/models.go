package cloud

import "time"

// NodeRegisterRequest 节点注册请求
// 云控平台收到后分配/确认节点ID
// Address: 节点对外服务地址（如Pod的外部访问地址）
type NodeRegisterRequest struct {
	NodeID  string            `json:"node_id"`        // 节点ID（可选，首次注册可为空）
	Address string            `json:"address"`        // 节点服务地址（IP:Port或域名）
	Version string            `json:"version"`        // 节点版本
	Meta    map[string]string `json:"meta,omitempty"` // 其他元数据
}

// NodeRegisterResponse 节点注册响应
type NodeRegisterResponse struct {
	NodeID  string `json:"node_id"` // 分配的节点ID
	Success bool   `json:"success"` // 是否注册成功
	Message string `json:"message"` // 错误信息
}

// NodeUnregisterRequest 节点反注册请求
type NodeUnregisterRequest struct {
	NodeID string `json:"node_id"`
}

// NodeHeartbeatRequest 节点心跳请求
type NodeHeartbeatRequest struct {
	NodeID  string    `json:"node_id"`
	Address string    `json:"address"` // 当前服务地址
	Time    time.Time `json:"time"`    // 心跳时间
	Version string    `json:"version"`
}

// NodeHeartbeatResponse 节点心跳响应
type NodeHeartbeatResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// NodeServiceInfo 节点服务信息（用于集群间转发）
type NodeServiceInfo struct {
	NodeID  string `json:"node_id"`
	Address string `json:"address"` // 对外服务地址
}

type User struct {
	ID        string     `json:"id"`         // 用户唯一标识
	Username  string     `json:"username"`   // 用户名
	Email     string     `json:"email"`      // 邮箱
	Status    UserStatus `json:"status"`     // 用户状态：active/suspended/deleted
	Type      UserType   `json:"type"`       // 用户类型：registered/anonymous
	CreatedAt time.Time  `json:"created_at"` // 创建时间
	UpdatedAt time.Time  `json:"updated_at"` // 更新时间
	Plan      UserPlan   `json:"plan"`       // 用户套餐：free/premium/enterprise
	Quota     UserQuota  `json:"quota"`      // 用户配额
}

type UserType string

const (
	UserTypeRegistered UserType = "registered" // 注册用户
	UserTypeAnonymous  UserType = "anonymous"  // 匿名用户（类似TeamViewer）
)

type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusDeleted   UserStatus = "deleted"
)

type UserPlan string

const (
	UserPlanFree       UserPlan = "free"
	UserPlanPremium    UserPlan = "premium"
	UserPlanEnterprise UserPlan = "enterprise"
)

type UserQuota struct {
	MaxClientIds   int   `json:"max_client_ids"`  // 最大ClientId数量
	MaxConnections int   `json:"max_connections"` // 最大并发连接数
	BandwidthLimit int64 `json:"bandwidth_limit"` // 带宽限制(字节/秒)
	StorageLimit   int64 `json:"storage_limit"`   // 存储限制(字节)
}

type Client struct {
	ID        string       `json:"id"`         // ClientId
	UserID    string       `json:"user_id"`    // 所属用户ID（匿名用户可能为空）
	Name      string       `json:"name"`       // 客户端名称
	AuthCode  string       `json:"auth_code"`  // 认证码
	SecretKey string       `json:"secret_key"` // 密钥(开发用)
	Status    ClientStatus `json:"status"`     // 客户端状态
	Config    ClientConfig `json:"config"`     // 客户端配置
	LastSeen  *time.Time   `json:"last_seen"`  // 最后在线时间
	CreatedAt time.Time    `json:"created_at"` // 创建时间
	UpdatedAt time.Time    `json:"updated_at"` // 更新时间
	NodeID    string       `json:"node_id"`    // 连接的节点ID
	IPAddress string       `json:"ip_address"` // 客户端IP地址
	Version   string       `json:"version"`    // 客户端版本
	Type      ClientType   `json:"type"`       // 客户端类型
}

type ClientType string

const (
	ClientTypeRegistered ClientType = "registered" // 注册用户的客户端
	ClientTypeAnonymous  ClientType = "anonymous"  // 匿名客户端（无需注册）
)

type ClientStatus string

const (
	ClientStatusOffline ClientStatus = "offline"
	ClientStatusOnline  ClientStatus = "online"
	ClientStatusBlocked ClientStatus = "blocked"
)

type ClientConfig struct {
	EnableCompression bool  `json:"enable_compression"` // 是否启用压缩
	BandwidthLimit    int64 `json:"bandwidth_limit"`    // 带宽限制(字节/秒)
	MaxConnections    int   `json:"max_connections"`    // 最大连接数
	AllowedPorts      []int `json:"allowed_ports"`      // 允许的端口范围
	BlockedPorts      []int `json:"blocked_ports"`      // 禁止的端口
	AutoReconnect     bool  `json:"auto_reconnect"`     // 自动重连
	HeartbeatInterval int   `json:"heartbeat_interval"` // 心跳间隔(秒)
}

type PortMapping struct {
	ID             string        `json:"id"`               // 映射ID
	UserID         string        `json:"user_id"`          // 所属用户ID（匿名映射可能为空）
	SourceClientID string        `json:"source_client_id"` // 源客户端ID
	TargetClientID string        `json:"target_client_id"` // 目标客户端ID
	Protocol       Protocol      `json:"protocol"`         // 协议：tcp/udp/http/socks
	SourcePort     int           `json:"source_port"`      // 源端口
	TargetHost     string        `json:"target_host"`      // 目标主机
	TargetPort     int           `json:"target_port"`      // 目标端口
	Status         MappingStatus `json:"status"`           // 映射状态
	Config         MappingConfig `json:"config"`           // 映射配置
	CreatedAt      time.Time     `json:"created_at"`       // 创建时间
	UpdatedAt      time.Time     `json:"updated_at"`       // 更新时间
	LastActive     *time.Time    `json:"last_active"`      // 最后活跃时间
	TrafficStats   TrafficStats  `json:"traffic_stats"`    // 流量统计
	Type           MappingType   `json:"type"`             // 映射类型
}

type MappingType string

const (
	MappingTypeRegistered MappingType = "registered" // 注册用户的映射
	MappingTypeAnonymous  MappingType = "anonymous"  // 匿名映射（无需注册）
)

type Protocol string

const (
	ProtocolTCP   Protocol = "tcp"
	ProtocolUDP   Protocol = "udp"
	ProtocolHTTP  Protocol = "http"
	ProtocolSOCKS Protocol = "socks"
)

type MappingStatus string

const (
	MappingStatusActive   MappingStatus = "active"
	MappingStatusInactive MappingStatus = "inactive"
	MappingStatusError    MappingStatus = "error"
)

type MappingConfig struct {
	EnableCompression bool  `json:"enable_compression"` // 是否启用压缩
	BandwidthLimit    int64 `json:"bandwidth_limit"`    // 带宽限制
	Timeout           int   `json:"timeout"`            // 超时时间(秒)
	RetryCount        int   `json:"retry_count"`        // 重试次数
}

type TrafficStats struct {
	BytesSent     int64 `json:"bytes_sent"`     // 发送字节数
	BytesReceived int64 `json:"bytes_received"` // 接收字节数
	Connections   int64 `json:"connections"`    // 连接数
}

type Node struct {
	ID        string            `json:"id"`             // 节点ID
	Name      string            `json:"name"`           // 节点名称
	Address   string            `json:"address"`        // 节点服务地址（IP:Port或域名）
	Meta      map[string]string `json:"meta,omitempty"` // 其他元数据
	CreatedAt time.Time         `json:"created_at"`     // 创建时间
	UpdatedAt time.Time         `json:"updated_at"`     // 更新时间
}

type AuthRequest struct {
	ClientID  string     `json:"client_id"`  // 客户端ID
	AuthCode  string     `json:"auth_code"`  // 认证码
	SecretKey string     `json:"secret_key"` // 密钥(可选)
	NodeID    string     `json:"node_id"`    // 节点ID
	Version   string     `json:"version"`    // 客户端版本
	IPAddress string     `json:"ip_address"` // IP地址
	Type      ClientType `json:"type"`       // 客户端类型（registered/anonymous）
}

type AuthResponse struct {
	Success   bool      `json:"success"`    // 认证是否成功
	Token     string    `json:"token"`      // 认证令牌
	Client    *Client   `json:"client"`     // 客户端信息
	Node      *Node     `json:"node"`       // 节点信息
	ExpiresAt time.Time `json:"expires_at"` // 令牌过期时间
	Message   string    `json:"message"`    // 错误消息
}

// 统计相关数据结构
type UserStats struct {
	UserID           string    `json:"user_id"`
	TotalClients     int       `json:"total_clients"`
	OnlineClients    int       `json:"online_clients"`
	TotalMappings    int       `json:"total_mappings"`
	ActiveMappings   int       `json:"active_mappings"`
	TotalTraffic     int64     `json:"total_traffic"`     // 总流量(字节)
	TotalConnections int64     `json:"total_connections"` // 总连接数
	LastActive       time.Time `json:"last_active"`
}

type ClientStats struct {
	ClientID         string    `json:"client_id"`
	UserID           string    `json:"user_id"`
	TotalMappings    int       `json:"total_mappings"`
	ActiveMappings   int       `json:"active_mappings"`
	TotalTraffic     int64     `json:"total_traffic"`     // 总流量(字节)
	TotalConnections int64     `json:"total_connections"` // 总连接数
	Uptime           int64     `json:"uptime"`            // 在线时长(秒)
	LastSeen         time.Time `json:"last_seen"`
}

type SystemStats struct {
	TotalUsers       int   `json:"total_users"`
	TotalClients     int   `json:"total_clients"`
	OnlineClients    int   `json:"online_clients"`
	TotalMappings    int   `json:"total_mappings"`
	ActiveMappings   int   `json:"active_mappings"`
	TotalNodes       int   `json:"total_nodes"`
	OnlineNodes      int   `json:"online_nodes"`
	TotalTraffic     int64 `json:"total_traffic"`     // 总流量(字节)
	TotalConnections int64 `json:"total_connections"` // 总连接数
	AnonymousUsers   int   `json:"anonymous_users"`   // 匿名用户数
}

type TrafficDataPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	BytesSent     int64     `json:"bytes_sent"`
	BytesReceived int64     `json:"bytes_received"`
	UserID        string    `json:"user_id,omitempty"`
	ClientID      string    `json:"client_id,omitempty"`
}

type ConnectionDataPoint struct {
	Timestamp   time.Time `json:"timestamp"`
	Connections int       `json:"connections"`
	UserID      string    `json:"user_id,omitempty"`
	ClientID    string    `json:"client_id,omitempty"`
}
