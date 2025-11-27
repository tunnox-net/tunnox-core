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
	CacheType            string                  `yaml:"cache_type"` // memory | redis
	EnablePersistent     bool                    `yaml:"enable_persistent"`
	PersistentPrefixes   []string                `yaml:"persistent_prefixes"`
	RuntimePrefixes      []string                `yaml:"runtime_prefixes"`
	DefaultPersistentTTL int64                   `yaml:"default_persistent_ttl"` // 秒
	DefaultRuntimeTTL    int64                   `yaml:"default_runtime_ttl"`    // 秒
	JSON                 JSONStorageConfigYAML   `yaml:"json"`                   // JSON 文件存储配置（优先）
	Remote               RemoteStorageConfigYAML `yaml:"remote"`
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
	// 验证存储配置
	if err := validateStorageConfig(&config.Storage); err != nil {
		return fmt.Errorf("invalid storage config: %w", err)
	}

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

	// ============================================================================
	// Redis 自动共享逻辑
	// ============================================================================

	// 初始化 MessageBroker.Redis map
	if config.MessageBroker.Redis == nil {
		config.MessageBroker.Redis = make(map[string]interface{})
	}

	// 规则 1: 如果 storage.redis 已配置，但 message_broker 未配置或为 memory
	//        自动使用 Redis 作为消息队列
	if config.Storage.Redis.Addr != "" {
		if config.MessageBroker.Type == "" || config.MessageBroker.Type == "memory" {
			utils.Infof("✅ Storage Redis detected, auto-configuring message_broker to use redis for multi-node support")
			config.MessageBroker.Type = "redis"

			// 使用 map 设置 Redis 配置
			config.MessageBroker.Redis["addr"] = config.Storage.Redis.Addr
			if config.Storage.Redis.Password != "" {
				config.MessageBroker.Redis["password"] = config.Storage.Redis.Password
			}
			if config.Storage.Redis.DB > 0 {
				config.MessageBroker.Redis["db"] = config.Storage.Redis.DB
			}
			if config.MessageBroker.Redis["channel"] == nil || config.MessageBroker.Redis["channel"] == "" {
				config.MessageBroker.Redis["channel"] = "tunnox:messages"
			}
		}
	}

	// 规则 2: 如果 message_broker.redis 已配置，但 storage.redis 未配置
	//        自动使用 message_broker 的 Redis 配置给 storage
	if config.MessageBroker.Type == "redis" {
		if addr, ok := config.MessageBroker.Redis["addr"].(string); ok && addr != "" {
			if config.Storage.Redis.Addr == "" {
				utils.Infof("✅ MessageBroker Redis detected, auto-configuring storage.redis for distributed cache")
				config.Storage.Redis.Addr = addr

				if password, ok := config.MessageBroker.Redis["password"].(string); ok {
					config.Storage.Redis.Password = password
				}
				if db, ok := config.MessageBroker.Redis["db"].(int); ok {
					config.Storage.Redis.DB = db
				}

				// 设置默认值
				if config.Storage.Redis.PoolSize <= 0 {
					config.Storage.Redis.PoolSize = 10
				}
			}
		}
	}

	// 验证 MessageBroker 配置
	if config.MessageBroker.Type == "" {
		config.MessageBroker.Type = "memory"
		utils.Infof("MessageBroker type not configured, using default: memory")
	}
	if config.MessageBroker.NodeID == "" {
		config.MessageBroker.NodeID = "node-001"
	}

	// 验证 Cloud 配置
	if config.Cloud.Type == "" {
		config.Cloud.Type = "built_in"
		utils.Infof("Cloud type not configured, using default: built_in")
	}

	// 验证 ManagementAPI 配置
	if config.ManagementAPI.ListenAddr == "" {
		config.ManagementAPI.ListenAddr = "0.0.0.0:9000"
	}
	// 默认启用 ManagementAPI
	if !config.ManagementAPI.Enabled {
		utils.Infof("ManagementAPI not enabled in config, enabling by default on %s", config.ManagementAPI.ListenAddr)
		config.ManagementAPI.Enabled = true
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

// validateStorageConfig 验证存储配置
func validateStorageConfig(config *StorageConfig) error {
	// 如果未配置，使用默认值
	if config.Type == "" {
		config.Type = "hybrid"
	}

	// 验证存储类型
	validTypes := []string{"memory", "redis", "hybrid"}
	if !containsString(validTypes, config.Type) {
		return fmt.Errorf("invalid storage type: %s, must be one of: %v", config.Type, validTypes)
	}

	// 如果是 Redis，验证 Redis 配置
	if config.Type == "redis" {
		if config.Redis.Addr == "" {
			return fmt.Errorf("redis.addr is required when storage type is redis")
		}
		// 设置默认值
		if config.Redis.PoolSize <= 0 {
			config.Redis.PoolSize = 10
		}
		if config.Redis.MaxRetries <= 0 {
			config.Redis.MaxRetries = 3
		}
		if config.Redis.DialTimeout <= 0 {
			config.Redis.DialTimeout = 5
		}
		if config.Redis.ReadTimeout <= 0 {
			config.Redis.ReadTimeout = 3
		}
		if config.Redis.WriteTimeout <= 0 {
			config.Redis.WriteTimeout = 3
		}
	}

	// 如果是 Hybrid，验证 Hybrid 配置
	if config.Type == "hybrid" {
		// 自动检测缓存类型：如果配置了 Redis，且缓存类型未显式设置或为 memory，自动升级为 redis
		// 这样可以支持多节点部署，共享运行时数据（会话、连接状态等）
		if config.Hybrid.CacheType == "" || config.Hybrid.CacheType == "memory" {
			if config.Redis.Addr != "" {
				utils.Infof("✅ Redis detected, auto-upgrading cache from 'memory' to 'redis' for multi-node support")
				config.Hybrid.CacheType = "redis"
			} else {
				// 没有配置 Redis，使用内存缓存
				if config.Hybrid.CacheType == "" {
					config.Hybrid.CacheType = "memory"
				}
			}
		}

		if config.Hybrid.CacheType != "memory" && config.Hybrid.CacheType != "redis" {
			return fmt.Errorf("invalid hybrid.cache_type: %s, must be 'memory' or 'redis'", config.Hybrid.CacheType)
		}

		if config.Hybrid.CacheType == "redis" && config.Redis.Addr == "" {
			return fmt.Errorf("redis.addr is required when hybrid.cache_type is redis")
		}

		// 设置默认前缀
		if len(config.Hybrid.PersistentPrefixes) == 0 {
			config.Hybrid.PersistentPrefixes = DefaultPersistentPrefixes()
		}
		if len(config.Hybrid.RuntimePrefixes) == 0 {
			config.Hybrid.RuntimePrefixes = DefaultRuntimePrefixes()
		}

		// 设置默认 TTL
		if config.Hybrid.DefaultRuntimeTTL <= 0 {
			config.Hybrid.DefaultRuntimeTTL = 86400 // 24小时
		}

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
			if config.Hybrid.Remote.Type != "" {
				if config.Hybrid.Remote.Type != "grpc" && config.Hybrid.Remote.Type != "http" {
					return fmt.Errorf("invalid hybrid.remote.type: %s, must be 'grpc' or 'http'", config.Hybrid.Remote.Type)
				}

				if config.Hybrid.Remote.Type == "grpc" && config.Hybrid.Remote.GRPC.Address == "" {
					return fmt.Errorf("hybrid.remote.grpc.address is required when remote.type is grpc")
				}

				// 设置默认超时
				if config.Hybrid.Remote.GRPC.Timeout <= 0 {
					config.Hybrid.Remote.GRPC.Timeout = 5
				}
				if config.Hybrid.Remote.GRPC.MaxRetries <= 0 {
					config.Hybrid.Remote.GRPC.MaxRetries = 3
				}
			}
		}
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

// DefaultPersistentPrefixes 默认持久化前缀
func DefaultPersistentPrefixes() []string {
	return []string{
		"tunnox:user:",
		"tunnox:client:",
		"tunnox:mapping:",
		"tunnox:quota:",
	}
}

// DefaultRuntimePrefixes 默认运行时前缀
func DefaultRuntimePrefixes() []string {
	return []string{
		"tunnox:runtime:",
		"tunnox:session:",
		"tunnox:connection:",
		"tunnox:id:used:",
	}
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
		Storage: StorageConfig{
			Type: "hybrid",
			Hybrid: HybridStorageConfigYAML{
				CacheType:            "memory",
				EnablePersistent:     true, // 默认启用持久化
				PersistentPrefixes:   DefaultPersistentPrefixes(),
				RuntimePrefixes:      DefaultRuntimePrefixes(),
				DefaultPersistentTTL: 0,     // 永久
				DefaultRuntimeTTL:    86400, // 24小时
				JSON: JSONStorageConfigYAML{
					FilePath:     "data/tunnox-data.json",
					AutoSave:     true,
					SaveInterval: 30,
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
		},
		UDPIngress: UDPIngressConfig{
			Enabled:   false,
			Listeners: []UDPIngressListenerConfig{},
		},
	}
}
