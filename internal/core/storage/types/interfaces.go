package types

import (
	"errors"
	"time"
)

// 存储相关错误
var (
	ErrKeyNotFound = errors.New("key not found")
	ErrInvalidType = errors.New("invalid type")
)

// ============================================================================
// 泛型存储接口（类型安全，推荐使用）
// ============================================================================

// TypedStorage 泛型存储接口
// 提供类型安全的键值存储操作
type TypedStorage[T any] interface {
	// Set 设置键值对
	Set(key string, value T, ttl time.Duration) error

	// Get 获取值
	Get(key string) (T, error)

	// Delete 删除键
	Delete(key string) error

	// Exists 检查键是否存在
	Exists(key string) (bool, error)
}

// TypedListStore 泛型列表存储接口
type TypedListStore[T any] interface {
	// SetList 设置列表
	SetList(key string, values []T, ttl time.Duration) error

	// GetList 获取列表
	GetList(key string) ([]T, error)

	// AppendToList 追加元素到列表
	AppendToList(key string, value T) error

	// RemoveFromList 从列表移除元素
	RemoveFromList(key string, value T) error
}

// TypedHashStore 泛型哈希存储接口
type TypedHashStore[T any] interface {
	// SetHash 设置哈希字段
	SetHash(key string, field string, value T) error

	// GetHash 获取哈希字段
	GetHash(key string, field string) (T, error)

	// GetAllHash 获取所有哈希字段
	GetAllHash(key string) (map[string]T, error)

	// DeleteHash 删除哈希字段
	DeleteHash(key string, field string) error
}

// TypedCASStore 泛型原子操作接口
type TypedCASStore[T any] interface {
	// SetNX 仅当键不存在时设置
	SetNX(key string, value T, ttl time.Duration) (bool, error)

	// CompareAndSwap 比较并交换
	CompareAndSwap(key string, oldValue, newValue T, ttl time.Duration) (bool, error)
}

// TypedWatchableStore 泛型监听接口
type TypedWatchableStore[T any] interface {
	// Watch 监听键变化
	Watch(key string, callback func(T)) error

	// Unwatch 取消监听
	Unwatch(key string) error
}

// ============================================================================
// 核心接口（所有存储必须实现）
// ============================================================================

// Storage 核心存储接口（必需实现）
// 包含最常用的基础操作，所有存储实现都必须支持
//
// 此接口使用 any 类型作为通用存储层。
// 如需类型安全，可使用 TypedStorageAdapter[T] 包装此接口。
//
// 示例:
//
//	rawStorage := storage.NewMemoryStorage(ctx)
//	userStorage := storage.NewTypedStorageAdapter[User](rawStorage)
//	err := userStorage.Set("user:1", user, 0)
type Storage interface {
	// 基础 KV 操作（必需）
	// 推荐: 使用 TypedStorageAdapter[T].Set 获得类型安全
	Set(key string, value any, ttl time.Duration) error
	// 推荐: 使用 TypedStorageAdapter[T].Get 获得类型安全
	Get(key string) (any, error)
	Delete(key string) error
	Exists(key string) (bool, error)

	// 过期时间（必需，不支持 TTL 的存储可以返回错误）
	SetExpiration(key string, ttl time.Duration) error
	GetExpiration(key string) (time.Duration, error)
	CleanupExpired() error

	// 关闭存储（必需）
	Close() error
}

// ============================================================================
// 扩展接口（可选实现）
// ============================================================================

// ListStore 列表操作扩展接口（可选）
// 如果存储支持列表操作，可以实现此接口
//
// 如需类型安全，可使用 TypedListStore[T] 或 TypedFullStorageAdapter[T]
type ListStore interface {
	SetList(key string, values []any, ttl time.Duration) error
	GetList(key string) ([]any, error)
	AppendToList(key string, value any) error
	RemoveFromList(key string, value any) error
}

// HashStore 哈希操作扩展接口（可选）
//
// 如需类型安全，可使用 TypedHashStore[T] 或 TypedFullStorageAdapter[T]
type HashStore interface {
	SetHash(key string, field string, value any) error
	GetHash(key string, field string) (any, error)
	GetAllHash(key string) (map[string]any, error)
	DeleteHash(key string, field string) error
}

// CounterStore 计数器操作扩展接口（可选）
type CounterStore interface {
	Incr(key string) (int64, error)
	IncrBy(key string, value int64) (int64, error)
}

// CASStore 原子操作扩展接口（可选）
// 用于分布式锁、原子更新等场景
//
// 如需类型安全，可使用 TypedCASStore[T] 或 TypedFullStorageAdapter[T]
type CASStore interface {
	SetNX(key string, value any, ttl time.Duration) (bool, error)
	CompareAndSwap(key string, oldValue, newValue any, ttl time.Duration) (bool, error)
}

