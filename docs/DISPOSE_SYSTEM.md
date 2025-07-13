# Dispose 系统使用指南

## 概述

Dispose 系统是 Tunnox Core 的统一资源管理系统，提供了优雅的资源释放和生命周期管理功能。系统包含以下核心组件：

- **Disposable 接口**: 统一的资源释放接口
- **ResourceManager**: 资源管理器，负责统一管理所有可释放资源
- **ServerManager**: 服务器管理器，提供优雅关闭的封装
- **全局资源管理**: 便捷的全局资源注册和释放

## 核心接口

### Disposable 接口

```go
type Disposable interface {
    Dispose() error
}
```

所有需要资源管理的组件都应该实现这个接口。

## 基本使用

### 1. 创建资源管理器

```go
// 创建独立的资源管理器
resourceMgr := utils.NewResourceManager()

// 或使用全局资源管理器
utils.RegisterGlobalResource("my-resource", myResource)
```

### 2. 注册资源

```go
// 注册资源，按注册顺序进行释放
err := resourceMgr.Register("database-connection", dbConn)
err = resourceMgr.Register("redis-client", redisClient)
err = resourceMgr.Register("file-handler", fileHandler)
```

### 3. 释放资源

```go
// 释放所有资源（按注册的相反顺序）
result := resourceMgr.DisposeAll()

// 检查释放结果
if result.HasErrors() {
    for _, err := range result.Errors {
        log.Printf("Resource disposal error: %v", err)
    }
}
```

### 4. 带超时的资源释放

```go
// 设置10秒超时
result := resourceMgr.DisposeWithTimeout(10 * time.Second)
if result.HasErrors() {
    // 处理超时或错误
}
```

## 服务器管理

### 1. 基本服务器启动

```go
// 使用默认配置启动服务器
ctx := context.Background()
handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("Hello, World!"))
})

err := utils.StartServerWithCleanup(ctx, ":8080", handler)
```

### 2. 自定义配置

```go
// 创建自定义配置
config := utils.DefaultServerConfig()
config.Addr = ":9090"
config.GracefulShutdownTimeout = 30 * time.Second
config.ResourceDisposeTimeout = 10 * time.Second
config.EnableSignalHandling = true

// 创建服务器管理器
serverMgr := utils.NewServerManager(config)

// 注册资源
serverMgr.RegisterResource("database", dbConn)
serverMgr.RegisterResource("cache", cacheClient)

// 启动服务器
err := serverMgr.RunWithManagedResources(ctx, handler)
```

### 3. 手动资源管理

```go
// 获取资源列表
resources := serverMgr.ListResources()
fmt.Printf("Registered resources: %v\n", resources)

// 获取资源数量
count := serverMgr.GetResourceCount()
fmt.Printf("Resource count: %d\n", count)

// 获取释放结果
result := serverMgr.GetDisposeResult()
if result != nil && result.HasErrors() {
    // 处理错误
}
```

## 组件集成

### 1. 流管理器集成

```go
// 创建流管理器
factory := stream.NewDefaultFactory()
streamMgr := stream.NewStreamManager(factory, ctx)

// 注册到资源管理器
resourceMgr.Register("stream-manager", streamMgr)

// 创建流
stream, err := streamMgr.CreateStream("stream-1", reader, writer)
```

### 2. 协议管理器集成

```go
// 创建协议管理器
protocolMgr := protocol.NewManager(ctx)

// 注册适配器
protocolMgr.Register(tcpAdapter)
protocolMgr.Register(udpAdapter)

// 注册到资源管理器
resourceMgr.Register("protocol-manager", protocolMgr)
```

### 3. 存储组件集成

```go
// 创建存储组件
storage := storages.NewMemoryStorage(ctx)

// 注册到资源管理器
resourceMgr.Register("storage", storage)
```

## 错误处理

### 1. 处理释放错误

```go
result := resourceMgr.DisposeAll()
if result.HasErrors() {
    for _, disposeErr := range result.Errors {
        log.Printf("Resource %s disposal failed: %v", 
            disposeErr.ResourceName, disposeErr.Err)
    }
}
```

### 2. 部分错误处理

```go
// 即使某些资源释放失败，其他资源仍会被释放
result := resourceMgr.DisposeAll()
if result.HasErrors() {
    log.Printf("Some resources failed to dispose: %d errors", 
        len(result.Errors))
    // 继续处理，不要中断程序
}
```

## 最佳实践

### 1. 资源注册顺序

