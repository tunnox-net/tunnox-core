# 架构重构设计文档

## Overview

本设计文档描述了 Tunnox 项目的架构重构方案，旨在解决当前存在的依赖倒置问题、提供简洁的 SDK 入口、降低代码复杂度，并改善并发管理的可维护性。

重构的核心目标是建立清晰的分层架构：
- **Kernel Layer（内核层）**：提供稳定的协议抽象、存储接口、流处理等核心基础设施
- **Control Plane（控制平面）**：提供 HTTP API、节点管理、客户端管理等业务功能

当前问题：
1. `internal/core/storage` 依赖 `internal/cloud/constants`，违反了依赖倒置原则
2. `cmd/server/main.go` 和 `cmd/client/main.go` 包含大量初始化逻辑，难以嵌入其他应用
3. 多个文件超过 500 行，职责混杂，难以维护
4. 长期运行的 goroutine 缺乏生命周期文档，难以调试并发问题

## Architecture

### 分层架构

```
┌─────────────────────────────────────────────────────────┐
│                    Application Layer                     │
│              (cmd/server, cmd/client)                    │
│                  - 参数解析                               │
│                  - 调用 SDK 入口                          │
└─────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────┐
│                      SDK Layer                           │
│         (internal/app/server, internal/app/client)       │
│                  - server.Run()                          │
│                  - client.Run()                          │
│                  - 生命周期管理                           │
└─────────────────────────────────────────────────────────┘
                            │
        ┌───────────────────┴───────────────────┐
        ▼                                       ▼
┌──────────────────────┐           ┌──────────────────────┐
│   Control Plane      │           │    Kernel Layer      │
│  (internal/cloud)    │──────────▶│   (internal/core)    │
│  - HTTP API          │           │   - Storage          │
│  - 业务逻辑          │           │   - Protocol         │
│  - 服务管理          │           │   - Stream           │
└──────────────────────┘           │   - Events           │
                                   └──────────────────────┘
                                            │
                                            ▼
                                   ┌──────────────────────┐
                                   │  Kernel Contract     │
                                   │ (internal/constants) │
                                   │  - Storage Keys      │
                                   │  - TTL Values        │
                                   │  - Event Names       │
                                   └──────────────────────┘
```

### 依赖流向规则

1. **单向依赖**：Control Plane → Kernel Layer → Kernel Contract
2. **禁止反向依赖**：Kernel Layer 不得导入 Control Plane 的任何包
3. **共享常量**：所有跨层常量必须定义在 `internal/constants`

## Components and Interfaces

### 1. Kernel Contract Package (`internal/constants`)

统一管理所有跨层常量，作为 Kernel Layer 和 Control Plane 之间的契约。

**文件组织：**
```
internal/constants/
├── constants.go        # 通用常量（网络、包相关）
├── storage_keys.go     # 存储键前缀
├── http.go            # HTTP 相关常量
├── log.go             # 日志相关常量
└── ttl.go             # TTL 默认值（新增）
```

**新增 `ttl.go`：**
```go
package constants

import "time"

// Storage TTL Constants
const (
    // DefaultDataTTL 默认数据过期时间（24小时）
    DefaultDataTTL = 24 * time.Hour
    
    // DefaultSessionTTL 默认会话过期时间（1小时）
    DefaultSessionTTL = 1 * time.Hour
    
    // DefaultCacheTTL 默认缓存过期时间（5分钟）
    DefaultCacheTTL = 5 * time.Minute
)
```

### 2. SDK Entry Points

#### Server SDK (`internal/app/server/sdk.go`)

```go
package server

import (
    "context"
    "fmt"
)

// ServerConfig SDK 配置
type ServerConfig struct {
    // 从现有 Config 提取核心字段
    ListenAddr    string
    StorageConfig StorageConfig
    LogLevel      string
    // ... 其他必要配置
}

// Run 启动服务器（阻塞直到 context 取消）
func Run(ctx context.Context, config *ServerConfig) error {
    // 1. 验证配置
    if err := validateConfig(config); err != nil {
        return fmt.Errorf("invalid config: %w", err)
    }
    
    // 2. 初始化核心组件
    server, err := newServer(ctx, config)
    if err != nil {
        return fmt.Errorf("failed to create server: %w", err)
    }
    
    // 3. 启动服务
    if err := server.start(); err != nil {
        return fmt.Errorf("failed to start server: %w", err)
    }
    
    // 4. 等待 context 取消
    <-ctx.Done()
    
    // 5. 优雅关闭
    return server.shutdown()
}
```

