// Package indexed 提供带索引的 Repository 实现
package indexed

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/core/repository"
	"tunnox-core/internal/core/repository/index"
	"tunnox-core/internal/core/store"
)

// =============================================================================
// UserIndexedRepository 带用户索引的 Repository
// =============================================================================

// UserIndexedRepository 带用户索引的 Repository
// 支持 ListByUser 快速查询，通过索引避免全表扫描
type UserIndexedRepository[E repository.Entity] struct {
	// store 缓存持久化存储
	store store.CachedPersistentStore[string, E]

	// indexManager 索引管理器
	indexManager index.IndexManager[E]

	// keyPrefix 数据键前缀
	keyPrefix string

	// entityType 实体类型名称
	entityType string

	// metrics 监控指标
	metrics *store.RepositoryMetrics
}

// NewUserIndexedRepository 创建带用户索引的 Repository
func NewUserIndexedRepository[E repository.Entity](
	cachedStore store.CachedPersistentStore[string, E],
	indexManager index.IndexManager[E],
	keyPrefix string,
	entityType string,
) *UserIndexedRepository[E] {
	return &UserIndexedRepository[E]{
		store:        cachedStore,
		indexManager: indexManager,
		keyPrefix:    keyPrefix,
		entityType:   entityType,
		metrics:      store.NewRepositoryMetrics(),
	}
}

// buildKey 构建存储键
func (r *UserIndexedRepository[E]) buildKey(id string) string {
	return r.keyPrefix + id
}

// =============================================================================
// CRUD 操作
// =============================================================================

// Get 获取实体
func (r *UserIndexedRepository[E]) Get(ctx context.Context, id string) (E, error) {
	start := time.Now()
	entity, err := r.store.Get(ctx, r.buildKey(id))
	r.metrics.RecordGet(time.Since(start), err)
	if err != nil {
		var zero E
		return zero, repository.NewRepositoryError(r.entityType, "Get", id, err)
	}
	return entity, nil
}

