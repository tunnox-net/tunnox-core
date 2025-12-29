package schema

import "time"

// StorageConfig contains storage configuration
type StorageConfig struct {
	Type        string              `yaml:"type" json:"type"` // memory/redis/hybrid
	Redis       RedisConfig         `yaml:"redis" json:"redis"`
	Persistence PersistenceConfig   `yaml:"persistence" json:"persistence"`
	Remote      RemoteStorageConfig `yaml:"remote" json:"remote"`
	Hybrid      HybridStorageConfig `yaml:"hybrid" json:"hybrid"`
}

// RedisConfig contains Redis settings
type RedisConfig struct {
	Enabled      bool          `yaml:"enabled" json:"enabled"`
	Addr         string        `yaml:"addr" json:"addr"`
	Password     Secret        `yaml:"password" json:"password"`
	DB           int           `yaml:"db" json:"db"`
	PoolSize     int           `yaml:"pool_size" json:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns" json:"min_idle_conns"`
	MaxRetries   int           `yaml:"max_retries" json:"max_retries"`
	DialTimeout  time.Duration `yaml:"dial_timeout" json:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`
}

// PersistenceConfig contains local persistence settings
type PersistenceConfig struct {
	Enabled      bool          `yaml:"enabled" json:"enabled"`
	File         string        `yaml:"file" json:"file"`
	AutoSave     bool          `yaml:"auto_save" json:"auto_save"`
	SaveInterval time.Duration `yaml:"save_interval" json:"save_interval"`
}

// RemoteStorageConfig contains remote gRPC storage settings
type RemoteStorageConfig struct {
	Enabled     bool                   `yaml:"enabled" json:"enabled"`
	GRPCAddress string                 `yaml:"grpc_address" json:"grpc_address"`
	Timeout     time.Duration          `yaml:"timeout" json:"timeout"`
	MaxRetries  int                    `yaml:"max_retries" json:"max_retries"`
	TLS         RemoteStorageTLSConfig `yaml:"tls" json:"tls"`
}

// RemoteStorageTLSConfig contains TLS settings for remote storage
type RemoteStorageTLSConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	CertFile string `yaml:"cert_file" json:"cert_file"`
	KeyFile  string `yaml:"key_file" json:"key_file"`
	CAFile   string `yaml:"ca_file" json:"ca_file"`
}

// HybridStorageConfig contains hybrid storage settings
type HybridStorageConfig struct {
	CacheType          string        `yaml:"cache_type" json:"cache_type"` // memory/redis
	EnablePersistent   bool          `yaml:"enable_persistent" json:"enable_persistent"`
	DefaultCacheTTL    time.Duration `yaml:"default_cache_ttl" json:"default_cache_ttl"`
	PersistentCacheTTL time.Duration `yaml:"persistent_cache_ttl" json:"persistent_cache_ttl"`
	SharedCacheTTL     time.Duration `yaml:"shared_cache_ttl" json:"shared_cache_ttl"`
}

// Storage type constants
const (
	StorageTypeMemory = "memory"
	StorageTypeRedis  = "redis"
	StorageTypeHybrid = "hybrid"
)
