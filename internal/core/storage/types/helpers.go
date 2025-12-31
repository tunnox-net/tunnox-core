package types

import (
	"encoding/json"
	"time"
)

// ============================================================================
// 辅助函数（类型安全的存取操作）
// 这些函数提供了便捷的类型安全操作，无需创建适配器实例
// ============================================================================

// GetTyped 从 Storage 中类型安全地获取值
// 使用 JSON 反序列化，适用于存储的是 JSON 字符串的场景
//
// 示例:
//
//	user, err := types.GetTyped[User](storage, "user:1")
func GetTyped[T any](storage Storage, key string) (T, error) {
	var zero T
	raw, err := storage.Get(key)
	if err != nil {
		return zero, err
	}
	return unmarshalAny[T](raw)
}

// SetTyped 类型安全地设置值到 Storage
// 使用 JSON 序列化，将值转换为 JSON 字符串存储
//
// 示例:
//
//	err := types.SetTyped(storage, "user:1", user, time.Hour)
func SetTyped[T any](storage Storage, key string, value T, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return storage.Set(key, string(data), ttl)
}

// GetTypedFromCache 从 CacheStorage 中类型安全地获取值
func GetTypedFromCache[T any](cache CacheStorage, key string) (T, error) {
	var zero T
	raw, err := cache.Get(key)
	if err != nil {
		return zero, err
	}
	return unmarshalAny[T](raw)
}

// SetTypedToCache 类型安全地设置值到 CacheStorage
func SetTypedToCache[T any](cache CacheStorage, key string, value T, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return cache.Set(key, string(data), ttl)
}

// GetTypedFromPersistent 从 PersistentStorage 中类型安全地获取值
func GetTypedFromPersistent[T any](persistent PersistentStorage, key string) (T, error) {
	var zero T
	raw, err := persistent.Get(key)
	if err != nil {
		return zero, err
	}
	return unmarshalAny[T](raw)
}

// SetTypedToPersistent 类型安全地设置值到 PersistentStorage
func SetTypedToPersistent[T any](persistent PersistentStorage, key string, value T) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return persistent.Set(key, string(data))
}

// unmarshalAny 通用的反序列化辅助函数
// 将 any 类型的原始值转换为目标类型 T
func unmarshalAny[T any](raw any) (T, error) {
	var zero T

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
