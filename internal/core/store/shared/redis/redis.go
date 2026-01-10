// Package redis 提供 Redis 存储实现
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"tunnox-core/internal/core/store"
)

// =============================================================================
// RedisStore Redis 存储实现
// =============================================================================

// RedisStore Redis 存储实现
type RedisStore[K comparable, V any] struct {
	client    *redis.Client
	keyPrefix string
	metrics   *store.StoreMetrics
}

// NewRedisStore 创建 Redis 存储
func NewRedisStore[K comparable, V any](client *redis.Client, keyPrefix string) *RedisStore[K, V] {
	return &RedisStore[K, V]{
		client:    client,
		keyPrefix: keyPrefix,
		metrics:   store.NewStoreMetrics(),
	}
}

// NewRedisStoreFromConfig 从配置创建 Redis 存储
func NewRedisStoreFromConfig[K comparable, V any](cfg *store.RedisConfig, keyPrefix string) (*RedisStore[K, V], error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		MaxRetries:   cfg.MaxRetries,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return NewRedisStore[K, V](client, keyPrefix), nil
}

// buildKey 构建 Redis 键
func (s *RedisStore[K, V]) buildKey(key K) string {
	return fmt.Sprintf("%s%v", s.keyPrefix, key)
}

// serialize 序列化值
func (s *RedisStore[K, V]) serialize(value V) ([]byte, error) {
	return json.Marshal(value)
}

// deserialize 反序列化值
func (s *RedisStore[K, V]) deserialize(data []byte) (V, error) {
	var value V
	err := json.Unmarshal(data, &value)
	return value, err
}

// Get 获取值
func (s *RedisStore[K, V]) Get(ctx context.Context, key K) (V, error) {
	start := time.Now()
	var zero V

	data, err := s.client.Get(ctx, s.buildKey(key)).Bytes()
	if err != nil {
		if err == redis.Nil {
			s.metrics.RecordGet(time.Since(start), store.ErrNotFound)
			return zero, store.ErrNotFound
		}
		s.metrics.RecordGet(time.Since(start), err)
		return zero, store.NewStoreError("redis", "Get", s.buildKey(key), err)
	}

	value, err := s.deserialize(data)
	if err != nil {
		s.metrics.RecordGet(time.Since(start), err)
		return zero, store.NewStoreError("redis", "Get", s.buildKey(key), store.ErrDeserializationFailed)
	}

	s.metrics.RecordGet(time.Since(start), nil)
	return value, nil
}

// Set 设置值
func (s *RedisStore[K, V]) Set(ctx context.Context, key K, value V) error {
	start := time.Now()

	data, err := s.serialize(value)
	if err != nil {
		s.metrics.RecordSet(time.Since(start), err)
		return store.NewStoreError("redis", "Set", s.buildKey(key), store.ErrSerializationFailed)
	}

	err = s.client.Set(ctx, s.buildKey(key), data, 0).Err()
	s.metrics.RecordSet(time.Since(start), err)
	if err != nil {
		return store.NewStoreError("redis", "Set", s.buildKey(key), err)
	}
	return nil
}

// Delete 删除值
func (s *RedisStore[K, V]) Delete(ctx context.Context, key K) error {
	start := time.Now()

	err := s.client.Del(ctx, s.buildKey(key)).Err()
	s.metrics.RecordDelete(time.Since(start), err)
	if err != nil {
		return store.NewStoreError("redis", "Delete", s.buildKey(key), err)
	}
	return nil
}

