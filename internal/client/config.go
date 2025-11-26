package client

import (
	"tunnox-core/internal/config"
)

// MappingConfig is an alias for config.MappingConfig for backward compatibility
type MappingConfig = config.MappingConfig

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
