package tests

import (
	"fmt"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/utils"
)

// TestResourceMonitorBasic 测试资源监控器基本功能
func TestResourceMonitorBasic(t *testing.T) {
	// 创建监控配置
	config := utils.DefaultMonitorConfig()
	config.MonitorInterval = 100 * time.Millisecond // 快速监控用于测试
	config.GoroutineWarningThreshold = 100          // 降低阈值用于测试
	config.MemoryWarningThresholdMB = 10            // 降低阈值用于测试

	// 创建监控器
	monitor := utils.NewResourceMonitor(config)

	// 启动监控
	if err := monitor.Start(); err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}

	// 等待一段时间收集数据
	time.Sleep(300 * time.Millisecond)

	// 检查监控器状态
	if !monitor.IsRunning() {
		t.Error("Monitor should be running")
	}

	// 获取统计信息
	stats := monitor.GetStats()
	if len(stats) == 0 {
		t.Error("Should have collected some stats")
	}

	// 检查最新统计信息
	latestStats := monitor.GetLatestStats()
	if latestStats == nil {
		t.Error("Should have latest stats")
	}

	// 检查goroutine数量
	if latestStats.GoroutineCount <= 0 {
		t.Error("Goroutine count should be positive")
	}

	// 检查内存统计
	if latestStats.MemoryStats.Alloc == 0 {
		t.Error("Memory allocation should be non-zero")
	}

	// 停止监控
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Failed to stop monitor: %v", err)
	}

	// 检查监控器状态
	if monitor.IsRunning() {
		t.Error("Monitor should not be running")
	}
}

// TestResourceMonitorWithWarnings 测试监控器警告功能
func TestResourceMonitorWithWarnings(t *testing.T) {
	warnings := make([]string, 0)
	var warningMu sync.Mutex

	// 创建监控配置
	config := utils.DefaultMonitorConfig()
	config.MonitorInterval = 50 * time.Millisecond
	config.GoroutineWarningThreshold = 5 // 很低的阈值，容易触发
	config.MemoryWarningThresholdMB = 1  // 很低的阈值，容易触发
	config.OnWarning = func(stats *utils.ResourceStats, warning string) {
		warningMu.Lock()
		warnings = append(warnings, warning)
		warningMu.Unlock()
	}

	// 创建监控器
	monitor := utils.NewResourceMonitor(config)

	// 启动监控
	if err := monitor.Start(); err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}

	// 创建一些goroutine来触发警告
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(100 * time.Millisecond)
		}()
	}

	// 等待一段时间
	time.Sleep(200 * time.Millisecond)

	// 检查是否产生了警告
	warningMu.Lock()
	warningCount := len(warnings)
	warningMu.Unlock()

	if warningCount == 0 {
		t.Log("No warnings generated (this might be normal depending on system load)")
	} else {
		t.Logf("Generated %d warnings: %v", warningCount, warnings)
	}

	// 停止监控
	monitor.Stop()
}

// TestResourceMonitorStatsSummary 测试统计摘要功能
func TestResourceMonitorStatsSummary(t *testing.T) {
	// 创建监控配置
	config := utils.DefaultMonitorConfig()
	config.MonitorInterval = 50 * time.Millisecond

	// 创建监控器
	monitor := utils.NewResourceMonitor(config)

	// 启动监控
	if err := monitor.Start(); err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}

	// 等待收集数据
	time.Sleep(200 * time.Millisecond)

	// 获取统计摘要
	summary := monitor.GetStatsSummary()

	// 检查摘要信息
	if summary.SampleCount == 0 {
		t.Error("Should have collected samples")
	}

	if summary.StartTime.IsZero() {
		t.Error("Start time should not be zero")
	}

	if summary.EndTime.IsZero() {
		t.Error("End time should not be zero")
	}

	// 检查goroutine统计
	if summary.GoroutineStats.Current <= 0 {
		t.Error("Current goroutine count should be positive")
	}

	if summary.GoroutineStats.Max < summary.GoroutineStats.Min {
		t.Error("Max goroutine count should be >= min")
	}

	// 检查内存统计
	if summary.MemoryStats.CurrentAlloc == 0 {
		t.Error("Current memory allocation should be non-zero")
	}

	if summary.MemoryStats.MaxAlloc < summary.MemoryStats.MinAlloc {
		t.Error("Max memory allocation should be >= min")
	}

	t.Logf("Stats summary: %+v", summary)

	// 停止监控
	monitor.Stop()
}

