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
// 自动区分持久化数据、共享数据和运行时数据，提供统一的存储接口
//
// 数据分类：
// - 持久化数据：写入持久化存储 + 缓存
// - 共享数据：写入共享缓存（Redis），用于跨节点通信
// - 运行时数据：仅写入本地缓存
type HybridStorage struct {
	*dispose.ManagerBase

	cache       CacheStorage      // 本地缓存存储（Memory）
	sharedCache CacheStorage      // 共享缓存存储（Redis，可选）
	persistent  PersistentStorage // 持久化存储（Database/gRPC，纯内存模式为 nil）
	config      *HybridConfig     // 配置

	mu sync.RWMutex // 保护配置修改
}

// NewHybridStorage 创建混合存储
func NewHybridStorage(parentCtx context.Context, cache CacheStorage, persistent PersistentStorage, config *HybridConfig) *HybridStorage {
	return NewHybridStorageWithSharedCache(parentCtx, cache, nil, persistent, config)
}

// NewHybridStorageWithSharedCache 创建带共享缓存的混合存储
// sharedCache 用于跨节点共享数据（如连接状态、隧道路由等）
func NewHybridStorageWithSharedCache(parentCtx context.Context, cache CacheStorage, sharedCache CacheStorage, persistent PersistentStorage, config *HybridConfig) *HybridStorage {
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
		sharedCache: sharedCache,
		persistent:  persistent,
		config:      config,
	}

	storage.SetCtx(parentCtx, storage.onClose)

	mode := "memory-only"
	if config.EnablePersistent {
		mode = "hybrid"
	}
	sharedMode := "disabled"
	if sharedCache != nil {
		sharedMode = "enabled"
	}
	dispose.Infof("HybridStorage: initialized in %s mode, shared cache: %s", mode, sharedMode)
	dispose.Infof("HybridStorage: SharedPrefixes=%v", config.SharedPrefixes)

	return storage
}

