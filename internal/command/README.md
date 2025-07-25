# Command Framework 使用说明

## 概述

这是一个极致优雅的分层命令处理框架，支持命令注册、链式调用、中间件扩展等功能。

## 架构分层

### 1. 传输层 (Transport Layer)
- 负责底层数据包的读写
- 处理压缩、加密、限速等传输细节
- 与现有的 `StreamProcessor` 对接

### 2. 协议层 (Protocol Layer)  
- 定义命令包和响应包的结构
- 处理序列化和反序列化
- 管理包类型和命令类型

### 3. 注册层 (Registry Layer)
- 命令处理器注册和管理
- 支持动态注册和注销
- 提供处理器查找功能

### 4. 执行层 (Execution Layer)
- 命令分发和执行
- 区分单向和双工调用
- 处理超时和错误

### 5. 工具层 (Utility Layer)
- 链式调用API
- 提供优雅的调用接口
- 支持同步/异步执行

### 6. 中间件层 (Middleware Layer)
- 拦截器机制
- 支持日志、监控、重试等
- 可插拔的扩展点

## 核心组件

### CommandContext
命令上下文，包含所有必要信息：
- `ConnectionID`: 连接ID
- `CommandType`: 命令类型
- `RequestBody`: JSON请求字符串
- `Session`: 会话对象
- `Context`: 上下文
- `Metadata`: 元数据

### CommandHandler
命令处理器接口：
```go
type CommandHandler interface {
    Handle(ctx *CommandContext) (*CommandResponse, error)
    GetResponseType() ResponseType
    GetCommandType() packet.CommandType
}
```

### CommandRegistry
命令注册器，管理所有命令处理器：
```go
registry := NewCommandRegistry()
registry.Register(handler)
```

### CommandExecutor
命令执行器，负责命令的分发和执行：
```go
executor := NewCommandExecutor(registry)
executor.AddMiddleware(middleware)
executor.Execute(streamPacket)
```

### CommandUtils
链式调用工具类：
```go
utils := NewCommandUtils(session)
response, err := utils
    .WithCommand(packet.TcpMap)
    .PutRequest(requestData)
    .ResultAs(&responseData)
    .Timeout(10 * time.Second)
    .Execute()
```

## 使用示例

### 1. 注册命令处理器
```go
// 创建注册器
registry := NewCommandRegistry()

// 注册各种命令处理器
registry.Register(NewTcpMapHandler())
registry.Register(NewHttpMapHandler())
registry.Register(NewSocksMapHandler())
registry.Register(NewDisconnectHandler())
registry.Register(NewDataInHandler())
registry.Register(NewForwardHandler())
registry.Register(NewDataOutHandler())
```

### 2. 创建执行器
```go
// 创建执行器
executor := NewCommandExecutor(registry)

// 添加中间件
executor.AddMiddleware(&LoggingMiddleware{})
executor.AddMiddleware(NewMetricsMiddleware(metricsCollector))
executor.AddMiddleware(NewRetryMiddleware(3, backoff, retryable))
```

### 3. 集成到Session
```go
// 在SessionManager中集成
type SessionManager struct {
    // ... 其他字段
    commandExecutor *command.CommandExecutor
}

// 在handleCommandPacket中使用
func (s *SessionManager) handleCommandPacket(connPacket *StreamPacket) error {
    return s.commandExecutor.Execute(connPacket)
}
```

### 4. 客户端使用链式调用
```go
// 创建命令工具
utils := NewCommandUtils(session)

// TCP映射命令
var tcpResponse map[string]interface{}
response, err := utils
    .WithConnectionID("conn-123")
    .TcpMap()
    .PutRequest(map[string]interface{}{
        "local_port":  8080,
        "remote_host": "example.com",
        "remote_port": 80,
    })
    .ResultAs(&tcpResponse)
    .Timeout(10 * time.Second)
    .Execute()

// HTTP映射命令
var httpResponse map[string]interface{}
response, err = utils
    .WithConnectionID("conn-123")
    .HttpMap()
    .PutRequest(map[string]interface{}{
        "local_port":  8081,
        "remote_host": "api.example.com",
        "remote_port": 443,
    })
    .ResultAs(&httpResponse)
    .Execute()

// 断开连接命令（单向）
response, err = utils
    .WithConnectionID("conn-123")
    .Disconnect()
    .PutRequest(map[string]interface{}{
        "reason": "user_request",
    })
    .Execute()
```