// Exists 检查键是否存在
func (s *RedisStore[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	result, err := s.client.Exists(ctx, s.buildKey(key)).Result()
	if err != nil {
		return false, store.NewStoreError("redis", "Exists", s.buildKey(key), err)
	}
	return result > 0, nil
}

// SetWithTTL 设置值并指定 TTL
func (s *RedisStore[K, V]) SetWithTTL(ctx context.Context, key K, value V, ttl time.Duration) error {
	start := time.Now()

	data, err := s.serialize(value)
	if err != nil {
		s.metrics.RecordSet(time.Since(start), err)
		return store.NewStoreError("redis", "SetWithTTL", s.buildKey(key), store.ErrSerializationFailed)
	}

	err = s.client.Set(ctx, s.buildKey(key), data, ttl).Err()
	s.metrics.RecordSet(time.Since(start), err)
	if err != nil {
		return store.NewStoreError("redis", "SetWithTTL", s.buildKey(key), err)
	}
	return nil
}

// GetTTL 获取剩余 TTL
func (s *RedisStore[K, V]) GetTTL(ctx context.Context, key K) (time.Duration, error) {
	ttl, err := s.client.TTL(ctx, s.buildKey(key)).Result()
	if err != nil {
		return 0, store.NewStoreError("redis", "GetTTL", s.buildKey(key), err)
	}
	if ttl < 0 {
		if ttl == -2 {
			return 0, store.ErrNotFound
		}
		return -1, nil // 永不过期
	}
	return ttl, nil
}

// Refresh 刷新 TTL
func (s *RedisStore[K, V]) Refresh(ctx context.Context, key K, ttl time.Duration) error {
	ok, err := s.client.Expire(ctx, s.buildKey(key), ttl).Result()
	if err != nil {
		return store.NewStoreError("redis", "Refresh", s.buildKey(key), err)
	}
	if !ok {
		return store.ErrNotFound
	}
	return nil
}

// BatchGet 批量获取
func (s *RedisStore[K, V]) BatchGet(ctx context.Context, keys []K) (map[K]V, error) {
	if len(keys) == 0 {
		return map[K]V{}, nil
	}

	// 构建 Redis 键
	redisKeys := make([]string, len(keys))
	keyMap := make(map[string]K, len(keys))
	for i, key := range keys {
		rkey := s.buildKey(key)
		redisKeys[i] = rkey
		keyMap[rkey] = key
	}

	// 使用 Pipeline 批量获取
	pipe := s.client.Pipeline()
	cmds := make([]*redis.StringCmd, len(redisKeys))
	for i, rkey := range redisKeys {
		cmds[i] = pipe.Get(ctx, rkey)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, store.NewStoreError("redis", "BatchGet", "", err)
	}

	result := make(map[K]V, len(keys))
	for i, cmd := range cmds {
		data, err := cmd.Bytes()
		if err != nil {
			continue // 跳过不存在的键
		}
		value, err := s.deserialize(data)
		if err != nil {
			// 记录反序列化失败的详细信息
			fmt.Printf("RedisStore.BatchGet: failed to deserialize key %v: %v, raw data: %s\n",
				keyMap[redisKeys[i]], err, string(data))
			continue
		}
		result[keyMap[redisKeys[i]]] = value
	}

	return result, nil
}

// BatchSet 批量设置
func (s *RedisStore[K, V]) BatchSet(ctx context.Context, items map[K]V) error {
	if len(items) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()
	for key, value := range items {
		data, err := s.serialize(value)
		if err != nil {
			return store.NewStoreError("redis", "BatchSet", s.buildKey(key), store.ErrSerializationFailed)
		}
		pipe.Set(ctx, s.buildKey(key), data, 0)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return store.NewStoreError("redis", "BatchSet", "", err)
	}
	return nil
}

// BatchDelete 批量删除
func (s *RedisStore[K, V]) BatchDelete(ctx context.Context, keys []K) error {
	if len(keys) == 0 {
		return nil
	}

	redisKeys := make([]string, len(keys))
	for i, key := range keys {
		redisKeys[i] = s.buildKey(key)
	}

	err := s.client.Del(ctx, redisKeys...).Err()
	if err != nil {
		return store.NewStoreError("redis", "BatchDelete", "", err)
	}
	return nil
}

// SetNX 仅当键不存在时设置
func (s *RedisStore[K, V]) SetNX(ctx context.Context, key K, value V) (bool, error) {
	data, err := s.serialize(value)
	if err != nil {
		return false, store.NewStoreError("redis", "SetNX", s.buildKey(key), store.ErrSerializationFailed)
	}

	ok, err := s.client.SetNX(ctx, s.buildKey(key), data, 0).Result()
	if err != nil {
		return false, store.NewStoreError("redis", "SetNX", s.buildKey(key), err)
	}
	return ok, nil
}

// SetNXWithTTL 仅当键不存在时设置并指定 TTL
func (s *RedisStore[K, V]) SetNXWithTTL(ctx context.Context, key K, value V, ttl time.Duration) (bool, error) {
	data, err := s.serialize(value)
	if err != nil {
		return false, store.NewStoreError("redis", "SetNXWithTTL", s.buildKey(key), store.ErrSerializationFailed)
	}

	ok, err := s.client.SetNX(ctx, s.buildKey(key), data, ttl).Result()
	if err != nil {
		return false, store.NewStoreError("redis", "SetNXWithTTL", s.buildKey(key), err)
	}
	return ok, nil
}

// Ping 健康检查
func (s *RedisStore[K, V]) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

// Close 关闭连接
func (s *RedisStore[K, V]) Close() error {
	return s.client.Close()
}

// Pipeline 获取管道
func (s *RedisStore[K, V]) Pipeline() store.Pipeline[K, V] {
	return &redisPipeline[K, V]{
		pipe:      s.client.Pipeline(),
		store:     s,
		keyPrefix: s.keyPrefix,
	}
}

// GetMetrics 获取指标
func (s *RedisStore[K, V]) GetMetrics() *store.StoreMetrics {
	return s.metrics
}

// GetClient 获取底层 Redis 客户端
func (s *RedisStore[K, V]) GetClient() *redis.Client {
	return s.client
}

// =============================================================================
// Redis Pipeline 实现
// =============================================================================

type redisPipeline[K comparable, V any] struct {
	pipe      redis.Pipeliner
	store     *RedisStore[K, V]
	keyPrefix string
	futures   []*store.FutureResult[V]
	getCmds   []*redis.StringCmd
}

func (p *redisPipeline[K, V]) Get(ctx context.Context, key K) *store.FutureResult[V] {
	future := &store.FutureResult[V]{}
	cmd := p.pipe.Get(ctx, p.store.buildKey(key))
	p.futures = append(p.futures, future)
	p.getCmds = append(p.getCmds, cmd)
	return future
}

func (p *redisPipeline[K, V]) Set(ctx context.Context, key K, value V, ttl time.Duration) {
	data, _ := p.store.serialize(value)
	p.pipe.Set(ctx, p.store.buildKey(key), data, ttl)
}

func (p *redisPipeline[K, V]) Delete(ctx context.Context, key K) {
	p.pipe.Del(ctx, p.store.buildKey(key))
}

func (p *redisPipeline[K, V]) SAdd(ctx context.Context, key K, member string) {
	p.pipe.SAdd(ctx, p.store.buildKey(key), member)
}

func (p *redisPipeline[K, V]) SRem(ctx context.Context, key K, member string) {
	p.pipe.SRem(ctx, p.store.buildKey(key), member)
}

func (p *redisPipeline[K, V]) Exec(ctx context.Context) error {
	_, err := p.pipe.Exec(ctx)

	// 处理 Get 结果
	for i, cmd := range p.getCmds {
		data, cmdErr := cmd.Bytes()
		if cmdErr != nil {
			p.futures[i].SetResult(*new(V), store.ErrNotFound)
			continue
		}
		value, deserErr := p.store.deserialize(data)
		if deserErr != nil {
			p.futures[i].SetResult(*new(V), deserErr)
			continue
		}
		p.futures[i].SetResult(value, nil)
	}

	if err != nil && err != redis.Nil {
		return err
	}
	return nil
}

// =============================================================================
// RedisSetStore Redis 集合存储
// =============================================================================

// RedisSetStore Redis 集合存储
type RedisSetStore struct {
	client    *redis.Client
	keyPrefix string
}

// NewRedisSetStore 创建 Redis 集合存储
func NewRedisSetStore(client *redis.Client, keyPrefix string) *RedisSetStore {
	return &RedisSetStore{
		client:    client,
		keyPrefix: keyPrefix,
	}
}

func (s *RedisSetStore) buildKey(key string) string {
	return s.keyPrefix + key
}

// Add 向集合添加元素
func (s *RedisSetStore) Add(ctx context.Context, key string, value string) error {
	return s.client.SAdd(ctx, s.buildKey(key), value).Err()
}

// Remove 从集合移除元素
func (s *RedisSetStore) Remove(ctx context.Context, key string, value string) error {
	return s.client.SRem(ctx, s.buildKey(key), value).Err()
}

// Contains 检查元素是否在集合中
func (s *RedisSetStore) Contains(ctx context.Context, key string, value string) (bool, error) {
	return s.client.SIsMember(ctx, s.buildKey(key), value).Result()
}

// Members 获取集合所有成员
func (s *RedisSetStore) Members(ctx context.Context, key string) ([]string, error) {
	return s.client.SMembers(ctx, s.buildKey(key)).Result()
}

// Size 获取集合大小
func (s *RedisSetStore) Size(ctx context.Context, key string) (int64, error) {
	return s.client.SCard(ctx, s.buildKey(key)).Result()
}

// Pipeline 获取管道
func (s *RedisSetStore) Pipeline() store.SetPipeline[string, string] {
	return &redisSetPipeline{
		pipe:      s.client.Pipeline(),
		keyPrefix: s.keyPrefix,
	}
}

// Close 关闭连接
func (s *RedisSetStore) Close() error {
	return s.client.Close()
}

// =============================================================================
// Redis Set Pipeline 实现
// =============================================================================

type redisSetPipeline struct {
	pipe      redis.Pipeliner
	keyPrefix string
}

func (p *redisSetPipeline) SAdd(ctx context.Context, key string, member string) {
	p.pipe.SAdd(ctx, p.keyPrefix+key, member)
}

func (p *redisSetPipeline) SRem(ctx context.Context, key string, member string) {
	p.pipe.SRem(ctx, p.keyPrefix+key, member)
}

func (p *redisSetPipeline) Exec(ctx context.Context) error {
	_, err := p.pipe.Exec(ctx)
	return err
}
