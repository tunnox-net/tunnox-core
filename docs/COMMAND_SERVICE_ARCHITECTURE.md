# CommandService 架构设计文档

## 概述

本文档描述了Session与Command集成的最优架构方案，通过引入CommandService抽象层，实现了完整的命令处理体系，并正确集成了Dispose资源管理体系。

## 架构设计原则

### 1. 单一职责原则
- **CommandService**: 负责命令的执行、统计和中间件管理
- **ResponseManager**: 负责响应的发送和格式化
- **CommandPipeline**: 负责中间件链的构建和执行
- **Session**: 负责连接管理和生命周期

### 2. 依赖倒置原则
- 所有组件都依赖抽象接口，而不是具体实现
- 通过依赖注入解耦组件间的依赖关系

### 3. 开闭原则
- 支持通过中间件扩展功能，无需修改核心代码
- 支持动态注册和注销命令处理器

### 4. 资源管理原则
- 所有组件都正确集成Dispose体系
- 支持优雅的资源清理和生命周期管理

## 核心组件

### 1. CommandService

**职责**: 命令处理的核心抽象层
**特性**:
- 统一的命令执行接口
- 内置统计信息收集
- 中间件支持
- 异步执行支持
- 完整的资源管理

```go
type CommandService interface {
    Execute(ctx *CommandContext) (*CommandResponse, error)
    ExecuteAsync(ctx *CommandContext) (<-chan *CommandResponse, <-chan error)
    Use(middleware Middleware)
    RegisterHandler(handler CommandHandler) error
    UnregisterHandler(commandType packet.CommandType) error
    GetStats() *CommandStats
    SetResponseSender(sender ResponseSender)
    Close() error
}
```

### 2. ResponseManager

**职责**: 响应发送管理
**特性**:
- 统一的响应发送接口
- 自动序列化和格式化
- 错误处理和重试
- 完整的资源管理

```go
type ResponseManager struct {
    session common.Session
    mu      sync.RWMutex
    utils.Dispose
}
```

### 3. CommandPipeline

**职责**: 中间件链管理
**特性**:
- 中间件链构建
- 超时控制
- 错误传播
- 性能监控

```go
type CommandPipeline struct {
    middleware []Middleware
    handler    CommandHandler
}
```

### 4. CommandStats

**职责**: 统计信息收集
**特性**:
- 线程安全的统计收集
- 实时性能监控
- 详细的执行指标

```go
type CommandStats struct {
    TotalCommands    int64
    SuccessCommands  int64
    FailedCommands   int64
    AverageLatency   time.Duration
    LastCommandTime  time.Time
    ActiveCommands   int64
    mu               sync.RWMutex
}
```

## 架构优势

### 1. 完整的资源管理
- 所有组件都集成Dispose体系
- 支持优雅关闭和资源清理
- 防止资源泄漏

### 2. 强大的扩展性
- 中间件机制支持功能扩展
- 动态处理器注册
- 插件化架构

### 3. 优秀的性能
- 异步执行支持
- 统计信息收集
- 性能监控和优化

### 4. 良好的可维护性
- 清晰的职责分离
- 统一的错误处理
- 完整的日志记录

### 5. 强类型安全
- 泛型支持
- 编译时类型检查
- 运行时类型安全

## 使用示例

### 1. 基本使用

```go
// 创建会话
session := protocol.NewConnectionSession(idManager, ctx)

// 创建命令服务
commandService := command.CreateDefaultService(ctx)

// 设置到会话
session.SetCommandService(commandService)
```

### 2. 中间件使用

```go
// 添加日志中间件
commandService.Use(&command.LoggingMiddleware{})

// 添加指标中间件
commandService.Use(command.NewMetricsMiddleware(metricsCollector))

// 添加自定义中间件
commandService.Use(&CustomMiddleware{})
```

### 3. 统计信息

```go
// 获取统计信息
stats := commandService.GetStats()
log.Printf("Total commands: %d", stats.GetStats().TotalCommands)
log.Printf("Average latency: %v", stats.GetStats().AverageLatency)
```

### 4. 异步执行

```go
// 异步执行命令
responseChan, errorChan := commandService.ExecuteAsync(ctx)

// 等待结果
select {
case response := <-responseChan:
    log.Printf("Success: %+v", response)
case err := <-errorChan:
    log.Printf("Error: %v", err)
}
```

## 资源管理

### 1. Dispose集成

所有组件都正确集成Dispose体系：

```go
// CommandService
type CommandServiceImpl struct {
    // ... 其他字段
    utils.Dispose
}

// ResponseManager
type ResponseManager struct {
    // ... 其他字段
    utils.Dispose
}

// Session
type ConnectionSession struct {
    // ... 其他字段
    utils.Dispose
}
```

### 2. 资源清理

```go
// CommandService清理
func (cs *CommandServiceImpl) onClose() error {
    // 清理统计信息
    cs.stats = &CommandStats{}
    
    // 清理中间件
    cs.middleware = make([]Middleware, 0)
    cs.responseSender = nil
    
    return nil
}

// ResponseManager清理
func (rm *ResponseManager) onClose() error {
    rm.session = nil
    return nil
}

// Session清理
func (s *ConnectionSession) onClose() error {
    // 清理命令服务
    if s.commandService != nil {
        s.commandService.Close()
    }
    
    // 清理响应管理器
    if s.responseManager != nil {
        s.responseManager.Dispose.Close()
    }
    
    // 清理连接
    // ...
    
    return nil
}
```

## 性能优化

### 1. 并发安全
- 所有组件都使用适当的锁机制
- 支持高并发访问
- 无锁的统计信息更新

### 2. 内存优化
- 对象池减少GC压力
- 高效的JSON序列化
- 最小化内存分配

### 3. 网络优化
- 异步响应发送
- 批量处理支持
- 连接复用

## 错误处理

### 1. 统一错误处理
- 所有错误都有明确的类型
- 支持错误传播和恢复
- 完整的错误日志

### 2. 超时控制
- 命令执行超时
- 响应发送超时
- 资源清理超时

### 3. 重试机制
- 可配置的重试策略
- 指数退避算法
- 熔断器模式

## 监控和调试

### 1. 统计信息
- 命令执行统计
- 性能指标收集
- 错误率监控

### 2. 日志记录
- 结构化日志
- 不同级别的日志
- 性能追踪

### 3. 调试支持
- 详细的调试信息
- 性能分析工具
- 内存使用监控

## 总结

这个最优架构方案通过以下方式解决了Session与Command集成的问题：

1. **引入CommandService抽象层**，统一命令处理逻辑
2. **创建ResponseManager**，解决响应发送问题
3. **实现CommandPipeline**，支持中间件扩展
4. **集成Dispose体系**，确保资源正确管理
5. **提供完整的统计和监控**，支持运维和调试

这个架构不仅解决了当前的问题，还为未来的扩展奠定了坚实的基础。 