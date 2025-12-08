package server

import (
	"fmt"
	"os"
	"path/filepath"
	"tunnox-core/internal/constants"
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/validation"
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
	Type     string              `yaml:"type"`
	BuiltIn  BuiltInCloudConfig  `yaml:"built_in"`
	External ExternalCloudConfig `yaml:"external"`
}

// BuiltInCloudConfig 内置云控配置
type BuiltInCloudConfig struct {
	Enabled bool `yaml:"enabled"`
}

// ExternalCloudConfig 外部云控配置
type ExternalCloudConfig struct {
	Endpoint string `yaml:"endpoint"`
	APIKey   string `yaml:"api_key"`
	Timeout  int    `yaml:"timeout"` // 秒
}

// MessageBrokerConfig 消息代理配置
type MessageBrokerConfig struct {
	Type   string               `yaml:"type"`
	NodeID string               `yaml:"node_id"`
	Redis  RedisBrokerConfig    `yaml:"redis"`
	Rabbit RabbitMQBrokerConfig `yaml:"rabbitmq"`
	Kafka  KafkaBrokerConfig    `yaml:"kafka"`
}

// RedisBrokerConfig Redis 消息队列配置
type RedisBrokerConfig struct {
	Addr        string `yaml:"addr"`
	Password    string `yaml:"password"`
	DB          int    `yaml:"db"`
	Channel     string `yaml:"channel"`
	PoolSize    int    `yaml:"pool_size"`
	ClusterMode bool   `yaml:"cluster_mode"`
}

// RabbitMQBrokerConfig RabbitMQ 消息队列配置
type RabbitMQBrokerConfig struct {
	URL          string `yaml:"url"`
	Exchange     string `yaml:"exchange"`
	ExchangeType string `yaml:"exchange_type"`
	RoutingKey   string `yaml:"routing_key"`
}

// KafkaBrokerConfig Kafka 消息队列配置
type KafkaBrokerConfig struct {
	Brokers []string `yaml:"brokers"`
	Topic   string   `yaml:"topic"`
	GroupID string   `yaml:"group_id"`
}

// BridgePoolConfig 桥接连接池配置
type BridgePoolConfig struct {
	Enabled             bool             `yaml:"enabled"`
	MinConnsPerNode     int32            `yaml:"min_conns_per_node"`
	MaxConnsPerNode     int32            `yaml:"max_conns_per_node"`
	MaxIdleTime         int              `yaml:"max_idle_time"` // 秒
	MaxStreamsPerConn   int32            `yaml:"max_streams_per_conn"`
	DialTimeout         int              `yaml:"dial_timeout"`          // 秒
	HealthCheckInterval int              `yaml:"health_check_interval"` // 秒
	GRPCServer          GRPCServerConfig `yaml:"grpc_server"`
}

// GRPCServerConfig gRPC 服务器配置
type GRPCServerConfig struct {
	Addr             string        `yaml:"addr"`
	Port             int           `yaml:"port"`
	EnableTLS        bool          `yaml:"enable_tls"`
	TLS              TLSConfigYAML `yaml:"tls"`
	MaxRecvMsgSize   int           `yaml:"max_recv_msg_size"` // MB
	MaxSendMsgSize   int           `yaml:"max_send_msg_size"` // MB
	KeepaliveTime    int           `yaml:"keepalive_time"`    // 秒
	KeepaliveTimeout int           `yaml:"keepalive_timeout"` // 秒
}

// ManagementAPIConfig 管理 API 配置
type ManagementAPIConfig struct {
	Enabled    bool            `yaml:"enabled"`
	ListenAddr string          `yaml:"listen_addr"`
	Auth       AuthConfig      `yaml:"auth"`
	CORS       CORSConfig      `yaml:"cors"`
	RateLimit  RateLimitConfig `yaml:"rate_limit"`
	PProf      PProfConfig     `yaml:"pprof"`
}

// PProfConfig PProf 性能分析配置
type PProfConfig struct {
	Enabled     bool   `yaml:"enabled"`      // 是否启用 pprof
	DataDir     string `yaml:"data_dir"`     // pprof 数据保存目录
	Retention   int    `yaml:"retention"`    // 保留分钟数（默认10分钟）
	AutoCapture bool   `yaml:"auto_capture"` // 是否自动抓取（默认true）
}

