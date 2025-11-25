package configs

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
