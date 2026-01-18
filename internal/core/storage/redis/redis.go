package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage/types"

	"github.com/redis/go-redis/v9"
)

// Client 是 Redis 客户端类型别名
type Client = redis.Client

// ErrRedisNil 是 Redis nil 错误的引用
var ErrRedisNil = redis.Nil

// Config Redis配置
type Config struct {
	Addr     string `json:"addr" yaml:"addr"`           // Redis地址，如 "localhost:6379"
	Password string `json:"password" yaml:"password"`   // Redis密码
	DB       int    `json:"db" yaml:"db"`               // 数据库编号
	PoolSize int    `json:"pool_size" yaml:"pool_size"` // 连接池大小
}

// Storage Redis存储实现
type Storage struct {
	client *redis.Client
	ctx    context.Context
	dispose.Dispose
}

// New 创建新的Redis存储
func New(parentCtx context.Context, config *Config) (*Storage, error) {
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

	storage := &Storage{
		client: client,
		ctx:    parentCtx,
	}
	storage.SetCtx(parentCtx, storage.onClose)

	dispose.Infof("RedisStorage: connected to Redis at %s, DB: %d", config.Addr, config.DB)
	return storage, nil
}

// onClose 资源释放回调
func (r *Storage) onClose() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// Set 设置键值对
// 注意：如果 value 已经是 string 或 []byte，直接使用，避免双重 JSON 编码
func (r *Storage) Set(key string, value interface{}, ttl time.Duration) error {
	// 处理值：如果已经是字符串/字节，直接使用；否则序列化
	var data []byte
	switch v := value.(type) {
	case string:
		// 值已经是字符串，直接使用（常见于已序列化的 JSON）
		data = []byte(v)
	case []byte:
		// 值已经是字节数组，直接使用
		data = v
	default:
		// 其他类型，序列化为 JSON
		var err error
		data, err = json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to marshal value: %w", err)
		}
	}

	// 设置到Redis
	var result *redis.StatusCmd
	if ttl > 0 {
		result = r.client.Set(r.ctx, key, data, ttl)
	} else {
		result = r.client.Set(r.ctx, key, data, 0)
	}

	if result.Err() != nil {
		dispose.Errorf("RedisStorage.Set: failed to set key %s: %v", key, result.Err())
		return fmt.Errorf("failed to set key %s: %w", key, result.Err())
	}

	return nil
}

// Get 获取值
// 注意：返回原始字符串（与 Set 对称），调用方自行反序列化
func (r *Storage) Get(key string) (interface{}, error) {
	result := r.client.Get(r.ctx, key)
	if result.Err() != nil {
		if result.Err() == redis.Nil {
			dispose.Debugf("RedisStorage.Get: key %s not found", key)
			return nil, types.ErrKeyNotFound
		}
		dispose.Errorf("RedisStorage.Get: failed to get key %s: %v", key, result.Err())
		return nil, fmt.Errorf("failed to get key %s: %w", key, result.Err())
	}

	// 直接返回字符串，与 Set 方法对称
	// 调用方传入 string，Get 返回 string
	return result.Val(), nil
}

// Delete 删除键
func (r *Storage) Delete(key string) error {
	result := r.client.Del(r.ctx, key)
	if result.Err() != nil {
		dispose.Errorf("RedisStorage.Delete: failed to delete key %s: %v", key, result.Err())
		return fmt.Errorf("failed to delete key %s: %w", key, result.Err())
	}

	return nil
}

// Exists 检查键是否存在
func (r *Storage) Exists(key string) (bool, error) {

	result := r.client.Exists(r.ctx, key)
	if result.Err() != nil {
		dispose.Errorf("RedisStorage.Exists: failed to check key %s: %v", key, result.Err())
		return false, fmt.Errorf("failed to check key %s: %w", key, result.Err())
	}

	exists := result.Val() > 0
	return exists, nil
}

func (r *Storage) Client() *redis.Client {
	return r.client
}