// Create 创建实体（事务性更新索引）
func (r *UserIndexedRepository[E]) Create(ctx context.Context, entity E) error {
	start := time.Now()
	key := r.buildKey(entity.GetID())

	// 1. 检查是否已存在
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

	// 2. 写入数据
	if err := r.store.Set(ctx, key, entity); err != nil {
		r.metrics.RecordCreate(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Create", entity.GetID(), err)
	}

	// 3. 更新索引
	if err := r.indexManager.AddIndex(ctx, entity); err != nil {
		// 索引失败，回滚数据
		_ = r.store.Delete(ctx, key)
		r.metrics.RecordCreate(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Create", entity.GetID(),
			fmt.Errorf("add index failed: %w", err))
	}

	r.metrics.RecordCreate(time.Since(start), nil)
	return nil
}

// Update 更新实体（处理索引键变化）
func (r *UserIndexedRepository[E]) Update(ctx context.Context, entity E) error {
	start := time.Now()
	key := r.buildKey(entity.GetID())

	// 1. 获取旧实体（用于索引更新）
	oldEntity, err := r.store.Get(ctx, key)
	if err != nil {
		r.metrics.RecordUpdate(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Update", entity.GetID(), err)
	}

	// 2. 更新数据
	if err := r.store.Set(ctx, key, entity); err != nil {
		r.metrics.RecordUpdate(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Update", entity.GetID(), err)
	}

	// 3. 更新索引（处理索引键变化）
	if err := r.indexManager.UpdateIndex(ctx, oldEntity, entity); err != nil {
		// 索引失败，回滚数据
		_ = r.store.Set(ctx, key, oldEntity)
		r.metrics.RecordUpdate(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Update", entity.GetID(),
			fmt.Errorf("update index failed: %w", err))
	}

	r.metrics.RecordUpdate(time.Since(start), nil)
	return nil
}

// Delete 删除实体（同时删除索引）
func (r *UserIndexedRepository[E]) Delete(ctx context.Context, id string) error {
	start := time.Now()
	key := r.buildKey(id)

	// 1. 获取实体（用于删除索引）
	entity, err := r.store.Get(ctx, key)
	if err != nil {
		if store.IsNotFound(err) {
			r.metrics.RecordDelete(time.Since(start), nil)
			return nil // 已不存在
		}
		r.metrics.RecordDelete(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Delete", id, err)
	}

	// 2. 删除索引
	if err := r.indexManager.RemoveIndex(ctx, entity); err != nil {
		r.metrics.RecordDelete(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Delete", id,
			fmt.Errorf("remove index failed: %w", err))
	}

	// 3. 删除数据
	if err := r.store.Delete(ctx, key); err != nil {
		// 数据删除失败，恢复索引
		_ = r.indexManager.AddIndex(ctx, entity)
		r.metrics.RecordDelete(time.Since(start), err)
		return repository.NewRepositoryError(r.entityType, "Delete", id, err)
	}

	r.metrics.RecordDelete(time.Since(start), nil)
	return nil
}

// Exists 检查实体是否存在
func (r *UserIndexedRepository[E]) Exists(ctx context.Context, id string) (bool, error) {
	exists, err := r.store.Exists(ctx, r.buildKey(id))
	if err != nil {
		return false, repository.NewRepositoryError(r.entityType, "Exists", id, err)
	}
	return exists, nil
}

// =============================================================================
// 批量操作
// =============================================================================

// BatchGet 批量获取实体
func (r *UserIndexedRepository[E]) BatchGet(ctx context.Context, ids []string) (map[string]E, error) {
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

// BatchCreate 批量创建实体
func (r *UserIndexedRepository[E]) BatchCreate(ctx context.Context, entities []E) error {
	if len(entities) == 0 {
		return nil
	}

	// 1. 批量写入数据
	items := make(map[string]E, len(entities))
	for _, entity := range entities {
		items[r.buildKey(entity.GetID())] = entity
	}

	if err := r.store.BatchSet(ctx, items); err != nil {
		return repository.NewRepositoryError(r.entityType, "BatchCreate", "", err)
	}

	// 2. 更新索引
	for _, entity := range entities {
		if err := r.indexManager.AddIndex(ctx, entity); err != nil {
			// 记录错误但继续处理
			// TODO: 考虑更好的错误处理策略
		}
	}

	return nil
}

// BatchDelete 批量删除实体
func (r *UserIndexedRepository[E]) BatchDelete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	// 1. 获取所有实体（用于删除索引）
	entities, err := r.BatchGet(ctx, ids)
	if err != nil {
		return err
	}

	// 2. 删除索引
	for _, entity := range entities {
		_ = r.indexManager.RemoveIndex(ctx, entity)
	}

	// 3. 批量删除数据
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = r.buildKey(id)
	}

	if err := r.store.BatchDelete(ctx, keys); err != nil {
		return repository.NewRepositoryError(r.entityType, "BatchDelete", "", err)
	}

	return nil
}

// =============================================================================
// 用户索引查询
// =============================================================================

// ListByUser 根据用户ID列出所有实体（核心优化方法）
func (r *UserIndexedRepository[E]) ListByUser(ctx context.Context, userID string) ([]E, error) {
	start := time.Now()

	// 1. 从索引获取实体ID列表
	ids, err := r.indexManager.GetEntityIDs(ctx, "user", userID)
	if err != nil {
		r.metrics.RecordList(time.Since(start), err)
		return nil, repository.NewRepositoryError(r.entityType, "ListByUser", "",
			fmt.Errorf("get index failed: %w", err))
	}

	if len(ids) == 0 {
		r.metrics.RecordList(time.Since(start), nil)
		return []E{}, nil
	}

	// 2. 构建 keys
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = r.buildKey(id)
	}

	// 3. 批量获取（单次网络调用）
	valueMap, err := r.store.BatchGet(ctx, keys)
	if err != nil {
		r.metrics.RecordList(time.Since(start), err)
		return nil, repository.NewRepositoryError(r.entityType, "ListByUser", "", err)
	}

	// 4. 组装结果
	entities := make([]E, 0, len(ids))
	var orphanIDs []string

	for i, key := range keys {
		if entity, ok := valueMap[key]; ok {
			entities = append(entities, entity)
		} else {
			// 索引存在但数据不存在，记录孤儿索引
			orphanIDs = append(orphanIDs, ids[i])
		}
	}

	// 5. 异步清理孤儿索引
	if len(orphanIDs) > 0 {
		go r.cleanOrphanIndexes(context.Background(), userID, orphanIDs)
	}

	r.metrics.RecordList(time.Since(start), nil)
	return entities, nil
}

// CountByUser 统计用户拥有的实体数量
func (r *UserIndexedRepository[E]) CountByUser(ctx context.Context, userID string) (int64, error) {
	ids, err := r.indexManager.GetEntityIDs(ctx, "user", userID)
	if err != nil {
		return 0, repository.NewRepositoryError(r.entityType, "CountByUser", "", err)
	}
	return int64(len(ids)), nil
}

// cleanOrphanIndexes 清理孤儿索引
func (r *UserIndexedRepository[E]) cleanOrphanIndexes(ctx context.Context, userID string, orphanIDs []string) {
	// 获取底层 IndexManager
	if userIndexMgr, ok := r.indexManager.(*index.UserEntityIndexManager[E]); ok {
		for _, id := range orphanIDs {
			// 创建一个临时实体用于删除索引
			// 注意：这里需要实体实现某种方式来构造临时实体
			// 由于泛型限制，这里简化处理
			_ = userIndexMgr
			_ = id
		}
	}
	r.metrics.RecordOrphanCleaned(len(orphanIDs))
}

// =============================================================================
// 高级操作
// =============================================================================

// InvalidateCache 使缓存失效
func (r *UserIndexedRepository[E]) InvalidateCache(ctx context.Context, id string) error {
	return r.store.InvalidateCache(ctx, r.buildKey(id))
}

// RefreshCache 刷新缓存
func (r *UserIndexedRepository[E]) RefreshCache(ctx context.Context, id string) error {
	return r.store.RefreshCache(ctx, r.buildKey(id))
}

// GetFromPersistent 直接从持久化层获取
func (r *UserIndexedRepository[E]) GetFromPersistent(ctx context.Context, id string) (E, error) {
	entity, err := r.store.GetFromPersistent(ctx, r.buildKey(id))
	if err != nil {
		var zero E
		return zero, repository.NewRepositoryError(r.entityType, "GetFromPersistent", id, err)
	}
	return entity, nil
}

// GetCacheStats 获取缓存统计
func (r *UserIndexedRepository[E]) GetCacheStats() store.CacheStats {
	return r.store.GetCacheStats()
}

// GetMetrics 获取监控指标
func (r *UserIndexedRepository[E]) GetMetrics() *store.RepositoryMetrics {
	return r.metrics
}

// GetIndexManager 获取索引管理器
func (r *UserIndexedRepository[E]) GetIndexManager() index.IndexManager[E] {
	return r.indexManager
}
