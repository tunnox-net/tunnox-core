package storage

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/core/dispose"
)

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

// CreateStorage 创建存储实例
func (f *StorageFactory) CreateStorage(storageType StorageType, config interface{}) (Storage, error) {
	switch storageType {
	case StorageTypeMemory:
		return f.createMemoryStorage()
	case StorageTypeRedis:
		return f.createRedisStorage(config)
	case StorageTypeHybrid:
		return f.createHybridStorage(config)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}

// createMemoryStorage 创建内存存储
func (f *StorageFactory) createMemoryStorage() (Storage, error) {
	storage := NewMemoryStorage(f.ctx)
	dispose.Infof("StorageFactory: created memory storage")
	return storage, nil
}

// createRedisStorage 创建Redis存储
func (f *StorageFactory) createRedisStorage(config interface{}) (Storage, error) {
	var redisConfig *RedisConfig

	if config != nil {
		if rc, ok := config.(*RedisConfig); ok {
			redisConfig = rc
		} else {
			return nil, fmt.Errorf("invalid Redis config type: %T", config)
		}
	}

	storage, err := NewRedisStorage(f.ctx, redisConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis storage: %w", err)
	}

	dispose.Infof("StorageFactory: created Redis storage")
	return storage, nil
}

// createHybridStorage 创建混合存储
func (f *StorageFactory) createHybridStorage(config interface{}) (Storage, error) {
	// 默认配置：纯内存模式
	hybridConfig := DefaultHybridConfig()

	var cache CacheStorage
	var sharedCache CacheStorage
	var persistent PersistentStorage

	// 解析配置
	if config != nil {
		if hc, ok := config.(*HybridStorageConfig); ok {
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
			return nil, fmt.Errorf("invalid HybridStorage config type: %T", config)
		}
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

// CreateStorageWithConfig 根据配置创建存储
func (f *StorageFactory) CreateStorageWithConfig(config map[string]interface{}) (Storage, error) {
	storageTypeStr, ok := config["type"].(string)
	if !ok {
		return nil, fmt.Errorf("storage type not specified in config")
	}

	storageType := StorageType(storageTypeStr)

	switch storageType {
	case StorageTypeMemory:
		return f.createMemoryStorage()
	case StorageTypeRedis:
		redisConfig := &RedisConfig{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			PoolSize: 10,
		}

		// 从配置中读取Redis参数
		if addr, ok := config["addr"].(string); ok {
			redisConfig.Addr = addr
		}
		if password, ok := config["password"].(string); ok {
			redisConfig.Password = password
		}
		if db, ok := config["db"].(int); ok {
			redisConfig.DB = db
		}
		if poolSize, ok := config["pool_size"].(int); ok {
			redisConfig.PoolSize = poolSize
		}

		return f.createRedisStorage(redisConfig)
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}