// onClose 资源释放回调
func (h *HybridStorage) onClose() error {
	dispose.Infof("HybridStorage: closing")

	var errs []error

	// 关闭本地缓存
	if h.cache != nil {
		if err := h.cache.Close(); err != nil {
			errs = append(errs, fmt.Errorf("cache close error: %w", err))
		}
	}

	// 关闭共享缓存
	if h.sharedCache != nil {
		if err := h.sharedCache.Close(); err != nil {
			errs = append(errs, fmt.Errorf("shared cache close error: %w", err))
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

// isShared 判断 key 是否为纯共享数据（仅跨节点共享，不持久化）
func (h *HybridStorage) isShared(key string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, prefix := range h.config.SharedPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// isSharedPersistent 判断 key 是否为共享且持久化数据
func (h *HybridStorage) isSharedPersistent(key string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, prefix := range h.config.SharedPersistentPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// getCategory 获取数据分类
func (h *HybridStorage) getCategory(key string) DataCategory {
	// 优先检查共享且持久化数据（热点缓存模式）
	if h.isSharedPersistent(key) {
		return DataCategorySharedPersistent
	}
	// 其次检查纯共享数据（跨节点通信优先级最高）
	if h.isShared(key) {
		return DataCategoryShared
	}
	if h.isPersistent(key) {
		return DataCategoryPersistent
	}
	return DataCategoryRuntime
}

// getCacheForKey 根据 key 获取应该使用的缓存
// 共享数据使用共享缓存（如果有），否则使用本地缓存
func (h *HybridStorage) getCacheForKey(key string) CacheStorage {
	if h.isShared(key) && h.sharedCache != nil {
		return h.sharedCache
	}
	return h.cache
}

// Set 设置键值对（自动识别数据类型）
func (h *HybridStorage) Set(key string, value interface{}, ttl time.Duration) error {
	category := h.getCategory(key)

	switch category {
	case DataCategoryPersistent:
		return h.setPersistent(key, value, ttl)
	case DataCategoryShared:
		return h.setShared(key, value, ttl)
	case DataCategorySharedPersistent:
		return h.setSharedPersistent(key, value, ttl)
	default:
		return h.setRuntime(key, value, ttl)
	}
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
	// 运行时数据仅写入本地缓存
	if ttl == 0 {
		ttl = h.config.DefaultCacheTTL
	}
	return h.cache.Set(key, value, ttl)
}

// setShared 设置共享数据（跨节点共享，不持久化）
func (h *HybridStorage) setShared(key string, value interface{}, ttl time.Duration) error {
	// 共享数据写入共享缓存（如果有），否则写入本地缓存
	cache := h.getCacheForKey(key)
	if ttl == 0 {
		ttl = h.config.DefaultCacheTTL
	}
	dispose.Debugf("HybridStorage.setShared: key=%s, hasSharedCache=%v, ttl=%v", key, h.sharedCache != nil, ttl)
	return cache.Set(key, value, ttl)
}

// setSharedPersistent 设置共享且持久化数据（热点缓存模式）
// 同时写入共享缓存和持久化存储，确保数据既能跨节点共享又能持久保存
func (h *HybridStorage) setSharedPersistent(key string, value interface{}, ttl time.Duration) error {
	// 1. 写入持久化存储（确保数据不丢失）
	if h.config.EnablePersistent {
		if err := h.persistent.Set(key, value); err != nil {
			return fmt.Errorf("persistent storage error: %w", err)
		}
	}

	// 2. 写入共享缓存（如果有）用于跨节点共享
	cacheTTL := ttl
	if cacheTTL == 0 {
		cacheTTL = h.config.SharedCacheTTL
	}

	if h.sharedCache != nil {
		if err := h.sharedCache.Set(key, value, cacheTTL); err != nil {
			// 缓存写入失败不影响持久化结果，仅记录日志
			dispose.Warnf("HybridStorage: shared cache set failed for key %s: %v", key, err)
		}
	} else {
		// 没有共享缓存时，写入本地缓存
		if err := h.cache.Set(key, value, cacheTTL); err != nil {
			dispose.Warnf("HybridStorage: local cache set failed for key %s: %v", key, err)
		}
	}

	dispose.Debugf("HybridStorage.setSharedPersistent: key=%s, hasSharedCache=%v, ttl=%v", key, h.sharedCache != nil, cacheTTL)
	return nil
}

// Get 获取值（自动识别数据类型）
func (h *HybridStorage) Get(key string) (interface{}, error) {
	category := h.getCategory(key)

	// 纯共享数据只从共享缓存读取（无持久化回落）
	if category == DataCategoryShared {
		cache := h.getCacheForKey(key)
		return cache.Get(key)
	}

	// 共享且持久化数据：热点缓存模式
	if category == DataCategorySharedPersistent {
		return h.getSharedPersistent(key)
	}

	// 1. 先从本地缓存读取
	if value, err := h.cache.Get(key); err == nil {
		return value, nil
	}

	// 2. 如果是持久化数据，从持久化存储读取
	if category == DataCategoryPersistent && h.config.EnablePersistent {
		value, err := h.persistent.Get(key)
		if err != nil {
			return nil, err
		}

		// 3. 写回本地缓存（异步）
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

// getSharedPersistent 获取共享且持久化数据（热点缓存模式）
// 读取顺序：共享缓存 -> 持久化存储 -> 回填共享缓存
func (h *HybridStorage) getSharedPersistent(key string) (interface{}, error) {
	// 1. 先尝试从共享缓存读取
	cache := h.sharedCache
	if cache == nil {
		cache = h.cache
	}

	if value, err := cache.Get(key); err == nil {
		dispose.Debugf("HybridStorage.getSharedPersistent: cache hit for key %s", key)
		return value, nil
	}

	// 2. 共享缓存 miss，从持久化存储读取
	if !h.config.EnablePersistent {
		return nil, ErrKeyNotFound
	}

	value, err := h.persistent.Get(key)
	if err != nil {
		return nil, err
	}

	dispose.Debugf("HybridStorage.getSharedPersistent: cache miss, loaded from persistent for key %s", key)

	// 3. 回填共享缓存（异步，不阻塞读取）
	go func() {
		cacheTTL := h.config.SharedCacheTTL
		if cacheTTL == 0 {
			cacheTTL = h.config.DefaultCacheTTL
		}
		if err := cache.Set(key, value, cacheTTL); err != nil {
			dispose.Warnf("HybridStorage: shared cache write-back failed for key %s: %v", key, err)
		} else {
			dispose.Debugf("HybridStorage.getSharedPersistent: write-back to cache for key %s, ttl=%v", key, cacheTTL)
		}
	}()

	return value, nil
}

// Delete 删除键（自动识别数据类型）
func (h *HybridStorage) Delete(key string) error {
	category := h.getCategory(key)

	var errs []error

	// 纯共享数据只从共享缓存删除
	if category == DataCategoryShared {
		cache := h.getCacheForKey(key)
		if err := cache.Delete(key); err != nil && err != ErrKeyNotFound {
			errs = append(errs, fmt.Errorf("shared cache delete error: %w", err))
		}
	} else if category == DataCategorySharedPersistent {
		// 共享且持久化数据：同时从共享缓存和持久化存储删除
		cache := h.sharedCache
		if cache == nil {
			cache = h.cache
		}
		if err := cache.Delete(key); err != nil && err != ErrKeyNotFound {
			errs = append(errs, fmt.Errorf("shared cache delete error: %w", err))
		}
		if h.config.EnablePersistent {
			if err := h.persistent.Delete(key); err != nil && err != ErrKeyNotFound {
				errs = append(errs, fmt.Errorf("persistent delete error: %w", err))
			}
		}
	} else {
		// 1. 从本地缓存删除
		if err := h.cache.Delete(key); err != nil && err != ErrKeyNotFound {
			errs = append(errs, fmt.Errorf("cache delete error: %w", err))
		}

		// 2. 如果是持久化数据，从持久化存储删除
		if category == DataCategoryPersistent && h.config.EnablePersistent {
			if err := h.persistent.Delete(key); err != nil && err != ErrKeyNotFound {
				errs = append(errs, fmt.Errorf("persistent delete error: %w", err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("delete errors: %v", errs)
	}

	return nil
}

// Exists 检查键是否存在
func (h *HybridStorage) Exists(key string) (bool, error) {
	category := h.getCategory(key)

	// 纯共享数据只检查共享缓存
	if category == DataCategoryShared {
		cache := h.getCacheForKey(key)
		return cache.Exists(key)
	}

	// 共享且持久化数据：先检查共享缓存，再检查持久化存储
	if category == DataCategorySharedPersistent {
		cache := h.sharedCache
		if cache == nil {
			cache = h.cache
		}
		if exists, err := cache.Exists(key); err == nil && exists {
			return true, nil
		}
		// 共享缓存不存在，检查持久化存储
		if h.config.EnablePersistent {
			return h.persistent.Exists(key)
		}
		return false, nil
	}

	// 1. 先检查本地缓存
	if exists, err := h.cache.Exists(key); err == nil && exists {
		return true, nil
	}

	// 2. 如果是持久化数据，检查持久化存储
	if category == DataCategoryPersistent && h.config.EnablePersistent {
		return h.persistent.Exists(key)
	}

	return false, nil
}
