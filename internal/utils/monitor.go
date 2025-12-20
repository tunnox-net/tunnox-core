package utils

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
)

// MonitorConfig 监控配置
type MonitorConfig struct {
	MonitorInterval           time.Duration
	EnableGoroutineMonitor    bool
	EnableMemoryMonitor       bool
	EnableResourceMonitor     bool
	GoroutineWarningThreshold int64
	MemoryWarningThresholdMB  int64
	OnWarning                 func(*ResourceStats, string)
}

// DefaultMonitorConfig 默认监控配置
func DefaultMonitorConfig() *MonitorConfig {
	return &MonitorConfig{
		MonitorInterval:           5 * time.Second,
		EnableGoroutineMonitor:    true,
		EnableMemoryMonitor:       true,
		EnableResourceMonitor:     true,
		GoroutineWarningThreshold: 1000,
		MemoryWarningThresholdMB:  100,
	}
}

// ResourceStats 资源统计信息
type ResourceStats struct {
	Timestamp      time.Time
	GoroutineCount int64
	MemoryStats    runtime.MemStats
	ResourceCount  int64
	DisposeCount   int64
}

// ResourceMonitor 资源监控器
type ResourceMonitor struct {
	config    *MonitorConfig
	stats     []*ResourceStats
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	isRunning int32
	dispose   dispose.Dispose
}

// NewResourceMonitor 创建资源监控器
func NewResourceMonitor(config *MonitorConfig, parentCtx context.Context) *ResourceMonitor {
	ctx, cancel := context.WithCancel(parentCtx)
	monitor := &ResourceMonitor{
		config: config,
		stats:  make([]*ResourceStats, 0),
		ctx:    ctx,
		cancel: cancel,
	}

	monitor.dispose.SetCtx(ctx, monitor.onClose)
	return monitor
}

// onClose 资源释放回调
func (rm *ResourceMonitor) onClose() error {
	rm.Stop()
	return nil
}

// Start 启动监控
func (rm *ResourceMonitor) Start() error {
	if !atomic.CompareAndSwapInt32(&rm.isRunning, 0, 1) {
		return fmt.Errorf("monitor is already running")
	}

	corelog.Infof("Starting resource monitor with interval: %v", rm.config.MonitorInterval)

	go rm.monitorLoop()
	return nil
}

// Stop 停止监控
func (rm *ResourceMonitor) Stop() error {
	if !atomic.CompareAndSwapInt32(&rm.isRunning, 1, 0) {
		return fmt.Errorf("monitor is not running")
	}

	rm.cancel()
	corelog.Infof("Resource monitor stopped")
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
		stats.MemoryStats = memStats
	}

	// 收集资源统计
	if rm.config.EnableResourceMonitor {
		// 这里暂时使用固定值，因为全局资源管理器需要重构
		stats.ResourceCount = 0
		stats.DisposeCount = 0
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

// GetStats 获取所有统计信息
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

// Dispose 实现Disposable接口
func (rm *ResourceMonitor) Dispose() error {
	return rm.dispose.CloseWithError()
}

// IncrementDisposeCount 增加释放计数
func IncrementDisposeCount() {
	// 这里暂时不实现，因为全局计数器需要重构
}
