# CommandHandler 类型信息设计

## 概述

为了实现对命令体的统一处理，我们在 `CommandHandler` 接口中添加了运行时类型信息支持。这个设计通过泛型和反射技术，提供了类型安全的命令处理能力。

## 设计目标

1. **类型安全**: 在编译时和运行时都能保证类型安全
2. **统一处理**: 提供统一的命令体解析和响应创建机制
3. **向后兼容**: 保持与现有代码的兼容性
4. **性能优化**: 最小化反射操作对性能的影响

## 接口设计

### CommandHandler 接口扩展

```go
type CommandHandler interface {
    // 原有方法
    Handle(ctx *CommandContext) (*CommandResponse, error)
    GetDirection() CommandDirection
    GetCommandType() packet.CommandType
    GetCategory() CommandCategory
    
    // 新增方法
    GetRequestType() reflect.Type   // 获取请求类型
    GetResponseType() reflect.Type  // 获取响应类型
}
```

### BaseCommandHandler 泛型实现

```go
type BaseCommandHandler[TRequest any, TResponse any] struct {
    // ... 字段
}

// 自动提供类型信息
func (b *BaseCommandHandler[TRequest, TResponse]) GetRequestType() reflect.Type {
    var zero TRequest
    if reflect.TypeOf(zero) == reflect.TypeOf((*interface{})(nil)).Elem() {
        return nil // 无请求体
    }
    return reflect.TypeOf(zero)
}

func (b *BaseCommandHandler[TRequest, TResponse]) GetResponseType() reflect.Type {
    var zero TResponse
    if reflect.TypeOf(zero) == reflect.TypeOf((*interface{})(nil)).Elem() {
        return nil // 无响应体
    }
    return reflect.TypeOf(zero)
}
```

## 使用示例

### 1. 双工命令处理器（有请求和响应）

```go
type ConnectRequest struct {
    ClientID   int64  `json:"client_id"`
    ClientName string `json:"client_name"`
    Protocol   string `json:"protocol"`
}

type ConnectResponse struct {
    Success    bool   `json:"success"`
    SessionID  string `json:"session_id"`
    ServerTime int64  `json:"server_time"`
}

type ConnectHandler struct {
    *BaseCommandHandler[ConnectRequest, ConnectResponse]
}

func NewConnectHandler() *ConnectHandler {
    base := NewBaseCommandHandler[ConnectRequest, ConnectResponse](
        packet.Connect,
        DirectionDuplex,
        DuplexMode,
    )
    return &ConnectHandler{BaseCommandHandler: base}
}
```

### 2. 单向命令处理器（有请求，无响应）

```go
type HeartbeatRequest struct {
    ClientID  int64 `json:"client_id"`
    Timestamp int64 `json:"timestamp"`
}

type HeartbeatHandler struct {
    *BaseCommandHandler[HeartbeatRequest, interface{}]
}

func NewHeartbeatHandler() *HeartbeatHandler {
    base := NewBaseCommandHandler[HeartbeatRequest, interface{}](
        packet.HeartbeatCmd,
        DirectionOneway,
        Simplex,
    )
    return &HeartbeatHandler{BaseCommandHandler: base}
}
```

### 3. 无请求体命令处理器

```go
type DisconnectHandler struct {
    *BaseCommandHandler[interface{}, interface{}]
}

func NewDisconnectHandler() *DisconnectHandler {
    base := NewBaseCommandHandler[interface{}, interface{}](
        packet.Disconnect,
        DirectionOneway,
        Simplex,
    )
    return &DisconnectHandler{BaseCommandHandler: base}
}
```

## 统一处理工具

### 1. 类型信息获取

```go
func GetHandlerTypeInfo(handler types.CommandHandler) {
    fmt.Printf("Command Type: %v\n", handler.GetCommandType())
    fmt.Printf("Direction: %v\n", handler.GetDirection())
    fmt.Printf("Category: %v\n", handler.GetCategory())
    fmt.Printf("Request Type: %v\n", handler.GetRequestType())
    fmt.Printf("Response Type: %v\n", handler.GetResponseType())
}
```

### 2. 统一的命令体处理

