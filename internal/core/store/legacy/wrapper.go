// Package legacy 提供旧存储接口的兼容层
//
// 用于在迁移过程中让新存储架构兼容旧代码
package legacy

import (
	"context"
	"encoding/json"
	"time"

	"tunnox-core/internal/core/store"
	storetypes "tunnox-core/internal/core/storage/types"
)

// =============================================================================
// LegacyStoreWrapper 旧存储接口包装器
// =============================================================================

// LegacyStoreWrapper 将新 Store 接口包装为旧 Storage 接口
// 用于迁移过程中的兼容
type LegacyStoreWrapper struct {
	// store 新存储接口
	store store.TTLStore[string, string]

	// listStore 列表存储接口（可选）
	listStore store.SetStore[string, string]

	// ctx 操作上下文
	ctx context.Context
}

// NewLegacyStoreWrapper 创建旧存储包装器
func NewLegacyStoreWrapper(
	newStore store.TTLStore[string, string],
	listStore store.SetStore[string, string],
	ctx context.Context,
) *LegacyStoreWrapper {
	return &LegacyStoreWrapper{
		store:     newStore,
		listStore: listStore,
		ctx:       ctx,
	}
}

// =============================================================================
// Storage 接口实现
// =============================================================================

// Set 设置键值对
func (w *LegacyStoreWrapper) Set(key string, value any, ttl time.Duration) error {
	// 序列化值
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}

	if ttl > 0 {
		return w.store.SetWithTTL(w.ctx, key, string(jsonBytes), ttl)
	}
	return w.store.Set(w.ctx, key, string(jsonBytes))
}

// Get 获取值
func (w *LegacyStoreWrapper) Get(key string) (any, error) {
	value, err := w.store.Get(w.ctx, key)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, storetypes.ErrKeyNotFound
		}
		return nil, err
	}
	return value, nil
}

// Delete 删除键
func (w *LegacyStoreWrapper) Delete(key string) error {
	return w.store.Delete(w.ctx, key)
}

// Exists 检查键是否存在
func (w *LegacyStoreWrapper) Exists(key string) (bool, error) {
	return w.store.Exists(w.ctx, key)
}

// SetExpiration 设置过期时间
func (w *LegacyStoreWrapper) SetExpiration(key string, ttl time.Duration) error {
	return w.store.Refresh(w.ctx, key, ttl)
}

// GetExpiration 获取过期时间
func (w *LegacyStoreWrapper) GetExpiration(key string) (time.Duration, error) {
	return w.store.GetTTL(w.ctx, key)
}

// CleanupExpired 清理过期数据（新存储自动处理，无需手动清理）
func (w *LegacyStoreWrapper) CleanupExpired() error {
	return nil
}

// Close 关闭存储
func (w *LegacyStoreWrapper) Close() error {
	if closer, ok := w.store.(store.Closer); ok {
		return closer.Close()
	}
	return nil
}

// =============================================================================
// ListStore 接口实现
// =============================================================================

// SetList 设置列表（使用 SET 存储实现）
func (w *LegacyStoreWrapper) SetList(key string, values []any, ttl time.Duration) error {
	if w.listStore == nil {
		return nil
	}

	// 先清空现有列表
	members, _ := w.listStore.Members(w.ctx, key)
	for _, member := range members {
		_ = w.listStore.Remove(w.ctx, key, member)
	}

	// 添加新元素
	for _, value := range values {
		jsonBytes, err := json.Marshal(value)
		if err != nil {
			continue
		}
		if err := w.listStore.Add(w.ctx, key, string(jsonBytes)); err != nil {
			return err
		}
	}

	return nil
}

// GetList 获取列表
func (w *LegacyStoreWrapper) GetList(key string) ([]any, error) {
	if w.listStore == nil {
		return nil, nil
	}

	members, err := w.listStore.Members(w.ctx, key)
	if err != nil {
		if store.IsNotFound(err) {
			return []any{}, nil
		}
		return nil, err
	}

	result := make([]any, 0, len(members))
	for _, member := range members {
		result = append(result, member)
	}

	return result, nil
}

