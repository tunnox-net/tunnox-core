package utils

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// ResourceStats 资源统计信息
type ResourceStats struct {
	Timestamp      time.Time
	GoroutineCount int64
	MemoryStats    MemoryStats
	ResourceCount  int64
	DisposeCount   int64
}

// MemoryStats 内存统计信息
type MemoryStats struct {
	Alloc      uint64
	TotalAlloc uint64
	Sys        uint64
	NumGC      uint32
}

// MonitorConfig 监控配置
type MonitorConfig struct {
	// 监控间隔
	MonitorInterval time.Duration
	// 是否启用goroutine监控
	EnableGoroutineMonitor bool
	// 是否启用内存监控
	EnableMemoryMonitor bool
	// 是否启用资源监控
	EnableResourceMonitor bool
	// goroutine数量警告阈值
	GoroutineWarningThreshold int64
	// 内存使用警告阈值（MB）
	MemoryWarningThresholdMB int64
	// 监控回调函数
	OnWarning func(stats *ResourceStats, warning string)
}

// DefaultMonitorConfig 默认监控配置
func DefaultMonitorConfig() *MonitorConfig {
	return &MonitorConfig{
		MonitorInterval:           30 * time.Second,
		EnableGoroutineMonitor:    true,
		EnableMemoryMonitor:       true,
		EnableResourceMonitor:     true,
		GoroutineWarningThreshold: 1000,
		MemoryWarningThresholdMB:  512, // 512MB
		OnWarning: func(stats *ResourceStats, warning string) {
			Warnf("Resource monitor warning: %s", warning)
		},
	}
}

// ResourceMonitor 资源监控器
type ResourceMonitor struct {
	config    *MonitorConfig
	stats     []*ResourceStats
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	isRunning int32
	dispose   Dispose
}

// NewResourceMonitor 创建资源监控器
func NewResourceMonitor(config *MonitorConfig) *ResourceMonitor {
	if config == nil {
		config = DefaultMonitorConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())
	monitor := &ResourceMonitor{
		config: config,
		stats:  make([]*ResourceStats, 0),
		ctx:    ctx,
		cancel: cancel,
	}

	monitor.dispose.SetCtx(ctx, monitor.onClose)
	return monitor
}

// Start 启动监控
func (rm *ResourceMonitor) Start() error {
	if !atomic.CompareAndSwapInt32(&rm.isRunning, 0, 1) {
		return fmt.Errorf("monitor is already running")
	}

	Infof("Starting resource monitor with interval: %v", rm.config.MonitorInterval)

	go rm.monitorLoop()
	return nil
}

// Stop 停止监控
func (rm *ResourceMonitor) Stop() error {
	if !atomic.CompareAndSwapInt32(&rm.isRunning, 1, 0) {
		return fmt.Errorf("monitor is not running")
	}

	rm.cancel()
	Infof("Resource monitor stopped")
	return nil
}

// IsRunning 检查监控是否正在运行
func (rm *ResourceMonitor) IsRunning() bool {
	return atomic.LoadInt32(&rm.isRunning) == 1
}

// monitorLoop 监控循环
func (rm *ResourceMonitor) monitorLoop() {
	ticker := time.NewTicker(rm.config.MonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-ticker.C:
			rm.collectStats()
		}
	}
}

// collectStats 收集统计信息
func (rm *ResourceMonitor) collectStats() {
	stats := &ResourceStats{
		Timestamp: time.Now(),
	}

	// 收集goroutine统计
	if rm.config.EnableGoroutineMonitor {
		stats.GoroutineCount = int64(runtime.NumGoroutine())
	}

	// 收集内存统计
	if rm.config.EnableMemoryMonitor {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		stats.MemoryStats = MemoryStats{
			Alloc:      memStats.Alloc,
			TotalAlloc: memStats.TotalAlloc,
			Sys:        memStats.Sys,
			NumGC:      memStats.NumGC,
		}
	}

	// 收集资源统计
	if rm.config.EnableResourceMonitor {
		stats.ResourceCount = int64(globalResourceManager.GetResourceCount())
		stats.DisposeCount = atomic.LoadInt64(&globalDisposeCount)
	}

	// 保存统计信息
	rm.mu.Lock()
	rm.stats = append(rm.stats, stats)
	// 保留最近100条记录
	if len(rm.stats) > 100 {
		rm.stats = rm.stats[len(rm.stats)-100:]
	}
	rm.mu.Unlock()

	// 检查警告条件
	rm.checkWarnings(stats)
}

// checkWarnings 检查警告条件
func (rm *ResourceMonitor) checkWarnings(stats *ResourceStats) {
	if rm.config.OnWarning == nil {
		return
	}

	// 检查goroutine数量
	if rm.config.EnableGoroutineMonitor &&
		stats.GoroutineCount > rm.config.GoroutineWarningThreshold {
		warning := fmt.Sprintf("High goroutine count: %d (threshold: %d)",
			stats.GoroutineCount, rm.config.GoroutineWarningThreshold)
		rm.config.OnWarning(stats, warning)
	}

	// 检查内存使用
	if rm.config.EnableMemoryMonitor {
		memoryMB := int64(stats.MemoryStats.Alloc / 1024 / 1024)
		if memoryMB > rm.config.MemoryWarningThresholdMB {
			warning := fmt.Sprintf("High memory usage: %d MB (threshold: %d MB)",
				memoryMB, rm.config.MemoryWarningThresholdMB)
			rm.config.OnWarning(stats, warning)
		}
	}

	// 检查资源数量
	if rm.config.EnableResourceMonitor && stats.ResourceCount > 100 {
		warning := fmt.Sprintf("High resource count: %d", stats.ResourceCount)
		rm.config.OnWarning(stats, warning)
	}
}

