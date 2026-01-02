// Package embedded 提供内嵌 Redis (miniredis) 实现
package embedded

import (
	"context"
	"fmt"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"tunnox-core/internal/core/store"
	redisstore "tunnox-core/internal/core/store/shared/redis"
)

// =============================================================================
// EmbeddedRedis 内嵌 Redis 服务
// =============================================================================

// EmbeddedRedis 内嵌 Redis 服务（基于 miniredis）
// 用于单机模式，无需外部 Redis 依赖
type EmbeddedRedis struct {
	server *miniredis.Miniredis
	client *redis.Client
}

// NewEmbeddedRedis 创建内嵌 Redis
func NewEmbeddedRedis() (*EmbeddedRedis, error) {
	server, err := miniredis.Run()
	if err != nil {
		return nil, fmt.Errorf("start miniredis failed: %w", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: server.Addr(),
	})

	return &EmbeddedRedis{
		server: server,
		client: client,
	}, nil
}

// GetClient 获取 Redis 客户端
func (e *EmbeddedRedis) GetClient() *redis.Client {
	return e.client
}

// GetAddr 获取服务地址
func (e *EmbeddedRedis) GetAddr() string {
	return e.server.Addr()
}

// Close 关闭服务
func (e *EmbeddedRedis) Close() error {
	if err := e.client.Close(); err != nil {
		return err
	}
	e.server.Close()
	return nil
}

// FastForward 快进时间（用于测试 TTL）
func (e *EmbeddedRedis) FastForward(d time.Duration) {
	e.server.FastForward(d)
}

// FlushAll 清空所有数据
func (e *EmbeddedRedis) FlushAll() {
	e.server.FlushAll()
}

// =============================================================================
// EmbeddedStore 内嵌 Redis 存储
// =============================================================================

// EmbeddedStore 内嵌 Redis 存储
// 封装 miniredis，提供与 RedisStore 相同的接口
type EmbeddedStore[K comparable, V any] struct {
	*redisstore.RedisStore[K, V]
	embedded *EmbeddedRedis
}

// NewEmbeddedStore 创建内嵌 Redis 存储
func NewEmbeddedStore[K comparable, V any](keyPrefix string) (*EmbeddedStore[K, V], error) {
	embedded, err := NewEmbeddedRedis()
	if err != nil {
		return nil, err
	}

	redisStore := redisstore.NewRedisStore[K, V](embedded.GetClient(), keyPrefix)

	return &EmbeddedStore[K, V]{
		RedisStore: redisStore,
		embedded:   embedded,
	}, nil
}

// Close 关闭存储
func (s *EmbeddedStore[K, V]) Close() error {
	return s.embedded.Close()
}

// GetEmbeddedRedis 获取内嵌 Redis 实例
func (s *EmbeddedStore[K, V]) GetEmbeddedRedis() *EmbeddedRedis {
	return s.embedded
}

// =============================================================================
// EmbeddedSetStore 内嵌 Redis 集合存储
// =============================================================================

// EmbeddedSetStore 内嵌 Redis 集合存储
type EmbeddedSetStore struct {
	*redisstore.RedisSetStore
	embedded *EmbeddedRedis
}

// NewEmbeddedSetStore 创建内嵌 Redis 集合存储
func NewEmbeddedSetStore(keyPrefix string) (*EmbeddedSetStore, error) {
	embedded, err := NewEmbeddedRedis()
	if err != nil {
		return nil, err
	}

	setStore := redisstore.NewRedisSetStore(embedded.GetClient(), keyPrefix)

	return &EmbeddedSetStore{
		RedisSetStore: setStore,
		embedded:      embedded,
	}, nil
}

// NewEmbeddedSetStoreWithRedis 使用已有的 EmbeddedRedis 创建集合存储
func NewEmbeddedSetStoreWithRedis(embedded *EmbeddedRedis, keyPrefix string) *EmbeddedSetStore {
	setStore := redisstore.NewRedisSetStore(embedded.GetClient(), keyPrefix)
	return &EmbeddedSetStore{
		RedisSetStore: setStore,
		embedded:      embedded,
	}
}

// Close 关闭存储
func (s *EmbeddedSetStore) Close() error {
	return s.embedded.Close()
}

// =============================================================================
// 工厂函数
// =============================================================================

// CreateEmbeddedStores 创建一组内嵌存储（共享同一个 miniredis 实例）
func CreateEmbeddedStores[K comparable, V any](
	dataKeyPrefix string,
	indexKeyPrefix string,
) (*EmbeddedStore[K, V], *EmbeddedSetStore, error) {
	// 创建共享的内嵌 Redis
	embedded, err := NewEmbeddedRedis()
	if err != nil {
		return nil, nil, err
	}

	// 创建数据存储
	dataStore := &EmbeddedStore[K, V]{
		RedisStore: redisstore.NewRedisStore[K, V](embedded.GetClient(), dataKeyPrefix),
		embedded:   embedded,
	}

	// 创建索引存储
	indexStore := NewEmbeddedSetStoreWithRedis(embedded, indexKeyPrefix)

	return dataStore, indexStore, nil
}

// =============================================================================
// Ping 实现
// =============================================================================

// Ping 健康检查
func (s *EmbeddedStore[K, V]) Ping(ctx context.Context) error {
	return s.embedded.client.Ping(ctx).Err()
}

// =============================================================================
// SharedStore 接口实现验证
// =============================================================================

// 确保 EmbeddedStore 实现了 SharedStore 接口
var _ store.SharedStore[string, string] = (*EmbeddedStore[string, string])(nil)
