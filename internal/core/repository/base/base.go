// Package base 提供 Repository 的基础实现
package base

import (
	"context"
	"time"

	"tunnox-core/internal/core/repository"
	"tunnox-core/internal/core/store"
)

// =============================================================================
// BaseRepository 基础实现
// =============================================================================

// BaseRepository 基础 Repository 实现
// 提供简单的 CRUD 操作，基于 Store 接口
type BaseRepository[E repository.Entity] struct {
	store      store.Store[string, E]
	keyPrefix  string
	entityType string
	metrics    *store.RepositoryMetrics
}

// NewBaseRepository 创建基础 Repository
func NewBaseRepository[E repository.Entity](
	s store.Store[string, E],
	keyPrefix string,
	entityType string,
) *BaseRepository[E] {
	return &BaseRepository[E]{
		store:      s,
		keyPrefix:  keyPrefix,
		entityType: entityType,
		metrics:    store.NewRepositoryMetrics(),
	}
}

// buildKey 构建存储键
func (r *BaseRepository[E]) buildKey(id string) string {
	return r.keyPrefix + id
}

// Get 获取实体
func (r *BaseRepository[E]) Get(ctx context.Context, id string) (E, error) {
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
func (r *BaseRepository[E]) Create(ctx context.Context, entity E) error {
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
func (r *BaseRepository[E]) Update(ctx context.Context, entity E) error {
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
func (r *BaseRepository[E]) Delete(ctx context.Context, id string) error {
	start := time.Now()
	err := r.store.Delete(ctx, r.buildKey(id))
	r.metrics.RecordDelete(time.Since(start), err)
	if err != nil {
		return repository.NewRepositoryError(r.entityType, "Delete", id, err)
	}
	return nil
}

// Exists 检查实体是否存在
func (r *BaseRepository[E]) Exists(ctx context.Context, id string) (bool, error) {
	exists, err := r.store.Exists(ctx, r.buildKey(id))
	if err != nil {
		return false, repository.NewRepositoryError(r.entityType, "Exists", id, err)
	}
	return exists, nil
}

// GetMetrics 获取指标
func (r *BaseRepository[E]) GetMetrics() *store.RepositoryMetrics {
	return r.metrics
}

// =============================================================================
// BatchBaseRepository 支持批量操作的基础 Repository
// =============================================================================

// BatchBaseRepository 支持批量操作的基础 Repository
type BatchBaseRepository[E repository.Entity] struct {
	*BaseRepository[E]
	batchStore store.BatchStore[string, E]
}

// NewBatchBaseRepository 创建支持批量操作的基础 Repository
func NewBatchBaseRepository[E repository.Entity](
	s store.BatchStore[string, E],
	keyPrefix string,
	entityType string,
) *BatchBaseRepository[E] {
	return &BatchBaseRepository[E]{
		BaseRepository: NewBaseRepository[E](s, keyPrefix, entityType),
		batchStore:     s,
	}
}

// BatchGet 批量获取实体
func (r *BatchBaseRepository[E]) BatchGet(ctx context.Context, ids []string) (map[string]E, error) {
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
	result, err := r.batchStore.BatchGet(ctx, keys)
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

// BatchCreate 批量创建实体
func (r *BatchBaseRepository[E]) BatchCreate(ctx context.Context, entities []E) error {
	if len(entities) == 0 {
		return nil
	}

	items := make(map[string]E, len(entities))
	for _, entity := range entities {
		items[r.buildKey(entity.GetID())] = entity
	}

	err := r.batchStore.BatchSet(ctx, items)
	if err != nil {
		return repository.NewRepositoryError(r.entityType, "BatchCreate", "", err)
	}
	return nil
}

// BatchDelete 批量删除实体
func (r *BatchBaseRepository[E]) BatchDelete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = r.buildKey(id)
	}

	err := r.batchStore.BatchDelete(ctx, keys)
	if err != nil {
		return repository.NewRepositoryError(r.entityType, "BatchDelete", "", err)
	}
	return nil
}
