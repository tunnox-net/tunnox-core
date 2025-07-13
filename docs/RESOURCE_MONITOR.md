# 资源监控系统使用指南

## 概述

资源监控系统是 Tunnox Core 的一个重要组件，提供了实时的资源使用情况监控，包括：

- **Goroutine 监控**: 监控当前运行的 goroutine 数量
- **内存监控**: 监控内存分配和使用情况
- **资源监控**: 监控注册的资源数量和释放次数
- **警告系统**: 当资源使用超过阈值时自动发出警告

## 核心功能

### 1. 实时监控

监控系统会定期收集以下信息：
- 当前 goroutine 数量
- 内存分配统计（当前分配、总分配、系统内存、GC 次数）
- 注册的资源数量
- 资源释放次数

### 2. 警告机制

当以下条件满足时会触发警告：
- Goroutine 数量超过阈值（默认 1000）
- 内存使用超过阈值（默认 512MB）
- 资源数量超过阈值（默认 100）

### 3. 统计摘要

提供详细的统计摘要，包括：
- 平均值、最大值、最小值
- 监控时间范围
- 样本数量

## 基本使用

### 1. 启动全局监控

```go
import "tunnox-core/internal/utils"

// 使用默认配置启动全局监控
if err := utils.StartGlobalMonitor(nil); err != nil {
    log.Fatalf("Failed to start global monitor: %v", err)
}

// 程序结束时停止监控
defer utils.StopGlobalMonitor()
```

### 2. 自定义监控配置

```go
// 创建自定义配置
config := utils.DefaultMonitorConfig()
config.MonitorInterval = 10 * time.Second        // 监控间隔
config.GoroutineWarningThreshold = 500          // goroutine 警告阈值
config.MemoryWarningThresholdMB = 256           // 内存警告阈值
config.OnWarning = func(stats *utils.ResourceStats, warning string) {
    log.Printf("WARNING: %s", warning)
    log.Printf("Goroutines: %d, Memory: %d MB", 
        stats.GoroutineCount, 
        stats.MemoryStats.Alloc/1024/1024)
}

// 启动监控
if err := utils.StartGlobalMonitor(config); err != nil {
    log.Fatalf("Failed to start monitor: %v", err)
}
```

### 3. 获取监控数据

```go
// 获取所有统计信息
stats := utils.GetGlobalStats()
for _, stat := range stats {
    fmt.Printf("Time: %v, Goroutines: %d, Memory: %d MB\n",
        stat.Timestamp,
        stat.GoroutineCount,
        stat.MemoryStats.Alloc/1024/1024)
}

// 获取统计摘要
summary := utils.GetGlobalStatsSummary()
fmt.Printf("Goroutine Stats: Avg=%.1f, Min=%d, Max=%d, Current=%d\n",
    summary.GoroutineStats.Average,
    summary.GoroutineStats.Min,
    summary.GoroutineStats.Max,
    summary.GoroutineStats.Current)
```

## 与服务管理器集成

### 1. 在服务管理器中启用监控

```go
// 创建服务配置
serviceConfig := utils.DefaultServiceConfig()
serviceConfig.EnableSignalHandling = true

// 创建服务管理器
serviceManager := utils.NewServiceManager(serviceConfig)

// 启动全局监控
if err := utils.StartGlobalMonitor(nil); err != nil {
    log.Fatalf("Failed to start monitor: %v", err)
}

// 注册服务和资源
serviceManager.RegisterService(httpService)
serviceManager.RegisterResource("database", dbConn)

// 运行服务管理器
if err := serviceManager.Run(); err != nil {
    log.Printf("Service manager error: %v", err)
}
```

### 2. 监控资源释放

监控系统会自动跟踪资源释放次数：

```go
// 创建资源管理器
resourceMgr := utils.NewResourceManager()

// 注册资源
resourceMgr.Register("my-resource", myResource)

// 释放资源（会自动增加释放计数）
result := resourceMgr.DisposeAll()

// 获取最新统计信息
latestStats := utils.GetGlobalMonitor().GetLatestStats()
fmt.Printf("Total dispose count: %d\n", latestStats.DisposeCount)
```

## 高级功能

