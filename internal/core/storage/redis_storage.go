package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/core/dispose"

	"github.com/redis/go-redis/v9"
)

// RedisClient 是 Redis 客户端类型别名
type RedisClient = redis.Client

// ErrRedisNil 是 Redis nil 错误的引用
var ErrRedisNil = redis.Nil

// RedisConfig Redis配置
type RedisConfig struct {
	Addr     string `json:"addr" yaml:"addr"`           // Redis地址，如 "localhost:6379"
	Password string `json:"password" yaml:"password"`   // Redis密码
	DB       int    `json:"db" yaml:"db"`               // 数据库编号
	PoolSize int    `json:"pool_size" yaml:"pool_size"` // 连接池大小
}

// RedisStorage Redis存储实现
type RedisStorage struct {
	client *redis.Client
	ctx    context.Context
	dispose.Dispose
}

// NewRedisStorage 创建新的Redis存储
func NewRedisStorage(parentCtx context.Context, config *RedisConfig) (*RedisStorage, error) {
	if config == nil {
		return nil, fmt.Errorf("redis config is required")
	}

	// 设置默认值
	if config.PoolSize <= 0 {
		config.PoolSize = 10
	}

	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
		PoolSize: config.PoolSize,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(parentCtx, 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	storage := &RedisStorage{
		client: client,
		ctx:    parentCtx,
	}
	storage.SetCtx(parentCtx, storage.onClose)

	dispose.Infof("RedisStorage: connected to Redis at %s, DB: %d", config.Addr, config.DB)
	return storage, nil
}

// onClose 资源释放回调
func (r *RedisStorage) onClose() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Set 设置键值对
func (r *RedisStorage) Set(key string, value interface{}, ttl time.Duration) error {
	// 序列化值
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// 设置到Redis
	var result *redis.StatusCmd
	if ttl > 0 {
		result = r.client.Set(r.ctx, key, data, ttl)
	} else {
		result = r.client.Set(r.ctx, key, data, 0)
	}

	if result.Err() != nil {
		dispose.Errorf("RedisStorage.Set: failed to set key %s: %v", key, err)
		return fmt.Errorf("failed to set key %s: %w", key, result.Err())
	}

	return nil
}

// Get 获取值
func (r *RedisStorage) Get(key string) (interface{}, error) {
	result := r.client.Get(r.ctx, key)
	if result.Err() != nil {
		if result.Err() == redis.Nil {
			dispose.Debugf("RedisStorage.Get: key %s not found", key)
			return nil, ErrKeyNotFound
		}
		dispose.Errorf("RedisStorage.Get: failed to get key %s: %v", key, result.Err())
		return nil, fmt.Errorf("failed to get key %s: %w", key, result.Err())
	}

	// 获取字节数据
	data, err := result.Bytes()
	if err != nil {
		dispose.Errorf("RedisStorage.Get: failed to get bytes for key %s: %v", key, err)
		return nil, fmt.Errorf("failed to get bytes for key %s: %w", key, err)
	}

	// 反序列化值
	var value interface{}
	if err := json.Unmarshal(data, &value); err != nil {
		dispose.Errorf("RedisStorage.Get: failed to unmarshal value for key %s: %v", key, err)
		return nil, fmt.Errorf("failed to unmarshal value for key %s: %w", key, err)
	}

	return value, nil
}

// Delete 删除键
func (r *RedisStorage) Delete(key string) error {
	result := r.client.Del(r.ctx, key)
	if result.Err() != nil {
		dispose.Errorf("RedisStorage.Delete: failed to delete key %s: %v", key, result.Err())
		return fmt.Errorf("failed to delete key %s: %w", key, result.Err())
	}

	return nil
}

// Exists 检查键是否存在
func (r *RedisStorage) Exists(key string) (bool, error) {

	result := r.client.Exists(r.ctx, key)
	if result.Err() != nil {
		dispose.Errorf("RedisStorage.Exists: failed to check key %s: %v", key, result.Err())
		return false, fmt.Errorf("failed to check key %s: %w", key, result.Err())
	}

	exists := result.Val() > 0
	return exists, nil
}
