package client

// ClientConfig 客户端配置
type ClientConfig struct {
	// 注册客户端认证
	ClientID  int64  `yaml:"client_id"`
	AuthToken string `yaml:"auth_token"`

	// 匿名客户端认证
	Anonymous bool   `yaml:"anonymous"`
	DeviceID  string `yaml:"device_id"`
	SecretKey string `yaml:"secret_key"` // 匿名客户端的密钥（服务端分配后保存）

	Server struct {
		Address  string `yaml:"address"`  // 服务器地址，例如 "localhost:7000"
		Protocol string `yaml:"protocol"` // tcp/websocket/quic
	} `yaml:"server"`

	// 日志配置
	Log LogConfig `yaml:"log"`

	// 注意：映射配置由服务器通过指令连接动态推送，不在配置文件中
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `yaml:"level"`  // 日志级别：debug, info, warn, error
	Format string `yaml:"format"` // 日志格式：text, json
	Output string `yaml:"output"` // 输出目标：stdout, stderr, file
	File   string `yaml:"file"`   // 日志文件路径（当output=file时）
}
