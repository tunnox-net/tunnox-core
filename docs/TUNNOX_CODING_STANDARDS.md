# Tunnox 编码规范

> **版本**: v1.0  
> **最后更新**: 2025-01-XX  
> **维护者**: 架构团队

---

## 📖 目录

1. [概述](#概述)
2. [文件组织与命名](#文件组织与命名)
3. [代码结构与分层](#代码结构与分层)
4. [命名规范](#命名规范)
5. [错误处理](#错误处理)
6. [资源管理](#资源管理)
7. [Context 使用规范](#context-使用规范)
8. [Goroutine 管理](#goroutine-管理)
9. [日志规范](#日志规范)
10. [测试规范](#测试规范)
11. [代码质量要求](#代码质量要求)
12. [最佳实践](#最佳实践)

---

## 概述

本文档定义了 Tunnox Core 项目的编码规范，旨在确保代码库的一致性、可维护性和高质量。所有开发人员必须遵循本规范。

### 核心原则

1. **一致性**: 统一的命名、结构和风格
2. **可维护性**: 清晰的职责划分和模块化设计
3. **可靠性**: 完善的错误处理和资源管理
4. **可测试性**: 良好的测试覆盖和可测试设计
5. **性能**: 高效的资源使用和并发处理

---

## 文件组织与命名

### 目录结构

项目遵循标准的 Go 项目布局：

```
tunnox-core/
├── cmd/                      # 应用入口
│   ├── server/              # 服务端入口
│   └── client/              # 客户端入口
├── internal/                # 内部实现（不对外暴露）
│   ├── api/                 # Management API
│   ├── app/                 # 应用层
│   ├── bridge/              # 集群通信
│   ├── broker/              # 消息广播
│   ├── client/              # 客户端实现
│   ├── cloud/               # 云控管理
│   ├── command/             # 命令处理
│   ├── core/                # 核心组件
│   │   ├── dispose/         # 资源管理
│   │   ├── errors/          # 错误处理
│   │   ├── storage/         # 存储抽象
│   │   └── ...
│   ├── protocol/            # 协议层
│   ├── stream/              # 流处理
│   └── utils/               # 工具函数
├── docs/                    # 文档
└── tests/                   # 测试
```

### 文件命名规范

#### 1. 普通文件

- **格式**: `snake_case.go`
- **示例**: `connection_manager.go`, `stream_processor.go`, `tunnel_bridge.go`
- **说明**: 使用小写字母和下划线，多个单词用下划线分隔

#### 2. 接口文件

- **格式**: `{name}_interface.go`
- **示例**: `connection_interface.go`, `tunnel_bridge_interface.go`
- **说明**: 接口定义文件使用 `_interface.go` 后缀

#### 3. 测试文件

- **格式**: `{name}_test.go`
- **示例**: `connection_manager_test.go`, `stream_processor_test.go`
- **说明**: 测试文件必须与被测试文件在同一目录

#### 4. 文件大小限制

- **单个文件不超过 500 行**
- 超过限制应拆分为多个文件，按功能模块划分
- 示例：`security/` 包按功能拆分为 `brute_force.go`, `ip_manager.go`, `token.go` 等

### 包命名规范

- **格式**: 小写字母，单数形式
- **示例**: `session`, `storage`, `errors`, `dispose`
- **说明**: 
  - 使用小写字母
  - 使用单数形式（`session` 而非 `sessions`）
  - 避免缩写，除非是广泛认知的（如 `api`, `http`, `tcp`）

---

## 代码结构与分层

### 架构分层

项目采用分层架构，各层职责明确：

```
┌─────────────────────────────────────┐
│  API 层 (internal/api/)              │  HTTP REST API
├─────────────────────────────────────┤
│  应用层 (internal/app/)               │  应用启动、配置、服务管理
├─────────────────────────────────────┤
│  业务层 (internal/cloud/)            │  业务逻辑、服务、管理器、仓库
├─────────────────────────────────────┤
│  协议层 (internal/protocol/)        │  协议适配、会话管理
├─────────────────────────────────────┤
│  流处理层 (internal/stream/)        │  压缩、加密、转换
├─────────────────────────────────────┤
│  核心层 (internal/core/)             │  存储、错误、资源管理、ID生成
└─────────────────────────────────────┘
```

### 职责划分

#### API 层 (`internal/api/`)
- **职责**: HTTP REST API 处理
- **包含**: 请求处理、响应格式化、路由管理
- **不包含**: 业务逻辑（应委托给业务层）

#### 应用层 (`internal/app/`)
- **职责**: 应用启动、配置管理、服务生命周期
- **包含**: 服务器启动、服务注册、优雅关闭
- **不包含**: 具体业务逻辑

#### 业务层 (`internal/cloud/`)
- **职责**: 业务逻辑实现
- **结构**:
  - `services/`: 业务服务（如 `UserService`, `MappingService`）
  - `managers/`: 业务管理器（如 `ConfigManager`, `AuthManager`）
  - `repos/`: 数据仓库（如 `UserRepository`, `MappingRepository`）
  - `models/`: 数据模型

#### 协议层 (`internal/protocol/`)
- **职责**: 协议适配和会话管理
- **结构**:
  - `adapter/`: 协议适配器（TCP, WebSocket, UDP, QUIC）
  - `session/`: 会话管理
  - `httppoll/`: HTTP 长轮询实现

#### 核心层 (`internal/core/`)
- **职责**: 核心基础设施
- **包含**:
  - `storage/`: 存储抽象
  - `errors/`: 错误处理
  - `dispose/`: 资源管理
  - `idgen/`: ID 生成

### 依赖方向

- **上层依赖下层，下层不依赖上层**
- **同层之间避免循环依赖**
- **使用接口解耦，避免直接依赖实现**

---

## 命名规范

### 接口命名

#### 标准接口
- **格式**: `I{Name}`
- **示例**: `IConnection`, `IStreamProcessor`, `IManager`
- **说明**: 所有接口使用 `I` 前缀

#### 访问器接口
- **格式**: `I{Name}Accessor`
- **示例**: `IConnectionAccessor`, `IStreamProcessorAccessor`
- **说明**: 访问器接口使用 `Accessor` 后缀

**示例**:
```go
// 标准接口
type IConnection interface {
    GetID() string
    Close() error
}

// 访问器接口
type IConnectionAccessor interface {
    GetConnection() IConnection
}
```

### 实现类命名

#### 默认实现
- **格式**: `Default{Name}`
- **示例**: `DefaultStreamProcessor`, `DefaultConnection`
- **说明**: 默认实现使用 `Default` 前缀

#### 具体实现
- **格式**: `{Protocol/Type}{Name}`
- **示例**: `TCPConnection`, `HTTPPollStreamProcessor`, `RedisBroker`
- **说明**: 具体实现使用协议/类型前缀

#### 客户端/服务端实现
- **客户端**: `Client{Name}` 或 `{Protocol}Client{Name}`
- **服务端**: `Server{Name}` 或 `{Protocol}Server{Name}`
- **示例**: `ClientStreamProcessor`, `HTTPPollServerStreamProcessor`

**示例**:
```go
// 默认实现
type DefaultStreamProcessor struct {
    // ...
}

// 具体实现
type HTTPPollClientStreamProcessor struct {
    // ...
}

type HTTPPollServerStreamProcessor struct {
    // ...
}
```

### 类型命名

- **格式**: `PascalCase`
- **示例**: `ConnectionManager`, `StreamProcessor`, `TunnelBridge`
- **说明**: 使用大驼峰命名

### 方法命名

- **格式**: `PascalCase`
- **示例**: `GetConnection()`, `CreateMapping()`, `Close()`
- **说明**: 
  - Get/Set 用于简单访问
  - Create/New 用于创建
  - Close/Dispose/Stop 统一为 `Close()`
  - 避免在方法名中包含类型信息：`GetControlConnection()` 而非 `GetControlConnectionInterface()`

### 变量命名

- **格式**: `camelCase`
- **示例**: `connectionID`, `clientID`, `mappingID`
- **说明**: 
  - 使用小驼峰命名
  - ID 统一使用 `ID` 而非 `Id` 或 `id`
  - 统一缩写规则：`connID` vs `connectionID` → 统一使用 `connectionID`

### 常量命名

- **格式**: `UPPER_SNAKE_CASE`
- **示例**: `MAX_RETRIES`, `DEFAULT_TIMEOUT`, `BLOCK_DURATION`
- **说明**: 使用大写下划线分隔

### 包级别变量命名

- **格式**: `camelCase`（私有）或 `PascalCase`（公开）
- **示例**: `defaultConfig`, `GlobalResourceFactory`
- **说明**: 遵循 Go 的可见性规则

---

## 错误处理

### 统一使用 TypedError

**所有错误必须使用 `TypedError`**（`internal/core/errors/typed_error.go`），禁止使用 `fmt.Errorf`、`StandardError` 或其他错误处理方式。

### 导入规范

```go
import (
    coreErrors "tunnox-core/internal/core/errors"
)
```

### 错误类型映射

| 错误场景 | 错误类型 | 可重试 | 需告警 |
|---------|---------|--------|--------|
| 网络连接失败 | `ErrorTypeNetwork` | ✅ | ❌ |
| 存储操作失败 | `ErrorTypeStorage` | ✅ | ✅ |
| 协议解析错误 | `ErrorTypeProtocol` | ❌ | ✅ |
| 认证失败 | `ErrorTypeAuth` | ❌ | ✅ |
| 配置错误 | `ErrorTypePermanent` | ❌ | ❌ |
| 临时错误 | `ErrorTypeTemporary` | ✅ | ❌ |
| 致命错误 | `ErrorTypeFatal` | ❌ | ✅ |

### 错误创建规范

#### 1. 包装现有错误

```go
// ✅ 正确
return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to connect")

// ✅ 正确（格式化）
return coreErrors.Wrapf(err, coreErrors.ErrorTypeStorage, "failed to set key %s", key)

// ❌ 错误
return fmt.Errorf("failed to connect: %w", err)
```

#### 2. 创建新错误

```go
// ✅ 正确
return coreErrors.New(coreErrors.ErrorTypePermanent, "config is required")

// ✅ 正确（格式化）
return coreErrors.Newf(coreErrors.ErrorTypePermanent, "invalid port: %d", port)

// ❌ 错误
return fmt.Errorf("config is required")
```

#### 3. 使用 Sentinel Errors

对于预定义的错误，使用 Sentinel Errors：

```go
// ✅ 正确
if errors.Is(err, coreErrors.ErrKeyNotFound) {
    // 处理键不存在的情况
}

// ✅ 正确
if errors.Is(err, coreErrors.ErrConnectionCodeExpired) {
    // 处理连接码过期的情况
}
```

### 错误处理最佳实践

1. **及时处理错误**: 不要忽略错误，必须处理或向上传播
2. **添加上下文**: 使用 `Wrap` 或 `Wrapf` 添加有意义的错误信息
3. **选择合适的错误类型**: 根据错误性质选择正确的 `ErrorType`
4. **检查可重试性**: 使用 `coreErrors.IsRetryable()` 判断是否可重试
5. **检查需告警性**: 使用 `coreErrors.IsAlertable()` 判断是否需要告警

**示例**:
```go
func (s *Storage) Get(key string) (string, error) {
    value, err := s.backend.Get(key)
    if err != nil {
        // 包装错误，添加上下文
        return "", coreErrors.Wrapf(err, coreErrors.ErrorTypeStorage, "failed to get key %s", key)
    }
    return value, nil
}

func (m *Manager) Process() error {
    err := m.storage.Get("key")
    if err != nil {
        // 检查是否可重试
        if coreErrors.IsRetryable(err) {
            // 重试逻辑
            return m.retry()
        }
        // 检查是否需要告警
        if coreErrors.IsAlertable(err) {
            m.alert(err)
        }
        return err
    }
    return nil
}
```

---

## 资源管理

### 统一使用 ResourceBase 体系

**所有需要资源管理的组件必须使用 `ResourceBase`、`ManagerBase` 或 `ServiceBase`**，禁止直接嵌入 `Dispose` 或调用 `SetCtx()`。

### 基类选择

| 组件类型 | 基类 | 说明 |
|---------|------|------|
| 管理器类 | `ManagerBase` | SessionManager, ProtocolManager 等 |
| 服务类 | `ServiceBase` | EventBus, Storage 等 |
| 传输层 | `ResourceBase` | Transport, Conn 等 |
| 工具类 | `ResourceBase` | RateLimiter, Compression 等 |

### 初始化规范

#### 1. 使用 Initialize 方法

```go
// ✅ 正确
type MyManager struct {
    *dispose.ManagerBase
    // ...
}

func NewMyManager(parentCtx context.Context) *MyManager {
    manager := &MyManager{
        ManagerBase: dispose.NewManager("MyManager", parentCtx),
        // ...
    }
    return manager
}

// ❌ 错误（旧方式）
type MyManager struct {
    dispose.Dispose
    // ...
}

func NewMyManager() *MyManager {
    manager := &MyManager{}
    manager.SetCtx(context.Background(), manager.onClose) // ❌ 禁止
    return manager
}
```

#### 2. 设置资源名称

```go
// ✅ 正确
manager := dispose.NewManager("SessionManager", parentCtx)

// ✅ 正确（自定义名称）
resource := dispose.NewResourceBase("CustomResource")
resource.Initialize(parentCtx)
```

### 关闭规范

#### 1. 统一使用 Close 方法

```go
// ✅ 正确
if err := resource.Close(); err != nil {
    utils.Errorf("Failed to close resource: %v", err)
}

// ❌ 错误（旧方式）
result := resource.Dispose.Close() // ❌ 禁止
```

#### 2. 实现清理逻辑

```go
type MyResource struct {
    *dispose.ResourceBase
    ticker *time.Ticker
}

func (r *MyResource) onClose() error {
    // 停止定时器
    if r.ticker != nil {
        r.ticker.Stop()
    }
    // 其他清理逻辑
    return nil
}
```

### 资源清理检查清单

- [ ] 所有 `time.Ticker` 都有 `defer ticker.Stop()` 或在 `onClose()` 中停止
- [ ] 所有 `time.Timer` 都有 `defer timer.Stop()` 或在 `onClose()` 中停止
- [ ] 所有 channel 在适当时候关闭
- [ ] 所有文件句柄都有 `defer Close()`
- [ ] 所有网络连接都有清理逻辑
- [ ] 所有 goroutine 都能正确退出（见 [Goroutine 管理](#goroutine-管理)）

---

## Context 使用规范

### 禁止使用 context.Background()

**禁止在业务代码中使用 `context.Background()`**，除非是在以下场景：

1. **main 函数入口**: 应用启动时创建根 context
2. **测试代码**: 测试中创建测试 context
3. **全局资源清理**: 全局资源清理的超时控制（需添加注释说明）

### Context 传递规范

#### 1. 从父 Context 派生

```go
// ✅ 正确
func NewMyService(parentCtx context.Context) *MyService {
    service := &MyService{
        ServiceBase: dispose.NewService("MyService", parentCtx),
        // ...
    }
    return service
}

// ✅ 正确（创建子 context）
func (s *MyService) DoWork() error {
    ctx, cancel := context.WithTimeout(s.Ctx(), 30*time.Second)
    defer cancel()
    // 使用 ctx
    return nil
}

// ❌ 错误
func (s *MyService) DoWork() error {
    ctx := context.Background() // ❌ 禁止
    // ...
}
```

#### 2. 通过参数传递

```go
// ✅ 正确
func (r *Repository) Get(ctx context.Context, key string) (string, error) {
    // 使用传入的 ctx
    return r.backend.Get(ctx, key)
}

// ❌ 错误
func (r *Repository) Get(key string) (string, error) {
    ctx := context.Background() // ❌ 禁止
    return r.backend.Get(ctx, key)
}
```

#### 3. 使用 ResourceBase 的 Context

```go
// ✅ 正确
func (r *MyResource) DoWork() error {
    ctx := r.Ctx() // 使用 ResourceBase 的 context
    // ...
}

// ❌ 错误
func (r *MyResource) DoWork() error {
    ctx := context.Background() // ❌ 禁止
    // ...
}
```

### Context 使用最佳实践

1. **始终传递 Context**: 所有可能长时间运行的操作都应接受 `context.Context`
2. **检查取消信号**: 在循环和长时间操作中检查 `ctx.Done()`
3. **设置超时**: 使用 `context.WithTimeout()` 为操作设置超时
4. **避免 fallback**: 不要使用 `context.Background()` 作为 fallback，应返回错误

**示例**:
```go
func (s *Service) Process(ctx context.Context) error {
    // 检查取消信号
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
    
    // 长时间操作
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case data := <-s.dataCh:
            // 处理数据
        }
    }
}
```

---

## Goroutine 管理

### 必须检查退出条件

**所有 goroutine 必须检查退出条件**，确保能够正确退出，避免泄漏。

### 退出条件检查

#### 1. 使用 Context

```go
// ✅ 正确
go func() {
    for {
        select {
        case <-ctx.Done():
            return // 退出 goroutine
        case data := <-ch:
            // 处理数据
        }
    }
}()

// ❌ 错误
go func() {
    for {
        data := <-ch // ❌ 没有退出条件
        // 处理数据
    }
}()
```

#### 2. 使用 Channel

```go
// ✅ 正确
go func() {
    for {
        select {
        case <-closeCh:
            return // 退出 goroutine
        case data := <-dataCh:
            // 处理数据
        }
    }
}()

// 确保在适当时候关闭 channel
defer close(closeCh)
```

#### 3. 使用 sync.WaitGroup

```go
// ✅ 正确
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        // 工作逻辑
    }(i)
}
wg.Wait() // 等待所有 goroutine 完成
```

### Goroutine 清理检查清单

- [ ] 所有 goroutine 都检查 `ctx.Done()` 或 `closeCh`
- [ ] 使用 `sync.WaitGroup` 跟踪 goroutine（如需要）
- [ ] Channel 正确关闭
- [ ] 定时器正确停止
- [ ] 资源正确释放

### Goroutine 管理最佳实践

1. **使用 Context 控制生命周期**: 所有 goroutine 应接受 `context.Context`
2. **及时退出**: 在收到取消信号时立即退出
3. **清理资源**: 在 goroutine 退出前清理所有资源
4. **避免泄漏**: 确保所有 goroutine 都能正确退出

**示例**:
```go
type Worker struct {
    *dispose.ResourceBase
    dataCh  chan Data
    closeCh chan struct{}
    wg      sync.WaitGroup
}

func (w *Worker) Start() {
    w.wg.Add(1)
    go w.run()
}

func (w *Worker) run() {
    defer w.wg.Done()
    for {
        select {
        case <-w.Ctx().Done():
            return // 退出
        case <-w.closeCh:
            return // 退出
        case data := <-w.dataCh:
            w.process(data)
        }
    }
}

func (w *Worker) onClose() error {
    close(w.closeCh) // 通知 goroutine 退出
    w.wg.Wait()      // 等待 goroutine 完成
    return nil
}
```

---

## 日志规范

### 日志级别

| 级别 | 使用场景 | 示例 |
|------|---------|------|
| `Debug` | 详细的调试信息，仅开发环境 | 函数调用、变量值、中间状态 |
| `Info` | 关键操作和状态变更 | 连接建立、配置更新、服务启动 |
| `Warn` | 可恢复的错误或异常情况 | 重试操作、降级处理 |
| `Error` | 需要关注的错误 | 操作失败、资源不足 |
| `Fatal` | 致命错误，程序无法继续 | 初始化失败、关键资源不可用 |

### 日志使用规范

#### 1. 选择合适的级别

```go
// ✅ 正确
utils.Infof("Client connecting to server %s", address)
utils.Errorf("Failed to close connection: %v", err)

// ❌ 错误
utils.Debugf("Client connecting to server %s", address) // 应该是 Info
utils.Warnf("Failed to close connection: %v", err)     // 应该是 Error
```

#### 2. 使用结构化日志

```go
// ✅ 正确（使用格式化）
utils.Infof("Connection established: clientID=%s, serverID=%s", clientID, serverID)

// ❌ 错误（字符串拼接）
utils.Info("Connection established: clientID=" + clientID + ", serverID=" + serverID)
```

#### 3. 避免在高频路径使用 Debug

```go
// ❌ 错误（高频路径）
func (p *Processor) Process(data []byte) {
    utils.Debugf("Processing data: %v", data) // 高频调用，影响性能
    // ...
}

// ✅ 正确
func (p *Processor) Process(data []byte) {
    // 移除 Debug 日志，或使用采样
    // ...
}
```

### 日志最佳实践

1. **关键操作使用 Info**: 连接建立、配置更新、服务启动等
2. **错误使用 Error**: 所有错误情况应使用 Error 级别
3. **避免敏感信息**: 不要在日志中输出密码、令牌等敏感信息
4. **提供上下文**: 日志应包含足够的上下文信息（ID、状态等）
5. **性能考虑**: 避免在高频路径使用 Debug 日志

---

## 测试规范

### 测试文件命名

- **格式**: `{name}_test.go`
- **示例**: `connection_manager_test.go`, `stream_processor_test.go`
- **说明**: 测试文件必须与被测试文件在同一目录

### 测试函数命名

- **格式**: `Test{Name}` 或 `Test{Name}_{Scenario}`
- **示例**: `TestConnectionManager`, `TestConnectionManager_CreateConnection`, `TestConnectionManager_Close`
- **说明**: 使用描述性的测试名称

### 测试覆盖率要求

| 层级 | 覆盖率目标 |
|------|-----------|
| 核心业务逻辑 | 80%+ |
| API 层 | 85%+ |
| 工具类 | 70%+ |

### 测试类型

#### 1. 单元测试

```go
func TestConnectionManager_CreateConnection(t *testing.T) {
    // 准备
    manager := NewConnectionManager(ctx)
    
    // 执行
    conn, err := manager.CreateConnection("client1")
    
    // 验证
    assert.NoError(t, err)
    assert.NotNil(t, conn)
}
```

#### 2. 集成测试

```go
func TestIntegration_ConnectionFlow(t *testing.T) {
    // 测试完整的连接流程
    // ...
}
```

#### 3. 并发测试

```go
func TestConnectionManager_ConcurrentAccess(t *testing.T) {
    // 使用 go test -race 运行
    // ...
}
```

### 测试最佳实践

1. **测试覆盖**: 每个功能至少包含正常流程、边界条件、错误处理测试
2. **使用表驱动测试**: 对于多个相似测试用例，使用表驱动测试
3. **清理资源**: 测试后清理所有资源
4. **并发安全**: 使用 `go test -race` 验证并发安全
5. **Mock 依赖**: 使用接口和 Mock 隔离依赖

---

## 代码质量要求

### 函数长度

- **单个函数不超过 50 行**
- 超过限制应拆分为多个函数
- 复杂逻辑应提取为独立函数

### 文件长度

- **单个文件不超过 500 行**
- 超过限制应拆分为多个文件，按功能模块划分

### 类型安全

- **禁止使用 `map[string]interface{}`、`interface{}`、`any`**
- **使用明确的结构体类型**
- **示例**: 使用 `map[string]*AttemptRecord` 而非 `map[string]interface{}`

### 魔法数字和字符串

- **所有魔法数字提取为常量**
- **所有魔法字符串提取为常量**
- **可配置的值放入配置，不可配置的值使用常量**

```go
// ❌ 错误
timeout := 30 * time.Second
maxRetries := 3

// ✅ 正确
const (
    DefaultTimeout  = 30 * time.Second
    MaxRetries     = 3
)

timeout := DefaultTimeout
maxRetries := MaxRetries
```

### 代码重复

- **避免代码重复**
- **提取公共函数**: 错误处理、配置读取、资源清理等
- **使用组合而非继承**: 通过组合复用代码

### 并发安全

- **使用 Mutex/RWMutex 保护共享状态**
- **使用 `go test -race` 验证并发安全**
- **避免 data race**: 确保所有共享状态的访问都受到保护

---

## 最佳实践

### 1. 接口设计

- **单一职责**: 每个接口只负责一个职责
- **小接口**: 接口应尽可能小，只包含必要的方法
- **使用组合**: 通过组合小接口构建大接口

### 2. 错误处理

- **及时处理**: 不要忽略错误
- **添加上下文**: 使用 `Wrap` 或 `Wrapf` 添加错误上下文
- **选择合适的错误类型**: 根据错误性质选择正确的 `ErrorType`

### 3. 资源管理

- **使用 ResourceBase 体系**: 统一使用 `ResourceBase`、`ManagerBase` 或 `ServiceBase`
- **及时清理**: 确保所有资源都能正确清理
- **避免泄漏**: 检查所有资源是否都能正确释放

### 4. 并发处理

- **使用 Context 控制生命周期**: 所有 goroutine 应接受 `context.Context`
- **检查退出条件**: 确保所有 goroutine 都能正确退出
- **并发安全**: 使用锁保护共享状态，使用 `go test -race` 验证

### 5. 性能优化

- **避免不必要的分配**: 重用对象，使用对象池
- **优化网络 I/O**: 使用连接池、批量操作
- **减少锁竞争**: 使用读写锁、分段锁

### 6. 代码审查

- **遵循本规范**: 所有代码必须遵循本编码规范
- **代码审查**: 提交前进行代码审查
- **持续改进**: 根据实践不断改进规范

---

## 附录

### A. 常见问题

#### Q1: 什么时候使用 `ManagerBase` vs `ServiceBase` vs `ResourceBase`?

- **ManagerBase**: 管理器类，管理多个资源（如 SessionManager）
- **ServiceBase**: 服务类，提供业务服务（如 EventBus, Storage）
- **ResourceBase**: 通用资源，其他情况使用

#### Q2: 什么时候可以使用 `context.Background()`?

仅在以下场景：
1. main 函数入口
2. 测试代码
3. 全局资源清理的超时控制（需添加注释说明）

#### Q3: 如何判断错误类型?

根据错误性质：
- 网络错误 → `ErrorTypeNetwork`
- 存储错误 → `ErrorTypeStorage`
- 协议错误 → `ErrorTypeProtocol`
- 认证错误 → `ErrorTypeAuth`
- 配置错误 → `ErrorTypePermanent`
- 临时错误 → `ErrorTypeTemporary`
- 致命错误 → `ErrorTypeFatal`

### B. 参考文档

- [错误处理迁移指南](ERROR_HANDLING_MIGRATION.md)
- [P0 任务审查](P0_TASKS_REVIEW.md)
- [代码审查报告](CODE_REVIEW_COMPREHENSIVE.md)
- [命名一致性改进](NAMING_CONSISTENCY_IMPROVEMENT.md)
- [架构设计文档](ARCHITECTURE_DESIGN_V2.2.md)

### C. 工具和检查

#### 代码检查工具

```bash
# 运行测试
go test ./...

# 检查并发安全
go test -race ./...

# 检查测试覆盖率
go test -cover ./...

# 静态分析
go vet ./...

# 格式化代码
go fmt ./...
```

#### 代码审查清单

- [ ] 遵循命名规范
- [ ] 使用 TypedError 处理错误
- [ ] 使用 ResourceBase 体系管理资源
- [ ] 正确使用 Context
- [ ] Goroutine 能正确退出
- [ ] 日志级别合适
- [ ] 测试覆盖充分
- [ ] 代码长度符合要求
- [ ] 无魔法数字/字符串
- [ ] 并发安全

---

**文档版本**: v1.0  
**最后更新**: 2025-01-XX  
**维护者**: 架构团队

