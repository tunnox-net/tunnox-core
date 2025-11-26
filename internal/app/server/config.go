package server

import (
	"fmt"
	"os"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/utils"

	"gopkg.in/yaml.v3"
)

// ProtocolConfig 协议配置
type ProtocolConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port"`
	Host    string `yaml:"host"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host         string                    `yaml:"host"`
	Port         int                       `yaml:"port"`
	ReadTimeout  int                       `yaml:"read_timeout"`
	WriteTimeout int                       `yaml:"write_timeout"`
	IdleTimeout  int                       `yaml:"idle_timeout"`
	Protocols    map[string]ProtocolConfig `yaml:"protocols"`
}

// CloudConfig 云控配置
type CloudConfig struct {
	Type     string                 `yaml:"type"`
	BuiltIn  map[string]interface{} `yaml:"built_in"`
	External map[string]interface{} `yaml:"external"`
}

// MessageBrokerConfig 消息代理配置
type MessageBrokerConfig struct {
	Type   string                 `yaml:"type"`
	NodeID string                 `yaml:"node_id"`
	Redis  map[string]interface{} `yaml:"redis"`
}

// BridgePoolConfig 桥接连接池配置
type BridgePoolConfig struct {
	Enabled             bool                   `yaml:"enabled"`
	MinConnsPerNode     int32                  `yaml:"min_conns_per_node"`
	MaxConnsPerNode     int32                  `yaml:"max_conns_per_node"`
	MaxIdleTime         int                    `yaml:"max_idle_time"` // 秒
	MaxStreamsPerConn   int32                  `yaml:"max_streams_per_conn"`
	DialTimeout         int                    `yaml:"dial_timeout"`          // 秒
	HealthCheckInterval int                    `yaml:"health_check_interval"` // 秒
	GRPCServer          map[string]interface{} `yaml:"grpc_server"`
}

// ManagementAPIConfig 管理 API 配置
type ManagementAPIConfig struct {
	Enabled    bool                   `yaml:"enabled"`
	ListenAddr string                 `yaml:"listen_addr"`
	Auth       map[string]interface{} `yaml:"auth"`
	CORS       map[string]interface{} `yaml:"cors"`
	RateLimit  map[string]interface{} `yaml:"rate_limit"`
}

// UDPIngressListenerConfig UDP接入监听配置
type UDPIngressListenerConfig struct {
	Name         string `yaml:"name"`
	Address      string `yaml:"address"`
	MappingID    string `yaml:"mapping_id"`
	IdleTimeout  int    `yaml:"idle_timeout"`
	MaxSessions  int    `yaml:"max_sessions"`
	FrameBacklog int    `yaml:"frame_backlog"`
}

// UDPIngressConfig UDP接入总体配置
type UDPIngressConfig struct {
	Enabled   bool                       `yaml:"enabled"`
	Listeners []UDPIngressListenerConfig `yaml:"listeners"`
}

// Config 应用配置
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Log           utils.LogConfig     `yaml:"log"`
	Cloud         CloudConfig         `yaml:"cloud"`
	MessageBroker MessageBrokerConfig `yaml:"message_broker"`
	BridgePool    BridgePoolConfig    `yaml:"bridge_pool"`
	ManagementAPI ManagementAPIConfig `yaml:"management_api"`
	UDPIngress    UDPIngressConfig    `yaml:"udp_ingress"`
}

// LoadConfig 加载配置文件
func LoadConfig(configPath string) (*Config, error) {
	// 如果配置文件不存在，使用默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		utils.Warnf(constants.MsgConfigFileNotFound, configPath)
		return GetDefaultConfig(), nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf(constants.MsgFailedToReadConfigFile, configPath, err)
	}

	// 解析YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf(constants.MsgFailedToParseConfigFile, configPath, err)
	}

	// 验证配置
	if err := ValidateConfig(&config); err != nil {
		return nil, fmt.Errorf(constants.MsgInvalidConfiguration, err)
	}

	utils.Infof(constants.MsgConfigLoadedFrom, configPath)
	return &config, nil
}

// ValidateConfig 验证配置
func ValidateConfig(config *Config) error {
	// 验证服务器配置
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.Server.Port <= 0 {
		config.Server.Port = 8080
	}

	// 验证协议配置
	if config.Server.Protocols == nil {
		config.Server.Protocols = make(map[string]ProtocolConfig)
	}

	// 设置默认协议配置
	defaultProtocols := map[string]ProtocolConfig{
		"tcp": {
			Enabled: true,
			Port:    8080,
			Host:    "0.0.0.0",
		},
		"websocket": {
			Enabled: true,
			Port:    8081,
			Host:    "0.0.0.0",
		},
		"udp": {
			Enabled: true,
			Port:    8082,
			Host:    "0.0.0.0",
		},
		"quic": {
			Enabled: true,
			Port:    8083,
			Host:    "0.0.0.0",
		},
	}

	// 合并默认配置
	for name, defaultConfig := range defaultProtocols {
		if _, exists := config.Server.Protocols[name]; !exists {
			config.Server.Protocols[name] = defaultConfig
		}
	}

	// 验证 UDP Ingress 配置
	if config.UDPIngress.Listeners == nil {
		config.UDPIngress.Listeners = []UDPIngressListenerConfig{}
	}
	for i := range config.UDPIngress.Listeners {
		if config.UDPIngress.Listeners[i].IdleTimeout <= 0 {
			config.UDPIngress.Listeners[i].IdleTimeout = 60
		}
		if config.UDPIngress.Listeners[i].FrameBacklog <= 0 {
			config.UDPIngress.Listeners[i].FrameBacklog = 64
		}
	}

	// 验证日志配置
	if config.Log.Level == "" {
		config.Log.Level = constants.LogLevelInfo
	}
	if config.Log.Format == "" {
		config.Log.Format = constants.LogFormatText
	}
	if config.Log.Output == "" {
		config.Log.Output = constants.LogOutputStdout
	}

	return nil
}

// GetDefaultConfig 获取默认配置
func GetDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         8080,
			ReadTimeout:  30,
			WriteTimeout: 30,
			IdleTimeout:  60,
			Protocols: map[string]ProtocolConfig{
				"tcp": {
					Enabled: true,
					Port:    8080,
					Host:    "0.0.0.0",
				},
				"websocket": {
					Enabled: true,
					Port:    8081,
					Host:    "0.0.0.0",
				},
				"udp": {
					Enabled: true,
					Port:    8082,
					Host:    "0.0.0.0",
				},
				"quic": {
					Enabled: true,
					Port:    8083,
					Host:    "0.0.0.0",
				},
			},
		},
		Log: utils.LogConfig{
			Level:  constants.LogLevelInfo,
			Format: constants.LogFormatText,
			Output: constants.LogOutputStdout,
		},
		Cloud: CloudConfig{
			Type: "built_in",
		},
		UDPIngress: UDPIngressConfig{
			Enabled:   false,
			Listeners: []UDPIngressListenerConfig{},
		},
	}
}
