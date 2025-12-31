package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrStorageNotFullStorage 当存储不支持 FullStorage 接口时返回
	ErrStorageNotFullStorage = errors.New("storage does not implement FullStorage interface")
)

// ExtendedTypedStorage 扩展的泛型类型安全存储接口
// 提供类型安全的存储操作，避免运行时类型断言
// 包含完整的存储功能：基础操作、列表、哈希、过期时间、分布式操作
type ExtendedTypedStorage[T any] interface {
	// 基础操作
	Set(key string, value T, ttl time.Duration) error
	Get(key string) (T, error)
	Delete(key string) error
	Exists(key string) (bool, error)

	// 列表操作
	SetList(key string, values []T, ttl time.Duration) error
	GetList(key string) ([]T, error)
	AppendToList(key string, value T) error
	RemoveFromList(key string, value T) error

	// 哈希操作
	SetHash(key string, field string, value T) error
	GetHash(key string, field string) (T, error)
	GetAllHash(key string) (map[string]T, error)
	DeleteHash(key string, field string) error

	// 过期时间
	SetExpiration(key string, ttl time.Duration) error
	GetExpiration(key string) (time.Duration, error)

	// 分布式操作
	SetNX(key string, value T, ttl time.Duration) (bool, error)
	CompareAndSwap(key string, oldValue, newValue T, ttl time.Duration) (bool, error)

	// 底层存储
	Underlying() FullStorage
}

// typedStorageAdapter 泛型存储适配器
// 将 Storage 接口适配为类型安全的 ExtendedTypedStorage
type typedStorageAdapter[T any] struct {
	storage FullStorage // 使用 FullStorage 以支持所有功能
}

// NewExtendedTypedStorage 创建扩展的泛型类型安全存储
// 使用示例:
//
//	stringStorage, err := NewExtendedTypedStorage[string](storage)
//	int64Storage, err := NewExtendedTypedStorage[int64](storage)
//	userStorage, err := NewExtendedTypedStorage[*models.User](storage)
func NewExtendedTypedStorage[T any](storage Storage) (ExtendedTypedStorage[T], error) {
	// 将 Storage 转换为 FullStorage（所有现有实现都实现了 FullStorage）
	fullStorage, ok := storage.(FullStorage)
	if !ok {
		return nil, ErrStorageNotFullStorage
	}
	return &typedStorageAdapter[T]{
		storage: fullStorage,
	}, nil
}

// Set 类型安全的设置操作
func (t *typedStorageAdapter[T]) Set(key string, value T, ttl time.Duration) error {
	return t.storage.Set(key, value, ttl)
}

// Get 类型安全的获取操作
func (t *typedStorageAdapter[T]) Get(key string) (T, error) {
	var zero T

	value, err := t.storage.Get(key)
	if err != nil {
		return zero, err
	}

	// 尝试类型断言
	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("%w: expected %T, got %T", ErrInvalidType, zero, value)
	}

	return typed, nil
}

// Delete 删除键
func (t *typedStorageAdapter[T]) Delete(key string) error {
	return t.storage.Delete(key)
}

// Exists 检查键是否存在
func (t *typedStorageAdapter[T]) Exists(key string) (bool, error) {
	return t.storage.Exists(key)
}

// SetList 设置列表
func (t *typedStorageAdapter[T]) SetList(key string, values []T, ttl time.Duration) error {
	// 转换为 []any
	anySlice := make([]any, len(values))
	for i, v := range values {
		anySlice[i] = v
	}
	return t.storage.SetList(key, anySlice, ttl)
}

// GetList 获取列表
func (t *typedStorageAdapter[T]) GetList(key string) ([]T, error) {
	var zero []T

	values, err := t.storage.GetList(key)
	if err != nil {
		return zero, err
	}

	// 转换为 []T
	result := make([]T, 0, len(values))
	for i, v := range values {
		typed, ok := v.(T)
		if !ok {
			var zeroT T
			return zero, fmt.Errorf("%w: list item[%d] expected %T, got %T", ErrInvalidType, i, zeroT, v)
		}
		result = append(result, typed)
	}

	return result, nil
}

// AppendToList 追加到列表
func (t *typedStorageAdapter[T]) AppendToList(key string, value T) error {
	return t.storage.AppendToList(key, value)
}

// RemoveFromList 从列表移除
func (t *typedStorageAdapter[T]) RemoveFromList(key string, value T) error {
	return t.storage.RemoveFromList(key, value)
}

// SetHash 设置哈希字段
func (t *typedStorageAdapter[T]) SetHash(key string, field string, value T) error {
	return t.storage.SetHash(key, field, value)
}

// GetHash 获取哈希字段
func (t *typedStorageAdapter[T]) GetHash(key string, field string) (T, error) {
	var zero T

	value, err := t.storage.GetHash(key, field)
	if err != nil {
		return zero, err
	}

	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("%w: expected %T, got %T", ErrInvalidType, zero, value)
	}

	return typed, nil
}

// GetAllHash 获取所有哈希字段
func (t *typedStorageAdapter[T]) GetAllHash(key string) (map[string]T, error) {
	values, err := t.storage.GetAllHash(key)
	if err != nil {
		return nil, err
	}

	result := make(map[string]T, len(values))
	for field, v := range values {
		typed, ok := v.(T)
		if !ok {
			var zero T
			return nil, fmt.Errorf("%w: hash field[%s] expected %T, got %T", ErrInvalidType, field, zero, v)
		}
		result[field] = typed
	}

	return result, nil
}

// DeleteHash 删除哈希字段
func (t *typedStorageAdapter[T]) DeleteHash(key string, field string) error {
	return t.storage.DeleteHash(key, field)
}

