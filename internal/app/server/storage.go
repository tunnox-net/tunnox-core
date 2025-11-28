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

	// ✅ 智能持久化策略：
	// - 单节点（无Redis）→ 自动启用JSON持久化
	// - 多节点（有Redis）→ JSON持久化自动禁用（避免写冲突），但远程存储可用
	enablePersistent := config.Hybrid.EnablePersistent

	if cacheType == "redis" {
		// 多节点模式（有Redis）→ JSON持久化有写冲突风险，需要禁用
		// 但远程存储（gRPC）本身支持多节点，可以保留
		hasRemoteStorage := config.Hybrid.Remote.GRPC.Address != ""

		if hasRemoteStorage {
			// 有远程存储配置，保留
			fmt.Printf("✅ Multi-node mode detected (Redis cache enabled)\n")
			fmt.Printf("   → Using remote gRPC persistent storage: %s\n", config.Hybrid.Remote.GRPC.Address)
			fmt.Printf("   → Remote storage supports multi-node concurrent writes\n")
			// 清空JSON配置（多节点不能用JSON）
			config.Hybrid.JSON.FilePath = ""
		} else {
			// 默认：多节点模式不持久化
			fmt.Printf("⚠️  Multi-node mode detected (Redis cache enabled)\n")
			fmt.Printf("   → JSON persistent storage DISABLED (avoid multi-node write conflicts)\n")
			fmt.Printf("   → Data synchronized via Redis cache only (not persisted to disk)\n")
			fmt.Printf("   → To enable persistence in multi-node, configure remote gRPC storage\n")
			// 禁用JSON持久化
			config.Hybrid.JSON.FilePath = ""
			enablePersistent = false
		}

	} else if cacheType == "memory" {
		// 单节点模式（Memory cache）→ 自动启用JSON持久化
		if config.Hybrid.JSON.FilePath == "" && config.Hybrid.Remote.GRPC.Address == "" {
			// 没有配置持久化路径，使用默认JSON
			if !enablePersistent {
				enablePersistent = true
			}
			config.Hybrid.JSON.FilePath = "data/tunnox-data.json" // 相对路径，避免权限问题
			config.Hybrid.JSON.AutoSave = true
			if config.Hybrid.JSON.SaveInterval == 0 {
				config.Hybrid.JSON.SaveInterval = 60
			}
			fmt.Printf("✅ Single-node mode detected (Memory cache)\n")
			fmt.Printf("   → Auto-enabled JSON persistent storage: %s\n", config.Hybrid.JSON.FilePath)
			fmt.Printf("   → Auto-save enabled (interval: %ds)\n", config.Hybrid.JSON.SaveInterval)
		} else if config.Hybrid.JSON.FilePath != "" {
			// 用户配置了JSON路径
			fmt.Printf("✅ Single-node mode with JSON persistent storage\n")
			fmt.Printf("   → JSON file: %s\n", config.Hybrid.JSON.FilePath)
		} else if config.Hybrid.Remote.GRPC.Address != "" {
			// 用户配置了远程存储
			fmt.Printf("✅ Single-node mode with remote persistent storage\n")
			fmt.Printf("   → Remote gRPC: %s\n", config.Hybrid.Remote.GRPC.Address)
		}
	}

	// 准备混合存储配置
	hybridConfig := &storage.HybridStorageConfig{
		CacheType:        cacheType,
		EnablePersistent: enablePersistent,
		HybridConfig: &storage.HybridConfig{
			PersistentPrefixes: config.Hybrid.PersistentPrefixes,
			EnablePersistent:   enablePersistent,
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

	// 如果启用持久化，配置具体的persistent实现
	if enablePersistent {
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
		// 如果都没配置，factory 会使用 NullPersistentStorage
	}
	// 多节点模式已经在上面输出提示信息了

	return factory.CreateStorage(storage.StorageTypeHybrid, hybridConfig)
}
