package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/core/dispose"

	"github.com/redis/go-redis/v9"
)

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

	dispose.Infof("RedisStorage.Set: stored key %s, value type: %T, ttl: %v", key, value, ttl)
	return nil
}

// Get 获取值
func (r *RedisStorage) Get(key string) (interface{}, error) {
	dispose.Infof("RedisStorage.Get: retrieving key %s", key)

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

	dispose.Infof("RedisStorage.Get: successfully retrieved key %s, value type: %T", key, value)
	return value, nil
}

// Delete 删除键
func (r *RedisStorage) Delete(key string) error {
	result := r.client.Del(r.ctx, key)
	if result.Err() != nil {
		dispose.Errorf("RedisStorage.Delete: failed to delete key %s: %v", key, result.Err())
		return fmt.Errorf("failed to delete key %s: %w", key, result.Err())
	}

	dispose.Infof("RedisStorage.Delete: deleted key %s", key)
	return nil
}

// Exists 检查键是否存在
func (r *RedisStorage) Exists(key string) (bool, error) {
	dispose.Infof("RedisStorage.Exists: checking key %s", key)

	result := r.client.Exists(r.ctx, key)
	if result.Err() != nil {
		dispose.Errorf("RedisStorage.Exists: failed to check key %s: %v", key, result.Err())
		return false, fmt.Errorf("failed to check key %s: %w", key, result.Err())
	}

	exists := result.Val() > 0
	dispose.Infof("RedisStorage.Exists: key %s exists: %v", key, exists)
	return exists, nil
}

// SetList 设置列表
func (r *RedisStorage) SetList(key string, values []interface{}, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()

	// 删除现有列表
	r.client.Del(ctx, key)

	// 序列化并添加每个值
	for _, value := range values {
		jsonData, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if err := r.client.RPush(ctx, key, jsonData).Err(); err != nil {
			return err
		}
	}

	// 设置过期时间
	if ttl > 0 {
		if err := r.client.Expire(ctx, key, ttl).Err(); err != nil {
			return err
		}
	}

	dispose.Infof("RedisStorage.SetList: set list key %s with %d items, ttl: %v", key, len(values), ttl)
	return nil
}

// GetList 获取列表
func (r *RedisStorage) GetList(key string) ([]interface{}, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()

	result := r.client.LRange(ctx, key, 0, -1)
	if result.Err() != nil {
		if result.Err() == redis.Nil {
			return nil, ErrKeyNotFound
		}
		return nil, result.Err()
	}

	values := make([]interface{}, 0, len(result.Val()))
	for _, jsonData := range result.Val() {
		var value interface{}
		if err := json.Unmarshal([]byte(jsonData), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}

	dispose.Infof("RedisStorage.GetList: retrieved list key %s with %d items", key, len(values))
	return values, nil
}

// AppendToList 追加到列表
func (r *RedisStorage) AppendToList(key string, value interface{}) error {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if err := r.client.RPush(ctx, key, jsonData).Err(); err != nil {
		return err
	}

	// 如果键是新创建的，设置默认过期时间
	if r.client.LLen(ctx, key).Val() == 1 {
		if err := r.client.Expire(ctx, key, constants.DefaultDataTTL).Err(); err != nil {
			return err
		}
	}

	dispose.Infof("RedisStorage.AppendToList: appended to list key %s", key)
	return nil
}

// RemoveFromList 从列表中移除
func (r *RedisStorage) RemoveFromList(key string, value interface{}) error {
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// 移除所有匹配的值
	if err := r.client.LRem(ctx, key, 0, jsonData).Err(); err != nil {
		return err
	}

	dispose.Infof("RedisStorage.RemoveFromList: removed from list key %s", key)
	return nil
}

// SetHash 设置哈希字段
func (r *RedisStorage) SetHash(key string, field string, value interface{}) error {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if err := r.client.HSet(ctx, key, field, jsonData).Err(); err != nil {
		return err
	}

	// 如果键是新创建的，设置默认过期时间
	if r.client.HLen(ctx, key).Val() == 1 {
		if err := r.client.Expire(ctx, key, constants.DefaultDataTTL).Err(); err != nil {
			return err
		}
	}

	dispose.Infof("RedisStorage.SetHash: set hash field %s:%s", key, field)
	return nil
}

// GetHash 获取哈希字段
func (r *RedisStorage) GetHash(key string, field string) (interface{}, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	result := r.client.HGet(ctx, key, field)
	if result.Err() != nil {
		if result.Err() == redis.Nil {
			return nil, ErrKeyNotFound
		}
		return nil, result.Err()
	}

	jsonData, err := result.Bytes()
	if err != nil {
		return nil, err
	}

	var value interface{}
	if err := json.Unmarshal(jsonData, &value); err != nil {
		return nil, err
	}

	dispose.Infof("RedisStorage.GetHash: retrieved hash field %s:%s", key, field)
	return value, nil
}

// GetAllHash 获取所有哈希字段
func (r *RedisStorage) GetAllHash(key string) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()

	result := r.client.HGetAll(ctx, key)
	if result.Err() != nil {
		return nil, result.Err()
	}

	hash := make(map[string]interface{})
	for field, jsonData := range result.Val() {
		var value interface{}
		if err := json.Unmarshal([]byte(jsonData), &value); err != nil {
			return nil, err
		}
		hash[field] = value
	}

	dispose.Infof("RedisStorage.GetAllHash: retrieved all hash fields for key %s", key)
	return hash, nil
}

// DeleteHash 删除哈希字段
func (r *RedisStorage) DeleteHash(key string, field string) error {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	if err := r.client.HDel(ctx, key, field).Err(); err != nil {
		return err
	}

	dispose.Infof("RedisStorage.DeleteHash: deleted hash field %s:%s", key, field)
	return nil
}