// AuthConfig 认证配置
type AuthConfig struct {
	Type   string `yaml:"type"`    // bearer | basic | none
	Token  string `yaml:"token"`   // Bearer token
	APIKey string `yaml:"api_key"` // API key
}

// CORSConfig 跨域配置
type CORSConfig struct {
	Enabled          bool     `yaml:"enabled"`
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
}

// RateLimitConfig 速率限制配置
type RateLimitConfig struct {
	Enabled bool `yaml:"enabled"`
	RPS     int  `yaml:"rps"`   // 每秒请求数
	Burst   int  `yaml:"burst"` // 突发容量
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

// MetricsConfig Metrics 配置
type MetricsConfig struct {
	Type string `yaml:"type"` // memory | prometheus
}

// StorageConfig 存储配置
type StorageConfig struct {
	Type   string                  `yaml:"type"`   // memory | redis | hybrid
	Redis  RedisStorageConfig      `yaml:"redis"`  // Redis存储配置
	Hybrid HybridStorageConfigYAML `yaml:"hybrid"` // 混合存储配置
}

// RedisStorageConfig Redis存储配置
type RedisStorageConfig struct {
	Addr         string `yaml:"addr"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	MaxRetries   int    `yaml:"max_retries"`
	DialTimeout  int    `yaml:"dial_timeout"`  // 秒
	ReadTimeout  int    `yaml:"read_timeout"`  // 秒
	WriteTimeout int    `yaml:"write_timeout"` // 秒
}

// HybridStorageConfigYAML 混合存储YAML配置
type HybridStorageConfigYAML struct {
	CacheType        string                  `yaml:"cache_type"`        // memory | redis
	EnablePersistent bool                    `yaml:"enable_persistent"` // 是否启用持久化
	JSON             JSONStorageConfigYAML   `yaml:"json"`             // JSON 文件存储配置（优先）
	Remote           RemoteStorageConfigYAML `yaml:"remote"`            // 远程存储配置
}

// JSONStorageConfigYAML JSON 文件存储配置
type JSONStorageConfigYAML struct {
	FilePath     string `yaml:"file_path"`     // JSON 文件路径
	AutoSave     bool   `yaml:"auto_save"`     // 是否自动保存
	SaveInterval int    `yaml:"save_interval"` // 自动保存间隔（秒）
}

// RemoteStorageConfigYAML 远程存储YAML配置
type RemoteStorageConfigYAML struct {
	Type string                `yaml:"type"` // grpc | http
	GRPC GRPCStorageConfigYAML `yaml:"grpc"`
	HTTP HTTPStorageConfigYAML `yaml:"http"`
}

// GRPCStorageConfigYAML gRPC存储YAML配置
type GRPCStorageConfigYAML struct {
	Address    string        `yaml:"address"`
	Timeout    int           `yaml:"timeout"` // 秒
	MaxRetries int           `yaml:"max_retries"`
	TLS        TLSConfigYAML `yaml:"tls"`
}

// HTTPStorageConfigYAML HTTP存储YAML配置
type HTTPStorageConfigYAML struct {
	BaseURL    string         `yaml:"base_url"`
	Timeout    int            `yaml:"timeout"` // 秒
	MaxRetries int            `yaml:"max_retries"`
	Auth       AuthConfigYAML `yaml:"auth"`
}

// TLSConfigYAML TLS配置
type TLSConfigYAML struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

// AuthConfigYAML 认证配置
type AuthConfigYAML struct {
	Type  string `yaml:"type"` // bearer | basic | none
	Token string `yaml:"token"`
}

// Config 应用配置
type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Storage       StorageConfig       `yaml:"storage"` // 存储配置
	Log           utils.LogConfig     `yaml:"log"`
	Cloud         CloudConfig         `yaml:"cloud"`
	MessageBroker MessageBrokerConfig `yaml:"message_broker"`
	BridgePool    BridgePoolConfig    `yaml:"bridge_pool"`
	ManagementAPI ManagementAPIConfig `yaml:"management_api"`
	UDPIngress    UDPIngressConfig    `yaml:"udp_ingress"`
	Metrics       MetricsConfig       `yaml:"metrics"`
}

// LoadConfig 加载配置文件
func LoadConfig(configPath string) (*Config, error) {
	// 如果配置文件不存在，使用默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		utils.Warnf(constants.MsgConfigFileNotFound, configPath)
		config := GetDefaultConfig()
		// ✅ 应用环境变量覆盖（即使没有配置文件）
		ApplyEnvOverrides(config)
		return config, nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, constants.MsgFailedToReadConfigFile, configPath)
	}

	// 解析YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, constants.MsgFailedToParseConfigFile, configPath)
	}

	// ✅ 应用环境变量覆盖（环境变量优先级高于配置文件）
	ApplyEnvOverrides(&config)

	// 确保日志输出到文件（不输出到console）
	if config.Log.Output == "" || config.Log.Output == constants.LogOutputStdout || config.Log.Output == constants.LogOutputStderr {
		config.Log.Output = constants.LogOutputFile
		if config.Log.File == "" {
			// 使用默认路径
			config.Log.File = utils.GetDefaultServerLogPath()
		} else {
			// 展开路径（支持 ~ 和相对路径）
			expandedPath, err := utils.ExpandPath(config.Log.File)
			if err != nil {
				return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to expand log file path %q", config.Log.File)
			}
			config.Log.File = expandedPath
		}

		// 确保日志目录存在
		logDir := filepath.Dir(config.Log.File)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to create log directory %q", logDir)
		}
	}

	// 验证配置
	if err := ValidateConfig(&config); err != nil {
		return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, constants.MsgInvalidConfiguration)
	}

	utils.Infof(constants.MsgConfigLoadedFrom, configPath)
	return &config, nil
}

// ValidateConfig 验证配置
func ValidateConfig(config *Config) error {
	result := &validation.ValidationResult{}

	// 验证存储配置
	if err := validateStorageConfig(&config.Storage); err != nil {
		result.AddError(coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "invalid storage config"))
	}

	// 验证服务器配置
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	} else {
		if err := validation.ValidateHost(config.Server.Host, "server.host"); err != nil {
			result.AddError(err)
		}
	}
	if config.Server.Port <= 0 {
		config.Server.Port = 8000
	} else {
		if err := validation.ValidatePort(config.Server.Port, "server.port"); err != nil {
			result.AddError(err)
		}
	}

	// 验证超时配置
	if config.Server.ReadTimeout < 0 {
		result.AddError(validation.ValidateTimeout(config.Server.ReadTimeout, "server.read_timeout"))
	}
	if config.Server.WriteTimeout < 0 {
		result.AddError(validation.ValidateTimeout(config.Server.WriteTimeout, "server.write_timeout"))
	}
	if config.Server.IdleTimeout < 0 {
		result.AddError(validation.ValidateTimeout(config.Server.IdleTimeout, "server.idle_timeout"))
	}

	// 验证协议配置
	if config.Server.Protocols == nil {
		config.Server.Protocols = make(map[string]ProtocolConfig)
	}

	// 设置默认协议配置
	defaultProtocols := map[string]ProtocolConfig{
		"tcp": {
			Enabled: true,
			Port:    8000,
			Host:    "0.0.0.0",
		},
		"websocket": {
			Enabled: true,
			Port:    8443,
			Host:    "0.0.0.0",
		},
		"udp": {
			Enabled: true,
			Port:    8000,
			Host:    "0.0.0.0",
		},
		"quic": {
			Enabled: true,
			Port:    443,
			Host:    "0.0.0.0",
		},
	}

	// 合并默认配置（智能合并：如果用户配置了协议但某些字段缺失，使用默认值填充）
	for name, defaultConfig := range defaultProtocols {
		if userConfig, exists := config.Server.Protocols[name]; exists {
			// 用户已配置该协议，合并缺失的字段
			if userConfig.Port <= 0 {
				userConfig.Port = defaultConfig.Port
			}
			if userConfig.Host == "" {
				userConfig.Host = defaultConfig.Host
			}
			// 如果用户没有明确设置 Enabled，保持默认值（但通常用户会设置）
			config.Server.Protocols[name] = userConfig
		} else {
			// 用户未配置该协议，使用默认配置
			config.Server.Protocols[name] = defaultConfig
		}
	}

	// ============================================================================
	// Redis 自动共享逻辑
	// ============================================================================

	// 规则 1: 如果 storage.redis 已配置，但 message_broker 未配置或为 memory
	//        自动使用 Redis 作为消息队列
	if config.Storage.Redis.Addr != "" {
		if config.MessageBroker.Type == "" || config.MessageBroker.Type == "memory" {
			config.MessageBroker.Type = "redis"
			config.MessageBroker.Redis.Addr = config.Storage.Redis.Addr
			config.MessageBroker.Redis.Password = config.Storage.Redis.Password
			config.MessageBroker.Redis.DB = config.Storage.Redis.DB
			if config.MessageBroker.Redis.Channel == "" {
				config.MessageBroker.Redis.Channel = "tunnox:messages"
			}
			if config.MessageBroker.Redis.PoolSize <= 0 {
				config.MessageBroker.Redis.PoolSize = 10
			}
		}
	}

	// 规则 2: 如果 message_broker.redis 已配置，但 storage.redis 未配置
	//        自动使用 message_broker 的 Redis 配置给 storage
	if config.MessageBroker.Type == "redis" && config.MessageBroker.Redis.Addr != "" {
		if config.Storage.Redis.Addr == "" {
			config.Storage.Redis.Addr = config.MessageBroker.Redis.Addr
			config.Storage.Redis.Password = config.MessageBroker.Redis.Password
			config.Storage.Redis.DB = config.MessageBroker.Redis.DB

			// 设置默认值
			if config.Storage.Redis.PoolSize <= 0 {
				config.Storage.Redis.PoolSize = 10
			}
		}
	}

	// 验证 MessageBroker 配置
	if config.MessageBroker.Type == "" {
		config.MessageBroker.Type = "memory"
	} else {
		validTypes := []string{"memory", "redis", "rabbitmq", "kafka"}
		if err := validation.ValidateStringInList(config.MessageBroker.Type, "message_broker.type", validTypes); err != nil {
			result.AddError(err)
		}
	}
	if config.MessageBroker.NodeID == "" {
		config.MessageBroker.NodeID = "node-001"
	} else {
		if err := validation.ValidateNonEmptyString(config.MessageBroker.NodeID, "message_broker.node_id"); err != nil {
			result.AddError(err)
		}
	}

	// 验证 Redis Broker 配置
	if config.MessageBroker.Type == "redis" && config.MessageBroker.Redis.Addr != "" {
		if err := validation.ValidateAddress(config.MessageBroker.Redis.Addr, "message_broker.redis.addr"); err != nil {
			result.AddError(err)
		}
		if config.MessageBroker.Redis.PoolSize > 0 {
			if err := validation.ValidatePositiveInt(config.MessageBroker.Redis.PoolSize, "message_broker.redis.pool_size"); err != nil {
				result.AddError(err)
			}
		}
	}

	// 验证 Cloud 配置
	if config.Cloud.Type == "" {
		config.Cloud.Type = "built_in"
	} else {
		validTypes := []string{"built_in", "external"}
		if err := validation.ValidateStringInList(config.Cloud.Type, "cloud.type", validTypes); err != nil {
			result.AddError(err)
		}
	}

	// 验证 External Cloud 配置
	if config.Cloud.Type == "external" {
		if config.Cloud.External.Endpoint != "" {
			if err := validation.ValidateURL(config.Cloud.External.Endpoint, "cloud.external.endpoint"); err != nil {
				result.AddError(err)
			}
		}
		if config.Cloud.External.Timeout > 0 {
			if err := validation.ValidateTimeout(config.Cloud.External.Timeout, "cloud.external.timeout"); err != nil {
				result.AddError(err)
			}
		}
	}

	// 验证 ManagementAPI 配置
	if config.ManagementAPI.ListenAddr == "" {
		config.ManagementAPI.ListenAddr = "0.0.0.0:9000"
	} else {
		if err := validation.ValidateAddress(config.ManagementAPI.ListenAddr, "management_api.listen_addr"); err != nil {
			result.AddError(err)
		}
	}
	// 默认启用 ManagementAPI
	if !config.ManagementAPI.Enabled {
		config.ManagementAPI.Enabled = true
	}

	// 验证 BridgePool 配置
	if config.BridgePool.Enabled {
		if config.BridgePool.MinConnsPerNode < 0 {
			result.AddError(validation.ValidateNonNegativeInt(int(config.BridgePool.MinConnsPerNode), "bridge_pool.min_conns_per_node"))
		}
		if config.BridgePool.MaxConnsPerNode > 0 {
			if err := validation.ValidatePositiveInt(int(config.BridgePool.MaxConnsPerNode), "bridge_pool.max_conns_per_node"); err != nil {
				result.AddError(err)
			}
			// 验证 MaxConnsPerNode >= MinConnsPerNode
			if config.BridgePool.MaxConnsPerNode < config.BridgePool.MinConnsPerNode {
				result.AddError(coreErrors.Newf(coreErrors.ErrorTypePermanent, "bridge_pool.max_conns_per_node (%d) must be >= min_conns_per_node (%d)", config.BridgePool.MaxConnsPerNode, config.BridgePool.MinConnsPerNode))
			}
		}
		if config.BridgePool.MaxIdleTime > 0 {
			if err := validation.ValidateTimeout(config.BridgePool.MaxIdleTime, "bridge_pool.max_idle_time"); err != nil {
				result.AddError(err)
			}
		}
		if config.BridgePool.MaxStreamsPerConn > 0 {
			if err := validation.ValidatePositiveInt(int(config.BridgePool.MaxStreamsPerConn), "bridge_pool.max_streams_per_conn"); err != nil {
				result.AddError(err)
			}
		}
		if config.BridgePool.DialTimeout > 0 {
			if err := validation.ValidateTimeout(config.BridgePool.DialTimeout, "bridge_pool.dial_timeout"); err != nil {
				result.AddError(err)
			}
		}
		if config.BridgePool.HealthCheckInterval > 0 {
			if err := validation.ValidateTimeout(config.BridgePool.HealthCheckInterval, "bridge_pool.health_check_interval"); err != nil {
				result.AddError(err)
			}
		}
		// 验证 gRPC Server 配置
		if config.BridgePool.GRPCServer.Port > 0 {
			if err := validation.ValidatePort(config.BridgePool.GRPCServer.Port, "bridge_pool.grpc_server.port"); err != nil {
				result.AddError(err)
			}
		}
		if config.BridgePool.GRPCServer.Addr != "" {
			if err := validation.ValidateHost(config.BridgePool.GRPCServer.Addr, "bridge_pool.grpc_server.addr"); err != nil {
				result.AddError(err)
			}
		}
	}

	// 验证 UDP Ingress 配置
	if config.UDPIngress.Listeners == nil {
		config.UDPIngress.Listeners = []UDPIngressListenerConfig{}
	}
	for i := range config.UDPIngress.Listeners {
		listener := &config.UDPIngress.Listeners[i]
		if listener.IdleTimeout <= 0 {
			listener.IdleTimeout = 60
		} else {
			if err := validation.ValidateTimeout(listener.IdleTimeout, fmt.Sprintf("udp_ingress.listeners[%d].idle_timeout", i)); err != nil {
				result.AddError(err)
			}
		}
		if listener.FrameBacklog <= 0 {
			listener.FrameBacklog = 64
		} else {
			if err := validation.ValidatePositiveInt(listener.FrameBacklog, fmt.Sprintf("udp_ingress.listeners[%d].frame_backlog", i)); err != nil {
				result.AddError(err)
			}
		}
		// 验证监听地址
		if listener.Address != "" {
			if err := validation.ValidateAddress(listener.Address, fmt.Sprintf("udp_ingress.listeners[%d].address", i)); err != nil {
				result.AddError(err)
			}
		}
	}

	// 验证日志配置
	if config.Log.Level == "" {
		config.Log.Level = constants.LogLevelInfo
	} else {
		validLevels := []string{constants.LogLevelDebug, constants.LogLevelInfo, constants.LogLevelWarn, constants.LogLevelError, constants.LogLevelFatal, constants.LogLevelPanic}
		if err := validation.ValidateStringInList(config.Log.Level, "log.level", validLevels); err != nil {
			result.AddError(err)
		}
	}
	if config.Log.Format == "" {
		config.Log.Format = constants.LogFormatText
	} else {
		validFormats := []string{constants.LogFormatText, constants.LogFormatJSON}
		if err := validation.ValidateStringInList(config.Log.Format, "log.format", validFormats); err != nil {
			result.AddError(err)
		}
	}
	if config.Log.Output == "" {
		config.Log.Output = constants.LogOutputStdout
	} else {
		validOutputs := []string{constants.LogOutputStdout, constants.LogOutputStderr, constants.LogOutputFile}
		if err := validation.ValidateStringInList(config.Log.Output, "log.output", validOutputs); err != nil {
			result.AddError(err)
		}
	}

	// 返回验证结果
	if !result.IsValid() {
		return result
	}
	return nil
}

// validateStorageConfig 验证存储配置
func validateStorageConfig(config *StorageConfig) error {
	result := &validation.ValidationResult{}

	// 如果未配置，使用默认值
	if config.Type == "" {
		config.Type = "hybrid"
	}

	// 验证存储类型
	validTypes := []string{"memory", "redis", "hybrid"}
	if err := validation.ValidateStringInList(config.Type, "storage.type", validTypes); err != nil {
		result.AddError(err)
	}

	// 如果是 Redis，验证 Redis 配置
	if config.Type == "redis" {
		if config.Redis.Addr == "" {
			result.AddError(coreErrors.New(coreErrors.ErrorTypePermanent, "storage.redis.addr is required when storage type is redis"))
		} else {
			// 验证地址格式（host:port）
			if err := validation.ValidateAddress(config.Redis.Addr, "storage.redis.addr"); err != nil {
				result.AddError(err)
			}
		}
		// 设置默认值并验证
		if config.Redis.PoolSize <= 0 {
			config.Redis.PoolSize = 10
		} else {
			if err := validation.ValidatePositiveInt(config.Redis.PoolSize, "storage.redis.pool_size"); err != nil {
				result.AddError(err)
			}
		}
		if config.Redis.MaxRetries <= 0 {
			config.Redis.MaxRetries = 3
		} else {
			if err := validation.ValidateNonNegativeInt(config.Redis.MaxRetries, "storage.redis.max_retries"); err != nil {
				result.AddError(err)
			}
		}
		if config.Redis.DialTimeout <= 0 {
			config.Redis.DialTimeout = 5
		} else {
			if err := validation.ValidateTimeout(config.Redis.DialTimeout, "storage.redis.dial_timeout"); err != nil {
				result.AddError(err)
			}
		}
		if config.Redis.ReadTimeout <= 0 {
			config.Redis.ReadTimeout = 3
		} else {
			if err := validation.ValidateTimeout(config.Redis.ReadTimeout, "storage.redis.read_timeout"); err != nil {
				result.AddError(err)
			}
		}
		if config.Redis.WriteTimeout <= 0 {
			config.Redis.WriteTimeout = 3
		} else {
			if err := validation.ValidateTimeout(config.Redis.WriteTimeout, "storage.redis.write_timeout"); err != nil {
				result.AddError(err)
			}
		}
	}

	// 如果是 Hybrid，验证 Hybrid 配置
	if config.Type == "hybrid" {
		// 自动检测缓存类型：如果配置了 Redis，且缓存类型未显式设置或为 memory，自动升级为 redis
		// 这样可以支持多节点部署，共享运行时数据（会话、连接状态等）
		if config.Hybrid.CacheType == "" || config.Hybrid.CacheType == "memory" {
			if config.Redis.Addr != "" {
				config.Hybrid.CacheType = "redis"
			} else {
				// 没有配置 Redis，使用内存缓存
				if config.Hybrid.CacheType == "" {
					config.Hybrid.CacheType = "memory"
				}
			}
		}

		if config.Hybrid.CacheType != "memory" && config.Hybrid.CacheType != "redis" {
			result.AddError(coreErrors.Newf(coreErrors.ErrorTypePermanent, "invalid hybrid.cache_type: %s, must be 'memory' or 'redis'", config.Hybrid.CacheType))
		}

		if config.Hybrid.CacheType == "redis" && config.Redis.Addr == "" {
			result.AddError(coreErrors.New(coreErrors.ErrorTypePermanent, "storage.redis.addr is required when hybrid.cache_type is redis"))
		}

		// 前缀和 TTL 使用 storage.DefaultHybridConfig() 中的默认值，不需要用户配置

		if config.Hybrid.EnablePersistent {
			// 检查是否配置了 JSON 或 Remote 存储
			hasJSONConfig := config.Hybrid.JSON.FilePath != ""
			hasRemoteConfig := config.Hybrid.Remote.Type != "" && config.Hybrid.Remote.GRPC.Address != ""

			if !hasJSONConfig && !hasRemoteConfig {
				// 使用默认 JSON 存储配置
				config.Hybrid.JSON.FilePath = "data/tunnox-data.json"
				config.Hybrid.JSON.AutoSave = true
				config.Hybrid.JSON.SaveInterval = 30
			}

			// 如果配置了 Remote 存储，验证配置
			if hasRemoteConfig {
				if config.Hybrid.Remote.Type != "grpc" && config.Hybrid.Remote.Type != "http" {
					result.AddError(coreErrors.Newf(coreErrors.ErrorTypePermanent, "invalid hybrid.remote.type: %s, must be 'grpc' or 'http'", config.Hybrid.Remote.Type))
				}
				if config.Hybrid.Remote.Type == "grpc" && config.Hybrid.Remote.GRPC.Address != "" {
					if err := validation.ValidateAddress(config.Hybrid.Remote.GRPC.Address, "storage.hybrid.remote.grpc.address"); err != nil {
						result.AddError(err)
					}
				}
				if config.Hybrid.Remote.Type == "grpc" && config.Hybrid.Remote.GRPC.Address == "" {
					result.AddError(coreErrors.New(coreErrors.ErrorTypePermanent, "storage.hybrid.remote.grpc.address is required when remote.type is grpc"))
				}
				// 设置默认超时
				if config.Hybrid.Remote.GRPC.Timeout <= 0 {
					config.Hybrid.Remote.GRPC.Timeout = 5
				} else {
					if err := validation.ValidateTimeout(config.Hybrid.Remote.GRPC.Timeout, "storage.hybrid.remote.grpc.timeout"); err != nil {
						result.AddError(err)
					}
				}
				if config.Hybrid.Remote.GRPC.MaxRetries <= 0 {
					config.Hybrid.Remote.GRPC.MaxRetries = 3
				} else {
					if err := validation.ValidateNonNegativeInt(config.Hybrid.Remote.GRPC.MaxRetries, "storage.hybrid.remote.grpc.max_retries"); err != nil {
						result.AddError(err)
					}
				}
			}
		}
	}

	// 返回验证结果
	if !result.IsValid() {
		return result
	}
	return nil
}

// containsString 检查字符串切片是否包含指定字符串
func containsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// 注意：前缀配置已移至 internal/core/storage/hybrid_config.go
// 使用 storage.DefaultHybridConfig() 获取默认配置

// GetDefaultConfig 获取默认配置
func GetDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:         "0.0.0.0",
			Port:         8000,
			ReadTimeout:  30,
			WriteTimeout: 30,
			IdleTimeout:  60,
			Protocols: map[string]ProtocolConfig{
				"tcp": {
					Enabled: true,
					Port:    8000,
					Host:    "0.0.0.0",
				},
				"websocket": {
					Enabled: true,
					Port:    8443,
					Host:    "0.0.0.0",
				},
				"udp": {
					Enabled: true,
					Port:    8000,
					Host:    "0.0.0.0",
				},
				"quic": {
					Enabled: true,
					Port:    443,
					Host:    "0.0.0.0",
				},
			},
		},
		Storage: StorageConfig{
			Type: "hybrid",
			Hybrid: HybridStorageConfigYAML{
				CacheType:        "memory",
				EnablePersistent: true, // 默认启用持久化（但会根据是否有Redis自动调整）
				JSON: JSONStorageConfigYAML{
					FilePath:     "", // 留空，由智能逻辑自动决定
					AutoSave:     true,
					SaveInterval: 30,
				},
			},
		},
		Log: utils.LogConfig{
			Level:  constants.LogLevelInfo,
			Format: constants.LogFormatText,
			Output: constants.LogOutputFile,
			File:   "logs/server.log",
		},
		Cloud: CloudConfig{
			Type: "built_in",
		},
		MessageBroker: MessageBrokerConfig{
			Type:   "memory",
			NodeID: "node-001",
		},
		BridgePool: BridgePoolConfig{
			Enabled: false,
		},
		ManagementAPI: ManagementAPIConfig{
			Enabled:    true,
			ListenAddr: "0.0.0.0:9000",
			Auth: AuthConfig{
				Type:  "bearer", // 默认需要 bearer token 认证
				Token: "",       // 需要在配置文件中设置
			},
			PProf: PProfConfig{
				Enabled:     true,                     // 默认启用 pprof（需要密钥访问）
				DataDir:     "logs/pprof",             // 默认保存目录
				Retention:   10,                        // 默认保留10分钟
				AutoCapture: true,                     // 默认启用自动抓取
			},
		},
		UDPIngress: UDPIngressConfig{
			Enabled:   false,
			Listeners: []UDPIngressListenerConfig{},
		},
		Metrics: MetricsConfig{
			Type: "memory", // 默认使用 memory
		},
	}
}
