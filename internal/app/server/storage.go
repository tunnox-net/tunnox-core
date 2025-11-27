package server

import (
	"fmt"
	"time"
	
	"tunnox-core/internal/core/storage"
)

// createStorage 根据配置创建存储
// 注意：所有storage类型都统一使用HybridStorage架构，只是内部配置不同
func createStorage(factory *storage.StorageFactory, config *StorageConfig) (storage.Storage, error) {
	switch storage.StorageType(config.Type) {
	case storage.StorageTypeMemory:
		// memory类型：使用HybridStorage，cache=memory，不启用persistent
		return createHybridStorageWithCacheType(factory, config, "memory", false)
		
	case storage.StorageTypeRedis:
		// redis类型：使用HybridStorage，cache=redis，不启用persistent
		// 这样可以保证数据分类逻辑（runtime vs persistent）正常工作
		return createHybridStorageWithCacheType(factory, config, "redis", false)
		
	case storage.StorageTypeHybrid:
		// hybrid类型：显式配置，完全由用户控制
		return createHybridStorage(factory, config)
		
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", config.Type)
	}
}

// createHybridStorageWithCacheType 使用指定cache类型创建HybridStorage（简化配置）
func createHybridStorageWithCacheType(factory *storage.StorageFactory, config *StorageConfig, cacheType string, enablePersistent bool) (storage.Storage, error) {
	hybridConfig := &storage.HybridStorageConfig{
		CacheType:        cacheType,
		EnablePersistent: enablePersistent,
		HybridConfig:     storage.DefaultHybridConfig(), // 使用默认的数据分类前缀
	}
	
	// 如果cache使用redis，提供redis配置
	if cacheType == "redis" {
		if config.Redis.Addr == "" {
			return nil, fmt.Errorf("redis storage type requires redis.addr configuration")
		}
		
		hybridConfig.RedisConfig = &storage.RedisConfig{
			Addr:     config.Redis.Addr,
			Password: config.Redis.Password,
			DB:       config.Redis.DB,
			PoolSize: config.Redis.PoolSize,
		}
		
		fmt.Printf("✅ Using HybridStorage with Redis cache (addr=%s)\n", config.Redis.Addr)
	} else {
		fmt.Printf("✅ Using HybridStorage with Memory cache\n")
	}
	
	return factory.CreateStorage(storage.StorageTypeHybrid, hybridConfig)
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