### 1. 自定义监控器

```go
// 创建独立的监控器
monitor := utils.NewResourceMonitor(config)

// 启动监控
if err := monitor.Start(); err != nil {
    log.Fatalf("Failed to start monitor: %v", err)
}

// 获取监控数据
stats := monitor.GetStats()
summary := monitor.GetStatsSummary()

// 停止监控
monitor.Stop()
```

### 2. 自定义警告处理

```go
config := utils.DefaultMonitorConfig()
config.OnWarning = func(stats *utils.ResourceStats, warning string) {
    // 发送告警邮件
    sendAlertEmail(warning, stats)
    
    // 记录到日志
    log.Printf("ALERT: %s", warning)
    
    // 触发自动清理
    if stats.GoroutineCount > 2000 {
        triggerGoroutineCleanup()
    }
}
```

### 3. 监控数据持久化

```go
// 定期保存监控数据
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            stats := utils.GetGlobalStats()
            saveStatsToDatabase(stats)
        }
    }
}()
```

## 配置选项

### MonitorConfig 字段说明

| 字段 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| MonitorInterval | time.Duration | 30s | 监控间隔 |
| EnableGoroutineMonitor | bool | true | 是否启用 goroutine 监控 |
| EnableMemoryMonitor | bool | true | 是否启用内存监控 |
| EnableResourceMonitor | bool | true | 是否启用资源监控 |
| GoroutineWarningThreshold | int64 | 1000 | goroutine 警告阈值 |
| MemoryWarningThresholdMB | int64 | 512 | 内存警告阈值（MB） |
| OnWarning | func | 默认日志 | 警告回调函数 |

## 性能考虑

### 1. 监控开销

- 监控系统本身会创建额外的 goroutine
- 内存统计收集有一定开销
- 建议在生产环境中适当调整监控间隔

### 2. 数据存储

- 默认保留最近 100 条统计记录
- 大量历史数据需要外部存储
- 考虑使用时间序列数据库存储长期数据

### 3. 警告频率

- 避免设置过低的警告阈值
- 考虑添加警告频率限制
- 实现智能警告聚合

## 最佳实践

### 1. 监控配置

```go
// 生产环境配置
config := utils.DefaultMonitorConfig()
config.MonitorInterval = 60 * time.Second  // 降低监控频率
config.GoroutineWarningThreshold = 2000   // 根据应用调整
config.MemoryWarningThresholdMB = 1024    // 根据服务器配置调整
```

### 2. 警告处理

```go
config.OnWarning = func(stats *utils.ResourceStats, warning string) {
    // 记录详细日志
    log.Printf("Resource warning: %s", warning)
    log.Printf("Current state: goroutines=%d, memory=%d MB, resources=%d",
        stats.GoroutineCount,
        stats.MemoryStats.Alloc/1024/1024,
        stats.ResourceCount)
    
    // 发送告警
    if shouldSendAlert(warning) {
        sendAlert(warning, stats)
    }
}
```

### 3. 监控数据使用

```go
// 定期分析监控数据
func analyzeMonitoringData() {
    summary := utils.GetGlobalStatsSummary()
    
    // 检查趋势
    if summary.GoroutineStats.Average > summary.GoroutineStats.Current*1.5 {
        log.Printf("Goroutine count is decreasing, possible cleanup")
    }
    
    // 检查内存泄漏
    if summary.MemoryStats.CurrentAlloc > summary.MemoryStats.AverageAlloc*2 {
        log.Printf("Possible memory leak detected")
    }
}
```

## 故障排除

### 1. 监控器无法启动

- 检查是否已经启动了全局监控器
- 确认配置参数正确
- 查看错误日志

### 2. 警告过于频繁

- 调整警告阈值
- 检查应用是否存在资源泄漏
- 优化资源使用模式

### 3. 监控数据不准确

- 确认监控间隔设置合理
- 检查系统时间是否正确
- 验证资源注册和释放逻辑

## 示例代码

完整的示例代码请参考 `examples/resource_monitor_example.go`，该示例展示了：

- 如何启动和配置监控系统
- 如何与服务管理器集成
- 如何处理警告和统计信息
- 如何优雅关闭监控系统 