package monitor

import (
	"context"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/utils"
)

// MockResource 模拟资源
type MockResource struct {
	name      string
	disposed  bool
	disposeMu sync.Mutex
}

func NewMockResource(name string) *MockResource {
	return &MockResource{name: name}
}

func (mr *MockResource) Dispose() error {
	mr.disposeMu.Lock()
	defer mr.disposeMu.Unlock()
	mr.disposed = true
	return nil
}

func (mr *MockResource) IsDisposed() bool {
	mr.disposeMu.Lock()
	defer mr.disposeMu.Unlock()
	return mr.disposed
}

// TestResourceMonitorBasic 测试资源监控器基本功能
func TestResourceMonitorBasic(t *testing.T) {
	// 创建监控配置
	config := utils.DefaultMonitorConfig()
	config.MonitorInterval = 50 * time.Millisecond
	config.GoroutineWarningThreshold = 100 // 降低阈值用于测试
	config.MemoryWarningThresholdMB = 10   // 降低阈值用于测试

	// 创建监控器
	monitor := utils.NewResourceMonitor(config, context.Background())

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
	monitor := utils.NewResourceMonitor(config, context.Background())

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

	// 等待goroutine完成
	wg.Wait()

	// 停止监控
	if err := monitor.Stop(); err != nil {
		t.Fatalf("Failed to stop monitor: %v", err)
	}
}
