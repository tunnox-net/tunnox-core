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
	// 准备混合存储配置
	hybridConfig := &storage.HybridStorageConfig{
		CacheType:        config.Hybrid.CacheType,
		EnablePersistent: config.Hybrid.EnablePersistent,
		HybridConfig: &storage.HybridConfig{
			PersistentPrefixes: config.Hybrid.PersistentPrefixes,
			EnablePersistent:   config.Hybrid.EnablePersistent,
		},
	}
	
	// 如果缓存类型是 Redis，提供 Redis 配置
	if config.Hybrid.CacheType == "redis" {
		hybridConfig.RedisConfig = &storage.RedisConfig{
			Addr:     config.Redis.Addr,
			Password: config.Redis.Password,
			DB:       config.Redis.DB,
			PoolSize: config.Redis.PoolSize,
		}
	}
	
	// 如果启用持久化，提供远程存储配置
	if config.Hybrid.EnablePersistent {
		hybridConfig.RemoteConfig = &storage.RemoteStorageConfig{
			GRPCAddress: config.Hybrid.Remote.GRPC.Address,
			Timeout:     time.Duration(config.Hybrid.Remote.GRPC.Timeout) * time.Second,
			MaxRetries:  config.Hybrid.Remote.GRPC.MaxRetries,
		}
	}
	
	return factory.CreateStorage(storage.StorageTypeHybrid, hybridConfig)
}