#### Client SDK (`internal/app/client/sdk.go`)

```go
package client

import (
    "context"
    "fmt"
)

// ClientConfig SDK 配置
type ClientConfig struct {
    ServerAddr string
    AuthToken  string
    Mappings   []MappingConfig
    // ... 其他必要配置
}

// Run 启动客户端（阻塞直到 context 取消）
func Run(ctx context.Context, config *ClientConfig) error {
    // 类似 server.Run 的结构
    // ...
}
```

### 3. 文件拆分策略

#### 3.1 `internal/app/server/config.go` (839 行)

**拆分方案：**
```
internal/app/server/
├── config.go              # 核心结构体定义（~150行）
├── config_defaults.go     # 默认值和常量（~100行）
├── config_validation.go   # 配置验证逻辑（~200行）
├── config_env.go          # 环境变量处理（保持现状）
└── config_loader.go       # 配置加载和合并（~200行）
```

**职责划分：**
- `config.go`: 只包含 `Config` 结构体定义和基本构造函数
- `config_defaults.go`: 所有默认值、常量定义
- `config_validation.go`: `Validate()` 方法和所有验证逻辑
- `config_loader.go`: 从文件/环境变量加载配置的逻辑

#### 3.2 `internal/protocol/session/packet_handler_tunnel.go` (600 行)

**拆分方案：**
```
internal/protocol/session/
├── packet_handler_tunnel.go          # 主处理器和路由（~150行）
├── packet_handler_tunnel_data.go     # 数据包处理（~150行）
├── packet_handler_tunnel_control.go  # 控制包处理（~150行）
└── packet_handler_tunnel_error.go    # 错误处理（~150行）
```

**职责划分：**
- `packet_handler_tunnel.go`: 包处理器接口和路由逻辑
- `packet_handler_tunnel_data.go`: 处理数据传输相关的包
- `packet_handler_tunnel_control.go`: 处理连接控制相关的包
- `packet_handler_tunnel_error.go`: 错误处理和恢复逻辑

#### 3.3 `internal/stream/stream_processor.go` (567 行)

**拆分方案：**
```
internal/stream/
├── stream_processor.go           # 核心接口和结构体（~150行）
├── stream_processor_init.go      # 初始化和配置（~150行）
├── stream_processor_transform.go # 数据转换逻辑（~150行）
└── stream_processor_lifecycle.go # 生命周期管理（~117行）
```

**职责划分：**
- `stream_processor.go`: `StreamProcessor` 接口和基本结构
- `stream_processor_init.go`: 创建、初始化、配置相关
- `stream_processor_transform.go`: 数据压缩、加密、转换逻辑
- `stream_processor_lifecycle.go`: 启动、停止、清理逻辑

#### 3.4 `internal/cloud/services/service_registry.go` (626 行)

**拆分方案：**
```
internal/cloud/services/
├── service_registry.go           # 注册表接口和结构（~150行）
├── service_registry_ops.go       # 注册/注销操作（~150行）
├── service_registry_lifecycle.go # 服务生命周期（~150行）
└── service_registry_di.go        # 依赖注入逻辑（~176行）
```

**职责划分：**
- `service_registry.go`: `ServiceRegistry` 接口和核心结构
- `service_registry_ops.go`: 服务注册、查找、注销操作
- `service_registry_lifecycle.go`: 服务启动、停止、健康检查
- `service_registry_di.go`: 依赖解析和注入逻辑

### 4. Goroutine 生命周期管理

#### 生命周期模式

所有长期运行的 goroutine 必须遵循以下模式：

```go
// 标准 goroutine 启动模式
func (s *Service) startWorker(ctx context.Context) {
    // Lifecycle: Managed by service context
    // Cleanup: Triggered by context cancellation
    // Shutdown: Waits for work completion before exit
    go func() {
        defer s.wg.Done() // 确保 WaitGroup 计数
        
        for {
            select {
            case <-ctx.Done():
                // 清理逻辑
                s.cleanup()
                return
            case work := <-s.workChan:
                // 处理工作
                s.process(work)
            }
        }
    }()
}
```

#### 文档化要求

每个 goroutine 启动点必须包含以下注释：

```go
// Lifecycle: <描述由哪个 context 控制>
// Cleanup: <描述清理触发方式>
// Shutdown: <描述关闭行为>
```

## Data Models

### 配置模型

#### ServerConfig (SDK)
```go
type ServerConfig struct {
    // 网络配置
    ListenAddr string
    TLSConfig  *TLSConfig
    
    // 存储配置
    Storage StorageConfig
    
    // 日志配置
    LogLevel  string
    LogOutput string
    
    // 性能配置
    MaxConnections int
    ReadTimeout    time.Duration
    WriteTimeout   time.Duration
}
```

