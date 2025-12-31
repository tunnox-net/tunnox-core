package client

// ClientConfig 客户端配置
// 统一认证模型：首次连接由服务端分配 clientId + secretKey，后续连接使用这两个字段认证
type ClientConfig struct {
	// 认证凭据（服务端分配，首次连接后持久化）
	ClientID  int64  `yaml:"client_id"`  // 客户端唯一标识
	SecretKey string `yaml:"secret_key"` // 认证密钥

	Server struct {
		Address  string `yaml:"address"`  // 服务器地址，例如 "localhost:7000"
		Protocol string `yaml:"protocol"` // tcp/websocket/quic
	} `yaml:"server"`

	// TLS 配置（QUIC 协议使用）
	TLS TLSConfig `yaml:"tls"`

	// 日志配置
	Log LogConfig `yaml:"log"`

	// 注意：映射配置由服务器通过指令连接动态推送，不在配置文件中
}

// TLSConfig TLS 配置
type TLSConfig struct {
	// 是否跳过证书验证（默认 true，适用于自签名证书）
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`

	// CA 证书文件路径（可选，用于验证服务器证书）
	CACertFile string `yaml:"ca_cert_file,omitempty"`

	// 服务器名称（用于证书验证，可选）
	ServerName string `yaml:"server_name,omitempty"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level"`            // 日志级别：debug, info, warn, error
	Format string `yaml:"format"`           // 日志格式：text, json
	Output string `yaml:"output,omitempty"` // 输出目标：由系统根据运行模式自动控制，不保存到配置文件
	File   string `yaml:"file"`             // 日志文件路径
}