// SetExpiration 设置过期时间
func (t *typedStorageAdapter[T]) SetExpiration(key string, ttl time.Duration) error {
	return t.storage.SetExpiration(key, ttl)
}

// GetExpiration 获取过期时间
func (t *typedStorageAdapter[T]) GetExpiration(key string) (time.Duration, error) {
	return t.storage.GetExpiration(key)
}

// SetNX 原子设置（仅当键不存在时）
func (t *typedStorageAdapter[T]) SetNX(key string, value T, ttl time.Duration) (bool, error) {
	return t.storage.SetNX(key, value, ttl)
}

// CompareAndSwap 原子比较并交换
func (t *typedStorageAdapter[T]) CompareAndSwap(key string, oldValue, newValue T, ttl time.Duration) (bool, error) {
	return t.storage.CompareAndSwap(key, oldValue, newValue, ttl)
}

// Underlying 返回底层存储
func (t *typedStorageAdapter[T]) Underlying() FullStorage {
	return t.storage
}

// ============================================================================
// 类型化 JSON 序列化存储（用于复杂类型）
// ============================================================================

// TypedJSONStorage 类型化 JSON 序列化存储
// 用于存储可序列化为 JSON 的复杂类型
type TypedJSONStorage[T any] interface {
	Set(key string, value T, ttl time.Duration) error
	Get(key string) (T, error)
	Delete(key string) error
	Exists(key string) (bool, error)
	SetExpiration(key string, ttl time.Duration) error
	Underlying() Storage
}

// typedJSONStorageAdapter JSON 存储适配器
type typedJSONStorageAdapter[T any] struct {
	storage Storage
}

// NewTypedJSONStorage 创建类型化 JSON 序列化存储
// 用于存储结构体等复杂类型，自动进行 JSON 序列化/反序列化
// 使用示例:
//
//	userStorage := NewTypedJSONStorage[models.User](storage)
//	configStorage := NewTypedJSONStorage[config.ServerConfig](storage)
func NewTypedJSONStorage[T any](storage Storage) TypedJSONStorage[T] {
	return &typedJSONStorageAdapter[T]{
		storage: storage,
	}
}

// Set JSON 序列化后存储
func (j *typedJSONStorageAdapter[T]) Set(key string, value T, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	return j.storage.Set(key, data, ttl)
}

// Get 获取并反序列化
func (j *typedJSONStorageAdapter[T]) Get(key string) (T, error) {
	var zero T

	value, err := j.storage.Get(key)
	if err != nil {
		return zero, err
	}

	// 尝试类型断言为 []byte
	data, ok := value.([]byte)
	if !ok {
		// 尝试 string
		if str, ok := value.(string); ok {
			data = []byte(str)
		} else {
			return zero, fmt.Errorf("%w: expected []byte or string, got %T", ErrInvalidType, value)
		}
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return result, nil
}

// Delete 删除键
func (j *typedJSONStorageAdapter[T]) Delete(key string) error {
	return j.storage.Delete(key)
}

// Exists 检查键是否存在
func (j *typedJSONStorageAdapter[T]) Exists(key string) (bool, error) {
	return j.storage.Exists(key)
}

// SetExpiration 设置过期时间
func (j *typedJSONStorageAdapter[T]) SetExpiration(key string, ttl time.Duration) error {
	return j.storage.SetExpiration(key, ttl)
}

// Underlying 返回底层存储
func (j *typedJSONStorageAdapter[T]) Underlying() Storage {
	return j.storage
}

// ============================================================================
// 常用类型别名（便捷使用）
// ============================================================================

// ExtendedStringStorage 扩展的字符串存储
type ExtendedStringStorage = ExtendedTypedStorage[string]

// ExtendedInt64Storage 扩展的 Int64 存储
type ExtendedInt64Storage = ExtendedTypedStorage[int64]

// ExtendedIntStorage 扩展的 Int 存储
type ExtendedIntStorage = ExtendedTypedStorage[int]

// ExtendedBoolStorage 扩展的布尔值存储
type ExtendedBoolStorage = ExtendedTypedStorage[bool]

// ExtendedBytesStorage 扩展的字节数组存储
type ExtendedBytesStorage = ExtendedTypedStorage[[]byte]

// ExtendedFloat64Storage 扩展的 Float64 存储
type ExtendedFloat64Storage = ExtendedTypedStorage[float64]

// ============================================================================
// 工厂函数（便捷创建）
// ============================================================================

// NewExtendedStringStorage 创建扩展的字符串存储
func NewExtendedStringStorage(storage Storage) (ExtendedStringStorage, error) {
	return NewExtendedTypedStorage[string](storage)
}

// NewExtendedInt64Storage 创建扩展的 Int64 存储
func NewExtendedInt64Storage(storage Storage) (ExtendedInt64Storage, error) {
	return NewExtendedTypedStorage[int64](storage)
}

// NewExtendedIntStorage 创建扩展的 Int 存储
func NewExtendedIntStorage(storage Storage) (ExtendedIntStorage, error) {
	return NewExtendedTypedStorage[int](storage)
}

// NewExtendedBoolStorage 创建扩展的布尔值存储
func NewExtendedBoolStorage(storage Storage) (ExtendedBoolStorage, error) {
	return NewExtendedTypedStorage[bool](storage)
}

// NewExtendedBytesStorage 创建扩展的字节数组存储
func NewExtendedBytesStorage(storage Storage) (ExtendedBytesStorage, error) {
	return NewExtendedTypedStorage[[]byte](storage)
}

// NewExtendedFloat64Storage 创建扩展的 Float64 存储
func NewExtendedFloat64Storage(storage Storage) (ExtendedFloat64Storage, error) {
	return NewExtendedTypedStorage[float64](storage)
}
