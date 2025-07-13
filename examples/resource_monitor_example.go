package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"tunnox-core/internal/utils"
)

func runResourceMonitorExample() {
	fmt.Println("=== Resource Monitor Example ===")

	// 1. 创建监控配置
	config := utils.DefaultMonitorConfig()
	config.MonitorInterval = 5 * time.Second
	config.GoroutineWarningThreshold = 100
	config.MemoryWarningThresholdMB = 256
	config.OnWarning = func(stats *utils.ResourceStats, warning string) {
		fmt.Printf("⚠️  WARNING: %s\n", warning)
		fmt.Printf("   Goroutines: %d, Memory: %d MB, Resources: %d\n",
			stats.GoroutineCount,
			stats.MemoryStats.Alloc/1024/1024,
			stats.ResourceCount)
	}

	// 2. 启动全局监控
	if err := utils.StartGlobalMonitor(config); err != nil {
		fmt.Printf("Failed to start global monitor: %v\n", err)
		return
	}
	defer utils.StopGlobalMonitor()

	fmt.Println("✅ Global resource monitor started")

	// 3. 创建服务管理器并注册资源
	serviceConfig := utils.DefaultServiceConfig()
	serviceConfig.EnableSignalHandling = false
	serviceManager := utils.NewServiceManager(serviceConfig)

	// 注册HTTP服务
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from monitored service!"))
	})
	httpService := utils.NewHTTPService(":8080", handler)
	serviceManager.RegisterService(httpService)

	// 注册一些模拟资源
	serviceManager.RegisterResource("database-connection", &MockDatabaseConnection{})
	serviceManager.RegisterResource("redis-client", &MockRedisClient{})
	serviceManager.RegisterResource("file-handler", &MockFileHandler{})

	fmt.Println("✅ Services and resources registered")

	// 4. 启动服务
	if err := serviceManager.StartAllServices(); err != nil {
		fmt.Printf("Failed to start services: %v\n", err)
		return
	}

	fmt.Println("✅ Services started")

	// 5. 模拟一些活动
	go simulateActivity()

	// 6. 运行一段时间并定期打印统计信息
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("\n🛑 Timeout reached, shutting down...")
			goto shutdown
		case <-ticker.C:
			printStats()
		}
	}

shutdown:
	// 7. 优雅关闭
	if err := serviceManager.StopAllServices(); err != nil {
		fmt.Printf("Failed to stop services: %v\n", err)
	}

	// 8. 打印最终统计信息
	fmt.Println("\n📊 Final Statistics:")
	printStats()

	fmt.Println("✅ Resource monitor example completed")
}

// simulateActivity 模拟一些活动来产生监控数据
func simulateActivity() {
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Second)

		// 创建一些临时goroutine
		go func(id int) {
			time.Sleep(1 * time.Second)
			fmt.Printf("🔄 Activity goroutine %d completed\n", id)
		}(i)

		// 注册一些临时资源
		tempResource := &MockResource{name: fmt.Sprintf("temp-resource-%d", i)}
		utils.RegisterGlobalResource(fmt.Sprintf("temp-%d", i), tempResource)
	}
}

// printStats 打印当前统计信息
func printStats() {
	summary := utils.GetGlobalStatsSummary()
	if summary.SampleCount == 0 {
		fmt.Println("📊 No statistics available yet")
		return
	}

	fmt.Printf("\n📊 Resource Monitor Statistics:\n")
	fmt.Printf("   Samples: %d\n", summary.SampleCount)
	fmt.Printf("   Duration: %v\n", summary.EndTime.Sub(summary.StartTime))

	fmt.Printf("   Goroutines:\n")
	fmt.Printf("     Current: %d\n", summary.GoroutineStats.Current)
	fmt.Printf("     Average: %.1f\n", summary.GoroutineStats.Average)
	fmt.Printf("     Min: %d, Max: %d\n", summary.GoroutineStats.Min, summary.GoroutineStats.Max)

	fmt.Printf("   Memory:\n")
	fmt.Printf("     Current: %d MB\n", summary.MemoryStats.CurrentAlloc/1024/1024)
	fmt.Printf("     Average: %.1f MB\n", summary.MemoryStats.AverageAlloc/1024/1024)
	fmt.Printf("     Min: %d MB, Max: %d MB\n",
		summary.MemoryStats.MinAlloc/1024/1024,
		summary.MemoryStats.MaxAlloc/1024/1024)
}

// MockResource 模拟资源
type MockResource struct {
	name     string
	disposed bool
}

func (m *MockResource) Dispose() error {
	if m.disposed {
		return nil
	}
	m.disposed = true
	fmt.Printf("🗑️  Resource %s disposed\n", m.name)
	return nil
}
