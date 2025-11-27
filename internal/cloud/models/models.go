package models

import (
	"time"
	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/stats"
)

// NodeRegisterRequest 节点注册请求
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

// NodeUnregisterRequest 节点注销请求
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

// NodeServiceInfo 节点服务信息
type NodeServiceInfo struct {
	NodeID  string `json:"node_id"`
	Address string `json:"address"` // 对外服务地址
}

// User 用户信息
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
	UserTypeAnonymous  UserType = "anonymous"  // 匿名用户
)

type UserStatus string

const (
	UserStatusActive    UserStatus = "active"    // 活跃
	UserStatusSuspended UserStatus = "suspended" // 暂停
	UserStatusDeleted   UserStatus = "deleted"   // 已删除
)

type UserPlan string

const (
	UserPlanFree       UserPlan = "free"       // 免费版
	UserPlanPremium    UserPlan = "premium"    // 高级版
	UserPlanEnterprise UserPlan = "enterprise" // 企业版
)

type UserQuota struct {
	MaxClientIDs   int   `json:"max_client_ids"`  // 最大ClientID数量
	MaxConnections int   `json:"max_connections"` // 最大并发连接数
	BandwidthLimit int64 `json:"bandwidth_limit"` // 带宽限制(字节/秒)
	StorageLimit   int64 `json:"storage_limit"`   // 存储限制(字节)
}

// Client类型定义已移至 client.go, client_config.go, client_state.go, client_token.go
// 以实现持久化配置和运行时状态的分离

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

type PortMapping struct {
	ID             string                `json:"id"`               // 映射ID
	UserID         string                `json:"user_id"`          // 所属用户ID（匿名映射可能为空）
	SourceClientID int64                 `json:"source_client_id"` // 源客户端ID
	TargetClientID int64                 `json:"target_client_id"` // 目标客户端ID
	Protocol       Protocol              `json:"protocol"`         // 协议：tcp/udp/http/socks
	SourcePort     int                   `json:"source_port"`      // 源端口
	TargetHost     string                `json:"target_host"`      // 目标主机
	TargetPort     int                   `json:"target_port"`      // 目标端口
	
	// ✅ 映射连接认证
	SecretKey      string                `json:"secret_key"`       // 映射连接固定秘钥（随机生成，用于 TunnelOpen 认证）
	
	Config         configs.MappingConfig `json:"config"`           // 映射配置
	Status         MappingStatus         `json:"status"`           // 映射状态
	CreatedAt      time.Time             `json:"created_at"`       // 创建时间
	UpdatedAt      time.Time             `json:"updated_at"`       // 更新时间
	LastActive     *time.Time            `json:"last_active"`      // 最后活跃时间
	TrafficStats   stats.TrafficStats    `json:"traffic_stats"`    // 流量统计
	Type           MappingType           `json:"type"`             // 映射类型
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

// ConnectionInfo 连接信息
type ConnectionInfo struct {
	ConnID        string    `json:"conn_id"`        // 连接ID
	MappingID     string    `json:"mapping_id"`     // 所属映射ID
	ClientID      int64     `json:"client_id"`      // 所属客户端ID
	SourceIP      string    `json:"source_ip"`      // 源IP地址
	EstablishedAt time.Time `json:"established_at"` // 建立时间
	LastActivity  time.Time `json:"last_activity"`  // 最后活动时间
	UpdatedAt     time.Time `json:"updated_at"`     // 更新时间
	BytesSent     int64     `json:"bytes_sent"`     // 发送字节数
	BytesReceived int64     `json:"bytes_received"` // 接收字节数
	Status        string    `json:"status"`         // 连接状态
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
	ClientID  int64      `json:"client_id"`  // 客户端ID
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
