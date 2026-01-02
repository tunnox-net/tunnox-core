package index

import (
	"context"
	"fmt"

	"tunnox-core/internal/core/repository"
	"tunnox-core/internal/core/store"
)

// =============================================================================
// UserEntityIndexManager 用户实体索引管理器
// =============================================================================

// UserEntityIndexManager 用户实体索引管理器
// 维护 用户ID -> 实体ID列表 的索引关系
type UserEntityIndexManager[E repository.Entity] struct {
	// setStore 集合存储，用于存储索引
	setStore store.SetStore[string, string]

	// keyPrefix 索引键前缀
	keyPrefix string

	// getUserID 获取实体所属用户ID的函数
	getUserID func(E) string

	// metrics 监控指标
	metrics *store.StoreMetrics
}

// NewUserEntityIndexManager 创建用户实体索引管理器
func NewUserEntityIndexManager[E repository.Entity](
	setStore store.SetStore[string, string],
	keyPrefix string,
	getUserID func(E) string,
) *UserEntityIndexManager[E] {
	return &UserEntityIndexManager[E]{
		setStore:  setStore,
		keyPrefix: keyPrefix,
		getUserID: getUserID,
		metrics:   store.NewStoreMetrics(),
	}
}

// buildIndexKey 构建索引键
func (m *UserEntityIndexManager[E]) buildIndexKey(userID string) string {
	return m.keyPrefix + userID
}

// AddIndex 为实体添加索引
func (m *UserEntityIndexManager[E]) AddIndex(ctx context.Context, entity E) error {
	userID := m.getUserID(entity)
	if userID == "" {
		return nil // 无用户ID，不建立索引
	}

	indexKey := m.buildIndexKey(userID)
	err := m.setStore.Add(ctx, indexKey, entity.GetID())
	if err != nil {
		return fmt.Errorf("add index failed: key=%s, entityID=%s: %w", indexKey, entity.GetID(), err)
	}

	m.metrics.IndexAddCount.Add(1)
	return nil
}

// RemoveIndex 移除实体的索引
func (m *UserEntityIndexManager[E]) RemoveIndex(ctx context.Context, entity E) error {
	userID := m.getUserID(entity)
	if userID == "" {
		return nil
	}

	indexKey := m.buildIndexKey(userID)
	err := m.setStore.Remove(ctx, indexKey, entity.GetID())
	if err != nil {
		return fmt.Errorf("remove index failed: key=%s, entityID=%s: %w", indexKey, entity.GetID(), err)
	}

	m.metrics.IndexRemoveCount.Add(1)
	return nil
}

// UpdateIndex 更新实体的索引
func (m *UserEntityIndexManager[E]) UpdateIndex(ctx context.Context, oldEntity, newEntity E) error {
	oldUserID := m.getUserID(oldEntity)
	newUserID := m.getUserID(newEntity)

	// 用户ID未变化，无需更新索引
	if oldUserID == newUserID {
		return nil
	}

	// 检查是否支持 Pipeline（原子操作）
	if pipelineStore, ok := m.setStore.(store.PipelineSetStore[string, string]); ok {
		pipe := pipelineStore.Pipeline()
		if oldUserID != "" {
			pipe.SRem(ctx, m.buildIndexKey(oldUserID), oldEntity.GetID())
		}
		if newUserID != "" {
			pipe.SAdd(ctx, m.buildIndexKey(newUserID), newEntity.GetID())
		}
		if err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("update index (pipeline) failed: %w", err)
		}
		m.metrics.IndexUpdateCount.Add(1)
		return nil
	}

	// 降级到非原子操作
	if oldUserID != "" {
		if err := m.RemoveIndex(ctx, oldEntity); err != nil {
			return err
		}
	}
	if newUserID != "" {
		if err := m.AddIndex(ctx, newEntity); err != nil {
			return err
		}
	}

	m.metrics.IndexUpdateCount.Add(1)
	return nil
}

