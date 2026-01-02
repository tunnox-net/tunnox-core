// Package memory 提供内存存储实现
package memory

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"tunnox-core/internal/core/store"
)

// =============================================================================
// MemoryStore 内存存储实现
// =============================================================================

// item 存储项
type item[V any] struct {
	value    V
	expireAt time.Time
}

// isExpired 检查是否过期
func (i *item[V]) isExpired() bool {
	if i.expireAt.IsZero() {
		return false
	}
	return time.Now().After(i.expireAt)
}

// MemoryStore 内存存储实现
type MemoryStore[K comparable, V any] struct {
	data    map[K]*item[V]
	mu      sync.RWMutex
	closed  bool
	metrics *store.StoreMetrics
}

// NewMemoryStore 创建内存存储
func NewMemoryStore[K comparable, V any]() *MemoryStore[K, V] {
	return &MemoryStore[K, V]{
		data:    make(map[K]*item[V]),
		metrics: store.NewStoreMetrics(),
	}
}

// Get 获取值
func (s *MemoryStore[K, V]) Get(ctx context.Context, key K) (V, error) {
	start := time.Now()
	s.mu.RLock()
	defer s.mu.RUnlock()

	var zero V
	if s.closed {
		s.metrics.RecordGet(time.Since(start), store.ErrClosed)
		return zero, store.ErrClosed
	}

	it, ok := s.data[key]
	if !ok || it.isExpired() {
		s.metrics.RecordGet(time.Since(start), store.ErrNotFound)
		return zero, store.ErrNotFound
	}

	s.metrics.RecordGet(time.Since(start), nil)
	return it.value, nil
}

// Set 设置值
func (s *MemoryStore[K, V]) Set(ctx context.Context, key K, value V) error {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		s.metrics.RecordSet(time.Since(start), store.ErrClosed)
		return store.ErrClosed
	}

	s.data[key] = &item[V]{value: value}
	s.metrics.RecordSet(time.Since(start), nil)
	return nil
}

// Delete 删除值
func (s *MemoryStore[K, V]) Delete(ctx context.Context, key K) error {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		s.metrics.RecordDelete(time.Since(start), store.ErrClosed)
		return store.ErrClosed
	}

	delete(s.data, key)
	s.metrics.RecordDelete(time.Since(start), nil)
	return nil
}

// Exists 检查键是否存在
func (s *MemoryStore[K, V]) Exists(ctx context.Context, key K) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return false, store.ErrClosed
	}

	it, ok := s.data[key]
	if !ok || it.isExpired() {
		return false, nil
	}
	return true, nil
}

// SetWithTTL 设置值并指定 TTL
func (s *MemoryStore[K, V]) SetWithTTL(ctx context.Context, key K, value V, ttl time.Duration) error {
	start := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		s.metrics.RecordSet(time.Since(start), store.ErrClosed)
		return store.ErrClosed
	}

	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = time.Now().Add(ttl)
	}

	s.data[key] = &item[V]{value: value, expireAt: expireAt}
	s.metrics.RecordSet(time.Since(start), nil)
	return nil
}

// GetTTL 获取剩余 TTL
func (s *MemoryStore[K, V]) GetTTL(ctx context.Context, key K) (time.Duration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, store.ErrClosed
	}

	it, ok := s.data[key]
	if !ok || it.isExpired() {
		return 0, store.ErrNotFound
	}

	if it.expireAt.IsZero() {
		return -1, nil // 永不过期
	}

	remaining := time.Until(it.expireAt)
	if remaining < 0 {
		return 0, store.ErrNotFound
	}
	return remaining, nil
}

// Refresh 刷新 TTL
func (s *MemoryStore[K, V]) Refresh(ctx context.Context, key K, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return store.ErrClosed
	}

	it, ok := s.data[key]
	if !ok || it.isExpired() {
		return store.ErrNotFound
	}

	if ttl > 0 {
		it.expireAt = time.Now().Add(ttl)
	} else {
		it.expireAt = time.Time{}
	}
	return nil
}

// BatchGet 批量获取
func (s *MemoryStore[K, V]) BatchGet(ctx context.Context, keys []K) (map[K]V, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, store.ErrClosed
	}

	result := make(map[K]V, len(keys))
	for _, key := range keys {
		if it, ok := s.data[key]; ok && !it.isExpired() {
			result[key] = it.value
		}
	}
	return result, nil
}

// BatchSet 批量设置
func (s *MemoryStore[K, V]) BatchSet(ctx context.Context, items map[K]V) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return store.ErrClosed
	}

	for key, value := range items {
		s.data[key] = &item[V]{value: value}
	}
	return nil
}

// BatchDelete 批量删除
func (s *MemoryStore[K, V]) BatchDelete(ctx context.Context, keys []K) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return store.ErrClosed
	}

	for _, key := range keys {
		delete(s.data, key)
	}
	return nil
}

