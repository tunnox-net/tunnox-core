package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
)

// HybridStorage 混合存储实现
// 自动区分持久化数据和运行时数据，提供统一的存储接口
type HybridStorage struct {
	*dispose.ManagerBase

	cache      CacheStorage      // 缓存存储（Memory/Redis）
	persistent PersistentStorage // 持久化存储（Database/gRPC，纯内存模式为 nil）
	config     *HybridConfig     // 配置

	mu sync.RWMutex // 保护配置修改
}

// NewHybridStorage 创建混合存储
func NewHybridStorage(parentCtx context.Context, cache CacheStorage, persistent PersistentStorage, config *HybridConfig) *HybridStorage {
	if config == nil {
		config = DefaultHybridConfig()
	}

	// 如果未启用持久化，使用空实现
	if !config.EnablePersistent || persistent == nil {
		persistent = NewNullPersistentStorage()
		config.EnablePersistent = false
	}

	storage := &HybridStorage{
		ManagerBase: dispose.NewManager("HybridStorage", parentCtx),
		cache:       cache,
		persistent:  persistent,
		config:      config,
	}

	storage.SetCtx(parentCtx, storage.onClose)

	mode := "memory-only"
	if config.EnablePersistent {
		mode = "hybrid"
	}
	dispose.Infof("HybridStorage: initialized in %s mode", mode)

	return storage
}

// onClose 资源释放回调
func (h *HybridStorage) onClose() error {
	dispose.Infof("HybridStorage: closing")

	var errs []error

	// 关闭缓存
	if h.cache != nil {
		if err := h.cache.Close(); err != nil {
			errs = append(errs, fmt.Errorf("cache close error: %w", err))
		}
	}

	// 关闭持久化存储
	if h.persistent != nil {
		if err := h.persistent.Close(); err != nil {
			errs = append(errs, fmt.Errorf("persistent close error: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("HybridStorage close errors: %v", errs)
	}

	return nil
}

// isPersistent 判断 key 是否为持久化数据
func (h *HybridStorage) isPersistent(key string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, prefix := range h.config.PersistentPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// getCategory 获取数据分类
func (h *HybridStorage) getCategory(key string) DataCategory {
	if h.isPersistent(key) {
		return DataCategoryPersistent
	}
	return DataCategoryRuntime
}

// Set 设置键值对（自动识别数据类型）
func (h *HybridStorage) Set(key string, value interface{}, ttl time.Duration) error {
	category := h.getCategory(key)

	if category == DataCategoryPersistent {
		return h.setPersistent(key, value, ttl)
	}
	return h.setRuntime(key, value, ttl)
}

// setPersistent 设置持久化数据
func (h *HybridStorage) setPersistent(key string, value interface{}, ttl time.Duration) error {
	// 1. 写入持久化存储
	if h.config.EnablePersistent {
		if err := h.persistent.Set(key, value); err != nil {
			return fmt.Errorf("persistent storage error: %w", err)
		}
	}

	// 2. 写入缓存（使用配置的 TTL）
	cacheTTL := ttl
	if cacheTTL == 0 {
		cacheTTL = h.config.PersistentCacheTTL
	}

	if err := h.cache.Set(key, value, cacheTTL); err != nil {
		// 缓存写入失败不影响持久化结果，仅记录日志
		dispose.Warnf("HybridStorage: cache set failed for key %s: %v", key, err)
	}

	return nil
}

// setRuntime 设置运行时数据
func (h *HybridStorage) setRuntime(key string, value interface{}, ttl time.Duration) error {
	// 运行时数据仅写入缓存
	if ttl == 0 {
		ttl = h.config.DefaultCacheTTL
	}
	return h.cache.Set(key, value, ttl)
}

// Get 获取值（自动识别数据类型）
func (h *HybridStorage) Get(key string) (interface{}, error) {
	// 1. 先从缓存读取
	if value, err := h.cache.Get(key); err == nil {
		return value, nil
	}

	// 2. 如果是持久化数据，从持久化存储读取
	category := h.getCategory(key)
	if category == DataCategoryPersistent && h.config.EnablePersistent {
		value, err := h.persistent.Get(key)
		if err != nil {
			return nil, err
		}

		// 3. 写回缓存（异步）
		go func() {
			cacheTTL := h.config.PersistentCacheTTL
			if err := h.cache.Set(key, value, cacheTTL); err != nil {
				dispose.Debugf("HybridStorage: cache write-back failed for key %s: %v", key, err)
			}
		}()

		return value, nil
	}

	// 3. 运行时数据不在缓存中，返回未找到
	return nil, ErrKeyNotFound
}

// Delete 删除键（自动识别数据类型）
func (h *HybridStorage) Delete(key string) error {
	category := h.getCategory(key)

	var errs []error

	// 1. 从缓存删除
	if err := h.cache.Delete(key); err != nil && err != ErrKeyNotFound {
		errs = append(errs, fmt.Errorf("cache delete error: %w", err))
	}

	// 2. 如果是持久化数据，从持久化存储删除
	if category == DataCategoryPersistent && h.config.EnablePersistent {
		if err := h.persistent.Delete(key); err != nil && err != ErrKeyNotFound {
			errs = append(errs, fmt.Errorf("persistent delete error: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("delete errors: %v", errs)
	}

	return nil
}

// Exists 检查键是否存在
func (h *HybridStorage) Exists(key string) (bool, error) {
	// 1. 先检查缓存
	if exists, err := h.cache.Exists(key); err == nil && exists {
		return true, nil
	}

	// 2. 如果是持久化数据，检查持久化存储
	category := h.getCategory(key)
	if category == DataCategoryPersistent && h.config.EnablePersistent {
		return h.persistent.Exists(key)
	}

	return false, nil
}

// SetPersistent 显式设置持久化数据（高级用法）
func (h *HybridStorage) SetPersistent(key string, value interface{}) error {
	return h.setPersistent(key, value, h.config.PersistentCacheTTL)
}

// SetRuntime 显式设置运行时数据（高级用法）
func (h *HybridStorage) SetRuntime(key string, value interface{}, ttl time.Duration) error {
	return h.setRuntime(key, value, ttl)
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

// 实现 Storage 接口的其他方法

func (h *HybridStorage) SetList(key string, values []interface{}, ttl time.Duration) error {
	// 列表操作暂不支持持久化，仅使用缓存
	return h.cache.Set(key, values, ttl)
}

func (h *HybridStorage) GetList(key string) ([]interface{}, error) {
	value, err := h.cache.Get(key)
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
	if err != nil && err != ErrKeyNotFound {
		return err
	}

	if list == nil {
		list = []interface{}{}
	}

	list = append(list, value)
	return h.cache.Set(key, list, h.config.DefaultCacheTTL)
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

	return h.cache.Set(key, newList, h.config.DefaultCacheTTL)
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
	// 仅支持缓存的原子操作
	if nxSetter, ok := h.cache.(interface {
		SetNX(string, interface{}, time.Duration) (bool, error)
	}); ok {
		return nxSetter.SetNX(key, value, ttl)
	}

	// 降级实现
	exists, err := h.cache.Exists(key)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	return true, h.cache.Set(key, value, ttl)
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