// AppendToList 追加元素到列表
func (w *LegacyStoreWrapper) AppendToList(key string, value any) error {
	if w.listStore == nil {
		return nil
	}

	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return w.listStore.Add(w.ctx, key, string(jsonBytes))
}

// RemoveFromList 从列表移除元素
func (w *LegacyStoreWrapper) RemoveFromList(key string, value any) error {
	if w.listStore == nil {
		return nil
	}

	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return w.listStore.Remove(w.ctx, key, string(jsonBytes))
}

// =============================================================================
// 接口验证
// =============================================================================

// 确保实现了 Storage 接口
var _ storetypes.Storage = (*LegacyStoreWrapper)(nil)

// 确保实现了 ListStore 接口
var _ storetypes.ListStore = (*LegacyStoreWrapper)(nil)

// =============================================================================
// NewStorageWrapper 反向包装器
// =============================================================================

// NewStorageWrapper 将旧 Storage 接口包装为新 Store 接口
// 用于让旧存储实现可以在新架构中使用
type NewStorageWrapper[V any] struct {
	// storage 旧存储接口
	storage storetypes.Storage

	// ctx 操作上下文
	ctx context.Context

	// keyPrefix 键前缀
	keyPrefix string
}

// NewNewStorageWrapper 创建新存储包装器
func NewNewStorageWrapper[V any](
	storage storetypes.Storage,
	keyPrefix string,
	ctx context.Context,
) *NewStorageWrapper[V] {
	return &NewStorageWrapper[V]{
		storage:   storage,
		keyPrefix: keyPrefix,
		ctx:       ctx,
	}
}

// buildKey 构建存储键
func (w *NewStorageWrapper[V]) buildKey(key string) string {
	return w.keyPrefix + key
}

// Get 获取值
func (w *NewStorageWrapper[V]) Get(ctx context.Context, key string) (V, error) {
	var zero V
	value, err := w.storage.Get(w.buildKey(key))
	if err != nil {
		if err == storetypes.ErrKeyNotFound {
			return zero, store.ErrNotFound
		}
		return zero, err
	}

	// 尝试类型转换
	if v, ok := value.(V); ok {
		return v, nil
	}

	// 尝试 JSON 反序列化
	if jsonStr, ok := value.(string); ok {
		var result V
		if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
			return zero, err
		}
		return result, nil
	}

	return zero, storetypes.ErrInvalidType
}

// Set 设置值
func (w *NewStorageWrapper[V]) Set(ctx context.Context, key string, value V) error {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.storage.Set(w.buildKey(key), string(jsonBytes), 0)
}

// SetWithTTL 设置值并指定 TTL
func (w *NewStorageWrapper[V]) SetWithTTL(ctx context.Context, key string, value V, ttl time.Duration) error {
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return w.storage.Set(w.buildKey(key), string(jsonBytes), ttl)
}

// Delete 删除值
func (w *NewStorageWrapper[V]) Delete(ctx context.Context, key string) error {
	return w.storage.Delete(w.buildKey(key))
}

// Exists 检查键是否存在
func (w *NewStorageWrapper[V]) Exists(ctx context.Context, key string) (bool, error) {
	return w.storage.Exists(w.buildKey(key))
}

// GetTTL 获取剩余 TTL
func (w *NewStorageWrapper[V]) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	return w.storage.GetExpiration(w.buildKey(key))
}

// Refresh 刷新 TTL
func (w *NewStorageWrapper[V]) Refresh(ctx context.Context, key string, ttl time.Duration) error {
	return w.storage.SetExpiration(w.buildKey(key), ttl)
}

// =============================================================================
// 接口验证
// =============================================================================

// 确保实现了 TTLStore 接口
var _ store.TTLStore[string, string] = (*NewStorageWrapper[string])(nil)