## 中间件使用

### 1. 日志中间件
```go
executor.AddMiddleware(&LoggingMiddleware{})
```

### 2. 指标中间件
```go
metricsCollector := &MyMetricsCollector{}
executor.AddMiddleware(NewMetricsMiddleware(metricsCollector))
```

### 3. 重试中间件
```go
backoff := NewExponentialBackoff(100*time.Millisecond, 5*time.Second)
retryable := func(err error) bool {
    // 定义可重试的错误类型
    return strings.Contains(err.Error(), "network")
}
executor.AddMiddleware(NewRetryMiddleware(3, backoff, retryable))
```

### 4. 超时中间件
```go
executor.AddMiddleware(NewTimeoutMiddleware(30 * time.Second))
```

## 自定义命令处理器

### 1. 继承BaseHandler
```go
type MyCustomHandler struct {
    *BaseHandler
}

func NewMyCustomHandler() *MyCustomHandler {
    return &MyCustomHandler{
        BaseHandler: NewBaseHandler(packet.TcpMap, Duplex),
    }
}
```

### 2. 实现Handle方法
```go
func (h *MyCustomHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
    // 解析请求数据
    var requestData map[string]interface{}
    if err := json.Unmarshal([]byte(ctx.RequestBody), &requestData); err != nil {
        return &CommandResponse{
            Success: false,
            Error:   fmt.Sprintf("failed to parse request: %v", err),
        }, nil
    }
    
    // 实现具体业务逻辑
    // ...
    
    // 返回响应
    return &CommandResponse{
        Success: true,
        Data: map[string]interface{}{
            "result": "success",
        },
    }, nil
}
```

### 3. 注册处理器
```go
registry.Register(NewMyCustomHandler())
```

## 响应类型

### Oneway (单向)
- 不等待响应
- 适用于通知类命令
- 如：断开连接、数据输入输出

### Duplex (双工)
- 需要等待响应
- 适用于需要确认的命令
- 如：创建映射、转发

## 错误处理

### 1. 框架级错误
- 参数验证错误
- 网络传输错误
- 超时错误

### 2. 业务级错误
- 业务逻辑错误
- 数据验证错误
- 权限错误

### 3. 错误处理策略
- 重试机制
- 降级策略
- 错误上报

## 性能优化

### 1. 对象池
- 复用CommandUtils实例
- 减少GC压力
- 提高并发性能

### 2. 连接复用
- 复用底层连接
- 减少连接建立开销
- 支持连接池管理

### 3. 缓存机制
- 缓存常用配置
- 缓存响应数据
- 减少重复计算

## 监控和调试

### 1. 指标收集
- 命令执行时间
- 成功率统计
- 错误类型分布

### 2. 链路追踪
- 请求ID传递
- 调用链追踪
- 性能分析

### 3. 日志记录
- 结构化日志
- 不同级别日志
- 上下文信息

## 最佳实践

### 1. 处理器设计
- 单一职责原则
- 错误处理完善
- 日志记录详细

### 2. 中间件使用
- 按需添加中间件
- 注意中间件顺序
- 避免性能瓶颈

### 3. 错误处理
- 区分错误类型
- 提供有意义的错误信息
- 实现优雅降级

### 4. 性能优化
- 合理使用对象池
- 避免不必要的序列化
- 优化网络传输

## 扩展点

### 1. 自定义中间件
实现Middleware接口，添加自定义逻辑

### 2. 自定义指标收集器
实现MetricsCollector接口，集成监控系统

### 3. 自定义退避策略
实现BackoffStrategy接口，自定义重试策略

### 4. 自定义错误处理
实现错误处理函数，自定义错误处理逻辑 