package store

import (
	"sync/atomic"
	"time"
)

// =============================================================================
// 存储层监控指标
// =============================================================================

// StoreMetrics 存储层监控指标
type StoreMetrics struct {
	// 基本操作计数
	GetCount    atomic.Int64 // Get 操作次数
	SetCount    atomic.Int64 // Set 操作次数
	DeleteCount atomic.Int64 // Delete 操作次数
	ExistsCount atomic.Int64 // Exists 操作次数

	// 批量操作计数
	BatchGetCount    atomic.Int64 // BatchGet 操作次数
	BatchSetCount    atomic.Int64 // BatchSet 操作次数
	BatchDeleteCount atomic.Int64 // BatchDelete 操作次数

	// 错误计数
	ErrorCount         atomic.Int64 // 错误总数
	NotFoundCount      atomic.Int64 // NotFound 错误数
	ConnectionErrCount atomic.Int64 // 连接错误数
	TimeoutErrCount    atomic.Int64 // 超时错误数

	// 延迟统计（纳秒）
	GetLatencySum    atomic.Int64 // Get 延迟累计
	SetLatencySum    atomic.Int64 // Set 延迟累计
	DeleteLatencySum atomic.Int64 // Delete 延迟累计

	// 缓存统计
	CacheHits          atomic.Int64 // 缓存命中
	CacheMisses        atomic.Int64 // 缓存未命中
	NegativeCacheHits  atomic.Int64 // 负缓存命中
	BloomFilterRejects atomic.Int64 // 布隆过滤器拒绝

	// 索引统计
	IndexAddCount    atomic.Int64 // 索引添加次数
	IndexRemoveCount atomic.Int64 // 索引移除次数
	IndexUpdateCount atomic.Int64 // 索引更新次数
	IndexQueryCount  atomic.Int64 // 索引查询次数
	OrphanIndexCount atomic.Int64 // 孤儿索引数量
}

// NewStoreMetrics 创建新的存储指标
func NewStoreMetrics() *StoreMetrics {
	return &StoreMetrics{}
}

// RecordGet 记录 Get 操作
func (m *StoreMetrics) RecordGet(duration time.Duration, err error) {
	m.GetCount.Add(1)
	m.GetLatencySum.Add(int64(duration))
	if err != nil {
		m.ErrorCount.Add(1)
		if IsNotFound(err) {
			m.NotFoundCount.Add(1)
		} else if IsConnectionFailed(err) {
			m.ConnectionErrCount.Add(1)
		} else if IsTimeout(err) {
			m.TimeoutErrCount.Add(1)
		}
	}
}

// RecordSet 记录 Set 操作
func (m *StoreMetrics) RecordSet(duration time.Duration, err error) {
	m.SetCount.Add(1)
	m.SetLatencySum.Add(int64(duration))
	if err != nil {
		m.ErrorCount.Add(1)
		if IsConnectionFailed(err) {
			m.ConnectionErrCount.Add(1)
		} else if IsTimeout(err) {
			m.TimeoutErrCount.Add(1)
		}
	}
}

// RecordDelete 记录 Delete 操作
func (m *StoreMetrics) RecordDelete(duration time.Duration, err error) {
	m.DeleteCount.Add(1)
	m.DeleteLatencySum.Add(int64(duration))
	if err != nil {
		m.ErrorCount.Add(1)
	}
}

// RecordCacheHit 记录缓存命中
func (m *StoreMetrics) RecordCacheHit() {
	m.CacheHits.Add(1)
}

// RecordCacheMiss 记录缓存未命中
func (m *StoreMetrics) RecordCacheMiss() {
	m.CacheMisses.Add(1)
}

// RecordNegativeCacheHit 记录负缓存命中
func (m *StoreMetrics) RecordNegativeCacheHit() {
	m.NegativeCacheHits.Add(1)
}

// RecordBloomFilterReject 记录布隆过滤器拒绝
func (m *StoreMetrics) RecordBloomFilterReject() {
	m.BloomFilterRejects.Add(1)
}

// GetCacheHitRate 获取缓存命中率
func (m *StoreMetrics) GetCacheHitRate() float64 {
	hits := m.CacheHits.Load()
	misses := m.CacheMisses.Load()
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// GetAvgGetLatency 获取平均 Get 延迟
func (m *StoreMetrics) GetAvgGetLatency() time.Duration {
	count := m.GetCount.Load()
	if count == 0 {
		return 0
	}
	return time.Duration(m.GetLatencySum.Load() / count)
}

// GetAvgSetLatency 获取平均 Set 延迟
func (m *StoreMetrics) GetAvgSetLatency() time.Duration {
	count := m.SetCount.Load()
	if count == 0 {
		return 0
	}
	return time.Duration(m.SetLatencySum.Load() / count)
}

// Snapshot 获取指标快照
func (m *StoreMetrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		GetCount:           m.GetCount.Load(),
		SetCount:           m.SetCount.Load(),
		DeleteCount:        m.DeleteCount.Load(),
		ErrorCount:         m.ErrorCount.Load(),
		CacheHits:          m.CacheHits.Load(),
		CacheMisses:        m.CacheMisses.Load(),
		NegativeCacheHits:  m.NegativeCacheHits.Load(),
		BloomFilterRejects: m.BloomFilterRejects.Load(),
		CacheHitRate:       m.GetCacheHitRate(),
		AvgGetLatency:      m.GetAvgGetLatency(),
		AvgSetLatency:      m.GetAvgSetLatency(),
	}
}

