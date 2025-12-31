package types

import (
	"encoding/json"
	"time"
)

// ============================================================================
// 类型安全适配器（将 any 类型接口转换为泛型接口）
// 这些适配器是推荐的类型安全存储访问方式
// ============================================================================

// TypedStorageAdapter 将 Storage 接口包装为类型安全的泛型接口
// 内部使用 JSON 序列化/反序列化实现类型转换
type TypedStorageAdapter[T any] struct {
	storage Storage
}

// NewTypedStorageAdapter 创建类型安全的存储适配器
func NewTypedStorageAdapter[T any](storage Storage) *TypedStorageAdapter[T] {
	return &TypedStorageAdapter[T]{storage: storage}
}

// Set 类型安全地设置键值对
func (a *TypedStorageAdapter[T]) Set(key string, value T, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return a.storage.Set(key, string(data), ttl)
}

// Get 类型安全地获取值
func (a *TypedStorageAdapter[T]) Get(key string) (T, error) {
	var zero T
	raw, err := a.storage.Get(key)
	if err != nil {
		return zero, err
	}

	// 处理不同类型的存储返回值
	var data []byte
	switch v := raw.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		// 如果存储返回的已经是目标类型，直接返回
		if typed, ok := raw.(T); ok {
			return typed, nil
		}
		return zero, ErrInvalidType
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, err
	}
	return result, nil
}

// Delete 删除键
func (a *TypedStorageAdapter[T]) Delete(key string) error {
	return a.storage.Delete(key)
}

// Exists 检查键是否存在
func (a *TypedStorageAdapter[T]) Exists(key string) (bool, error) {
	return a.storage.Exists(key)
}

// Storage 返回底层存储
func (a *TypedStorageAdapter[T]) Storage() Storage {
	return a.storage
}

// ============================================================================
// TypedCacheAdapter
// ============================================================================

// TypedCacheAdapter 将 CacheStorage 接口包装为类型安全的泛型接口
type TypedCacheAdapter[T any] struct {
	cache CacheStorage
}

// NewTypedCacheAdapter 创建类型安全的缓存适配器
func NewTypedCacheAdapter[T any](cache CacheStorage) *TypedCacheAdapter[T] {
	return &TypedCacheAdapter[T]{cache: cache}
}

// Set 类型安全地设置键值对
func (a *TypedCacheAdapter[T]) Set(key string, value T, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return a.cache.Set(key, string(data), ttl)
}

// Get 类型安全地获取值
func (a *TypedCacheAdapter[T]) Get(key string) (T, error) {
	var zero T
	raw, err := a.cache.Get(key)
	if err != nil {
		return zero, err
	}

	// 处理不同类型的存储返回值
	var data []byte
	switch v := raw.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		// 如果存储返回的已经是目标类型，直接返回
		if typed, ok := raw.(T); ok {
			return typed, nil
		}
		return zero, ErrInvalidType
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, err
	}
	return result, nil
}

// Delete 删除键
func (a *TypedCacheAdapter[T]) Delete(key string) error {
	return a.cache.Delete(key)
}

// Exists 检查键是否存在
func (a *TypedCacheAdapter[T]) Exists(key string) (bool, error) {
	return a.cache.Exists(key)
}

// Close 关闭缓存
func (a *TypedCacheAdapter[T]) Close() error {
	return a.cache.Close()
}

// Cache 返回底层缓存
func (a *TypedCacheAdapter[T]) Cache() CacheStorage {
	return a.cache
}

// ============================================================================
// TypedPersistentAdapter
// ============================================================================

// TypedPersistentAdapter 将 PersistentStorage 接口包装为类型安全的泛型接口
type TypedPersistentAdapter[T any] struct {
	persistent PersistentStorage
}

// NewTypedPersistentAdapter 创建类型安全的持久化存储适配器
func NewTypedPersistentAdapter[T any](persistent PersistentStorage) *TypedPersistentAdapter[T] {
	return &TypedPersistentAdapter[T]{persistent: persistent}
}

// Set 类型安全地设置键值对
func (a *TypedPersistentAdapter[T]) Set(key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return a.persistent.Set(key, string(data))
}

