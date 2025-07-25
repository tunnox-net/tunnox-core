# Command与Session集成改进

## 概述

本次改进实现了Command体系与Session的深度集成，提供了更直接、更高效的命令处理机制。

## 主要改进

### 1. Session接口扩展

在`types.Session`接口中新增了以下Command相关方法：

```go
// 注册和注销命令处理器
RegisterCommandHandler(cmdType packet.CommandType, handler CommandHandler) error
UnregisterCommandHandler(cmdType packet.CommandType) error

// 直接处理命令
ProcessCommand(connID string, cmd *packet.CommandPacket) (*CommandResponse, error)

// 获取Command相关组件
GetCommandRegistry() CommandRegistry
GetCommandExecutor() CommandExecutor
SetCommandExecutor(executor CommandExecutor) error
```

### 2. CommandExecutor接口定义

新增了`types.CommandExecutor`接口：

```go
type CommandExecutor interface {
    Execute(streamPacket *StreamPacket) error
    AddMiddleware(middleware Middleware)
    SetSession(session Session)
    GetRegistry() CommandRegistry
}
```

### 3. SessionManager实现

在`SessionManager`中实现了完整的Command集成：

- 自动创建`CommandRegistry`和`CommandExecutor`
- 在构造函数中建立双向引用关系
- 提供完整的命令处理流程

## 使用方式

### 基本用法

```go
// 创建会话
session := session.NewSessionManager(idManager, ctx)

// 注册命令处理器
connectHandler := NewConnectHandler()
session.RegisterCommandHandler(packet.Connect, connectHandler)

// 添加中间件
commandExecutor := session.GetCommandExecutor()
commandExecutor.AddMiddleware(&LoggingMiddleware{})

// 直接处理命令
response, err := session.ProcessCommand(connID, commandPacket)
```

### 命令处理器示例

```go
type ConnectHandler struct{}

func (h *ConnectHandler) Handle(ctx *types.CommandContext) (*types.CommandResponse, error) {
    // 处理连接命令
    return &types.CommandResponse{
        Success: true,
        Data:    "Connected successfully",
    }, nil
}

func (h *ConnectHandler) GetDirection() types.CommandDirection {
    return types.DirectionDuplex
}

func (h *ConnectHandler) GetCommandType() packet.CommandType {
    return packet.Connect
}

// ... 其他接口方法
```

### 中间件示例

```go
type LoggingMiddleware struct{}

func (m *LoggingMiddleware) Process(ctx *types.CommandContext, next func(*types.CommandContext) (*types.CommandResponse, error)) (*types.CommandResponse, error) {
    start := time.Now()
    response, err := next(ctx)
    duration := time.Since(start)
    
    utils.Infof("Command %v completed in %v", ctx.CommandType, duration)
    return response, err
}
```

## 处理流程

### 1. 命令接收流程

```
数据包 -> Session.HandlePacket() -> handleCommandPacket() -> CommandExecutor.Execute()
```

### 2. 命令处理优先级

1. **优先使用Command集成**：直接通过`CommandExecutor`处理
2. **回退到事件总线**：如果Command执行器不可用，使用事件驱动
3. **最后使用默认处理**：如果都不可用，使用默认处理器

### 3. 中间件链处理

```
请求 -> 中间件1 -> 中间件2 -> ... -> 处理器 -> 中间件2 -> 中间件1 -> 响应
```

## 优势

### 1. 直接集成
- 无需通过事件总线，减少延迟
- 更直接的状态同步
- 更好的错误处理

### 2. 灵活的回退机制
- 保持向后兼容
- 支持多种处理方式
- 渐进式迁移

### 3. 完整的中间件支持
- 支持处理前和处理后逻辑
- 可组合的中间件链
- 统一的错误处理

### 4. 类型安全
- 强类型的接口定义
- 编译时错误检查
- 更好的IDE支持

## 迁移指南

### 从事件驱动迁移

**之前（事件驱动）：**
```go
// 通过事件总线处理命令
eventBus.Publish(events.NewCommandReceivedEvent(...))
```

**现在（直接集成）：**
```go
// 直接处理命令
response, err := session.ProcessCommand(connID, commandPacket)
```

### 注册命令处理器

**之前：**
```go
commandService.RegisterHandler(handler)
```

**现在：**
```go
session.RegisterCommandHandler(cmdType, handler)
```

## 注意事项

1. **资源管理**：Command相关资源会在Session关闭时自动清理
2. **并发安全**：所有操作都是线程安全的
3. **错误处理**：提供了完整的错误处理和回退机制
4. **性能考虑**：直接集成比事件驱动有更低的延迟

## 未来扩展

1. **命令路由**：支持基于规则的命令路由
2. **命令缓存**：支持命令结果缓存
3. **命令限流**：支持基于连接的命令限流
4. **命令监控**：支持详细的命令执行监控 