// WatchableStore 监听扩展接口（可选）
// 用于键变化通知
//
// 如需类型安全，可使用 TypedWatchableStore[T] 或 TypedFullStorageAdapter[T]
type WatchableStore interface {
	Watch(key string, callback func(any)) error
	Unwatch(key string) error
}

// ============================================================================
// 完整存储接口（向后兼容，包含所有功能）
// ============================================================================

// FullStorage 完整存储接口（包含所有功能）
// 用于向后兼容，所有现有存储实现都实现了此接口
// 新代码应该使用核心接口 + 扩展接口的组合
type FullStorage interface {
	Storage
	ListStore
	HashStore
	CounterStore
	CASStore
	WatchableStore
}

// ============================================================================
// 持久化存储接口
// ============================================================================

// PersistentStorage 持久化存储接口
// 用于数据库或远程 gRPC 存储
//
// 如需类型安全，可使用 TypedPersistentAdapter[T] 包装此接口
type PersistentStorage interface {
	// Set 设置键值对（持久化，不设置 TTL）
	Set(key string, value any) error

	// Get 获取值
	Get(key string) (any, error)

	// Delete 删除键
	Delete(key string) error

	// Exists 检查键是否存在
	Exists(key string) (bool, error)

	// BatchSet 批量设置
	BatchSet(items map[string]any) error

	// BatchGet 批量获取
	BatchGet(keys []string) (map[string]any, error)

	// BatchDelete 批量删除
	BatchDelete(keys []string) error

	// QueryByField 按字段查询（扫描匹配前缀的所有键，解析 JSON，过滤字段）
	// keyPrefix: 键前缀（如 "tunnox:port_mapping:"）
	// fieldName: 字段名（如 "listen_client_id"）
	// fieldValue: 字段值（如 int64(19072689)）
	// 返回：匹配的 JSON 字符串列表
	QueryByField(keyPrefix string, fieldName string, fieldValue any) ([]string, error)

	// QueryByPrefix 按前缀查询所有键值对
	// prefix: 键前缀（如 "tunnox:persist:client:config:"）
	// limit: 返回结果数量限制，0 表示无限制
	// 返回：map[key]jsonValue，key 是完整键名，jsonValue 是 JSON 序列化的值
	QueryByPrefix(prefix string, limit int) (map[string]string, error)

	// Close 关闭连接
	Close() error
}

// TypedPersistentStorage 泛型持久化存储接口
type TypedPersistentStorage[T any] interface {
	// Set 设置键值对（持久化，不设置 TTL）
	Set(key string, value T) error

	// Get 获取值
	Get(key string) (T, error)

	// Delete 删除键
	Delete(key string) error

	// Exists 检查键是否存在
	Exists(key string) (bool, error)

	// BatchSet 批量设置
	BatchSet(items map[string]T) error

	// BatchGet 批量获取
	BatchGet(keys []string) (map[string]T, error)

	// BatchDelete 批量删除
	BatchDelete(keys []string) error

	// Close 关闭连接
	Close() error
}

// ============================================================================
// 缓存存储接口
// ============================================================================

// CacheStorage 缓存存储接口（对 Storage 的子集）
//
// 如需类型安全，可使用 TypedCacheAdapter[T] 包装此接口
type CacheStorage interface {
	Set(key string, value any, ttl time.Duration) error
	Get(key string) (any, error)
	Delete(key string) error
	Exists(key string) (bool, error)
	Close() error
}

// TypedCacheStorage 泛型缓存存储接口
type TypedCacheStorage[T any] interface {
	Set(key string, value T, ttl time.Duration) error
	Get(key string) (T, error)
	Delete(key string) error
	Exists(key string) (bool, error)
	Close() error
}

// ============================================================================
// 完整泛型存储接口（推荐使用）
// ============================================================================

// TypedFullStorage 完整的泛型存储接口
// 提供类型安全的所有存储操作，包括 KV、列表、哈希、计数器、CAS、监听
type TypedFullStorage[T any] interface {
	TypedStorage[T]
	TypedListStore[T]
	TypedHashStore[T]
	TypedCASStore[T]
	TypedWatchableStore[T]

	// 过期时间操作
	SetExpiration(key string, ttl time.Duration) error
	GetExpiration(key string) (time.Duration, error)
	CleanupExpired() error

	// 计数器操作（仅当 T 为 int64 时有意义）
	Incr(key string) (int64, error)
	IncrBy(key string, value int64) (int64, error)

	// 底层存储访问
	Storage() FullStorage
	Close() error
}
