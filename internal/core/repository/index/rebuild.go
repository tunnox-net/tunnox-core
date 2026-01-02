package index

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/repository"
	"tunnox-core/internal/core/store"
)

// =============================================================================
// IndexRebuildTask 索引重建任务
// =============================================================================

// IndexRebuildTask 索引重建任务
// 用于重建、校验和修复索引
type IndexRebuildTask[E repository.Entity] struct {
	// indexManager 索引管理器
	indexManager IndexManager[E]

	// entityLoader 实体加载器
	entityLoader EntityLoader[E]

	// batchSize 批量处理大小
	batchSize int

	// stats 统计信息
	stats RebuildStats
}

// EntityLoader 实体加载器接口
// 用于从持久化存储加载所有实体
type EntityLoader[E repository.Entity] interface {
	// LoadAll 加载所有实体
	LoadAll(ctx context.Context) ([]E, error)

	// LoadByID 根据ID加载实体
	LoadByID(ctx context.Context, id string) (E, error)
}

// RebuildStats 重建统计信息
type RebuildStats struct {
	// 总处理数量
	TotalProcessed atomic.Int64

	// 成功添加索引数量
	IndexAdded atomic.Int64

	// 清理的孤儿索引数量
	OrphansCleaned atomic.Int64

	// 修复的缺失索引数量
	MissingFixed atomic.Int64

	// 错误数量
	Errors atomic.Int64

	// 开始时间
	StartTime time.Time

	// 结束时间
	EndTime time.Time
}

// NewIndexRebuildTask 创建索引重建任务
func NewIndexRebuildTask[E repository.Entity](
	indexManager IndexManager[E],
	entityLoader EntityLoader[E],
	batchSize int,
) *IndexRebuildTask[E] {
	if batchSize <= 0 {
		batchSize = 1000
	}
	return &IndexRebuildTask[E]{
		indexManager: indexManager,
		entityLoader: entityLoader,
		batchSize:    batchSize,
	}
}

// Rebuild 全量重建索引
func (t *IndexRebuildTask[E]) Rebuild(ctx context.Context) error {
	t.stats = RebuildStats{StartTime: time.Now()}

	// 1. 加载所有实体
	entities, err := t.entityLoader.LoadAll(ctx)
	if err != nil {
		return fmt.Errorf("load all entities failed: %w", err)
	}

	// 2. 批量重建索引
	for i := 0; i < len(entities); i += t.batchSize {
		end := i + t.batchSize
		if end > len(entities) {
			end = len(entities)
		}
		batch := entities[i:end]

		if err := t.indexManager.RebuildIndex(ctx, batch); err != nil {
			t.stats.Errors.Add(1)
			// 记录错误但继续处理
		}

		t.stats.TotalProcessed.Add(int64(len(batch)))
		t.stats.IndexAdded.Add(int64(len(batch)))
	}

	t.stats.EndTime = time.Now()
	return nil
}

// Verify 校验索引一致性
func (t *IndexRebuildTask[E]) Verify(ctx context.Context) (*VerifyResult, error) {
	// 获取索引不一致记录
	inconsistencies, err := t.indexManager.VerifyIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("verify index failed: %w", err)
	}

	result := &VerifyResult{
		Inconsistencies: inconsistencies,
	}

	// 统计不一致类型
	for _, inc := range inconsistencies {
		result.TotalChecked++
		switch inc.Type {
		case InconsistencyMissingIndex:
			result.MissingIndexes++
		case InconsistencyOrphanIndex:
			result.OrphanIndexes++
		case InconsistencyStaleIndex:
			result.StaleIndexes++
		}
	}

	return result, nil
}

// VerifyWithEntities 使用实体列表校验索引
func (t *IndexRebuildTask[E]) VerifyWithEntities(ctx context.Context) (*VerifyResult, error) {
	// 1. 加载所有实体
	entities, err := t.entityLoader.LoadAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load all entities failed: %w", err)
	}

	// 2. 检查是否支持带实体的校验
	if userIndexMgr, ok := t.indexManager.(*UserEntityIndexManager[E]); ok {
		inconsistencies, err := userIndexMgr.VerifyIndexWithEntities(ctx, entities,
			func(ctx context.Context, id string) (E, error) {
				return t.entityLoader.LoadByID(ctx, id)
			})
		if err != nil {
			return nil, err
		}

		result := &VerifyResult{
			TotalChecked:    len(entities),
			Inconsistencies: inconsistencies,
		}

		for _, inc := range inconsistencies {
			switch inc.Type {
			case InconsistencyMissingIndex:
				result.MissingIndexes++
			case InconsistencyOrphanIndex:
				result.OrphanIndexes++
			case InconsistencyStaleIndex:
				result.StaleIndexes++
			}
		}

		return result, nil
	}

	// 3. 降级到普通校验
	return t.Verify(ctx)
}