// SetNX 仅当键不存在时设置
func (s *MemoryStore[K, V]) SetNX(ctx context.Context, key K, value V) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false, store.ErrClosed
	}

	if it, ok := s.data[key]; ok && !it.isExpired() {
		return false, nil
	}

	s.data[key] = &item[V]{value: value}
	return true, nil
}

// SetNXWithTTL 仅当键不存在时设置并指定 TTL
func (s *MemoryStore[K, V]) SetNXWithTTL(ctx context.Context, key K, value V, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return false, store.ErrClosed
	}

	if it, ok := s.data[key]; ok && !it.isExpired() {
		return false, nil
	}

	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = time.Now().Add(ttl)
	}

	s.data[key] = &item[V]{value: value, expireAt: expireAt}
	return true, nil
}

// Clear 清空所有数据
func (s *MemoryStore[K, V]) Clear(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return store.ErrClosed
	}

	s.data = make(map[K]*item[V])
	return nil
}

// Keys 获取所有键
func (s *MemoryStore[K, V]) Keys(ctx context.Context, pattern string) ([]K, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, store.ErrClosed
	}

	keys := make([]K, 0, len(s.data))
	for key, it := range s.data {
		if !it.isExpired() {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

// Close 关闭存储
func (s *MemoryStore[K, V]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.data = nil
	return nil
}

// GetMetrics 获取指标
func (s *MemoryStore[K, V]) GetMetrics() *store.StoreMetrics {
	return s.metrics
}

// CleanExpired 清理过期数据
func (s *MemoryStore[K, V]) CleanExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for key, it := range s.data {
		if it.isExpired() {
			delete(s.data, key)
			count++
		}
	}
	return count
}

// StartCleanupRoutine 启动定期清理协程
func (s *MemoryStore[K, V]) StartCleanupRoutine(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.CleanExpired()
			}
		}
	}()
}

// =============================================================================
// MemorySetStore 内存集合存储
// =============================================================================

// MemorySetStore 内存集合存储
type MemorySetStore[K comparable, V comparable] struct {
	data   map[K]map[V]struct{}
	mu     sync.RWMutex
	closed bool
}

// NewMemorySetStore 创建内存集合存储
func NewMemorySetStore[K comparable, V comparable]() *MemorySetStore[K, V] {
	return &MemorySetStore[K, V]{
		data: make(map[K]map[V]struct{}),
	}
}

// Add 向集合添加元素
func (s *MemorySetStore[K, V]) Add(ctx context.Context, key K, value V) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return store.ErrClosed
	}

	if s.data[key] == nil {
		s.data[key] = make(map[V]struct{})
	}
	s.data[key][value] = struct{}{}
	return nil
}

// Remove 从集合移除元素
func (s *MemorySetStore[K, V]) Remove(ctx context.Context, key K, value V) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return store.ErrClosed
	}

	if set, ok := s.data[key]; ok {
		delete(set, value)
		if len(set) == 0 {
			delete(s.data, key)
		}
	}
	return nil
}

// Contains 检查元素是否在集合中
func (s *MemorySetStore[K, V]) Contains(ctx context.Context, key K, value V) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return false, store.ErrClosed
	}

	if set, ok := s.data[key]; ok {
		_, exists := set[value]
		return exists, nil
	}
	return false, nil
}

// Members 获取集合所有成员
func (s *MemorySetStore[K, V]) Members(ctx context.Context, key K) ([]V, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, store.ErrClosed
	}

	set, ok := s.data[key]
	if !ok {
		return []V{}, nil
	}

	members := make([]V, 0, len(set))
	for v := range set {
		members = append(members, v)
	}
	return members, nil
}

// Size 获取集合大小
func (s *MemorySetStore[K, V]) Size(ctx context.Context, key K) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return 0, store.ErrClosed
	}

	if set, ok := s.data[key]; ok {
		return int64(len(set)), nil
	}
	return 0, nil
}

// Close 关闭存储
func (s *MemorySetStore[K, V]) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.data = nil
	return nil
}

// =============================================================================
// JSON 序列化辅助
// =============================================================================

// JSONMemoryStore JSON 序列化的内存存储
// 用于需要持久化快照的场景
type JSONMemoryStore[K comparable, V any] struct {
	*MemoryStore[K, V]
}

// NewJSONMemoryStore 创建 JSON 序列化的内存存储
func NewJSONMemoryStore[K comparable, V any]() *JSONMemoryStore[K, V] {
	return &JSONMemoryStore[K, V]{
		MemoryStore: NewMemoryStore[K, V](),
	}
}

// Snapshot 获取数据快照（JSON 格式）
func (s *JSONMemoryStore[K, V]) Snapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 过滤掉过期项
	snapshot := make(map[K]V)
	for key, it := range s.data {
		if !it.isExpired() {
			snapshot[key] = it.value
		}
	}

	return json.Marshal(snapshot)
}

// Restore 从快照恢复数据
func (s *JSONMemoryStore[K, V]) Restore(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshot := make(map[K]V)
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return err
	}

	s.data = make(map[K]*item[V], len(snapshot))
	for key, value := range snapshot {
		s.data[key] = &item[V]{value: value}
	}
	return nil
}
