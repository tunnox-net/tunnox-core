// Package storage 提供统一的存储抽象层
//
// 本包作为存储子包的门面（facade），提供向后兼容的类型别名和工厂函数。
// 实际的存储实现位于各个子包中：
//   - memory: 内存存储
//   - redis: Redis 存储
//   - remote: gRPC 远程存储
//   - hybrid: 混合存储（缓存 + 持久化）
//   - json: JSON 文件存储
//   - types: 接口和类型定义
package storage

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage/hybrid"
	jsonstorage "tunnox-core/internal/core/storage/json"
	"tunnox-core/internal/core/storage/memory"
	redisstorage "tunnox-core/internal/core/storage/redis"
	"tunnox-core/internal/core/storage/remote"
	"tunnox-core/internal/core/storage/types"
)

// ============================================================================
// 核心类型导出
// ============================================================================

// 错误类型
var (
	ErrKeyNotFound = types.ErrKeyNotFound
	ErrInvalidType = types.ErrInvalidType
)

type (
	Storage           = types.Storage
	FullStorage       = types.FullStorage
	ListStore         = types.ListStore
	HashStore         = types.HashStore
	CounterStore      = types.CounterStore
	CASStore          = types.CASStore
	WatchableStore    = types.WatchableStore
	PersistentStorage = types.PersistentStorage
	CacheStorage      = types.CacheStorage
	SortedSetStore    = types.SortedSetStore
)

// ============================================================================
// 泛型类型别名（推荐使用）
// ============================================================================

// TypedStorage 泛型存储接口别名
// 使用示例: var s TypedStorage[models.User]
type TypedStorage[T any] = types.TypedStorage[T]

// TypedListStore 泛型列表存储接口别名
type TypedListStore[T any] = types.TypedListStore[T]

// TypedHashStore 泛型哈希存储接口别名
type TypedHashStore[T any] = types.TypedHashStore[T]

// TypedCASStore 泛型 CAS 存储接口别名
type TypedCASStore[T any] = types.TypedCASStore[T]

// TypedWatchableStore 泛型监听存储接口别名
type TypedWatchableStore[T any] = types.TypedWatchableStore[T]

// TypedFullStorage 完整的泛型存储接口别名
type TypedFullStorage[T any] = types.TypedFullStorage[T]

// TypedPersistentStorage 泛型持久化存储接口别名
type TypedPersistentStorage[T any] = types.TypedPersistentStorage[T]

// TypedCacheStorage 泛型缓存存储接口别名
type TypedCacheStorage[T any] = types.TypedCacheStorage[T]

// TypedStorageAdapter 泛型存储适配器类型别名
type TypedStorageAdapter[T any] = types.TypedStorageAdapter[T]

// TypedFullStorageAdapter 完整泛型存储适配器类型别名
type TypedFullStorageAdapter[T any] = types.TypedFullStorageAdapter[T]

// TypedCacheAdapter 泛型缓存适配器类型别名
type TypedCacheAdapter[T any] = types.TypedCacheAdapter[T]

// TypedPersistentAdapter 泛型持久化适配器类型别名
type TypedPersistentAdapter[T any] = types.TypedPersistentAdapter[T]

// 存储实现类型别名
type (
	MemoryStorage = memory.Storage
	RedisStorage  = redisstorage.Storage
	RemoteStorage = remote.Storage
	HybridStorage = hybrid.Storage
	JSONStorage   = jsonstorage.Storage
)

// 配置类型别名
type (
	RedisConfig         = redisstorage.Config
	RemoteStorageConfig = remote.Config
	HybridConfig        = hybrid.Config
	JSONStorageConfig   = jsonstorage.Config
	DataCategory        = hybrid.DataCategory
)

// 数据分类常量
const (
	DataCategoryRuntime          = hybrid.DataCategoryRuntime
	DataCategoryPersistent       = hybrid.DataCategoryPersistent
	DataCategoryShared           = hybrid.DataCategoryShared
	DataCategorySharedPersistent = hybrid.DataCategorySharedPersistent
)

// Redis 相关导出
type RedisClient = redisstorage.Client

var ErrRedisNil = redisstorage.ErrRedisNil

// ============================================================================
// 构造函数 - 向后兼容
// ============================================================================

// NewMemoryStorage 创建内存存储
func NewMemoryStorage(parentCtx context.Context) Storage {
	return memory.New(parentCtx)
}

// NewRedisStorage 创建 Redis 存储
func NewRedisStorage(parentCtx context.Context, config *RedisConfig) (*RedisStorage, error) {
	return redisstorage.New(parentCtx, config)
}