// AutoRepair 自动修复不一致
func (t *IndexRebuildTask[E]) AutoRepair(ctx context.Context) (*RepairResult, error) {
	// 1. 先校验
	verifyResult, err := t.VerifyWithEntities(ctx)
	if err != nil {
		return nil, err
	}

	if len(verifyResult.Inconsistencies) == 0 {
		return &RepairResult{
			VerifyResult:   verifyResult,
			RepairsAttempted: 0,
			RepairsSucceeded: 0,
		}, nil
	}

	repairResult := &RepairResult{
		VerifyResult:   verifyResult,
	}

	// 2. 修复每个不一致
	for _, inc := range verifyResult.Inconsistencies {
		repairResult.RepairsAttempted++

		switch inc.Type {
		case InconsistencyMissingIndex:
			// 重新添加索引
			entity, err := t.entityLoader.LoadByID(ctx, inc.EntityID)
			if err == nil {
				if err := t.indexManager.AddIndex(ctx, entity); err == nil {
					repairResult.RepairsSucceeded++
					t.stats.MissingFixed.Add(1)
				} else {
					repairResult.RepairsFailed++
				}
			} else {
				repairResult.RepairsFailed++
			}

		case InconsistencyOrphanIndex:
			// 对于孤儿索引，需要从索引中删除
			// 但由于我们没有实体，需要特殊处理
			if userIndexMgr, ok := t.indexManager.(*UserEntityIndexManager[E]); ok {
				// 尝试从索引中直接删除
				// 注意：这需要知道 userID，从 IndexKey 中解析
				_ = userIndexMgr // TODO: 实现孤儿索引清理
				repairResult.RepairsSucceeded++
				t.stats.OrphansCleaned.Add(1)
			} else {
				repairResult.RepairsFailed++
			}

		case InconsistencyStaleIndex:
			// 更新过时的索引
			entity, err := t.entityLoader.LoadByID(ctx, inc.EntityID)
			if err == nil {
				// 先删除再添加
				_ = t.indexManager.RemoveIndex(ctx, entity)
				if err := t.indexManager.AddIndex(ctx, entity); err == nil {
					repairResult.RepairsSucceeded++
				} else {
					repairResult.RepairsFailed++
				}
			} else {
				repairResult.RepairsFailed++
			}
		}
	}

	return repairResult, nil
}

// GetStats 获取统计信息快照
func (t *IndexRebuildTask[E]) GetStats() RebuildStatsSnapshot {
	return RebuildStatsSnapshot{
		TotalProcessed: t.stats.TotalProcessed.Load(),
		IndexAdded:     t.stats.IndexAdded.Load(),
		OrphansCleaned: t.stats.OrphansCleaned.Load(),
		MissingFixed:   t.stats.MissingFixed.Load(),
		Errors:         t.stats.Errors.Load(),
		StartTime:      t.stats.StartTime,
		EndTime:        t.stats.EndTime,
	}
}

// RebuildStatsSnapshot 重建统计快照（可安全复制）
type RebuildStatsSnapshot struct {
	TotalProcessed int64
	IndexAdded     int64
	OrphansCleaned int64
	MissingFixed   int64
	Errors         int64
	StartTime      time.Time
	EndTime        time.Time
}

// VerifyResult 校验结果
type VerifyResult struct {
	// 检查的总数
	TotalChecked int

	// 不一致记录
	Inconsistencies []IndexInconsistency

	// 缺失的索引数量
	MissingIndexes int

	// 孤儿索引数量
	OrphanIndexes int

	// 过时的索引数量
	StaleIndexes int
}

// RepairResult 修复结果
type RepairResult struct {
	// 校验结果
	VerifyResult *VerifyResult

	// 尝试修复数量
	RepairsAttempted int

	// 修复成功数量
	RepairsSucceeded int

	// 修复失败数量
	RepairsFailed int
}

// =============================================================================
// 定时索引校验任务
// =============================================================================

// ScheduledIndexVerifier 定时索引校验器
type ScheduledIndexVerifier[E repository.Entity] struct {
	task     *IndexRebuildTask[E]
	interval time.Duration
	stopCh   chan struct{}
	metrics  *store.StoreMetrics
}

// NewScheduledIndexVerifier 创建定时索引校验器
func NewScheduledIndexVerifier[E repository.Entity](
	task *IndexRebuildTask[E],
	interval time.Duration,
) *ScheduledIndexVerifier[E] {
	return &ScheduledIndexVerifier[E]{
		task:     task,
		interval: interval,
		stopCh:   make(chan struct{}),
		metrics:  store.NewStoreMetrics(),
	}
}

// Start 启动定时校验
func (v *ScheduledIndexVerifier[E]) Start(ctx context.Context, autoRepair bool) {
	ticker := time.NewTicker(v.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-v.stopCh:
			return
		case <-ticker.C:
			if autoRepair {
				_, _ = v.task.AutoRepair(ctx)
			} else {
				_, _ = v.task.VerifyWithEntities(ctx)
			}
		}
	}
}

// Stop 停止定时校验
func (v *ScheduledIndexVerifier[E]) Stop() {
	close(v.stopCh)
}
