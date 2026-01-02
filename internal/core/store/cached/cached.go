// Package cached 提供缓存+持久化组合存储实现
package cached

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"

	"tunnox-core/internal/core/store"
)

// =============================================================================
// CachedPersistentStore 缓存+持久化组合存储
// =============================================================================

// cachedPersistentStore 缓存+持久化组合存储实现
type cachedPersistentStore[K comparable, V any] struct {
	cache      store.SharedStore[K, V]
	persistent store.PersistentStore[K, V]
	config     store.CacheConfig

	// 负缓存（LRU）
	negativeCache *lru.Cache[K, time.Time]
	negativeMu    sync.RWMutex

	// 统计指标
	stats cacheStats
}

// cacheStats 缓存统计
type cacheStats struct {
	hits              atomic.Int64
	misses            atomic.Int64
	negativeHits      atomic.Int64
	bloomFilterRejects atomic.Int64
}

// NewCachedPersistentStore 创建缓存+持久化组合存储
func NewCachedPersistentStore[K comparable, V any](
	cache store.SharedStore[K, V],
	persistent store.PersistentStore[K, V],
	config store.CacheConfig,
) (store.CachedPersistentStore[K, V], error) {
	// 验证配置
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// 创建负缓存
	var negativeCache *lru.Cache[K, time.Time]
	if config.PenetrationProtection {
		var err error
		negativeCache, err = lru.New[K, time.Time](config.MaxNegativeCacheSize)
		if err != nil {
			return nil, err
		}
	}

	return &cachedPersistentStore[K, V]{
		cache:         cache,
		persistent:    persistent,
		config:        config,
		negativeCache: negativeCache,
	}, nil
}

// Get 获取值（Read-Through）
func (s *cachedPersistentStore[K, V]) Get(ctx context.Context, key K) (V, error) {
	var zero V

	// 1. 检查负缓存
	if s.config.PenetrationProtection && s.negativeCache != nil {
		s.negativeMu.RLock()
		if expireAt, ok := s.negativeCache.Get(key); ok {
			s.negativeMu.RUnlock()
			if time.Now().Before(expireAt) {
				s.stats.negativeHits.Add(1)
				return zero, store.ErrNotFound
			}
			// 过期了，从负缓存中移除
			s.negativeMu.Lock()
			s.negativeCache.Remove(key)
			s.negativeMu.Unlock()
		} else {
			s.negativeMu.RUnlock()
		}
	}

	// 2. 查缓存
	value, err := s.cache.Get(ctx, key)
	if err == nil {
		s.stats.hits.Add(1)
		return value, nil
	}

	// 缓存未命中
	s.stats.misses.Add(1)

	// 3. 查持久化存储
	if !s.config.LoadOnMiss {
		return zero, store.ErrNotFound
	}

	value, err = s.persistent.Get(ctx, key)
	if err != nil {
		// 记录负缓存
		if s.config.PenetrationProtection && errors.Is(err, store.ErrNotFound) {
			s.negativeMu.Lock()
			s.negativeCache.Add(key, time.Now().Add(s.config.NegativeTTL))
			s.negativeMu.Unlock()
		}
		return zero, err
	}

	// 4. 回填缓存（异步）
	go func() {
		ctx := context.Background()
		_ = s.cache.SetWithTTL(ctx, key, value, s.config.TTL)
	}()

	return value, nil
}

// Set 设置值
func (s *cachedPersistentStore[K, V]) Set(ctx context.Context, key K, value V) error {
	// 1. 删除负缓存
	if s.config.PenetrationProtection && s.negativeCache != nil {
		s.negativeMu.Lock()
		s.negativeCache.Remove(key)
		s.negativeMu.Unlock()
	}

	// 2. 根据写入策略执行
	switch s.config.WritePolicy {
	case store.WriteThrough:
		// 先写持久化
		if err := s.persistent.Set(ctx, key, value); err != nil {
			return err
		}
		// 再写缓存
		return s.cache.SetWithTTL(ctx, key, value, s.config.TTL)

	case store.WriteBehind:
		// 先写持久化
		if err := s.persistent.Set(ctx, key, value); err != nil {
			return err
		}
		// 异步写缓存
		go func() {
			_ = s.cache.SetWithTTL(context.Background(), key, value, s.config.TTL)
		}()
		return nil

	default:
		return s.persistent.Set(ctx, key, value)
	}
}

// Delete 删除值
func (s *cachedPersistentStore[K, V]) Delete(ctx context.Context, key K) error {
	// 删除负缓存
	if s.config.PenetrationProtection && s.negativeCache != nil {
		s.negativeMu.Lock()
		s.negativeCache.Remove(key)
		s.negativeMu.Unlock()
	}

	// 删除缓存
	_ = s.cache.Delete(ctx, key)

	// 删除持久化
	return s.persistent.Delete(ctx, key)
}