```go
// 按照依赖关系注册资源
resourceMgr.Register("config", configManager)      // 配置管理器
resourceMgr.Register("database", databaseConn)     // 数据库连接
resourceMgr.Register("cache", cacheClient)         // 缓存客户端
resourceMgr.Register("stream-manager", streamMgr)  // 流管理器
resourceMgr.Register("protocol-manager", protocolMgr) // 协议管理器

// 释放时会按照相反顺序：protocol-manager -> stream-manager -> cache -> database -> config
```

### 2. 错误处理

```go
// 实现 Disposable 接口时，确保幂等性
func (r *MyResource) Dispose() error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if r.disposed {
        return nil // 已经释放过，直接返回
    }
    
    // 执行释放逻辑
    err := r.doDispose()
    if err == nil {
        r.disposed = true
    }
    return err
}
```

### 3. 超时设置

```go
// 根据资源类型设置合适的超时时间
config := utils.DefaultServerConfig()
config.ResourceDisposeTimeout = 30 * time.Second  // 数据库连接等慢速资源
config.GracefulShutdownTimeout = 60 * time.Second // 服务器优雅关闭
```

### 4. 并发安全

```go
// ResourceManager 是并发安全的，可以在多个 goroutine 中使用
go func() {
    resourceMgr.Register("async-resource", resource)
}()

go func() {
    result := resourceMgr.DisposeAll()
    // 处理结果
}()
```

## 测试

### 1. 单元测试

```go
func TestMyResourceDispose(t *testing.T) {
    resource := NewMyResource()
    resourceMgr := utils.NewResourceManager()
    
    // 注册资源
    err := resourceMgr.Register("test-resource", resource)
    require.NoError(t, err)
    
    // 释放资源
    result := resourceMgr.DisposeAll()
    require.False(t, result.HasErrors())
    
    // 验证资源已释放
    require.True(t, resource.IsDisposed())
}
```

### 2. 集成测试

```go
func TestServerDispose(t *testing.T) {
    config := utils.DefaultServerConfig()
    config.Addr = ":0" // 使用随机端口
    config.GracefulShutdownTimeout = 1 * time.Second
    
    serverMgr := utils.NewServerManager(config)
    
    // 注册测试资源
    testResource := NewMockResource("test")
    serverMgr.RegisterResource("test-resource", testResource)
    
    // 启动服务器
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    go func() {
        handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusOK)
        })
        serverMgr.StartServerWithCleanup(ctx, handler)
    }()
    
    // 等待上下文取消
    <-ctx.Done()
    
    // 验证资源释放
    result := serverMgr.GetDisposeResult()
    require.Nil(t, result) // 或检查具体错误
}
```

## 性能考虑

### 1. 大量资源处理

```go
// 对于大量资源，考虑分批处理
const batchSize = 1000
for i := 0; i < totalResources; i += batchSize {
    batch := resourceMgr.GetResourceBatch(i, batchSize)
    // 处理批次
}
```

### 2. 超时设置

```go
// 根据资源数量调整超时时间
timeout := time.Duration(resourceCount) * time.Millisecond
if timeout < 100*time.Millisecond {
    timeout = 100 * time.Millisecond
}
result := resourceMgr.DisposeWithTimeout(timeout)
```

## 故障排除

### 1. 常见问题

**Q: 资源释放超时怎么办？**
A: 检查资源是否实现了正确的 Dispose 方法，避免阻塞操作。

**Q: 某些资源释放失败会影响其他资源吗？**
A: 不会，系统会继续释放其他资源，但会记录所有错误。

**Q: 如何调试资源释放顺序？**
A: 使用日志记录资源注册和释放过程，或查看 `ListResources()` 返回的顺序。

### 2. 调试技巧

```go
// 启用详细日志
utils.SetLogLevel(utils.DebugLevel)

// 检查资源状态
for _, name := range resourceMgr.ListResources() {
    fmt.Printf("Resource: %s\n", name)
}

// 监控释放过程
result := resourceMgr.DisposeAll()
for _, err := range result.Errors {
    fmt.Printf("Failed to dispose %s: %v\n", err.ResourceName, err.Err)
}
```

## 总结

Dispose 系统提供了完整的资源生命周期管理解决方案，包括：

- 统一的资源释放接口
- 自动的资源释放顺序管理
- 优雅的服务器关闭
- 完善的错误处理和超时机制
- 并发安全的操作
- 详细的测试支持

通过正确使用 Dispose 系统，可以确保应用程序在关闭时能够正确释放所有资源，避免资源泄漏和内存问题。 