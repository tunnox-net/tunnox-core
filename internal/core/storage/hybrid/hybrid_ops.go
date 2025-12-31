package hybrid

import (
	"errors"
	"fmt"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage/types"
)

// 实现 Storage 接口的高级操作方法

func (h *Storage) SetList(key string, values []interface{}, ttl time.Duration) error {
	// 使用 Set 方法，自动根据 key 类别选择正确的存储
	return h.Set(key, values, ttl)
}

func (h *Storage) GetList(key string) ([]interface{}, error) {
	// 使用 Get 方法，自动根据 key 类别选择正确的存储和热点缓存逻辑
	value, err := h.Get(key)
	if err != nil {
		return nil, err
	}

	if list, ok := value.([]interface{}); ok {
		return list, nil
	}
	return nil, types.ErrInvalidType
}

func (h *Storage) AppendToList(key string, value interface{}) error {
	// 简化实现：获取列表，追加，重新设置
	list, err := h.GetList(key)
	if err != nil && !errors.Is(err, types.ErrKeyNotFound) {
		return err
	}

	if list == nil {
		list = []interface{}{}
	}

	list = append(list, value)

	// 根据数据类别确定 TTL
	category := h.getCategory(key)
	var ttl time.Duration
	switch category {
	case DataCategorySharedPersistent:
		ttl = h.config.SharedCacheTTL
	case DataCategoryPersistent:
		ttl = h.config.PersistentCacheTTL
	default:
		ttl = h.config.DefaultCacheTTL
	}

	// 使用 Set 方法，它会根据 key 类别自动选择正确的存储（本地缓存/共享缓存/持久化）
	return h.Set(key, list, ttl)
}

func (h *Storage) RemoveFromList(key string, value interface{}) error {
	list, err := h.GetList(key)
	if err != nil {
		return err
	}

	// 移除匹配的元素
	newList := []interface{}{}
	for _, item := range list {
		if item != value {
			newList = append(newList, item)
		}
	}

	// 根据数据类别确定 TTL
	category := h.getCategory(key)
	var ttl time.Duration
	switch category {
	case DataCategorySharedPersistent:
		ttl = h.config.SharedCacheTTL
	case DataCategoryPersistent:
		ttl = h.config.PersistentCacheTTL
	default:
		ttl = h.config.DefaultCacheTTL
	}

	// 使用 Set 方法，它会根据 key 类别自动选择正确的存储
	return h.Set(key, newList, ttl)
}

func (h *Storage) SetHash(key string, field string, value interface{}) error {
	// Hash 操作暂不支持持久化，仅使用缓存
	hashKey := fmt.Sprintf("%s:%s", key, field)
	return h.cache.Set(hashKey, value, h.config.DefaultCacheTTL)
}

func (h *Storage) GetHash(key string, field string) (interface{}, error) {
	hashKey := fmt.Sprintf("%s:%s", key, field)
	return h.cache.Get(hashKey)
}

func (h *Storage) GetAllHash(key string) (map[string]interface{}, error) {
	// 简化实现：不支持
	return nil, coreerrors.New(coreerrors.CodeNotImplemented, "GetAllHash not supported in HybridStorage")
}

func (h *Storage) DeleteHash(key string, field string) error {
	hashKey := fmt.Sprintf("%s:%s", key, field)
	return h.cache.Delete(hashKey)
}

func (h *Storage) Incr(key string) (int64, error) {
	// 计数器操作暂不支持持久化，仅使用缓存
	value, err := h.cache.Get(key)
	if err != nil && err != types.ErrKeyNotFound {
		return 0, err
	}

	var count int64
	if value != nil {
		if c, ok := value.(int64); ok {
			count = c
		}
	}

	count++
	if err := h.cache.Set(key, count, h.config.DefaultCacheTTL); err != nil {
		return 0, err
	}

	return count, nil
}

func (h *Storage) IncrBy(key string, delta int64) (int64, error) {
	value, err := h.cache.Get(key)
	if err != nil && err != types.ErrKeyNotFound {
		return 0, err
	}

	var count int64
	if value != nil {
		if c, ok := value.(int64); ok {
			count = c
		}
	}

	count += delta
	if err := h.cache.Set(key, count, h.config.DefaultCacheTTL); err != nil {
		return 0, err
	}

	return count, nil
}

func (h *Storage) SetExpiration(key string, ttl time.Duration) error {
	// 仅支持缓存的过期时间设置
	value, err := h.cache.Get(key)
	if err != nil {
		return err
	}
	return h.cache.Set(key, value, ttl)
}

func (h *Storage) GetExpiration(key string) (time.Duration, error) {
	// 不支持
	return 0, coreerrors.New(coreerrors.CodeNotImplemented, "GetExpiration not supported in HybridStorage")
}

func (h *Storage) CleanupExpired() error {
	// 委托给缓存存储
	if cleaner, ok := h.cache.(interface{ CleanupExpired() error }); ok {
		return cleaner.CleanupExpired()
	}
	return nil
}

func (h *Storage) SetNX(key string, value interface{}, ttl time.Duration) (bool, error) {
	// 获取应该使用的缓存
	cache := h.getCacheForKey(key)

	// 尝试使用原子操作
	if nxSetter, ok := cache.(interface {
		SetNX(string, interface{}, time.Duration) (bool, error)
	}); ok {
		return nxSetter.SetNX(key, value, ttl)
	}

	// 降级实现
	exists, err := cache.Exists(key)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	return true, cache.Set(key, value, ttl)
}

func (h *Storage) CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error) {
	// 不支持
	return false, coreerrors.New(coreerrors.CodeNotImplemented, "CompareAndSwap not supported in HybridStorage")
}

func (h *Storage) Watch(key string, callback func(interface{})) error {
	// 不支持
	return coreerrors.New(coreerrors.CodeNotImplemented, "Watch not supported in HybridStorage")
}

func (h *Storage) Unwatch(key string) error {
	// 不支持
	return coreerrors.New(coreerrors.CodeNotImplemented, "Unwatch not supported in HybridStorage")
}

func (h *Storage) Close() error {
	h.ManagerBase.Close()
	return nil
}

// SetNXRuntime 原子设置运行时数据（仅当键不存在时）
// 用于节点 ID 分配等需要原子操作的场景
func (h *Storage) SetNXRuntime(key string, value interface{}, ttl time.Duration) (bool, error) {
	// 运行时数据优先写入共享缓存（Redis），确保多节点原子性
	if h.sharedCache != nil {
		if nxSetter, ok := h.sharedCache.(interface {
			SetNX(string, interface{}, time.Duration) (bool, error)
		}); ok {
			return nxSetter.SetNX(key, value, ttl)
		}
	}

	// 回退到本地缓存
	if nxSetter, ok := h.cache.(interface {
		SetNX(string, interface{}, time.Duration) (bool, error)
	}); ok {
		return nxSetter.SetNX(key, value, ttl)
	}

	// 降级实现（非原子，仅用于兼容）
	exists, err := h.Exists(key)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	return true, h.setRuntime(key, value, ttl)
}
