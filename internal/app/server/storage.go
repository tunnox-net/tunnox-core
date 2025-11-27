package server

import (
	"fmt"
	"time"
	
	"tunnox-core/internal/core/storage"
)

// createStorage 根据配置创建存储
func createStorage(factory *storage.StorageFactory, config *StorageConfig) (storage.Storage, error) {
	switch storage.StorageType(config.Type) {
	case storage.StorageTypeMemory:
		return factory.CreateStorage(storage.StorageTypeMemory, nil)
		
	case storage.StorageTypeRedis:
		redisConfig := &storage.RedisConfig{
			Addr:     config.Redis.Addr,
			Password: config.Redis.Password,
			DB:       config.Redis.DB,
			PoolSize: config.Redis.PoolSize,
		}
		return factory.CreateStorage(storage.StorageTypeRedis, redisConfig)
		
	case storage.StorageTypeHybrid:
		return createHybridStorage(factory, config)
		
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}

// createHybridStorage 创建混合存储
func createHybridStorage(factory *storage.StorageFactory, config *StorageConfig) (storage.Storage, error) {
	// 自动检测是否应该使用 Redis 缓存
	// 规则：如果配置了 Redis（无论是存储还是消息队列），且缓存类型未显式设置，则自动使用 Redis
	cacheType := config.Hybrid.CacheType
	autoUpgraded := false
	
	// 如果缓存类型为 memory，但 Redis 已配置，自动升级为 redis
	if cacheType == "memory" || cacheType == "" {
		if config.Redis.Addr != "" {
			cacheType = "redis"
			autoUpgraded = true
		}
	}
	
	// 准备混合存储配置
	hybridConfig := &storage.HybridStorageConfig{
		CacheType:        cacheType,
		EnablePersistent: config.Hybrid.EnablePersistent,
		HybridConfig: &storage.HybridConfig{
			PersistentPrefixes: config.Hybrid.PersistentPrefixes,
			EnablePersistent:   config.Hybrid.EnablePersistent,
		},
	}
	
	// 如果缓存类型是 Redis，提供 Redis 配置
	if cacheType == "redis" {
		if config.Redis.Addr == "" {
			return nil, fmt.Errorf("redis cache enabled but redis.addr not configured")
		}
		
		hybridConfig.RedisConfig = &storage.RedisConfig{
			Addr:     config.Redis.Addr,
			Password: config.Redis.Password,
			DB:       config.Redis.DB,
			PoolSize: config.Redis.PoolSize,
		}
		
		if autoUpgraded {
			fmt.Printf("✅ Auto-upgraded cache to Redis (multi-node support enabled)\n")
		}
	}
	
	// 如果启用持久化
	if config.Hybrid.EnablePersistent {
		// 优先使用 JSON 文件存储
		if config.Hybrid.JSON.FilePath != "" {
			hybridConfig.JSONConfig = &storage.JSONStorageConfig{
				FilePath:     config.Hybrid.JSON.FilePath,
				AutoSave:     config.Hybrid.JSON.AutoSave,
				SaveInterval: time.Duration(config.Hybrid.JSON.SaveInterval) * time.Second,
			}
		} else if config.Hybrid.Remote.GRPC.Address != "" {
			// 使用远程存储
			hybridConfig.RemoteConfig = &storage.RemoteStorageConfig{
				GRPCAddress: config.Hybrid.Remote.GRPC.Address,
				Timeout:     time.Duration(config.Hybrid.Remote.GRPC.Timeout) * time.Second,
				MaxRetries:  config.Hybrid.Remote.GRPC.MaxRetries,
			}
		}
		// 如果都没配置，factory 会使用默认的 JSON 存储
	}
	
	return factory.CreateStorage(storage.StorageTypeHybrid, hybridConfig)
}

