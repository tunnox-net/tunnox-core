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

// Storage 存储接口
type Storage interface {
	// 基础操作
	Set(key string, value interface{}, ttl time.Duration) error
	Get(key string) (interface{}, error)
	Delete(key string) error
	Exists(key string) (bool, error)

	// 列表操作
	SetList(key string, values []interface{}, ttl time.Duration) error
	GetList(key string) ([]interface{}, error)
	AppendToList(key string, value interface{}) error
	RemoveFromList(key string, value interface{}) error

	// 哈希操作
	SetHash(key string, field string, value interface{}) error
	GetHash(key string, field string) (interface{}, error)
	GetAllHash(key string) (map[string]interface{}, error)
	DeleteHash(key string, field string) error

	// 计数器操作
	Incr(key string) (int64, error)
	IncrBy(key string, value int64) (int64, error)

	// 过期时间
	SetExpiration(key string, ttl time.Duration) error
	GetExpiration(key string) (time.Duration, error)

	// 清理过期数据
	CleanupExpired() error

	// 分布式操作
	SetNX(key string, value interface{}, ttl time.Duration) (bool, error)                       // 原子设置，仅当键不存在时
	CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error) // 原子比较并交换
	Watch(key string, callback func(interface{})) error                                         // 监听键变化
	Unwatch(key string) error                                                                   // 取消监听

	// 关闭存储
	Close() error
}