// GetStats 获取统计信息
func (rm *ResourceMonitor) GetStats() []*ResourceStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	stats := make([]*ResourceStats, len(rm.stats))
	copy(stats, rm.stats)
	return stats
}

// GetLatestStats 获取最新的统计信息
func (rm *ResourceMonitor) GetLatestStats() *ResourceStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if len(rm.stats) == 0 {
		return nil
	}
	return rm.stats[len(rm.stats)-1]
}

// GetStatsSummary 获取统计摘要
func (rm *ResourceMonitor) GetStatsSummary() *StatsSummary {
	stats := rm.GetStats()
	if len(stats) == 0 {
		return &StatsSummary{}
	}

	summary := &StatsSummary{
		SampleCount: len(stats),
		StartTime:   stats[0].Timestamp,
		EndTime:     stats[len(stats)-1].Timestamp,
	}

	// 计算goroutine统计
	var totalGoroutines int64
	var maxGoroutines int64
	var minGoroutines int64 = stats[0].GoroutineCount

	for _, stat := range stats {
		totalGoroutines += stat.GoroutineCount
		if stat.GoroutineCount > maxGoroutines {
			maxGoroutines = stat.GoroutineCount
		}
		if stat.GoroutineCount < minGoroutines {
			minGoroutines = stat.GoroutineCount
		}
	}

	summary.GoroutineStats = GoroutineStats{
		Average: float64(totalGoroutines) / float64(len(stats)),
		Max:     maxGoroutines,
		Min:     minGoroutines,
		Current: stats[len(stats)-1].GoroutineCount,
	}

	// 计算内存统计
	var totalAlloc uint64
	var maxAlloc uint64
	var minAlloc uint64 = stats[0].MemoryStats.Alloc

	for _, stat := range stats {
		totalAlloc += stat.MemoryStats.Alloc
		if stat.MemoryStats.Alloc > maxAlloc {
			maxAlloc = stat.MemoryStats.Alloc
		}
		if stat.MemoryStats.Alloc < minAlloc {
			minAlloc = stat.MemoryStats.Alloc
		}
	}

	summary.MemoryStats = MemoryStatsSummary{
		AverageAlloc: float64(totalAlloc) / float64(len(stats)),
		MaxAlloc:     maxAlloc,
		MinAlloc:     minAlloc,
		CurrentAlloc: stats[len(stats)-1].MemoryStats.Alloc,
	}

	return summary
}

// onClose 关闭回调
func (rm *ResourceMonitor) onClose() error {
	rm.Stop()
	return nil
}

// Dispose 实现Disposable接口
func (rm *ResourceMonitor) Dispose() error {
	return rm.dispose.CloseWithError()
}

// StatsSummary 统计摘要
type StatsSummary struct {
	SampleCount    int
	StartTime      time.Time
	EndTime        time.Time
	GoroutineStats GoroutineStats
	MemoryStats    MemoryStatsSummary
}

// GoroutineStats goroutine统计
type GoroutineStats struct {
	Average float64
	Max     int64
	Min     int64
	Current int64
}

// MemoryStatsSummary 内存统计摘要
type MemoryStatsSummary struct {
	AverageAlloc float64
	MaxAlloc     uint64
	MinAlloc     uint64
	CurrentAlloc uint64
}

// 全局变量
var (
	globalDisposeCount int64
	globalMonitor      *ResourceMonitor
	monitorOnce        sync.Once
)

// StartGlobalMonitor 启动全局监控
func StartGlobalMonitor(config *MonitorConfig) error {
	var err error
	monitorOnce.Do(func() {
		globalMonitor = NewResourceMonitor(config)
		err = globalMonitor.Start()
	})
	return err
}

// StopGlobalMonitor 停止全局监控
func StopGlobalMonitor() error {
	if globalMonitor != nil {
		return globalMonitor.Stop()
	}
	return nil
}

// GetGlobalMonitor 获取全局监控器
func GetGlobalMonitor() *ResourceMonitor {
	return globalMonitor
}

// GetGlobalStats 获取全局统计信息
func GetGlobalStats() []*ResourceStats {
	if globalMonitor != nil {
		return globalMonitor.GetStats()
	}
	return nil
}

// GetGlobalStatsSummary 获取全局统计摘要
func GetGlobalStatsSummary() *StatsSummary {
	if globalMonitor != nil {
		return globalMonitor.GetStatsSummary()
	}
	return &StatsSummary{}
}

// IncrementDisposeCount 增加释放计数
func IncrementDisposeCount() {
	atomic.AddInt64(&globalDisposeCount, 1)
}
