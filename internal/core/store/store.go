// Package store 提供统一的存储抽象层
//
// 存储层次结构:
//   - Store[K,V]: 基础键值存储接口
//   - TTLStore[K,V]: 支持 TTL 的存储
//   - BatchStore[K,V]: 支持批量操作的存储
//   - AtomicStore[K,V]: 支持原子操作的存储
//   - SetStore[K,V]: 集合存储（用于索引）
//   - SharedStore[K,V]: 共享存储（Redis）
//   - PersistentStore[K,V]: 持久化存储（gRPC）
//   - CachedPersistentStore[K,V]: 缓存+持久化组合
package store

import (
	"context"
	"time"
)

// =============================================================================
// 基础存储接口
// =============================================================================

// Store 基础键值存储接口（所有存储必须实现）
type Store[K comparable, V any] interface {
	// Get 获取值，不存在返回 ErrNotFound
	Get(ctx context.Context, key K) (V, error)

	// Set 设置值
	Set(ctx context.Context, key K, value V) error

	// Delete 删除值，不存在不返回错误
	Delete(ctx context.Context, key K) error

	// Exists 检查键是否存在
	Exists(ctx context.Context, key K) (bool, error)
}

// =============================================================================
// 扩展接口
// =============================================================================

// TTLStore 支持 TTL 的存储
type TTLStore[K comparable, V any] interface {
	Store[K, V]

	// SetWithTTL 设置值并指定 TTL
	SetWithTTL(ctx context.Context, key K, value V, ttl time.Duration) error

	// GetTTL 获取剩余 TTL，不存在返回 ErrNotFound
	GetTTL(ctx context.Context, key K) (time.Duration, error)

	// Refresh 刷新 TTL，不存在返回 ErrNotFound
	Refresh(ctx context.Context, key K, ttl time.Duration) error
}

// BatchStore 支持批量操作的存储
type BatchStore[K comparable, V any] interface {
	Store[K, V]

	// BatchGet 批量获取，返回找到的键值对（不存在的键不在结果中）
	BatchGet(ctx context.Context, keys []K) (map[K]V, error)

	// BatchSet 批量设置
	BatchSet(ctx context.Context, items map[K]V) error

	// BatchDelete 批量删除
	BatchDelete(ctx context.Context, keys []K) error
}

// AtomicStore 支持原子操作的存储
type AtomicStore[K comparable, V any] interface {
	Store[K, V]

	// SetNX 仅当键不存在时设置，返回是否成功设置
	SetNX(ctx context.Context, key K, value V) (bool, error)

	// SetNXWithTTL 仅当键不存在时设置并指定 TTL
	SetNXWithTTL(ctx context.Context, key K, value V, ttl time.Duration) (bool, error)
}

// SetStore 集合存储（用于索引）
type SetStore[K comparable, V comparable] interface {
	// Add 向集合添加元素
	Add(ctx context.Context, key K, value V) error

	// Remove 从集合移除元素
	Remove(ctx context.Context, key K, value V) error

	// Contains 检查元素是否在集合中
	Contains(ctx context.Context, key K, value V) (bool, error)

	// Members 获取集合所有成员
	Members(ctx context.Context, key K) ([]V, error)

	// Size 获取集合大小
	Size(ctx context.Context, key K) (int64, error)
}

// HealthChecker 健康检查接口
type HealthChecker interface {
	// Ping 检查连接是否正常
	Ping(ctx context.Context) error
}

// Closer 关闭接口
type Closer interface {
	// Close 关闭存储连接
	Close() error
}

// =============================================================================
// Pipeline 接口（用于批量操作优化）
// =============================================================================

// FutureResult 异步结果
type FutureResult[V any] struct {
	value V
	err   error
	done  bool
}

// Get 获取结果（必须在 Exec 之后调用）
func (f *FutureResult[V]) Get() (V, error) {
	return f.value, f.err
}

// SetResult 设置结果（内部使用）
func (f *FutureResult[V]) SetResult(value V, err error) {
	f.value = value
	f.err = err
	f.done = true
}

// Pipeline Redis 管道接口
type Pipeline[K comparable, V any] interface {
	// Get 添加获取操作到管道
	Get(ctx context.Context, key K) *FutureResult[V]

	// Set 添加设置操作到管道
	Set(ctx context.Context, key K, value V, ttl time.Duration)

	// Delete 添加删除操作到管道
	Delete(ctx context.Context, key K)

	// SAdd 添加集合添加操作到管道
	SAdd(ctx context.Context, key K, member string)

	// SRem 添加集合移除操作到管道
	SRem(ctx context.Context, key K, member string)

	// Exec 执行管道中的所有操作
	Exec(ctx context.Context) error
}

// PipelineSetStore 支持 Pipeline 的 SetStore
type PipelineSetStore[K comparable, V comparable] interface {
	SetStore[K, V]

	// Pipeline 获取管道
	Pipeline() SetPipeline[K, V]
}

// SetPipeline SetStore 专用管道
type SetPipeline[K comparable, V comparable] interface {
	// SAdd 添加集合添加操作到管道
	SAdd(ctx context.Context, key K, member V)

	// SRem 添加集合移除操作到管道
	SRem(ctx context.Context, key K, member V)

	// Exec 执行管道中的所有操作
	Exec(ctx context.Context) error
}