// GetEntityIDs 根据索引获取实体ID列表
func (m *UserEntityIndexManager[E]) GetEntityIDs(ctx context.Context, indexType string, indexValue string) ([]string, error) {
	if indexType != "user" {
		return nil, fmt.Errorf("unsupported index type: %s", indexType)
	}

	indexKey := m.buildIndexKey(indexValue)
	ids, err := m.setStore.Members(ctx, indexKey)
	if err != nil {
		return nil, fmt.Errorf("get index members failed: key=%s: %w", indexKey, err)
	}

	m.metrics.IndexQueryCount.Add(1)
	return ids, nil
}

// RebuildIndex 重建所有索引
func (m *UserEntityIndexManager[E]) RebuildIndex(ctx context.Context, entities []E) error {
	// 按用户分组
	userEntities := make(map[string][]string)
	for _, entity := range entities {
		userID := m.getUserID(entity)
		if userID != "" {
			userEntities[userID] = append(userEntities[userID], entity.GetID())
		}
	}

	// 重建每个用户的索引
	for userID, entityIDs := range userEntities {
		indexKey := m.buildIndexKey(userID)

		// 先清空现有索引（获取所有成员并删除）
		existingIDs, err := m.setStore.Members(ctx, indexKey)
		if err == nil {
			for _, id := range existingIDs {
				_ = m.setStore.Remove(ctx, indexKey, id)
			}
		}

		// 添加新索引
		for _, entityID := range entityIDs {
			if err := m.setStore.Add(ctx, indexKey, entityID); err != nil {
				return fmt.Errorf("rebuild index failed: userID=%s, entityID=%s: %w", userID, entityID, err)
			}
		}
	}

	return nil
}

// VerifyIndex 校验索引一致性
// 注意：此方法需要外部提供实体列表来进行完整校验
func (m *UserEntityIndexManager[E]) VerifyIndex(ctx context.Context) ([]IndexInconsistency, error) {
	// 此方法需要配合 IndexRebuildTask 使用
	// 单独调用时返回空列表
	return []IndexInconsistency{}, nil
}

// VerifyIndexWithEntities 使用实体列表校验索引一致性
func (m *UserEntityIndexManager[E]) VerifyIndexWithEntities(
	ctx context.Context,
	entities []E,
	getEntity func(ctx context.Context, id string) (E, error),
) ([]IndexInconsistency, error) {
	var inconsistencies []IndexInconsistency

	// 1. 构建预期索引 map
	expectedIndex := make(map[string]map[string]bool) // userID -> entityIDs
	for _, entity := range entities {
		userID := m.getUserID(entity)
		if userID == "" {
			continue
		}
		if expectedIndex[userID] == nil {
			expectedIndex[userID] = make(map[string]bool)
		}
		expectedIndex[userID][entity.GetID()] = true
	}

	// 2. 检查每个用户的索引
	for userID, expectedIDs := range expectedIndex {
		indexKey := m.buildIndexKey(userID)
		actualIDs, err := m.setStore.Members(ctx, indexKey)
		if err != nil {
			continue
		}

		actualIDSet := make(map[string]bool)
		for _, id := range actualIDs {
			actualIDSet[id] = true
		}

		// 检查缺失的索引
		for entityID := range expectedIDs {
			if !actualIDSet[entityID] {
				inconsistencies = append(inconsistencies, IndexInconsistency{
					IndexKey:    indexKey,
					EntityID:    entityID,
					Type:        InconsistencyMissingIndex,
					Description: fmt.Sprintf("entity %s missing in index %s", entityID, indexKey),
				})
			}
		}

		// 检查孤儿索引
		for _, entityID := range actualIDs {
			if !expectedIDs[entityID] {
				// 检查实体是否存在
				_, err := getEntity(ctx, entityID)
				if err != nil {
					inconsistencies = append(inconsistencies, IndexInconsistency{
						IndexKey:    indexKey,
						EntityID:    entityID,
						Type:        InconsistencyOrphanIndex,
						Description: fmt.Sprintf("orphan index: entity %s not found", entityID),
					})
					m.metrics.OrphanIndexCount.Add(1)
				}
			}
		}
	}

	return inconsistencies, nil
}

// GetMetrics 获取监控指标
func (m *UserEntityIndexManager[E]) GetMetrics() *store.StoreMetrics {
	return m.metrics
}