#### ClientConfig (SDK)
```go
type ClientConfig struct {
    // 服务器配置
    ServerAddr string
    TLSConfig  *TLSConfig
    
    // 认证配置
    AuthToken string
    ClientID  string
    
    // 映射配置
    Mappings []MappingConfig
    
    // 重连配置
    ReconnectInterval time.Duration
    MaxReconnectAttempts int
}
```

### 常量迁移映射

| 当前位置 | 新位置 | 说明 |
|---------|--------|------|
| `internal/cloud/constants.DefaultDataTTL` | `internal/constants.DefaultDataTTL` | 存储默认 TTL |
| `internal/cloud/constants.DefaultSessionTTL` | `internal/constants.DefaultSessionTTL` | 会话默认 TTL |
| `internal/cloud/constants.*` (存储相关) | `internal/constants/storage_keys.go` | 所有存储键前缀 |

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system-essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*


### Property 1: Kernel layer import isolation
*For any* Go source file in the kernel layer (`internal/core/**`), the file's import statements should not reference any packages from the cloud layer (`internal/cloud/**`)
**Validates: Requirements 1.1**

### Property 2: Dependency graph acyclicity
*For any* pair of packages (kernel, cloud) where kernel is in `internal/core/**` and cloud is in `internal/cloud/**`, the import graph should show edges from cloud to kernel but never from kernel to cloud
**Validates: Requirements 1.2**

### Property 3: No circular dependencies between layers
*For any* analysis of the codebase dependency graph, there should be zero circular dependencies between kernel layer and cloud layer packages
**Validates: Requirements 1.4**

### Property 4: Storage key constants sourcing
*For any* usage of storage key constants in the codebase, the constant should be imported from `internal/constants/storage_keys.go`
**Validates: Requirements 2.1, 7.1**

### Property 5: TTL constants sourcing
*For any* usage of TTL default values in storage operations, the constant should be imported from `internal/constants` package
**Validates: Requirements 2.2**

### Property 6: Event name constants sourcing
*For any* usage of event name constants, the constant should be imported from `internal/constants` package
**Validates: Requirements 2.3**

### Property 7: Storage test isolation
*For any* test file for storage implementations (`internal/core/storage/**/*_test.go`), the test should not import any packages from the cloud layer
**Validates: Requirements 3.3**

### Property 8: SDK lifecycle management
*For any* call to `server.Run()` or `client.Run()` with a cancelable context, canceling the context should trigger graceful shutdown and cleanup of all resources
**Validates: Requirements 4.3**

### Property 9: Configuration validation error clarity
*For any* invalid configuration passed to SDK entry points, the returned error message should clearly indicate which configuration field is invalid and why
**Validates: Requirements 4.6**

### Property 10: File size constraint
*For any* file in the refactored modules (`internal/app/server/config*.go`, `internal/protocol/session/packet_handler_tunnel*.go`, `internal/stream/stream_processor*.go`, `internal/cloud/services/service_registry*.go`), the file should contain fewer than 500 lines of code
**Validates: Requirements 5.5**

### Property 11: Goroutine lifecycle documentation
*For any* goroutine launch in the codebase (identified by `go func()` or `go <function>`), the code should include a comment within 3 lines above documenting which context controls its lifetime
**Validates: Requirements 6.1**

### Property 12: Goroutine cleanup documentation
*For any* goroutine that performs cleanup operations, the code should include a comment documenting where and how the cleanup is triggered
**Validates: Requirements 6.2**

### Property 13: Goroutine termination timeout
*For any* context cancellation in the system, all goroutines controlled by that context should terminate within 30 seconds
**Validates: Requirements 6.5**

### Property 14: HTTP constants sourcing
*For any* usage of HTTP-related constants (status codes, headers, etc.), the constant should be imported from `internal/constants/http.go`
**Validates: Requirements 7.2**

### Property 15: Logging constants sourcing
*For any* usage of logging-related constants (log levels, formats, etc.), the constant should be imported from `internal/constants/log.go`
**Validates: Requirements 7.3**

### Property 16: Cloud constants isolation
*For any* constant defined in `internal/cloud/constants`, it should not be imported by any file in the kernel layer (`internal/core/**`)
**Validates: Requirements 7.4**

## Error Handling

### 配置验证错误

所有配置验证错误必须提供清晰的错误信息：