// Reset 重置指标
func (m *StoreMetrics) Reset() {
	m.GetCount.Store(0)
	m.SetCount.Store(0)
	m.DeleteCount.Store(0)
	m.ExistsCount.Store(0)
	m.BatchGetCount.Store(0)
	m.BatchSetCount.Store(0)
	m.BatchDeleteCount.Store(0)
	m.ErrorCount.Store(0)
	m.NotFoundCount.Store(0)
	m.ConnectionErrCount.Store(0)
	m.TimeoutErrCount.Store(0)
	m.GetLatencySum.Store(0)
	m.SetLatencySum.Store(0)
	m.DeleteLatencySum.Store(0)
	m.CacheHits.Store(0)
	m.CacheMisses.Store(0)
	m.NegativeCacheHits.Store(0)
	m.BloomFilterRejects.Store(0)
	m.IndexAddCount.Store(0)
	m.IndexRemoveCount.Store(0)
	m.IndexUpdateCount.Store(0)
	m.IndexQueryCount.Store(0)
	m.OrphanIndexCount.Store(0)
}

// MetricsSnapshot 指标快照
type MetricsSnapshot struct {
	GetCount           int64         `json:"get_count"`
	SetCount           int64         `json:"set_count"`
	DeleteCount        int64         `json:"delete_count"`
	ErrorCount         int64         `json:"error_count"`
	CacheHits          int64         `json:"cache_hits"`
	CacheMisses        int64         `json:"cache_misses"`
	NegativeCacheHits  int64         `json:"negative_cache_hits"`
	BloomFilterRejects int64         `json:"bloom_filter_rejects"`
	CacheHitRate       float64       `json:"cache_hit_rate"`
	AvgGetLatency      time.Duration `json:"avg_get_latency"`
	AvgSetLatency      time.Duration `json:"avg_set_latency"`
}

// =============================================================================
// Repository 层监控指标
// =============================================================================

// RepositoryMetrics Repository 层监控指标
type RepositoryMetrics struct {
	// CRUD 操作
	CreateCount  atomic.Int64
	GetCount     atomic.Int64
	UpdateCount  atomic.Int64
	DeleteCount  atomic.Int64
	ListCount    atomic.Int64

	// 延迟统计（纳秒）
	CreateLatencySum  atomic.Int64
	GetLatencySum     atomic.Int64
	UpdateLatencySum  atomic.Int64
	DeleteLatencySum  atomic.Int64
	ListLatencySum    atomic.Int64

	// 错误统计
	CreateErrorCount atomic.Int64
	GetErrorCount    atomic.Int64
	UpdateErrorCount atomic.Int64
	DeleteErrorCount atomic.Int64
	ListErrorCount   atomic.Int64

	// 索引相关
	IndexRebuildCount atomic.Int64
	OrphanCleaned     atomic.Int64
}

// NewRepositoryMetrics 创建新的 Repository 指标
func NewRepositoryMetrics() *RepositoryMetrics {
	return &RepositoryMetrics{}
}

// RecordCreate 记录 Create 操作
func (m *RepositoryMetrics) RecordCreate(duration time.Duration, err error) {
	m.CreateCount.Add(1)
	m.CreateLatencySum.Add(int64(duration))
	if err != nil {
		m.CreateErrorCount.Add(1)
	}
}

// RecordGet 记录 Get 操作
func (m *RepositoryMetrics) RecordGet(duration time.Duration, err error) {
	m.GetCount.Add(1)
	m.GetLatencySum.Add(int64(duration))
	if err != nil {
		m.GetErrorCount.Add(1)
	}
}

// RecordUpdate 记录 Update 操作
func (m *RepositoryMetrics) RecordUpdate(duration time.Duration, err error) {
	m.UpdateCount.Add(1)
	m.UpdateLatencySum.Add(int64(duration))
	if err != nil {
		m.UpdateErrorCount.Add(1)
	}
}

// RecordDelete 记录 Delete 操作
func (m *RepositoryMetrics) RecordDelete(duration time.Duration, err error) {
	m.DeleteCount.Add(1)
	m.DeleteLatencySum.Add(int64(duration))
	if err != nil {
		m.DeleteErrorCount.Add(1)
	}
}

// RecordList 记录 List 操作
func (m *RepositoryMetrics) RecordList(duration time.Duration, err error) {
	m.ListCount.Add(1)
	m.ListLatencySum.Add(int64(duration))
	if err != nil {
		m.ListErrorCount.Add(1)
	}
}

// RecordOrphanCleaned 记录孤儿索引清理
func (m *RepositoryMetrics) RecordOrphanCleaned(count int) {
	m.OrphanCleaned.Add(int64(count))
}

// GetAvgListLatency 获取平均 List 延迟
func (m *RepositoryMetrics) GetAvgListLatency() time.Duration {
	count := m.ListCount.Load()
	if count == 0 {
		return 0
	}
	return time.Duration(m.ListLatencySum.Load() / count)
}