// Incr 递增计数器
func (r *RedisStorage) Incr(key string) (int64, error) {
	return r.IncrBy(key, 1)
}

// IncrBy 按指定值递增
func (r *RedisStorage) IncrBy(key string, value int64) (int64, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	result := r.client.IncrBy(ctx, key, value)
	if result.Err() != nil {
		return 0, result.Err()
	}

	// 如果键是新创建的，设置默认过期时间
	if result.Val() == value {
		if err := r.client.Expire(ctx, key, constants.DefaultDataTTL).Err(); err != nil {
			return result.Val(), err
		}
	}

	dispose.Infof("RedisStorage.IncrBy: incremented key %s by %d, new value: %d", key, value, result.Val())
	return result.Val(), nil
}

// SetExpiration 设置过期时间
func (r *RedisStorage) SetExpiration(key string, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	if ttl > 0 {
		if err := r.client.Expire(ctx, key, ttl).Err(); err != nil {
			return err
		}
	} else {
		if err := r.client.Persist(ctx, key).Err(); err != nil {
			return err
		}
	}

	dispose.Infof("RedisStorage.SetExpiration: set expiration for key %s to %v", key, ttl)
	return nil
}

// GetExpiration 获取过期时间
func (r *RedisStorage) GetExpiration(key string) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	result := r.client.TTL(ctx, key)
	if result.Err() != nil {
		return 0, result.Err()
	}

	ttl := result.Val()
	if ttl == -1 {
		return 0, nil // 永不过期
	}
	if ttl == -2 {
		return 0, ErrKeyNotFound // 键不存在
	}

	dispose.Infof("RedisStorage.GetExpiration: key %s TTL: %v", key, ttl)
	return ttl, nil
}

// CleanupExpired 清理过期数据（Redis自动处理，这里只是日志）
func (r *RedisStorage) CleanupExpired() error {
	dispose.Infof("RedisStorage.CleanupExpired: Redis automatically handles expiration")
	return nil
}

// SetNX 原子设置，仅当键不存在时
func (r *RedisStorage) SetNX(key string, value interface{}, ttl time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	jsonData, err := json.Marshal(value)
	if err != nil {
		return false, err
	}

	var expiration time.Duration
	if ttl > 0 {
		expiration = ttl
	}

	result := r.client.SetNX(ctx, key, jsonData, expiration)
	if result.Err() != nil {
		return false, result.Err()
	}

	dispose.Infof("RedisStorage.SetNX: set key %s with NX flag, success: %v", key, result.Val())
	return result.Val(), nil
}

// CompareAndSwap 原子比较并交换
func (r *RedisStorage) CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()

	// 使用Lua脚本实现原子CAS操作
	script := `
		local key = KEYS[1]
		local old_value = ARGV[1]
		local new_value = ARGV[2]
		local ttl = tonumber(ARGV[3])
		
		local current_value = redis.call('GET', key)
		
		if current_value == false then
			if old_value == '' then
				redis.call('SET', key, new_value)
				if ttl > 0 then
					redis.call('EXPIRE', key, ttl)
				end
				return 1
			else
				return 0
			end
		end
		
		if current_value == old_value then
			redis.call('SET', key, new_value)
			if ttl > 0 then
				redis.call('EXPIRE', key, ttl)
			end
			return 1
		else
			return 0
		end
	`

	oldValueStr := ""
	if oldValue != nil {
		oldValueBytes, err := json.Marshal(oldValue)
		if err != nil {
			return false, err
		}
		oldValueStr = string(oldValueBytes)
	}

	newValueBytes, err := json.Marshal(newValue)
	if err != nil {
		return false, err
	}

	var ttlSeconds int64
	if ttl > 0 {
		ttlSeconds = int64(ttl.Seconds())
	}

	result := r.client.Eval(ctx, script, []string{key}, oldValueStr, string(newValueBytes), ttlSeconds)
	if result.Err() != nil {
		return false, result.Err()
	}

	success := result.Val().(int64) == 1
	dispose.Infof("RedisStorage.CompareAndSwap: CAS operation for key %s, success: %v", key, success)
	return success, nil
}

// Watch 监听键变化（简化实现）
func (r *RedisStorage) Watch(key string, callback func(interface{})) error {
	// 简化实现：立即执行一次回调
	if value, err := r.Get(key); err == nil {
		callback(value)
	}
	return nil
}

// Unwatch 取消监听
func (r *RedisStorage) Unwatch(key string) error {
	// 简化实现：无操作
	return nil
}

// Close 关闭存储
func (r *RedisStorage) Close() error {
	return r.Dispose.Close()
}

// GetClient 获取Redis客户端（用于高级操作）
func (r *RedisStorage) GetClient() *redis.Client {
	return r.client
}

// Ping 测试连接
func (r *RedisStorage) Ping() error {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	return r.client.Ping(ctx).Err()
}

// FlushDB 清空当前数据库
func (r *RedisStorage) FlushDB() error {
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()

	return r.client.FlushDB(ctx).Err()
}

// GetKeys 获取匹配模式的键
func (r *RedisStorage) GetKeys(pattern string) ([]string, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()

	result := r.client.Keys(ctx, pattern)
	if result.Err() != nil {
		return nil, result.Err()
	}

	return result.Val(), nil
}

// GetKeyCount 获取键数量
func (r *RedisStorage) GetKeyCount() (int64, error) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()

	result := r.client.DBSize(ctx)
	if result.Err() != nil {
		return 0, result.Err()
	}

	return result.Val(), nil
}