// NewRemoteStorage 创建远程存储
func NewRemoteStorage(parentCtx context.Context, config *RemoteStorageConfig) (*RemoteStorage, error) {
	return remote.New(parentCtx, config)
}

// NewHybridStorage 创建混合存储
func NewHybridStorage(parentCtx context.Context, cache CacheStorage, persistent PersistentStorage, config *HybridConfig) *HybridStorage {
	return hybrid.New(parentCtx, cache, persistent, config)
}

// NewHybridStorageWithSharedCache 创建带共享缓存的混合存储
func NewHybridStorageWithSharedCache(parentCtx context.Context, cache CacheStorage, sharedCache CacheStorage, persistent PersistentStorage, config *HybridConfig) *HybridStorage {
	return hybrid.NewWithSharedCache(parentCtx, cache, sharedCache, persistent, config)
}

// NewJSONStorage 创建 JSON 存储
func NewJSONStorage(config *JSONStorageConfig) (*JSONStorage, error) {
	return jsonstorage.New(config)
}

// NewNullPersistentStorage 创建空持久化存储
func NewNullPersistentStorage() PersistentStorage {
	return types.NewNullPersistentStorage()
}

// DefaultHybridConfig 返回默认混合存储配置
func DefaultHybridConfig() *HybridConfig {
	return hybrid.DefaultConfig()
}

// ============================================================================
// 工厂模式
// ============================================================================

// StorageType 存储类型
type StorageType string

const (
	StorageTypeMemory StorageType = "memory"
	StorageTypeRedis  StorageType = "redis"
	StorageTypeHybrid StorageType = "hybrid"
)

// StorageFactory 存储工厂
type StorageFactory struct {
	ctx context.Context
}

// NewStorageFactory 创建存储工厂
func NewStorageFactory(ctx context.Context) *StorageFactory {
	return &StorageFactory{
		ctx: ctx,
	}
}

// StorageConfig 存储配置接口
// 所有存储类型的配置都必须实现此接口
type StorageConfig interface {
	// StorageType 返回存储类型
	StorageType() StorageType
}

// MemoryStorageConfig 内存存储配置
type MemoryStorageConfig struct{}

// StorageType 返回存储类型
func (c *MemoryStorageConfig) StorageType() StorageType {
	return StorageTypeMemory
}

// RedisStorageConfig Redis 存储配置（实现 StorageConfig 接口）
type RedisStorageConfig struct {
	*RedisConfig
}

// StorageType 返回存储类型
func (c *RedisStorageConfig) StorageType() StorageType {
	return StorageTypeRedis
}

// NewRedisStorageConfig 创建 Redis 存储配置
func NewRedisStorageConfig(config *RedisConfig) *RedisStorageConfig {
	return &RedisStorageConfig{RedisConfig: config}
}

// HybridStorageFactoryConfig 混合存储工厂配置（实现 StorageConfig 接口）
type HybridStorageFactoryConfig struct {
	*HybridStorageConfig
}

// StorageType 返回存储类型
func (c *HybridStorageFactoryConfig) StorageType() StorageType {
	return StorageTypeHybrid
}

// NewHybridStorageFactoryConfig 创建混合存储工厂配置
func NewHybridStorageFactoryConfig(config *HybridStorageConfig) *HybridStorageFactoryConfig {
	return &HybridStorageFactoryConfig{HybridStorageConfig: config}
}

// CreateStorage 创建存储实例（使用类型安全的配置）
func (f *StorageFactory) CreateStorage(config StorageConfig) (Storage, error) {
	if config == nil {
		return nil, fmt.Errorf("storage config is nil")
	}

	switch config.StorageType() {
	case StorageTypeMemory:
		return f.createMemoryStorage()
	case StorageTypeRedis:
		redisConfig, ok := config.(*RedisStorageConfig)
		if !ok {
			return nil, fmt.Errorf("invalid Redis config type: %T, expected *RedisStorageConfig", config)
		}
		return f.createRedisStorageTyped(redisConfig.RedisConfig)
	case StorageTypeHybrid:
		// 支持两种配置类型
		if factoryConfig, ok := config.(*HybridStorageFactoryConfig); ok {
			return f.createHybridStorageTyped(factoryConfig.HybridStorageConfig)
		}
		if hybridConfig, ok := config.(*HybridStorageConfig); ok {
			return f.createHybridStorageTyped(hybridConfig)
		}
		return nil, fmt.Errorf("invalid Hybrid config type: %T, expected *HybridStorageConfig or *HybridStorageFactoryConfig", config)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.StorageType())
	}
}

