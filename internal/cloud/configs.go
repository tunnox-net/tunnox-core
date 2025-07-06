package cloud

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
	EnableCompression bool  `json:"enable_compression"` // 是否启用压缩
	BandwidthLimit    int64 `json:"bandwidth_limit"`    // 带宽限制(字节/秒)
	MaxConnections    int   `json:"max_connections"`    // 最大连接数
	Timeout           int   `json:"timeout"`            // 超时时间(秒)
	RetryCount        int   `json:"retry_count"`        // 重试次数
	EnableLogging     bool  `json:"enable_logging"`     // 是否启用日志
}

// NodeConfig 节点配置
type NodeConfig struct {
	MaxConnections    int   `json:"max_connections"`    // 最大连接数
	HeartbeatInterval int   `json:"heartbeat_interval"` // 心跳间隔(秒)
	Timeout           int   `json:"timeout"`            // 超时时间(秒)
	EnableCompression bool  `json:"enable_compression"` // 是否启用压缩
	BandwidthLimit    int64 `json:"bandwidth_limit"`    // 带宽限制(字节/秒)
}