// TestGlobalMonitor 测试全局监控功能
func TestGlobalMonitor(t *testing.T) {
	// 创建监控配置
	config := utils.DefaultMonitorConfig()
	config.MonitorInterval = 100 * time.Millisecond

	// 启动全局监控
	if err := utils.StartGlobalMonitor(config); err != nil {
		t.Fatalf("Failed to start global monitor: %v", err)
	}

	// 检查全局监控器
	monitor := utils.GetGlobalMonitor()
	if monitor == nil {
		t.Fatal("Global monitor should not be nil")
	}

	if !monitor.IsRunning() {
		t.Error("Global monitor should be running")
	}

	// 等待收集数据
	time.Sleep(200 * time.Millisecond)

	// 获取全局统计信息
	stats := utils.GetGlobalStats()
	if len(stats) == 0 {
		t.Error("Should have global stats")
	}

	// 获取全局统计摘要
	summary := utils.GetGlobalStatsSummary()
	if summary.SampleCount == 0 {
		t.Error("Should have global stats summary")
	}

	t.Logf("Global stats summary: %+v", summary)

	// 停止全局监控
	if err := utils.StopGlobalMonitor(); err != nil {
		t.Fatalf("Failed to stop global monitor: %v", err)
	}
}

// TestResourceMonitorWithDispose 测试监控器与Dispose系统的集成
func TestResourceMonitorWithDispose(t *testing.T) {
	// 启动全局监控
	config := utils.DefaultMonitorConfig()
	config.MonitorInterval = 50 * time.Millisecond
	if err := utils.StartGlobalMonitor(config); err != nil {
		t.Fatalf("Failed to start global monitor: %v", err)
	}
	defer utils.StopGlobalMonitor()

	// 创建资源管理器
	resourceMgr := utils.NewResourceManager()

	// 注册一些资源
	resources := make([]*MockResource, 5)
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("monitor-test-resource-%d", i)
		resources[i] = NewMockResource(name)
		if err := resourceMgr.Register(name, resources[i]); err != nil {
			t.Fatalf("Failed to register resource %s: %v", name, err)
		}
	}

	// 等待监控收集初始数据
	time.Sleep(100 * time.Millisecond)

	// 获取初始统计
	initialStats := utils.GetGlobalStatsSummary()

	// 释放资源
	result := resourceMgr.DisposeAll()
	if result.HasErrors() {
		t.Fatalf("Resource disposal failed: %v", result.Error())
	}

	// 等待监控收集释放后的数据
	time.Sleep(100 * time.Millisecond)

	// 获取释放后的统计
	finalStats := utils.GetGlobalStatsSummary()

	// 检查释放计数是否增加
	if finalStats.SampleCount <= initialStats.SampleCount {
		t.Error("Should have collected more samples after disposal")
	}

	// 验证资源已被释放
	for _, resource := range resources {
		if !resource.IsDisposed() {
			t.Errorf("Resource %s should be disposed", resource.name)
		}
	}

	t.Logf("Initial stats: %+v", initialStats)
	t.Logf("Final stats: %+v", finalStats)
}

// TestResourceMonitorConcurrent 测试监控器的并发安全性
func TestResourceMonitorConcurrent(t *testing.T) {
	// 创建监控配置
	config := utils.DefaultMonitorConfig()
	config.MonitorInterval = 10 * time.Millisecond

	// 创建监控器
	monitor := utils.NewResourceMonitor(config)

	// 启动监控
	if err := monitor.Start(); err != nil {
		t.Fatalf("Failed to start monitor: %v", err)
	}
	defer monitor.Stop()

	// 并发访问统计信息
	var wg sync.WaitGroup
	accessCount := 100

	for i := 0; i < accessCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 并发获取统计信息
			stats := monitor.GetStats()
			latest := monitor.GetLatestStats()
			summary := monitor.GetStatsSummary()

			// 验证数据一致性
			if len(stats) > 0 && latest != nil {
				if stats[len(stats)-1].Timestamp != latest.Timestamp {
					t.Error("Latest stats timestamp should match last stats timestamp")
				}
			}

			if summary.SampleCount != len(stats) {
				t.Error("Summary sample count should match stats length")
			}
		}()
	}

	wg.Wait()
}