// createMemoryStorage 创建内存存储
func (f *StorageFactory) createMemoryStorage() (Storage, error) {
	storage := NewMemoryStorage(f.ctx)
	dispose.Infof("StorageFactory: created memory storage")
	return storage, nil
}

// createRedisStorageTyped 创建 Redis 存储（类型安全版本）
func (f *StorageFactory) createRedisStorageTyped(config *RedisConfig) (Storage, error) {
	if config == nil {
		return nil, fmt.Errorf("redis config is nil")
	}

	storage, err := NewRedisStorage(f.ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis storage: %w", err)
	}

	dispose.Infof("StorageFactory: created Redis storage")
	return storage, nil
}

// createHybridStorageTyped 创建混合存储（类型安全版本）
func (f *StorageFactory) createHybridStorageTyped(hc *HybridStorageConfig) (Storage, error) {
	// 默认配置：纯内存模式
	hybridConfig := DefaultHybridConfig()

	var cache CacheStorage
	var sharedCache CacheStorage
	var persistent PersistentStorage

	// 解析配置
	if hc != nil {
		// 创建本地缓存存储
		if hc.CacheType == "redis" && hc.RedisConfig != nil {
			redisStorage, err := NewRedisStorage(f.ctx, hc.RedisConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to create Redis cache: %w", err)
			}
			cache = redisStorage
			// 如果本地缓存是 Redis，共享缓存也使用同一个 Redis
			sharedCache = redisStorage
		} else {
			cache = NewMemoryStorage(f.ctx)
		}

		// 创建共享缓存（如果配置了独立的共享缓存）
		if hc.SharedCacheConfig != nil {
			sharedRedis, err := NewRedisStorage(f.ctx, hc.SharedCacheConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to create shared Redis cache: %w", err)
			}
			sharedCache = sharedRedis
			dispose.Infof("StorageFactory: shared cache configured (Redis: %s)", hc.SharedCacheConfig.Addr)
		}

		// 创建持久化存储
		if hc.EnablePersistent {
			// 优先使用 JSON 文件存储（如果配置了）
			if hc.JSONConfig != nil {
				jsonStorage, err := NewJSONStorage(hc.JSONConfig)
				if err != nil {
					return nil, fmt.Errorf("failed to create JSON persistent storage: %w", err)
				}
				persistent = jsonStorage
			} else if hc.RemoteConfig != nil {
				// 使用远程存储
				remoteStorage, err := NewRemoteStorage(f.ctx, hc.RemoteConfig)
				if err != nil {
					return nil, fmt.Errorf("failed to create remote storage: %w", err)
				}
				persistent = remoteStorage
			} else {
				// 默认使用 JSON 文件存储
				jsonStorage, err := NewJSONStorage(&JSONStorageConfig{
					FilePath:     "data/tunnox-data.json",
					AutoSave:     true,
					SaveInterval: 30 * time.Second,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to create default JSON storage: %w", err)
				}
				persistent = jsonStorage
			}
		}

		// 更新配置
		if hc.HybridConfig != nil {
			hybridConfig = hc.HybridConfig
		}
		hybridConfig.EnablePersistent = hc.EnablePersistent
	} else {
		// 默认：纯内存模式
		cache = NewMemoryStorage(f.ctx)
	}

	storage := NewHybridStorageWithSharedCache(f.ctx, cache, sharedCache, persistent, hybridConfig)
	dispose.Infof("StorageFactory: created Hybrid storage")
	return storage, nil
}

// HybridStorageConfig 混合存储工厂配置
type HybridStorageConfig struct {
	// 缓存类型：memory 或 redis
	CacheType string

	// Redis 缓存配置（如果 CacheType 为 redis）
	RedisConfig *RedisConfig

	// 是否启用持久化
	EnablePersistent bool

	// JSON 文件存储配置（如果 EnablePersistent 为 true，优先使用）
	JSONConfig *JSONStorageConfig

	// 远程存储配置（如果 EnablePersistent 为 true 且未配置 JSON）
	RemoteConfig *RemoteStorageConfig

	// 混合存储配置
	HybridConfig *HybridConfig

	// 共享缓存配置（用于跨节点共享数据）
	// 如果设置，共享数据（如连接状态、隧道路由）将写入此缓存
	// 如果未设置，共享数据将写入本地缓存（单节点模式）
	SharedCacheConfig *RedisConfig
}

// StorageType 返回存储类型（实现 StorageConfig 接口）
func (c *HybridStorageConfig) StorageType() StorageType {
	return StorageTypeHybrid
}

// StorageConfigMap 存储配置映射（用于从 YAML/JSON 配置解析）
// 这是一个类型安全的配置结构，替代 map[string]interface{}
type StorageConfigMap struct {
	// Type 存储类型
	Type StorageType `json:"type" yaml:"type"`

	// Redis 配置（当 Type 为 redis 时使用）
	Addr     string `json:"addr" yaml:"addr"`
	Password string `json:"password" yaml:"password"`
	DB       int    `json:"db" yaml:"db"`
	PoolSize int    `json:"pool_size" yaml:"pool_size"`
}

// ToStorageConfig 将配置映射转换为类型安全的 StorageConfig
func (m *StorageConfigMap) ToStorageConfig() (StorageConfig, error) {
	switch m.Type {
	case StorageTypeMemory:
		return &MemoryStorageConfig{}, nil
	case StorageTypeRedis:
		redisConfig := &RedisConfig{
			Addr:     m.Addr,
			Password: m.Password,
			DB:       m.DB,
			PoolSize: m.PoolSize,
		}
		// 设置默认值
		if redisConfig.Addr == "" {
			redisConfig.Addr = "localhost:6379"
		}
		if redisConfig.PoolSize <= 0 {
			redisConfig.PoolSize = 10
		}
		return NewRedisStorageConfig(redisConfig), nil
	case StorageTypeHybrid:
		return NewHybridStorageFactoryConfig(&HybridStorageConfig{}), nil
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", m.Type)
	}
}

// CreateStorageWithConfigMap 根据类型安全的配置映射创建存储
func (f *StorageFactory) CreateStorageWithConfigMap(config *StorageConfigMap) (Storage, error) {
	if config == nil {
		return nil, fmt.Errorf("storage config is nil")
	}

	storageConfig, err := config.ToStorageConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to convert config: %w", err)
	}

	return f.CreateStorage(storageConfig)
}

