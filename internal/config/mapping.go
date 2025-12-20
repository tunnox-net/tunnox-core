package config

// MappingConfig 映射配置（客户端和服务端共享）
type MappingConfig struct {
	MappingID  string `json:"mapping_id" yaml:"mapping_id"`
	SecretKey  string `json:"secret_key" yaml:"secret_key"`
	Protocol   string `json:"protocol" yaml:"protocol"` // tcp/socks5
	LocalPort  int    `json:"local_port" yaml:"local_port"`
	TargetHost string `json:"target_host" yaml:"target_host"`
	TargetPort int    `json:"target_port" yaml:"target_port"`

	// SOCKS5 代理配置（仅 Protocol=socks5 时使用）
	TargetClientID int64 `json:"target_client_id,omitempty" yaml:"target_client_id"` // 出口客户端ID

	// 🔒 商业化控制配置
	BandwidthLimit int64 `json:"bandwidth_limit" yaml:"bandwidth_limit"` // bytes/s, 0=无限制
	MaxConnections int   `json:"max_connections" yaml:"max_connections"` // 最大并发连接数, 0=使用用户配额

	// 压缩和加密配置
	EnableCompression bool   `json:"enable_compression" yaml:"enable_compression"`
	CompressionLevel  int    `json:"compression_level" yaml:"compression_level"` // 1-9
	EnableEncryption  bool   `json:"enable_encryption" yaml:"enable_encryption"`
	EncryptionMethod  string `json:"encryption_method" yaml:"encryption_method"` // aes-256-gcm
	EncryptionKey     string `json:"encryption_key" yaml:"encryption_key"`       // base64 encoded
}
