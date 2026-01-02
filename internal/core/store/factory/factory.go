// Package factory 提供存储工厂
package factory

import (
	"fmt"

	"tunnox-core/internal/core/store"
	"tunnox-core/internal/core/store/cached"
	"tunnox-core/internal/core/store/memory"
	redisstore "tunnox-core/internal/core/store/shared/redis"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// StoreFactory 存储工厂
// =============================================================================

// StoreFactory 存储工厂
type StoreFactory struct {
	config      *store.StorageConfig
	redisClient *redis.Client
	isEmbedded  bool
}

// NewStoreFactory 创建存储工厂
func NewStoreFactory(config *store.StorageConfig) (*StoreFactory, error) {
	if config == nil {
		config = store.DefaultStorageConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid storage config: %w", err)
	}

	factory := &StoreFactory{
		config: config,
	}

	// 初始化共享存储
	if err := factory.initSharedStorage(); err != nil {
		return nil, err
	}

	return factory, nil
}

// initSharedStorage 初始化共享存储
func (f *StoreFactory) initSharedStorage() error {
	switch f.config.Shared.Type {
	case "redis":
		if f.config.Shared.Redis == nil {
			f.config.Shared.Redis = store.DefaultRedisConfig()
		}
		client := redis.NewClient(&redis.Options{
			Addr:         f.config.Shared.Redis.Addr,
			Password:     f.config.Shared.Redis.Password,
			DB:           f.config.Shared.Redis.DB,
			PoolSize:     f.config.Shared.Redis.PoolSize,
			MinIdleConns: f.config.Shared.Redis.MinIdleConns,
			DialTimeout:  f.config.Shared.Redis.DialTimeout,
			ReadTimeout:  f.config.Shared.Redis.ReadTimeout,
			WriteTimeout: f.config.Shared.Redis.WriteTimeout,
			MaxRetries:   f.config.Shared.Redis.MaxRetries,
		})
		f.redisClient = client
		f.isEmbedded = false

	case "embedded":
		// 对于 embedded 模式，使用 miniredis
		// 这里简化处理，创建一个内存模式的 Redis 客户端配置
		// 实际使用时应该通过 embedded 包创建
		f.isEmbedded = true
		// 暂时设置为 nil，需要外部调用 SetRedisClient
		f.redisClient = nil

	default:
		return fmt.Errorf("unsupported shared storage type: %s", f.config.Shared.Type)
	}

	return nil
}

// SetRedisClient 设置 Redis 客户端（用于 embedded 模式）
func (f *StoreFactory) SetRedisClient(client *redis.Client) {
	f.redisClient = client
}

// GetRedisClient 获取 Redis 客户端
func (f *StoreFactory) GetRedisClient() *redis.Client {
	return f.redisClient
}

// =============================================================================
// 创建存储的独立函数（Go 不支持方法带类型参数）
// =============================================================================

// CreateSharedStore 创建共享存储
func CreateSharedStore[K comparable, V any](f *StoreFactory, keyPrefix string) store.SharedStore[K, V] {
	return redisstore.NewRedisStore[K, V](f.redisClient, keyPrefix)
}

// CreateMemoryStore 创建内存存储
func CreateMemoryStore[K comparable, V any]() store.MemoryStore[K, V] {
	return memory.NewMemoryStore[K, V]()
}

// CreateMemorySetStore 创建内存集合存储
func CreateMemorySetStore[K comparable, V comparable]() store.SetStore[K, V] {
	return memory.NewMemorySetStore[K, V]()
}

// CreateCachedPersistentStore 创建缓存+持久化组合存储
func CreateCachedPersistentStore[K comparable, V any](
	f *StoreFactory,
	cache store.SharedStore[K, V],
	persistent store.PersistentStore[K, V],
) (store.CachedPersistentStore[K, V], error) {
	return cached.NewCachedPersistentStore(cache, persistent, f.config.ToCacheConfig())
}

// =============================================================================
// 便捷方法（非泛型）
// =============================================================================

// CreateSetStore 创建集合存储
func (f *StoreFactory) CreateSetStore(keyPrefix string) store.SetStore[string, string] {
	return redisstore.NewRedisSetStore(f.redisClient, keyPrefix)
}

// CreateUserClientIndexStore 创建用户客户端索引存储
func (f *StoreFactory) CreateUserClientIndexStore() store.SetStore[string, string] {
	return f.CreateSetStore("tunnox:index:user:clients:")
}

// CreateUserMappingIndexStore 创建用户映射索引存储
func (f *StoreFactory) CreateUserMappingIndexStore() store.SetStore[string, string] {
	return f.CreateSetStore("tunnox:index:user:mappings:")
}

// =============================================================================
// 生命周期管理
// =============================================================================

// Close 关闭工厂
func (f *StoreFactory) Close() error {
	if f.redisClient != nil {
		return f.redisClient.Close()
	}
	return nil
}

// GetConfig 获取配置
func (f *StoreFactory) GetConfig() *store.StorageConfig {
	return f.config
}

// IsEmbedded 是否使用内嵌 Redis
func (f *StoreFactory) IsEmbedded() bool {
	return f.isEmbedded
}

// IsSingleMode 是否为单机模式
func (f *StoreFactory) IsSingleMode() bool {
	return f.config.IsSingleMode()
}

// =============================================================================
// 全局实例（可选）
// =============================================================================

var defaultFactory *StoreFactory

// InitDefaultFactory 初始化默认工厂
func InitDefaultFactory(config *store.StorageConfig) error {
	factory, err := NewStoreFactory(config)
	if err != nil {
		return err
	}
	defaultFactory = factory
	return nil
}

// GetDefaultFactory 获取默认工厂
func GetDefaultFactory() *StoreFactory {
	return defaultFactory
}

// CloseDefaultFactory 关闭默认工厂
func CloseDefaultFactory() error {
	if defaultFactory != nil {
		return defaultFactory.Close()
	}
	return nil
}