```go
func ProcessCommandBody(handler types.CommandHandler, requestBody string) (interface{}, error) {
    requestType := handler.GetRequestType()
    if requestType == nil {
        return nil, nil // 无请求体
    }
    
    // 使用反射创建请求实例
    requestValue := reflect.New(requestType)
    request := requestValue.Interface()
    
    // 解析JSON到请求实例
    if err := json.Unmarshal([]byte(requestBody), request); err != nil {
        return nil, fmt.Errorf("failed to parse request body: %w", err)
    }
    
    return request, nil
}
```

### 3. 类型安全的响应创建

```go
func CreateTypedResponse(handler types.CommandHandler, data interface{}) (*types.CommandResponse, error) {
    responseType := handler.GetResponseType()
    if responseType == nil {
        return &types.CommandResponse{Success: true}, nil
    }
    
    // 验证数据类型
    if data != nil && reflect.TypeOf(data) != responseType {
        return nil, fmt.Errorf("response data type mismatch: expected %v, got %T", responseType, data)
    }
    
    // 序列化响应数据
    var responseData string
    if data != nil {
        if jsonData, err := json.Marshal(data); err == nil {
            responseData = string(jsonData)
        } else {
            return nil, fmt.Errorf("failed to marshal response data: %w", err)
        }
    }
    
    return &types.CommandResponse{
        Success: true,
        Data:    responseData,
    }, nil
}
```

## 向后兼容性

为了保持向后兼容性，我们为现有的处理器提供了默认实现：

```go
// 在 BaseHandler 中
func (h *BaseHandler) GetRequestType() reflect.Type { return nil }
func (h *BaseHandler) GetResponseType() reflect.Type { return nil }
```

这样，现有的处理器可以继续工作，只是不会提供类型信息。

## 性能考虑

### 基准测试结果

```
BenchmarkGetHandlerTypeInfo-16          131667094                8.942 ns/op
BenchmarkProcessCommandBody-16           2024582               587.6 ns/op
```

- 类型信息获取：约9纳秒/操作
- 命令体处理：约588纳秒/操作（包含JSON解析）

### 性能优化建议

1. **缓存类型信息**: 对于频繁使用的处理器，可以缓存类型信息
2. **减少反射调用**: 在关键路径上避免不必要的反射操作
3. **使用泛型**: 尽可能使用泛型而不是反射来获得更好的性能

## 最佳实践

### 1. 处理器设计

```go
// 推荐：使用泛型设计
type MyHandler struct {
    *BaseCommandHandler[MyRequest, MyResponse]
}

// 不推荐：手动实现类型信息
type MyHandler struct {
    // ... 字段
}

func (h *MyHandler) GetRequestType() reflect.Type {
    return reflect.TypeOf(MyRequest{}) // 容易出错
}
```

### 2. 错误处理

```go
// 推荐：使用类型安全的错误处理
func (h *MyHandler) Handle(ctx *types.CommandContext) (*types.CommandResponse, error) {
    request, err := h.ParseRequest(ctx)
    if err != nil {
        return h.CreateErrorResponse(err, ctx.RequestID), nil
    }
    
    response, err := h.ProcessRequest(ctx, request)
    if err != nil {
        return h.CreateErrorResponse(err, ctx.RequestID), nil
    }
    
    return h.CreateSuccessResponse(response, ctx.RequestID), nil
}
```

### 3. 类型验证

```go
// 推荐：在处理器中验证类型
func (h *MyHandler) ValidateRequest(request *MyRequest) error {
    if request.ClientID <= 0 {
        return fmt.Errorf("invalid client ID: %d", request.ClientID)
    }
    return nil
}
```

## 迁移指南

### 从旧处理器迁移

1. **确定请求和响应类型**
2. **使用泛型重构**
3. **更新测试**
4. **验证功能**

```go
// 旧版本
type OldHandler struct {
    *BaseHandler
}

// 新版本
type NewHandler struct {
    *BaseCommandHandler[RequestType, ResponseType]
}
```

## 总结

这个设计通过以下方式实现了目标：

1. **类型安全**: 泛型确保编译时类型安全，反射提供运行时类型信息
2. **统一处理**: 提供了通用的命令体处理工具函数
3. **向后兼容**: 现有代码无需修改即可继续工作
4. **性能优化**: 最小化反射操作，提供良好的性能表现

这个设计为未来的命令处理系统提供了强大的类型安全基础，同时保持了代码的简洁性和可维护性。 