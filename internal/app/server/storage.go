package server

import (
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// createStorage 根据配置创建存储
// 根据新的配置结构自动推断存储类型
func createStorage(factory *storage.StorageFactory, config *Config) (storage.Storage, error) {
	// 自动推断存储类型：
	// 1. storage.enabled -> 使用远程存储
	// 2. redis.enabled -> 使用 Redis 缓存 + 可选持久化
	// 3. persistence.enabled -> 使用内存缓存 + JSON 持久化
	// 4. 默认 -> 纯内存存储

	if config.Storage.Enabled {
		return createRemoteStorage(factory, config)
	}

	if config.Redis.Enabled {
		return createRedisStorage(factory, config)
	}

	if config.Persistence.Enabled {
		return createPersistentStorage(factory, config)
	}

	return createMemoryStorage(factory)
}

// createRemoteStorage 创建远程存储（连接 tunnox-storage 服务）
func createRemoteStorage(factory *storage.StorageFactory, config *Config) (storage.Storage, error) {
	corelog.Infof("Using Remote Storage, URL: %s", config.Storage.URL)

	// 本地缓存类型（用于持久化数据的缓存）
	cacheType := "memory"

	// 共享缓存配置（用于跨节点共享数据）
	var sharedCacheConfig *storage.RedisConfig
	if config.Redis.Enabled {
		sharedCacheConfig = &storage.RedisConfig{
			Addr:     config.Redis.Addr,
			Password: config.Redis.Password,
			DB:       config.Redis.DB,
			PoolSize: 10,
		}
		corelog.Infof("Remote Storage config: Local Cache=Memory, Shared Cache=Redis(%s)", config.Redis.Addr)
	} else {
		corelog.Infof("Remote Storage config: Cache=Memory (single node mode)")
	}

	hybridConfig := &storage.HybridStorageConfig{
		CacheType:         cacheType,
		EnablePersistent:  true,
		HybridConfig:      storage.DefaultHybridConfig(),
		SharedCacheConfig: sharedCacheConfig,
		RemoteConfig: &storage.RemoteStorageConfig{
			GRPCAddress: config.Storage.URL,
			Timeout:     time.Duration(config.Storage.Timeout) * time.Second,
			MaxRetries:  3,
		},
	}
	hybridConfig.HybridConfig.EnablePersistent = true

	return factory.CreateStorage(storage.StorageTypeHybrid, hybridConfig)
}

// createRedisStorage 创建 Redis 存储（集群模式）
func createRedisStorage(factory *storage.StorageFactory, config *Config) (storage.Storage, error) {
	corelog.Infof("Using Redis Storage (Cluster Mode), Addr: %s", config.Redis.Addr)

	// 集群模式下不启用 JSON 持久化（避免多节点写冲突）
	// 如果需要持久化，应该使用远程存储
	enablePersistent := false
	if config.Storage.Enabled {
		corelog.Infof("Redis Storage config: Cache=Redis, Persistent=Remote(%s)", config.Storage.URL)
		enablePersistent = true
	} else {
		corelog.Infof("Redis Storage config: Cache=Redis, Persistent=Disabled (cluster mode)")
	}

	redisConfig := &storage.RedisConfig{
		Addr:     config.Redis.Addr,
		Password: config.Redis.Password,
		DB:       config.Redis.DB,
		PoolSize: 10,
	}

	hybridConfig := &storage.HybridStorageConfig{
		CacheType:        "redis",
		EnablePersistent: enablePersistent,
		HybridConfig:     storage.DefaultHybridConfig(),
		RedisConfig:      redisConfig,
		// Redis 模式下，本地缓存和共享缓存都是 Redis，不需要单独配置 SharedCacheConfig
	}
	hybridConfig.HybridConfig.EnablePersistent = enablePersistent

	if enablePersistent {
		hybridConfig.RemoteConfig = &storage.RemoteStorageConfig{
			GRPCAddress: config.Storage.URL,
			Timeout:     time.Duration(config.Storage.Timeout) * time.Second,
			MaxRetries:  3,
		}
	}

	return factory.CreateStorage(storage.StorageTypeHybrid, hybridConfig)
}

// createPersistentStorage 创建持久化存储（单节点模式）
func createPersistentStorage(factory *storage.StorageFactory, config *Config) (storage.Storage, error) {
	corelog.Infof("Using Persistent Storage (Standalone Mode), Cache=Memory, Persistent=LocalJSON(%s)", config.Persistence.File)
	if config.Persistence.AutoSave {
		corelog.Infof("Persistent Storage auto-save enabled, interval: %ds", config.Persistence.SaveInterval)
	}

	hybridConfig := &storage.HybridStorageConfig{
		CacheType:        "memory",
		EnablePersistent: true,
		HybridConfig:     storage.DefaultHybridConfig(),
		JSONConfig: &storage.JSONStorageConfig{
			FilePath:     config.Persistence.File,
			AutoSave:     config.Persistence.AutoSave,
			SaveInterval: time.Duration(config.Persistence.SaveInterval) * time.Second,
		},
	}
	hybridConfig.HybridConfig.EnablePersistent = true

	return factory.CreateStorage(storage.StorageTypeHybrid, hybridConfig)
}

// createMemoryStorage 创建纯内存存储
func createMemoryStorage(factory *storage.StorageFactory) (storage.Storage, error) {
	corelog.Infof("Using Memory Storage (No Persistence), Cache=Memory")

	hybridConfig := &storage.HybridStorageConfig{
		CacheType:        "memory",
		EnablePersistent: false,
		HybridConfig:     storage.DefaultHybridConfig(),
	}
	hybridConfig.HybridConfig.EnablePersistent = false

	return factory.CreateStorage(storage.StorageTypeHybrid, hybridConfig)
}
