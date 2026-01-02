package store

import "time"

// =============================================================================
// 统一存储配置
// =============================================================================

// StorageConfig 统一存储配置
type StorageConfig struct {
	// Mode 部署模式：single | cluster
	Mode DeploymentMode `yaml:"mode"`

	// Shared 共享存储配置
	Shared SharedConfig `yaml:"shared"`

	// Persistent 持久化存储配置
	Persistent PersistentConfig `yaml:"persistent"`

	// Cache 缓存配置
	Cache CacheSettings `yaml:"cache"`

	// Index 索引配置
	Index IndexSettings `yaml:"index"`

	// Fallback 降级配置
	Fallback FallbackSettings `yaml:"fallback"`

	// Metrics 监控配置
	Metrics MetricsSettings `yaml:"metrics"`
}

// DeploymentMode 部署模式
type DeploymentMode string

const (
	// ModeSingle 单机模式（使用 miniredis + JSON/Memory）
	ModeSingle DeploymentMode = "single"

	// ModeCluster 集群模式（使用 Redis + gRPC）
	ModeCluster DeploymentMode = "cluster"
)

// SharedConfig 共享存储配置
type SharedConfig struct {
	// Type 存储类型：redis | embedded
	Type string `yaml:"type"`

	// Redis Redis 配置（Type=redis 时使用）
	Redis *RedisConfig `yaml:"redis,omitempty"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	// Addr Redis 地址
	Addr string `yaml:"addr"`

	// Password 密码
	Password string `yaml:"password"`

	// DB 数据库编号
	DB int `yaml:"db"`

	// PoolSize 连接池大小
	PoolSize int `yaml:"pool_size"`

	// MinIdleConns 最小空闲连接数
	MinIdleConns int `yaml:"min_idle_conns"`

	// DialTimeout 连接超时
	DialTimeout time.Duration `yaml:"dial_timeout"`

	// ReadTimeout 读取超时
	ReadTimeout time.Duration `yaml:"read_timeout"`

	// WriteTimeout 写入超时
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// MaxRetries 最大重试次数
	MaxRetries int `yaml:"max_retries"`
}

// PersistentConfig 持久化存储配置
type PersistentConfig struct {
	// Type 存储类型：grpc | json | memory
	Type string `yaml:"type"`

	// GRPC gRPC 配置（Type=grpc 时使用）
	GRPC *GRPCConfig `yaml:"grpc,omitempty"`

	// JSON JSON 文件配置（Type=json 时使用）
	JSON *JSONConfig `yaml:"json,omitempty"`
}

// GRPCConfig gRPC 存储配置
type GRPCConfig struct {
	// Address gRPC 服务地址
	Address string `yaml:"address"`

	// Timeout 请求超时
	Timeout time.Duration `yaml:"timeout"`

	// MaxRetries 最大重试次数
	MaxRetries int `yaml:"max_retries"`

	// TLS TLS 配置
	TLS *TLSConfig `yaml:"tls,omitempty"`
}

// TLSConfig TLS 配置
type TLSConfig struct {
	// Enabled 是否启用 TLS
	Enabled bool `yaml:"enabled"`

	// CertFile 证书文件路径
	CertFile string `yaml:"cert_file"`

	// KeyFile 私钥文件路径
	KeyFile string `yaml:"key_file"`

	// CAFile CA 证书文件路径
	CAFile string `yaml:"ca_file"`

	// InsecureSkipVerify 跳过证书验证
	InsecureSkipVerify bool `yaml:"insecure_skip_verify"`
}

// JSONConfig JSON 文件存储配置
type JSONConfig struct {
	// Directory 数据目录
	Directory string `yaml:"directory"`

	// AutoSaveInterval 自动保存间隔
	AutoSaveInterval time.Duration `yaml:"auto_save_interval"`

	// CompactOnSave 保存时压缩
	CompactOnSave bool `yaml:"compact_on_save"`
}

// CacheSettings 缓存设置
type CacheSettings struct {
	// DefaultTTL 默认缓存 TTL
	DefaultTTL time.Duration `yaml:"default_ttl"`

	// NegativeTTL 负缓存 TTL
	NegativeTTL time.Duration `yaml:"negative_ttl"`

	// PenetrationProtect 启用穿透保护
	PenetrationProtect bool `yaml:"penetration_protect"`

	// BloomFilterSize 布隆过滤器大小，0 表示不启用
	BloomFilterSize int `yaml:"bloom_filter_size"`
}

// IndexSettings 索引设置
type IndexSettings struct {
	// Enabled 启用索引
	Enabled bool `yaml:"enabled"`

	// RebuildOnStartup 启动时重建索引
	RebuildOnStartup bool `yaml:"rebuild_on_startup"`

	// VerifyInterval 索引校验间隔
	VerifyInterval time.Duration `yaml:"verify_interval"`

	// AutoRepair 自动修复不一致
	AutoRepair bool `yaml:"auto_repair"`
}

// FallbackSettings 降级设置
type FallbackSettings struct {
	// SharedPolicy 共享存储降级策略：fail_fast | fallback_to_embedded
	SharedPolicy string `yaml:"shared_policy"`

	// PersistentPolicy 持久化存储降级策略：fail_fast | fallback_to_memory
	PersistentPolicy string `yaml:"persistent_policy"`

	// RetryCount 重试次数
	RetryCount int `yaml:"retry_count"`

	// RetryInterval 重试间隔
	RetryInterval time.Duration `yaml:"retry_interval"`
}

// MetricsSettings 监控设置
type MetricsSettings struct {
	// Enabled 启用监控
	Enabled bool `yaml:"enabled"`

	// Prefix 指标前缀
	Prefix string `yaml:"prefix"`
}

// =============================================================================
// 默认配置
// =============================================================================

// DefaultStorageConfig 默认存储配置
func DefaultStorageConfig() *StorageConfig {
	return &StorageConfig{
		Mode: ModeSingle,
		Shared: SharedConfig{
			Type: "embedded",
		},
		Persistent: PersistentConfig{
			Type: "memory",
		},
		Cache: CacheSettings{
			DefaultTTL:         30 * time.Minute,
			NegativeTTL:        5 * time.Minute,
			PenetrationProtect: true,
			BloomFilterSize:    0,
		},
		Index: IndexSettings{
			Enabled:          true,
			RebuildOnStartup: false,
			VerifyInterval:   time.Hour,
			AutoRepair:       false,
		},
		Fallback: FallbackSettings{
			SharedPolicy:     "fallback_to_embedded",
			PersistentPolicy: "fallback_to_memory",
			RetryCount:       3,
			RetryInterval:    time.Second,
		},
		Metrics: MetricsSettings{
			Enabled: true,
			Prefix:  "tunnox_storage",
		},
	}
}

// DefaultRedisConfig 默认 Redis 配置
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 2,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		MaxRetries:   3,
	}
}

// DefaultGRPCConfig 默认 gRPC 配置
func DefaultGRPCConfig() *GRPCConfig {
	return &GRPCConfig{
		Address:    "localhost:50051",
		Timeout:    5 * time.Second,
		MaxRetries: 3,
	}
}

// DefaultJSONConfig 默认 JSON 配置
func DefaultJSONConfig() *JSONConfig {
	return &JSONConfig{
		Directory:        "./data",
		AutoSaveInterval: 5 * time.Minute,
		CompactOnSave:    false,
	}
}

// =============================================================================
// 配置验证
// =============================================================================

// Validate 验证配置
func (c *StorageConfig) Validate() error {
	if c.Mode == "" {
		c.Mode = ModeSingle
	}

	if c.Shared.Type == "" {
		if c.Mode == ModeSingle {
			c.Shared.Type = "embedded"
		} else {
			c.Shared.Type = "redis"
		}
	}

	if c.Persistent.Type == "" {
		if c.Mode == ModeSingle {
			c.Persistent.Type = "memory"
		} else {
			c.Persistent.Type = "grpc"
		}
	}

	// 设置默认值
	if c.Cache.DefaultTTL <= 0 {
		c.Cache.DefaultTTL = 30 * time.Minute
	}
	if c.Cache.NegativeTTL <= 0 {
		c.Cache.NegativeTTL = 5 * time.Minute
	}
	if c.Index.VerifyInterval <= 0 {
		c.Index.VerifyInterval = time.Hour
	}
	if c.Fallback.RetryCount <= 0 {
		c.Fallback.RetryCount = 3
	}
	if c.Fallback.RetryInterval <= 0 {
		c.Fallback.RetryInterval = time.Second
	}
	if c.Metrics.Prefix == "" {
		c.Metrics.Prefix = "tunnox_storage"
	}

	return nil
}

// IsSingleMode 是否为单机模式
func (c *StorageConfig) IsSingleMode() bool {
	return c.Mode == ModeSingle
}

// IsClusterMode 是否为集群模式
func (c *StorageConfig) IsClusterMode() bool {
	return c.Mode == ModeCluster
}

// ToCacheConfig 转换为 CacheConfig
func (c *StorageConfig) ToCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:                   c.Cache.DefaultTTL,
		NegativeTTL:           c.Cache.NegativeTTL,
		WritePolicy:           WriteThrough,
		LoadOnMiss:            true,
		PenetrationProtection: c.Cache.PenetrationProtect,
		MaxNegativeCacheSize:  10000,
		BloomFilterSize:       c.Cache.BloomFilterSize,
		BloomFilterFPRate:     0.01,
	}
}
