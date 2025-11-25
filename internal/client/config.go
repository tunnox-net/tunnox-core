package client

// ClientConfig 客户端配置
type ClientConfig struct {
	// 注册客户端认证
	ClientID  int64  `yaml:"client_id"`
	AuthToken string `yaml:"auth_token"`

	// 匿名客户端认证
	Anonymous bool   `yaml:"anonymous"`
	DeviceID  string `yaml:"device_id"`

	Server struct {
		Address  string `yaml:"address"`  // 服务器地址，例如 "localhost:7000"
		Protocol string `yaml:"protocol"` // tcp/websocket/quic
	} `yaml:"server"`
	// 注意：映射配置由服务器通过指令连接动态推送，不在配置文件中
}

// MappingConfig 映射配置
type MappingConfig struct {
	MappingID  string `yaml:"mapping_id"`
	SecretKey  string `yaml:"secret_key"`
	Protocol   string `yaml:"protocol"` // tcp/udp/socks5
	LocalPort  int    `yaml:"local_port"`
	TargetHost string `yaml:"target_host"`
	TargetPort int    `yaml:"target_port"`

	// ✅ 压缩、加密配置（从服务器推送）
	EnableCompression bool   `json:"enable_compression"`
	CompressionLevel  int    `json:"compression_level"`
	EnableEncryption  bool   `json:"enable_encryption"`
	EncryptionMethod  string `json:"encryption_method"`
	EncryptionKey     string `json:"encryption_key"`
}