// Get 类型安全地获取值
func (a *TypedPersistentAdapter[T]) Get(key string) (T, error) {
	var zero T
	raw, err := a.persistent.Get(key)
	if err != nil {
		return zero, err
	}

	// 处理不同类型的存储返回值
	var data []byte
	switch v := raw.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		// 如果存储返回的已经是目标类型，直接返回
		if typed, ok := raw.(T); ok {
			return typed, nil
		}
		return zero, ErrInvalidType
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, err
	}
	return result, nil
}

// Delete 删除键
func (a *TypedPersistentAdapter[T]) Delete(key string) error {
	return a.persistent.Delete(key)
}

// Exists 检查键是否存在
func (a *TypedPersistentAdapter[T]) Exists(key string) (bool, error) {
	return a.persistent.Exists(key)
}

// BatchSet 批量设置
func (a *TypedPersistentAdapter[T]) BatchSet(items map[string]T) error {
	converted := make(map[string]any, len(items))
	for k, v := range items {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		converted[k] = string(data)
	}
	return a.persistent.BatchSet(converted)
}

// BatchGet 批量获取
func (a *TypedPersistentAdapter[T]) BatchGet(keys []string) (map[string]T, error) {
	raw, err := a.persistent.BatchGet(keys)
	if err != nil {
		return nil, err
	}

	result := make(map[string]T, len(raw))
	for k, v := range raw {
		var data []byte
		switch typed := v.(type) {
		case string:
			data = []byte(typed)
		case []byte:
			data = typed
		default:
			// 如果存储返回的已经是目标类型，直接使用
			if typedValue, ok := v.(T); ok {
				result[k] = typedValue
				continue
			}
			return nil, ErrInvalidType
		}

		var value T
		if err := json.Unmarshal(data, &value); err != nil {
			return nil, err
		}
		result[k] = value
	}
	return result, nil
}

// BatchDelete 批量删除
func (a *TypedPersistentAdapter[T]) BatchDelete(keys []string) error {
	return a.persistent.BatchDelete(keys)
}

// Close 关闭连接
func (a *TypedPersistentAdapter[T]) Close() error {
	return a.persistent.Close()
}

// Persistent 返回底层持久化存储
func (a *TypedPersistentAdapter[T]) Persistent() PersistentStorage {
	return a.persistent
}

// ============================================================================
// TypedFullStorageAdapter - 完整泛型存储适配器
// ============================================================================

// TypedFullStorageAdapter 将 FullStorage 接口包装为类型安全的完整泛型接口
// 支持所有存储操作：基础 KV、列表、哈希、计数器、CAS、监听
type TypedFullStorageAdapter[T any] struct {
	storage FullStorage
}

// NewTypedFullStorageAdapter 创建完整的类型安全存储适配器
func NewTypedFullStorageAdapter[T any](storage FullStorage) *TypedFullStorageAdapter[T] {
	return &TypedFullStorageAdapter[T]{storage: storage}
}

// Set 类型安全地设置键值对
func (a *TypedFullStorageAdapter[T]) Set(key string, value T, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return a.storage.Set(key, string(data), ttl)
}

// Get 类型安全地获取值
func (a *TypedFullStorageAdapter[T]) Get(key string) (T, error) {
	var zero T
	raw, err := a.storage.Get(key)
	if err != nil {
		return zero, err
	}
	return unmarshalValue[T](raw)
}

// Delete 删除键
func (a *TypedFullStorageAdapter[T]) Delete(key string) error {
	return a.storage.Delete(key)
}

// Exists 检查键是否存在
func (a *TypedFullStorageAdapter[T]) Exists(key string) (bool, error) {
	return a.storage.Exists(key)
}

// SetExpiration 设置过期时间
func (a *TypedFullStorageAdapter[T]) SetExpiration(key string, ttl time.Duration) error {
	return a.storage.SetExpiration(key, ttl)
}

// GetExpiration 获取过期时间
func (a *TypedFullStorageAdapter[T]) GetExpiration(key string) (time.Duration, error) {
	return a.storage.GetExpiration(key)
}

// CleanupExpired 清理过期键
func (a *TypedFullStorageAdapter[T]) CleanupExpired() error {
	return a.storage.CleanupExpired()
}