```go
type ConfigValidationError struct {
    Field   string // 字段名
    Value   interface{} // 无效值
    Reason  string // 失败原因
}

func (e *ConfigValidationError) Error() string {
    return fmt.Sprintf("invalid config field '%s' (value: %v): %s", 
        e.Field, e.Value, e.Reason)
}
```

### 依赖检查错误

在编译时或 CI 中检测依赖违规：

```go
type DependencyViolationError struct {
    SourcePackage string // 违规的源包
    TargetPackage string // 不应依赖的目标包
    FilePath      string // 违规文件路径
    LineNumber    int    // 违规行号
}
```

### SDK 启动错误

SDK 入口点应该返回结构化错误：

```go
// 初始化失败
ErrInitializationFailed = errors.New("failed to initialize server")

// 配置无效
ErrInvalidConfig = errors.New("invalid configuration")

// 启动失败
ErrStartupFailed = errors.New("failed to start server")
```

## Testing Strategy

### 单元测试

#### 配置验证测试
- 测试各种无效配置组合
- 验证错误消息的清晰度
- 测试默认值应用

#### 常量迁移测试
- 验证所有存储操作使用正确的常量源
- 验证 TTL 值来自 `internal/constants`

#### 文件拆分测试
- 验证拆分后的文件可以正常编译
- 验证功能完整性未受影响

### 属性测试

本项目使用 **Go 标准库的 testing/quick** 包进行属性测试。

#### 测试配置
- 每个属性测试至少运行 **100 次迭代**
- 使用 `quick.Config{MaxCount: 100}` 配置

#### 属性测试标注格式
每个属性测试必须使用以下格式标注：

```go
// **Feature: dependency-inversion-refactor, Property 1: Kernel layer import isolation**
func TestProperty_KernelLayerImportIsolation(t *testing.T) {
    // 测试实现
}
```

### 集成测试

#### SDK 生命周期测试
```go
func TestServerSDK_Lifecycle(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    
    // 启动服务器
    errChan := make(chan error, 1)
    go func() {
        errChan <- server.Run(ctx, testConfig)
    }()
    
    // 等待启动
    time.Sleep(100 * time.Millisecond)
    
    // 取消 context
    cancel()
    
    // 验证优雅关闭
    select {
    case err := <-errChan:
        assert.NoError(t, err)
    case <-time.After(5 * time.Second):
        t.Fatal("server did not shutdown within timeout")
    }
}
```

#### 依赖图分析测试
```go
func TestDependencyGraph_NoKernelToCloudDeps(t *testing.T) {
    // 使用 go/packages 分析依赖图
    // 验证没有从 internal/core 到 internal/cloud 的依赖
}
```

### 静态分析测试

#### 导入检查
使用 `go/parser` 和 `go/ast` 检查导入语句：

```go
func TestStaticAnalysis_KernelImports(t *testing.T) {
    kernelFiles := findGoFiles("internal/core")
    
    for _, file := range kernelFiles {
        imports := extractImports(file)
        for _, imp := range imports {
            assert.NotContains(t, imp, "internal/cloud",
                "kernel file %s imports cloud package %s", file, imp)
        }
    }
}
```

#### 行数检查
```go
func TestStaticAnalysis_FileSizeConstraints(t *testing.T) {
    refactoredFiles := []string{
        "internal/app/server/config.go",
        "internal/app/server/config_defaults.go",
        // ... 其他拆分后的文件
    }
    
    for _, file := range refactoredFiles {
        lineCount := countLines(file)
        assert.Less(t, lineCount, 500,
            "file %s has %d lines, exceeds 500 line limit", file, lineCount)
    }
}
```

#### Goroutine 文档检查
```go
func TestStaticAnalysis_GoroutineDocumentation(t *testing.T) {
    goFiles := findAllGoFiles("internal")
    
    for _, file := range goFiles {
        goroutines := findGoroutineLaunches(file)
        for _, gr := range goroutines {
            hasLifecycleDoc := checkLifecycleComment(file, gr.LineNumber)
            assert.True(t, hasLifecycleDoc,
                "goroutine at %s:%d missing lifecycle documentation",
                file, gr.LineNumber)
        }
    }
}
```

### CI/CD 集成

在 CI 流程中添加依赖检查：

```yaml
# .github/workflows/dependency-check.yml
name: Dependency Check

on: [push, pull_request]

jobs:
  check-dependencies:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Check kernel layer dependencies
        run: |
          go test -v ./tests/static/... -run TestDependencyGraph
      
      - name: Check file size constraints
        run: |
          go test -v ./tests/static/... -run TestFileSizeConstraints
      
      - name: Check goroutine documentation
        run: |
          go test -v ./tests/static/... -run TestGoroutineDocumentation
```

