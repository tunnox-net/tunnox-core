package storage

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
// 核心接口（所有存储必须实现）
// ============================================================================

// Storage 核心存储接口（必需实现）
// 包含最常用的基础操作，所有存储实现都必须支持
type Storage interface {
	// 基础 KV 操作（必需）
	Set(key string, value interface{}, ttl time.Duration) error
	Get(key string) (interface{}, error)
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
type ListStore interface {
	SetList(key string, values []interface{}, ttl time.Duration) error
	GetList(key string) ([]interface{}, error)
	AppendToList(key string, value interface{}) error
	RemoveFromList(key string, value interface{}) error
}

// HashStore 哈希操作扩展接口（可选）
type HashStore interface {
	SetHash(key string, field string, value interface{}) error
	GetHash(key string, field string) (interface{}, error)
	GetAllHash(key string) (map[string]interface{}, error)
	DeleteHash(key string, field string) error
}

// CounterStore 计数器操作扩展接口（可选）
type CounterStore interface {
	Incr(key string) (int64, error)
	IncrBy(key string, value int64) (int64, error)
}

// CASStore 原子操作扩展接口（可选）
// 用于分布式锁、原子更新等场景
type CASStore interface {
	SetNX(key string, value interface{}, ttl time.Duration) (bool, error)
	CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error)
}

// WatchableStore 监听扩展接口（可选）
// 用于键变化通知
type WatchableStore interface {
	Watch(key string, callback func(interface{})) error
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
