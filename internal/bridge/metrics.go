package bridge

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"sync"
	"time"
	
	"tunnox-core/internal/core/dispose"
)

// MetricsCollector 连接池指标收集器
type MetricsCollector struct {
	*dispose.ManagerBase
	
	mu                sync.RWMutex
	nodeStats         map[string]*NodeStats
	globalStats       *GlobalStats
	startTime         time.Time
}

// NodeStats 节点统计信息
type NodeStats struct {
	NodeID            string
	TotalConnections  int32
	ActiveStreams     int32
	SessionsCreated   int64
	SessionsClosed    int64
	ErrorCount        int64
	LastError         string
	LastErrorTime     time.Time
	LastUpdateTime    time.Time
}

// GlobalStats 全局统计信息
type GlobalStats struct {
	TotalNodes        int32
	TotalConnections  int32
	TotalActiveStreams int32
	TotalSessionsCreated int64
	TotalSessionsClosed  int64
	TotalErrors       int64
	Uptime            time.Duration
}

// PoolMetrics 连接池指标
type PoolMetrics struct {
	NodeStats   map[string]*NodeStats
	GlobalStats *GlobalStats
}

// NewMetricsCollector 创建指标收集器
func NewMetricsCollector(parentCtx context.Context) *MetricsCollector {
	collector := &MetricsCollector{
		ManagerBase: dispose.NewManager("MetricsCollector", parentCtx),
		nodeStats: make(map[string]*NodeStats),
		globalStats: &GlobalStats{
			TotalNodes:        0,
			TotalConnections:  0,
			TotalActiveStreams: 0,
			TotalSessionsCreated: 0,
			TotalSessionsClosed:  0,
			TotalErrors:       0,
		},
		startTime: time.Now(),
	}
	
	// 注册清理处理器
	collector.AddCleanHandler(func() error {
		corelog.Infof("MetricsCollector: cleaning up resources")
		
		collector.mu.Lock()
		defer collector.mu.Unlock()
		
		// 清理所有节点统计
		collector.nodeStats = make(map[string]*NodeStats)
		collector.globalStats = &GlobalStats{
			TotalNodes:        0,
			TotalConnections:  0,
			TotalActiveStreams: 0,
			TotalSessionsCreated: 0,
			TotalSessionsClosed:  0,
			TotalErrors:       0,
		}
		
		return nil
	})
	
	return collector
}

// Close 关闭收集器
func (mc *MetricsCollector) Close() error {
	mc.ManagerBase.Close()
	return nil
}

// RecordSessionCreated 记录会话创建
func (m *MetricsCollector) RecordSessionCreated(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := m.getOrCreateNodeStats(nodeID)
	stats.SessionsCreated++
	stats.LastUpdateTime = time.Now()

	m.globalStats.TotalSessionsCreated++
}

// RecordSessionClosed 记录会话关闭
func (m *MetricsCollector) RecordSessionClosed(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := m.getOrCreateNodeStats(nodeID)
	stats.SessionsClosed++
	stats.LastUpdateTime = time.Now()

	m.globalStats.TotalSessionsClosed++
}

// RecordError 记录错误
func (m *MetricsCollector) RecordError(nodeID, errorMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := m.getOrCreateNodeStats(nodeID)
	stats.ErrorCount++
	stats.LastError = errorMsg
	stats.LastErrorTime = time.Now()
	stats.LastUpdateTime = time.Now()

	m.globalStats.TotalErrors++
}

// UpdatePoolStats 更新连接池统计
func (m *MetricsCollector) UpdatePoolStats(nodeID string, totalConns, activeStreams int32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := m.getOrCreateNodeStats(nodeID)
	stats.TotalConnections = totalConns
	stats.ActiveStreams = activeStreams
	stats.LastUpdateTime = time.Now()

	// 更新全局统计
	m.recalculateGlobalStats()
}

// getOrCreateNodeStats 获取或创建节点统计（内部方法，调用者需持有锁）
func (m *MetricsCollector) getOrCreateNodeStats(nodeID string) *NodeStats {
	if stats, exists := m.nodeStats[nodeID]; exists {
		return stats
	}

	stats := &NodeStats{
		NodeID:            nodeID,
		TotalConnections:  0,
		ActiveStreams:     0,
		SessionsCreated:   0,
		SessionsClosed:    0,
		ErrorCount:        0,
		LastUpdateTime:    time.Now(),
	}
	m.nodeStats[nodeID] = stats
	return stats
}

// recalculateGlobalStats 重新计算全局统计（内部方法，调用者需持有锁）
func (m *MetricsCollector) recalculateGlobalStats() {
	var totalConns, totalStreams int32
	var totalCreated, totalClosed, totalErrors int64

	for _, stats := range m.nodeStats {
		totalConns += stats.TotalConnections
		totalStreams += stats.ActiveStreams
		totalCreated += stats.SessionsCreated
		totalClosed += stats.SessionsClosed
		totalErrors += stats.ErrorCount
	}

	m.globalStats.TotalNodes = int32(len(m.nodeStats))
	m.globalStats.TotalConnections = totalConns
	m.globalStats.TotalActiveStreams = totalStreams
	m.globalStats.TotalSessionsCreated = totalCreated
	m.globalStats.TotalSessionsClosed = totalClosed
	m.globalStats.TotalErrors = totalErrors
	m.globalStats.Uptime = time.Since(m.startTime)
}

// GetMetrics 获取指标快照
func (m *MetricsCollector) GetMetrics() *PoolMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 复制节点统计
	nodeStats := make(map[string]*NodeStats)
	for nodeID, stats := range m.nodeStats {
		statsCopy := *stats
		nodeStats[nodeID] = &statsCopy
	}

	// 复制全局统计
	globalStatsCopy := *m.globalStats
	globalStatsCopy.Uptime = time.Since(m.startTime)

	return &PoolMetrics{
		NodeStats:   nodeStats,
		GlobalStats: &globalStatsCopy,
	}
}

// Reset 重置指标
func (m *MetricsCollector) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nodeStats = make(map[string]*NodeStats)
	m.globalStats = &GlobalStats{}
	m.startTime = time.Now()
}

// RemoveNodeStats 移除节点统计
func (m *MetricsCollector) RemoveNodeStats(nodeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.nodeStats, nodeID)
	m.recalculateGlobalStats()
}

