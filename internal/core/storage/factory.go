package storage

import (
	"context"
	"fmt"
	"tunnox-core/internal/utils"
)

// StorageType 存储类型
type StorageType string

const (
	StorageTypeMemory StorageType = "memory"
	StorageTypeRedis  StorageType = "redis"
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
	default:
		return nil, fmt.Errorf("unsupported storage type: %s", storageType)
	}
}

// createMemoryStorage 创建内存存储
func (f *StorageFactory) createMemoryStorage() (Storage, error) {
	storage := NewMemoryStorage(f.ctx)
	utils.Infof("StorageFactory: created memory storage")
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

	utils.Infof("StorageFactory: created Redis storage")
	return storage, nil
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
