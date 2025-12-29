package configs

import (
	"time"
	"tunnox-core/internal/cloud/constants"
)

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

// DefaultControlConfig 返回默认配置
func DefaultControlConfig() *ControlConfig {
	return &ControlConfig{
		APIEndpoint:       "http://localhost:8080",
		Timeout:           30 * time.Second,
		UseBuiltIn:        true,
		JWTSecretKey:      "your-secret-key",
		JWTExpiration:     constants.DefaultDataTTL,
		RefreshExpiration: 7 * constants.DefaultDataTTL, // 7天
		JWTIssuer:         "tunnox",
	}
}

// ClientConfig 客户端配置
type ClientConfig struct {
	EnableCompression bool  `json:"enable_compression"` // 是否启用压缩
	BandwidthLimit    int64 `json:"bandwidth_limit"`    // 带宽限制(字节/秒)
	MaxConnections    int   `json:"max_connections"`    // 最大连接数
	AllowedPorts      []int `json:"allowed_ports"`      // 允许的端口范围
	BlockedPorts      []int `json:"blocked_ports"`      // 禁止的端口
	AutoReconnect     bool  `json:"auto_reconnect"`     // 自动重连
	HeartbeatInterval int   `json:"heartbeat_interval"` // 心跳间隔(秒)
}

// MappingConfig 端口映射配置
type MappingConfig struct {
	// ✅ 压缩配置（端到端：ClientA ↔ ClientB）
	EnableCompression bool `json:"enable_compression"` // 是否启用压缩
	CompressionLevel  int  `json:"compression_level"`  // 压缩级别 (1-9, 默认 6)

	// ✅ 加密配置（端到端：ClientA ↔ ClientB）
	EnableEncryption bool   `json:"enable_encryption"` // 是否启用加密
	EncryptionMethod string `json:"encryption_method"` // 加密算法：aes-256-gcm, chacha20-poly1305
	EncryptionKey    string `json:"encryption_key"`    // 加密密钥（Base64编码）

	// 其他配置
	BandwidthLimit int64 `json:"bandwidth_limit"` // 带宽限制(字节/秒)
	MaxConnections int   `json:"max_connections"` // 最大连接数
	Timeout        int   `json:"timeout"`         // 超时时间(秒)
	RetryCount     int   `json:"retry_count"`     // 重试次数
	EnableLogging  bool  `json:"enable_logging"`  // 是否启用日志
}

// NodeConfig 节点配置
type NodeConfig struct {
	MaxConnections    int   `json:"max_connections"`    // 最大连接数
	HeartbeatInterval int   `json:"heartbeat_interval"` // 心跳间隔(秒)
	Timeout           int   `json:"timeout"`            // 超时时间(秒)
	EnableCompression bool  `json:"enable_compression"` // 是否启用压缩
	BandwidthLimit    int64 `json:"bandwidth_limit"`    // 带宽限制(字节/秒)
}
