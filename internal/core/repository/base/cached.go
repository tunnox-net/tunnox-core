package base

import (
	"context"
	"time"

	"tunnox-core/internal/core/repository"
	"tunnox-core/internal/core/store"
)

// =============================================================================
// CachedRepository 带缓存的 Repository
// =============================================================================

// CachedRepository 带缓存的 Repository 实现
// 基于 CachedPersistentStore 提供缓存 + 持久化功能
type CachedRepository[E repository.Entity] struct {
	store      store.CachedPersistentStore[string, E]
	keyPrefix  string
	entityType string
	metrics    *store.RepositoryMetrics
}

// NewCachedRepository 创建带缓存的 Repository
func NewCachedRepository[E repository.Entity](
	s store.CachedPersistentStore[string, E],
	keyPrefix string,
	entityType string,
) *CachedRepository[E] {
	return &CachedRepository[E]{
		store:      s,
		keyPrefix:  keyPrefix,
		entityType: entityType,
		metrics:    store.NewRepositoryMetrics(),
	}
}

// buildKey 构建存储键
func (r *CachedRepository[E]) buildKey(id string) string {
	return r.keyPrefix + id
}

// Get 获取实体
func (r *CachedRepository[E]) Get(ctx context.Context, id string) (E, error) {
	start := time.Now()
	entity, err := r.store.Get(ctx, r.buildKey(id))
	r.metrics.RecordGet(time.Since(start), err)
	if err != nil {
		var zero E
		return zero, repository.NewRepositoryError(r.entityType, "Get", id, err)
	}
	return entity, nil
}

// Create 创建实体
func (r *CachedRepository[E]) Create(ctx context.Context, entity E) error {
	start := time.Now()
	key := r.buildKey(entity.GetID())

	// 检查是否已存在
	exists, err := r.store.Exists(ctx, key)
	if err != nil {
		r.metrics.RecordCreate(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Create", entity.GetID(), err)
	}
	if exists {
		err := store.ErrAlreadyExists
		r.metrics.RecordCreate(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Create", entity.GetID(), err)
	}

	// 写入存储
	err = r.store.Set(ctx, key, entity)
	r.metrics.RecordCreate(time.Since(start), err)
	if err != nil {
		return repository.NewRepositoryError(r.entityType, "Create", entity.GetID(), err)
	}
	return nil
}

// Update 更新实体
func (r *CachedRepository[E]) Update(ctx context.Context, entity E) error {
	start := time.Now()
	key := r.buildKey(entity.GetID())

	// 检查是否存在
	exists, err := r.store.Exists(ctx, key)
	if err != nil {
		r.metrics.RecordUpdate(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Update", entity.GetID(), err)
	}
	if !exists {
		err := store.ErrNotFound
		r.metrics.RecordUpdate(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Update", entity.GetID(), err)
	}

	// 更新存储
	err = r.store.Set(ctx, key, entity)
	r.metrics.RecordUpdate(time.Since(start), err)
	if err != nil {
		return repository.NewRepositoryError(r.entityType, "Update", entity.GetID(), err)
	}
	return nil
}

// Delete 删除实体
func (r *CachedRepository[E]) Delete(ctx context.Context, id string) error {
	start := time.Now()
	err := r.store.Delete(ctx, r.buildKey(id))
	r.metrics.RecordDelete(time.Since(start), err)
	if err != nil {
		return repository.NewRepositoryError(r.entityType, "Delete", id, err)
	}
	return nil
}

// Exists 检查实体是否存在
func (r *CachedRepository[E]) Exists(ctx context.Context, id string) (bool, error) {
	exists, err := r.store.Exists(ctx, r.buildKey(id))
	if err != nil {
		return false, repository.NewRepositoryError(r.entityType, "Exists", id, err)
	}
	return exists, nil
}

// BatchGet 批量获取实体
func (r *CachedRepository[E]) BatchGet(ctx context.Context, ids []string) (map[string]E, error) {
	if len(ids) == 0 {
		return map[string]E{}, nil
	}

	// 构建 keys
	keys := make([]string, len(ids))
	keyToID := make(map[string]string, len(ids))
	for i, id := range ids {
		key := r.buildKey(id)
		keys[i] = key
		keyToID[key] = id
	}

	// 批量获取
	result, err := r.store.BatchGet(ctx, keys)
	if err != nil {
		return nil, repository.NewRepositoryError(r.entityType, "BatchGet", "", err)
	}

	// 转换 key -> id
	entities := make(map[string]E, len(result))
	for key, entity := range result {
		if id, ok := keyToID[key]; ok {
			entities[id] = entity
		}
	}

	return entities, nil
}

// BatchSet 批量设置实体
func (r *CachedRepository[E]) BatchSet(ctx context.Context, entities []E) error {
	if len(entities) == 0 {
		return nil
	}

	items := make(map[string]E, len(entities))
	for _, entity := range entities {
		items[r.buildKey(entity.GetID())] = entity
	}

	err := r.store.BatchSet(ctx, items)
	if err != nil {
		return repository.NewRepositoryError(r.entityType, "BatchSet", "", err)
	}
	return nil
}

// BatchDelete 批量删除实体
func (r *CachedRepository[E]) BatchDelete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = r.buildKey(id)
	}

	err := r.store.BatchDelete(ctx, keys)
	if err != nil {
		return repository.NewRepositoryError(r.entityType, "BatchDelete", "", err)
	}
	return nil
}

// InvalidateCache 使缓存失效
func (r *CachedRepository[E]) InvalidateCache(ctx context.Context, id string) error {
	return r.store.InvalidateCache(ctx, r.buildKey(id))
}

// RefreshCache 刷新缓存
func (r *CachedRepository[E]) RefreshCache(ctx context.Context, id string) error {
	return r.store.RefreshCache(ctx, r.buildKey(id))
}

// GetFromPersistent 直接从持久化层获取（绕过缓存）
func (r *CachedRepository[E]) GetFromPersistent(ctx context.Context, id string) (E, error) {
	entity, err := r.store.GetFromPersistent(ctx, r.buildKey(id))
	if err != nil {
		var zero E
		return zero, repository.NewRepositoryError(r.entityType, "GetFromPersistent", id, err)
	}
	return entity, nil
}

// GetCacheStats 获取缓存统计信息
func (r *CachedRepository[E]) GetCacheStats() store.CacheStats {
	return r.store.GetCacheStats()
}

// GetMetrics 获取指标
func (r *CachedRepository[E]) GetMetrics() *store.RepositoryMetrics {
	return r.metrics
}

// GetStore 获取底层存储（用于高级操作）
func (r *CachedRepository[E]) GetStore() store.CachedPersistentStore[string, E] {
	return r.store
}