## Implementation Notes

### 迁移策略

#### 阶段 1: 常量迁移（P0）
1. 创建 `internal/constants/ttl.go`
2. 将 `internal/cloud/constants` 中的跨层常量移动到 `internal/constants`
3. 更新所有引用点
4. 删除 `internal/cloud/constants` 中的重复定义

#### 阶段 2: SDK 入口点（P0）
1. 创建 `internal/app/server/sdk.go`
2. 创建 `internal/app/client/sdk.go`
3. 重构 `cmd/server/main.go` 使用新的 SDK
4. 重构 `cmd/client/main.go` 使用新的 SDK

#### 阶段 3: 文件拆分（P1）
1. 按优先级拆分高复杂度文件
2. 确保每个拆分后的文件 < 500 行
3. 运行测试验证功能完整性

#### 阶段 4: Goroutine 文档化（P1）
1. 识别所有长期运行的 goroutine
2. 添加生命周期注释
3. 创建生命周期图文档

### 向后兼容性

- 保持现有的公共 API 不变
- 旧的配置结构体保留但标记为 deprecated
- 提供迁移指南文档

### 性能考虑

- 常量迁移不影响运行时性能
- SDK 封装增加的开销可忽略不计（< 1ms）
- 文件拆分不影响编译后的二进制大小

## Documentation Requirements

### 必须创建的文档

1. **架构决策记录 (ADR)**
   - `docs/adr/001-dependency-inversion.md`
   - 记录为什么进行这次重构

2. **迁移指南**
   - `docs/migration/sdk-migration-guide.md`
   - 指导用户从旧 API 迁移到新 SDK

3. **Goroutine 生命周期图**
   - `docs/architecture/goroutine-lifecycle.md`
   - 包含 Mermaid 图表展示所有长期 goroutine

4. **常量使用指南**
   - `docs/development/constants-guide.md`
   - 说明何时在哪个包中定义常量

### 代码注释要求

- 所有 goroutine 启动点必须有生命周期注释
- 所有公共 API 必须有 godoc 注释
- 复杂的配置验证逻辑必须有解释性注释

## Rollout Plan

### 第 1 周：P0 任务
- 常量迁移
- SDK 入口点实现
- 更新 cmd/server 和 cmd/client

### 第 2 周：P1 任务（第一批）
- 拆分 `internal/app/server/config.go`
- 拆分 `internal/protocol/session/packet_handler_tunnel.go`

### 第 3 周：P1 任务（第二批）
- 拆分 `internal/stream/stream_processor.go`
- 拆分 `internal/cloud/services/service_registry.go`

### 第 4 周：文档和验证
- 完成所有文档
- 运行完整测试套件
- 性能基准测试
- 代码审查

## Success Criteria

重构成功的标准：

1. ✅ 所有属性测试通过（16 个属性）
2. ✅ 静态分析测试通过（导入检查、行数检查、文档检查）
3. ✅ 现有功能测试全部通过
4. ✅ 性能基准测试无退化
5. ✅ 代码覆盖率保持或提升
6. ✅ 所有文档完成
7. ✅ 代码审查通过

## Risks and Mitigation

### 风险 1: 破坏现有功能
**缓解措施**: 
- 每个阶段后运行完整测试套件
- 保持向后兼容性
- 使用 feature flag 逐步启用新代码

### 风险 2: 迁移工作量大
**缓解措施**:
- 使用自动化工具辅助重构（如 `gofmt`, `goimports`）
- 分阶段进行，每个阶段可独立验证
- 优先完成 P0 任务

### 风险 3: 团队学习曲线
**缓解措施**:
- 提供详细的迁移指南
- 进行代码审查培训
- 创建示例代码

### 风险 4: 性能退化
**缓解措施**:
- 在每个阶段运行性能基准测试
- 监控关键路径的性能指标
- 如有退化立即回滚并分析

## Appendix

### 相关文档链接

- [Go 依赖管理最佳实践](https://go.dev/doc/modules/managing-dependencies)
- [Clean Architecture in Go](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Effective Go](https://go.dev/doc/effective_go)

### 工具推荐

- **依赖分析**: `go mod graph`, `go list -m all`
- **静态分析**: `go vet`, `staticcheck`, `golangci-lint`
- **代码重构**: `gofmt`, `goimports`, `gorename`
- **测试覆盖**: `go test -cover`, `go tool cover`