// Exists 检查键是否存在
func (s *cachedPersistentStore[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	// 先查缓存
	exists, err := s.cache.Exists(ctx, key)
	if err == nil && exists {
		return true, nil
	}

	// 再查持久化
	return s.persistent.Exists(ctx, key)
}

// BatchGet 批量获取
func (s *cachedPersistentStore[K, V]) BatchGet(ctx context.Context, keys []K) (map[K]V, error) {
	if len(keys) == 0 {
		return map[K]V{}, nil
	}

	result := make(map[K]V, len(keys))
	var missedKeys []K

	// 1. 批量查缓存
	cacheResult, _ := s.cache.BatchGet(ctx, keys)
	for _, key := range keys {
		if value, ok := cacheResult[key]; ok {
			result[key] = value
			s.stats.hits.Add(1)
		} else {
			missedKeys = append(missedKeys, key)
			s.stats.misses.Add(1)
		}
	}

	if len(missedKeys) == 0 {
		return result, nil
	}

	// 2. 批量查持久化存储
	persistentResult, err := s.persistent.BatchGet(ctx, missedKeys)
	if err != nil {
		return result, err
	}

	// 3. 合并结果并回填缓存
	for key, value := range persistentResult {
		result[key] = value
		// 异步回填缓存
		go func(k K, v V) {
			_ = s.cache.SetWithTTL(context.Background(), k, v, s.config.TTL)
		}(key, value)
	}

	return result, nil
}

// BatchSet 批量设置
func (s *cachedPersistentStore[K, V]) BatchSet(ctx context.Context, items map[K]V) error {
	if len(items) == 0 {
		return nil
	}

	// 清除负缓存
	if s.config.PenetrationProtection && s.negativeCache != nil {
		s.negativeMu.Lock()
		for key := range items {
			s.negativeCache.Remove(key)
		}
		s.negativeMu.Unlock()
	}

	// 写持久化
	if err := s.persistent.BatchSet(ctx, items); err != nil {
		return err
	}

	// 写缓存
	for key, value := range items {
		_ = s.cache.SetWithTTL(ctx, key, value, s.config.TTL)
	}

	return nil
}

// BatchDelete 批量删除
func (s *cachedPersistentStore[K, V]) BatchDelete(ctx context.Context, keys []K) error {
	if len(keys) == 0 {
		return nil
	}

	// 清除负缓存
	if s.config.PenetrationProtection && s.negativeCache != nil {
		s.negativeMu.Lock()
		for _, key := range keys {
			s.negativeCache.Remove(key)
		}
		s.negativeMu.Unlock()
	}

	// 删除缓存
	_ = s.cache.BatchDelete(ctx, keys)

	// 删除持久化
	return s.persistent.BatchDelete(ctx, keys)
}

// InvalidateCache 使缓存失效
func (s *cachedPersistentStore[K, V]) InvalidateCache(ctx context.Context, key K) error {
	return s.cache.Delete(ctx, key)
}

// RefreshCache 刷新缓存（从持久化层重新加载）
func (s *cachedPersistentStore[K, V]) RefreshCache(ctx context.Context, key K) error {
	// 从持久化层获取
	value, err := s.persistent.Get(ctx, key)
	if err != nil {
		// 如果不存在，删除缓存
		if errors.Is(err, store.ErrNotFound) {
			_ = s.cache.Delete(ctx, key)
		}
		return err
	}

	// 更新缓存
	return s.cache.SetWithTTL(ctx, key, value, s.config.TTL)
}

// GetFromPersistent 直接从持久化层获取（绕过缓存）
func (s *cachedPersistentStore[K, V]) GetFromPersistent(ctx context.Context, key K) (V, error) {
	return s.persistent.Get(ctx, key)
}

// GetCacheStats 获取缓存统计
func (s *cachedPersistentStore[K, V]) GetCacheStats() store.CacheStats {
	hits := s.stats.hits.Load()
	misses := s.stats.misses.Load()
	total := hits + misses

	var hitRate float64
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}

	return store.CacheStats{
		Hits:               hits,
		Misses:             misses,
		NegativeHits:       s.stats.negativeHits.Load(),
		BloomFilterRejects: s.stats.bloomFilterRejects.Load(),
		HitRate:            hitRate,
	}
}

// Close 关闭存储
func (s *cachedPersistentStore[K, V]) Close() error {
	// 关闭缓存
	if closer, ok := s.cache.(store.Closer); ok {
		_ = closer.Close()
	}

	// 关闭持久化
	if closer, ok := s.persistent.(store.Closer); ok {
		_ = closer.Close()
	}

	return nil
}

// =============================================================================
// 工厂函数
// =============================================================================

// NewDefaultCachedPersistentStore 使用默认配置创建
func NewDefaultCachedPersistentStore[K comparable, V any](
	cache store.SharedStore[K, V],
	persistent store.PersistentStore[K, V],
) (store.CachedPersistentStore[K, V], error) {
	return NewCachedPersistentStore(cache, persistent, store.DefaultCacheConfig())
}