// ============================================================================
// 泛型适配器工厂函数（推荐使用）
// ============================================================================

// NewTypedStorageAdapter 创建泛型存储适配器
// 将 Storage 接口包装为类型安全的泛型接口
// 使用示例:
//
//	userStorage := storage.NewTypedStorageAdapter[models.User](rawStorage)
//	err := userStorage.Set("user:1", user, 0)
func NewTypedStorageAdapter[T any](storage Storage) *TypedStorageAdapter[T] {
	return types.NewTypedStorageAdapter[T](storage)
}

// NewTypedFullStorageAdapter 创建完整的泛型存储适配器
// 将 FullStorage 接口包装为类型安全的完整泛型接口
// 支持所有操作：KV、列表、哈希、计数器、CAS、监听
// 使用示例:
//
//	userStorage := storage.NewTypedFullStorageAdapter[models.User](fullStorage)
//	err := userStorage.SetHash("users", "1", user)
func NewTypedFullStorageAdapter[T any](fullStorage FullStorage) *TypedFullStorageAdapter[T] {
	return types.NewTypedFullStorageAdapter[T](fullStorage)
}

// NewTypedCacheAdapter 创建泛型缓存适配器
// 使用示例:
//
//	sessionCache := storage.NewTypedCacheAdapter[models.Session](cache)
func NewTypedCacheAdapter[T any](cache CacheStorage) *TypedCacheAdapter[T] {
	return types.NewTypedCacheAdapter[T](cache)
}

// NewTypedPersistentAdapter 创建泛型持久化存储适配器
// 使用示例:
//
//	configStorage := storage.NewTypedPersistentAdapter[config.Settings](persistent)
func NewTypedPersistentAdapter[T any](persistent PersistentStorage) *TypedPersistentAdapter[T] {
	return types.NewTypedPersistentAdapter[T](persistent)
}

// AsFullStorage 将 Storage 转换为 FullStorage
// 如果存储不支持 FullStorage 接口，返回 nil 和 false
func AsFullStorage(s Storage) (FullStorage, bool) {
	fs, ok := s.(FullStorage)
	return fs, ok
}

// MustAsFullStorage 将 Storage 转换为 FullStorage
// 如果存储不支持 FullStorage 接口，panic
func MustAsFullStorage(s Storage) FullStorage {
	fs, ok := s.(FullStorage)
	if !ok {
		panic("storage does not implement FullStorage interface")
	}
	return fs
}
