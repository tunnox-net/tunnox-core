package storage

import (
	"errors"
	"fmt"
	"time"

	"tunnox-core/internal/core/dispose"
)

// 实现 Storage 接口的高级操作方法

func (h *HybridStorage) SetList(key string, values []interface{}, ttl time.Duration) error {
	// 使用 Set 方法，自动根据 key 类别选择正确的存储
	return h.Set(key, values, ttl)
}

func (h *HybridStorage) GetList(key string) ([]interface{}, error) {
	// 使用 Get 方法，自动根据 key 类别选择正确的存储和热点缓存逻辑
	value, err := h.Get(key)
	if err != nil {
		return nil, err
	}

	if list, ok := value.([]interface{}); ok {
		return list, nil
	}
	return nil, ErrInvalidType
}

func (h *HybridStorage) AppendToList(key string, value interface{}) error {
	// 简化实现：获取列表，追加，重新设置
	list, err := h.GetList(key)
	if err != nil && !errors.Is(err, ErrKeyNotFound) {
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

func (h *HybridStorage) RemoveFromList(key string, value interface{}) error {
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

func (h *HybridStorage) SetHash(key string, field string, value interface{}) error {
	// Hash 操作暂不支持持久化，仅使用缓存
	hashKey := fmt.Sprintf("%s:%s", key, field)
	return h.cache.Set(hashKey, value, h.config.DefaultCacheTTL)
}

func (h *HybridStorage) GetHash(key string, field string) (interface{}, error) {
	hashKey := fmt.Sprintf("%s:%s", key, field)
	return h.cache.Get(hashKey)
}

func (h *HybridStorage) GetAllHash(key string) (map[string]interface{}, error) {
	// 简化实现：不支持
	return nil, fmt.Errorf("GetAllHash not supported in HybridStorage")
}

func (h *HybridStorage) DeleteHash(key string, field string) error {
	hashKey := fmt.Sprintf("%s:%s", key, field)
	return h.cache.Delete(hashKey)
}

func (h *HybridStorage) Incr(key string) (int64, error) {
	// 计数器操作暂不支持持久化，仅使用缓存
	value, err := h.cache.Get(key)
	if err != nil && err != ErrKeyNotFound {
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

func (h *HybridStorage) IncrBy(key string, delta int64) (int64, error) {
	value, err := h.cache.Get(key)
	if err != nil && err != ErrKeyNotFound {
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

func (h *HybridStorage) SetExpiration(key string, ttl time.Duration) error {
	// 仅支持缓存的过期时间设置
	value, err := h.cache.Get(key)
	if err != nil {
		return err
	}
	return h.cache.Set(key, value, ttl)
}

func (h *HybridStorage) GetExpiration(key string) (time.Duration, error) {
	// 不支持
	return 0, fmt.Errorf("GetExpiration not supported in HybridStorage")
}

func (h *HybridStorage) CleanupExpired() error {
	// 委托给缓存存储
	if cleaner, ok := h.cache.(interface{ CleanupExpired() error }); ok {
		return cleaner.CleanupExpired()
	}
	return nil
}

func (h *HybridStorage) SetNX(key string, value interface{}, ttl time.Duration) (bool, error) {
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

func (h *HybridStorage) CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error) {
	// 不支持
	return false, fmt.Errorf("CompareAndSwap not supported in HybridStorage")
}

func (h *HybridStorage) Watch(key string, callback func(interface{})) error {
	// 不支持
	return fmt.Errorf("Watch not supported in HybridStorage")
}

func (h *HybridStorage) Unwatch(key string) error {
	// 不支持
	return fmt.Errorf("Unwatch not supported in HybridStorage")
}

func (h *HybridStorage) Close() error {
	h.ManagerBase.Close()
	return nil
}

// GetRemoteStorage 获取 RemoteStorage 实例（如果持久化存储是 RemoteStorage）
func (h *HybridStorage) GetRemoteStorage() *RemoteStorage {
	if h.persistent == nil {
		return nil
	}
	if remote, ok := h.persistent.(*RemoteStorage); ok {
		return remote
	}
	return nil
}

// SetNXRuntime 原子设置运行时数据（仅当键不存在时）
// 用于节点 ID 分配等需要原子操作的场景
func (h *HybridStorage) SetNXRuntime(key string, value interface{}, ttl time.Duration) (bool, error) {
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

// GetConfig 获取配置（只读）
func (h *HybridStorage) GetConfig() *HybridConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 返回副本，防止外部修改
	configCopy := *h.config
	configCopy.PersistentPrefixes = append([]string{}, h.config.PersistentPrefixes...)
	return &configCopy
}

// UpdatePersistentPrefixes 更新持久化前缀列表（运行时配置）
func (h *HybridStorage) UpdatePersistentPrefixes(prefixes []string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.config.PersistentPrefixes = append([]string{}, prefixes...)
	dispose.Infof("HybridStorage: updated persistent prefixes: %v", prefixes)
}

// IsPersistentEnabled 检查是否启用持久化
func (h *HybridStorage) IsPersistentEnabled() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.config.EnablePersistent
}

// GetPersistentStorage 获取持久化存储实例（用于按字段查询）
func (h *HybridStorage) GetPersistentStorage() PersistentStorage {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.persistent
}

// SetPersistent 显式设置持久化数据（高级用法）
func (h *HybridStorage) SetPersistent(key string, value interface{}) error {
	return h.setPersistent(key, value, h.config.PersistentCacheTTL)
}

// SetRuntime 显式设置运行时数据（高级用法）
func (h *HybridStorage) SetRuntime(key string, value interface{}, ttl time.Duration) error {
	return h.setRuntime(key, value, ttl)
}