// SetList 设置列表
func (a *TypedFullStorageAdapter[T]) SetList(key string, values []T, ttl time.Duration) error {
	anySlice := make([]any, len(values))
	for i, v := range values {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		anySlice[i] = string(data)
	}
	return a.storage.SetList(key, anySlice, ttl)
}

// GetList 获取列表
func (a *TypedFullStorageAdapter[T]) GetList(key string) ([]T, error) {
	raw, err := a.storage.GetList(key)
	if err != nil {
		return nil, err
	}
	result := make([]T, 0, len(raw))
	for _, item := range raw {
		value, err := unmarshalValue[T](item)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

// AppendToList 追加元素到列表
func (a *TypedFullStorageAdapter[T]) AppendToList(key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return a.storage.AppendToList(key, string(data))
}

// RemoveFromList 从列表移除元素
func (a *TypedFullStorageAdapter[T]) RemoveFromList(key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return a.storage.RemoveFromList(key, string(data))
}

// SetHash 设置哈希字段
func (a *TypedFullStorageAdapter[T]) SetHash(key string, field string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return a.storage.SetHash(key, field, string(data))
}

// GetHash 获取哈希字段
func (a *TypedFullStorageAdapter[T]) GetHash(key string, field string) (T, error) {
	var zero T
	raw, err := a.storage.GetHash(key, field)
	if err != nil {
		return zero, err
	}
	return unmarshalValue[T](raw)
}

// GetAllHash 获取所有哈希字段
func (a *TypedFullStorageAdapter[T]) GetAllHash(key string) (map[string]T, error) {
	raw, err := a.storage.GetAllHash(key)
	if err != nil {
		return nil, err
	}
	result := make(map[string]T, len(raw))
	for k, v := range raw {
		value, err := unmarshalValue[T](v)
		if err != nil {
			return nil, err
		}
		result[k] = value
	}
	return result, nil
}

// DeleteHash 删除哈希字段
func (a *TypedFullStorageAdapter[T]) DeleteHash(key string, field string) error {
	return a.storage.DeleteHash(key, field)
}

// Incr 自增
func (a *TypedFullStorageAdapter[T]) Incr(key string) (int64, error) {
	return a.storage.Incr(key)
}

// IncrBy 增加指定值
func (a *TypedFullStorageAdapter[T]) IncrBy(key string, value int64) (int64, error) {
	return a.storage.IncrBy(key, value)
}

// SetNX 仅当键不存在时设置
func (a *TypedFullStorageAdapter[T]) SetNX(key string, value T, ttl time.Duration) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	return a.storage.SetNX(key, string(data), ttl)
}

// CompareAndSwap 比较并交换
func (a *TypedFullStorageAdapter[T]) CompareAndSwap(key string, oldValue, newValue T, ttl time.Duration) (bool, error) {
	oldData, err := json.Marshal(oldValue)
	if err != nil {
		return false, err
	}
	newData, err := json.Marshal(newValue)
	if err != nil {
		return false, err
	}
	return a.storage.CompareAndSwap(key, string(oldData), string(newData), ttl)
}

// Watch 监听键变化
func (a *TypedFullStorageAdapter[T]) Watch(key string, callback func(T)) error {
	return a.storage.Watch(key, func(raw any) {
		value, err := unmarshalValue[T](raw)
		if err == nil {
			callback(value)
		}
	})
}

// Unwatch 取消监听
func (a *TypedFullStorageAdapter[T]) Unwatch(key string) error {
	return a.storage.Unwatch(key)
}

// Storage 返回底层 FullStorage
func (a *TypedFullStorageAdapter[T]) Storage() FullStorage {
	return a.storage
}

// Close 关闭存储
func (a *TypedFullStorageAdapter[T]) Close() error {
	return a.storage.Close()
}

// unmarshalValue 通用的反序列化辅助函数
// 将 any 类型的原始值转换为目标类型 T
func unmarshalValue[T any](raw any) (T, error) {
	var zero T
	var data []byte
	switch v := raw.(type) {
	case string:
		data = []byte(v)
	case []byte:
		data = v
	default:
		if typed, ok := raw.(T); ok {
			return typed, nil
		}
		return zero, ErrInvalidType
	}
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, err
	}
	return result, nil
}
