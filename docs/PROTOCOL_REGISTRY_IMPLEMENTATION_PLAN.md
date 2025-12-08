# 协议注册框架实现方案

## 概述

本文档基于 `PROTOCOL_REGISTRY_DESIGN.md` 设计，结合当前代码库实际情况，提供具体的实现方案。方案严格遵循 Tunnox 编码规范，确保代码质量、类型安全、架构清晰。

## 当前代码分析

### 现有组件

1. **ProtocolFactory** (`internal/app/server/services.go`)
   - 使用 switch-case 创建适配器
   - 硬编码协议创建逻辑

2. **ProtocolManager** (`internal/protocol/manager.go`)
   - 管理适配器生命周期
   - 已集成 dispose 体系

3. **Container** (`internal/cloud/container/container.go`)
   - 已有依赖注入容器实现
   - 支持单例和瞬态服务

4. **ProtocolConfig** (`internal/app/server/config.go`)
   - 已有协议配置结构

## 实现方案

### Phase 1: 核心框架（第1周）

#### 1.1 创建协议注册表 (`internal/protocol/registry/registry.go`)

```go
package registry

import (
    "fmt"
    "sync"
    coreErrors "tunnox-core/internal/core/errors"
)

// Protocol 协议接口
type Protocol interface {
    // Name 返回协议名称
    Name() string
    
    // Initialize 初始化协议
    // ctx: 从 dispose 体系分配的上下文
    // container: 依赖注入容器
    // config: 协议配置
    Initialize(ctx context.Context, container Container, config *Config) (adapter.Adapter, error)
    
    // Dependencies 返回协议依赖的服务名称列表
    Dependencies() []string
    
    // ValidateConfig 验证配置
    ValidateConfig(config *Config) error
}

// Config 协议配置（扩展 server.ProtocolConfig）
type Config struct {
    Name    string
    Enabled bool
    Host    string
    Port    int
    Options map[string]interface{} // 协议特定选项
}

// Registry 协议注册表
type Registry struct {
    protocols map[string]Protocol
    mu        sync.RWMutex
}

// NewRegistry 创建协议注册表
func NewRegistry() *Registry {
    return &Registry{
        protocols: make(map[string]Protocol),
    }
}

// Register 注册协议
func (r *Registry) Register(protocol Protocol) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    name := protocol.Name()
    if name == "" {
        return coreErrors.New(coreErrors.ErrorTypePermanent, "protocol name cannot be empty")
    }
    
    if _, exists := r.protocols[name]; exists {
        return coreErrors.Newf(coreErrors.ErrorTypePermanent, "protocol %s already registered", name)
    }
    
    r.protocols[name] = protocol
    return nil
}

// Get 获取协议
func (r *Registry) Get(name string) (Protocol, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    protocol, exists := r.protocols[name]
    if !exists {
        return nil, coreErrors.Newf(coreErrors.ErrorTypePermanent, "protocol %s not registered", name)
    }
    return protocol, nil
}

// List 列出所有已注册的协议
func (r *Registry) List() []string {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    names := make([]string, 0, len(r.protocols))
    for name := range r.protocols {
        names = append(names, name)
    }
    return names
}
```

#### 1.2 扩展容器接口 (`internal/protocol/registry/container.go`)

```go
package registry

import (
    "tunnox-core/internal/cloud/container"
)

// Container 依赖注入容器接口（包装现有容器）
// 注意：使用接口而非直接依赖具体类型，遵循依赖倒置原则
type Container interface {
    // Resolve 解析服务
    Resolve(name string) (interface{}, error)
    
    // ResolveTyped 解析指定类型的服务（类型安全）
    ResolveTyped(name string, target interface{}) error
    
    // HasService 检查服务是否存在
    HasService(name string) bool
    
    // ListServices 列出所有服务
    ListServices() []string
}

// containerAdapter 适配现有 container.Container
// 将 container.Container 适配为 registry.Container 接口
type containerAdapter struct {
    container *container.Container
}

// NewContainerAdapter 创建容器适配器
func NewContainerAdapter(c *container.Container) Container {
    return &containerAdapter{container: c}
}

func (a *containerAdapter) Resolve(name string) (interface{}, error) {
    return a.container.Resolve(name)
}

func (a *containerAdapter) ResolveTyped(name string, target interface{}) error {
    // 直接使用现有容器的 ResolveTyped 方法（已实现类型安全）
    return a.container.ResolveTyped(name, target)
}

func (a *containerAdapter) HasService(name string) bool {
    return a.container.HasService(name)
}

func (a *containerAdapter) ListServices() []string {
    return a.container.ListServices()
}
```

#### 1.3 重构 ProtocolManager (`internal/protocol/manager.go`)

```go
package protocol

import (
    "context"
    "sync"
    "tunnox-core/internal/core/dispose"
    "tunnox-core/internal/protocol/adapter"
    "tunnox-core/internal/protocol/registry"
    "tunnox-core/internal/utils"
    coreErrors "tunnox-core/internal/core/errors"
)

// ProtocolManager 协议管理器（重构版）
type ProtocolManager struct {
    *dispose.ManagerBase
    registry  *registry.Registry
    container registry.Container
    adapters  map[string]adapter.Adapter
    mu        sync.RWMutex
}

// NewProtocolManager 创建协议管理器
func NewProtocolManager(parentCtx context.Context, container registry.Container) *ProtocolManager {
    manager := &ProtocolManager{
        ManagerBase: dispose.NewManager("ProtocolManager", parentCtx),
        registry:    registry.NewRegistry(),
        container:   container,
        adapters:    make(map[string]adapter.Adapter),
    }
    return manager
}

// RegisterProtocol 注册协议实现
func (pm *ProtocolManager) RegisterProtocol(protocol registry.Protocol) error {
    return pm.registry.Register(protocol)
}

// InitializeProtocols 初始化所有启用的协议
func (pm *ProtocolManager) InitializeProtocols(configs map[string]*registry.Config) error {
    // 1. 验证依赖
    if err := pm.validateDependencies(configs); err != nil {
        return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "dependency validation failed")
    }
    
    // 2. 按依赖顺序初始化
    initOrder, err := pm.resolveInitOrder(configs)
    if err != nil {
        return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to resolve init order")
    }
    
    // 3. 依次初始化
    for _, protocolName := range initOrder {
        config := configs[protocolName]
        if !config.Enabled {
            continue
        }
        
        protocol, err := pm.registry.Get(protocolName)
        if err != nil {
            return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "protocol %s not registered", protocolName)
        }
        
        // 验证配置
        if err := protocol.ValidateConfig(config); err != nil {
            return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "invalid config for protocol %s", protocolName)
        }
        
        // 初始化协议（使用 dispose 体系的上下文）
        adapter, err := protocol.Initialize(pm.Ctx(), pm.container, config)
        if err != nil {
            return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to initialize protocol %s", protocolName)
        }
        
        pm.mu.Lock()
        pm.adapters[protocolName] = adapter
        pm.mu.Unlock()
    }
    
    return nil
}

// validateDependencies 验证依赖
func (pm *ProtocolManager) validateDependencies(configs map[string]*registry.Config) error {
    for protocolName, config := range configs {
        if !config.Enabled {
            continue
        }
        
        protocol, err := pm.registry.Get(protocolName)
        if err != nil {
            continue // 未注册的协议跳过
        }
        
        deps := protocol.Dependencies()
        for _, depName := range deps {
            if !pm.container.HasService(depName) {
                return coreErrors.Newf(coreErrors.ErrorTypePermanent, 
                    "protocol %s requires service %s, but it's not available", protocolName, depName)
            }
        }
    }
    return nil
}

// resolveInitOrder 解析初始化顺序（拓扑排序）
func (pm *ProtocolManager) resolveInitOrder(configs map[string]*registry.Config) ([]string, error) {
    // 构建依赖图
    graph := make(map[string][]string)
    for protocolName, config := range configs {
        if !config.Enabled {
            continue
        }
        protocol, err := pm.registry.Get(protocolName)
        if err != nil {
            continue
        }
        graph[protocolName] = protocol.Dependencies()
    }
    
    // 拓扑排序
    return topologicalSort(graph)
}

// StartAll 启动所有适配器
func (pm *ProtocolManager) StartAll() error {
    pm.mu.RLock()
    defer pm.mu.RUnlock()
    
    for name, a := range pm.adapters {
        utils.Infof("Starting %s adapter on %s", name, a.GetAddr())
        if err := a.ListenFrom(a.GetAddr()); err != nil {
            utils.LogErrorf(err, "Failed to start adapter %s: %v", name, err)
            return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to start adapter %s", name)
        }
        utils.Infof("Successfully started %s adapter on %s", name, a.GetAddr())
    }
    return nil
}

// onClose 资源清理回调
func (pm *ProtocolManager) onClose() error {
    pm.mu.Lock()
    defer pm.mu.Unlock()
    
    var lastErr error
    for _, adapter := range pm.adapters {
        if err := adapter.Close(); err != nil {
            utils.LogErrorf(err, "Failed to close adapter %s: %v", adapter.Name(), err)
            lastErr = err
        }
    }
    
    pm.adapters = nil
    return lastErr
}
```

#### 1.4 拓扑排序实现 (`internal/protocol/registry/topological_sort.go`)

```go
package registry

import (
    "fmt"
    coreErrors "tunnox-core/internal/core/errors"
)

// topologicalSort 拓扑排序（解析初始化顺序）
func topologicalSort(graph map[string][]string) ([]string, error) {
    // 计算入度
    inDegree := make(map[string]int)
    for node := range graph {
        inDegree[node] = 0
    }
    for _, deps := range graph {
        for _, dep := range deps {
            inDegree[dep]++
        }
    }
    
    // 找到所有入度为 0 的节点
    queue := make([]string, 0)
    for node, degree := range inDegree {
        if degree == 0 {
            queue = append(queue, node)
        }
    }
    
    result := make([]string, 0)
    for len(queue) > 0 {
        node := queue[0]
        queue = queue[1:]
        result = append(result, node)
        
        // 减少依赖节点的入度
        for _, dep := range graph[node] {
            inDegree[dep]--
            if inDegree[dep] == 0 {
                queue = append(queue, dep)
            }
        }
    }
    
    // 检查是否有循环依赖
    if len(result) != len(graph) {
        return nil, coreErrors.New(coreErrors.ErrorTypePermanent, "circular dependency detected in protocol initialization")
    }
    
    return result, nil
}
```

### Phase 2: 协议实现（第2周）

#### 2.1 TCP 协议实现 (`internal/protocol/registry/protocols/tcp.go`)

```go
package protocols

import (
    "context"
    "fmt"
    "tunnox-core/internal/protocol/adapter"
    "tunnox-core/internal/protocol/registry"
    "tunnox-core/internal/protocol/session"
    coreErrors "tunnox-core/internal/core/errors"
)

// TCPProtocol TCP 协议实现
type TCPProtocol struct{}

// NewTCPProtocol 创建 TCP 协议
func NewTCPProtocol() *TCPProtocol {
    return &TCPProtocol{}
}

// Name 返回协议名称
func (p *TCPProtocol) Name() string {
    return "tcp"
}

// Dependencies 返回依赖服务
func (p *TCPProtocol) Dependencies() []string {
    return []string{"session_manager"}
}

// ValidateConfig 验证配置
func (p *TCPProtocol) ValidateConfig(config *registry.Config) error {
    if config.Port <= 0 || config.Port > 65535 {
        return coreErrors.Newf(coreErrors.ErrorTypePermanent, "TCP port must be in range [1, 65535], got %d", config.Port)
    }
    return nil
}

// Initialize 初始化协议
func (p *TCPProtocol) Initialize(ctx context.Context, container registry.Container, config *registry.Config) (adapter.Adapter, error) {
    // 1. 解析 SessionManager
    var sessionMgr *session.SessionManager
    if err := container.ResolveTyped("session_manager", &sessionMgr); err != nil {
        return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to resolve session_manager")
    }
    
    // 2. 创建适配器
    adapter := adapter.NewTcpAdapter(ctx, sessionMgr)
    
    // 3. 配置地址
    addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
    adapter.SetAddr(addr)
    
    return adapter, nil
}
```

#### 2.2 UDP 协议实现 (`internal/protocol/registry/protocols/udp.go`)

```go
package protocols

import (
    "context"
    "fmt"
    "tunnox-core/internal/protocol/adapter"
    "tunnox-core/internal/protocol/registry"
    "tunnox-core/internal/protocol/session"
    coreErrors "tunnox-core/internal/core/errors"
)

// UDPProtocol UDP 协议实现
type UDPProtocol struct{}

// NewUDPProtocol 创建 UDP 协议
func NewUDPProtocol() *UDPProtocol {
    return &UDPProtocol{}
}

// Name 返回协议名称
func (p *UDPProtocol) Name() string {
    return "udp"
}

// Dependencies 返回依赖服务
func (p *UDPProtocol) Dependencies() []string {
    return []string{"session_manager"}
}

// ValidateConfig 验证配置
func (p *UDPProtocol) ValidateConfig(config *registry.Config) error {
    if config.Port <= 0 || config.Port > 65535 {
        return coreErrors.Newf(coreErrors.ErrorTypePermanent, "UDP port must be in range [1, 65535], got %d", config.Port)
    }
    return nil
}

// Initialize 初始化协议
func (p *UDPProtocol) Initialize(ctx context.Context, container registry.Container, config *registry.Config) (adapter.Adapter, error) {
    var sessionMgr *session.SessionManager
    if err := container.ResolveTyped("session_manager", &sessionMgr); err != nil {
        return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to resolve session_manager")
    }
    
    adapter := adapter.NewUdpAdapter(ctx, sessionMgr)
    addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
    adapter.SetAddr(addr)
    
    return adapter, nil
}
```

#### 2.3 协议注册 (`internal/protocol/registry/protocols/init.go`)

```go
package protocols

import (
    "tunnox-core/internal/protocol/registry"
)

var (
    globalRegistry *registry.Registry
)

// GetGlobalRegistry 获取全局协议注册表
func GetGlobalRegistry() *registry.Registry {
    if globalRegistry == nil {
        globalRegistry = registry.NewRegistry()
        registerAll(globalRegistry)
    }
    return globalRegistry
}

// registerAll 注册所有协议
func registerAll(r *registry.Registry) {
    _ = r.Register(NewTCPProtocol())
    _ = r.Register(NewUDPProtocol())
    _ = r.Register(NewWebSocketProtocol())
    _ = r.Register(NewQUICProtocol())
    // HTTP 长轮询协议将在后续实现
}
```

### Phase 3: 集成迁移（第3周）

#### 3.1 重构服务器初始化 (`internal/app/server/wiring.go`)

```go
// setupProtocolAdapters 设置协议适配器（重构版）
func (s *Server) setupProtocolAdapters() error {
    // 1. 创建容器适配器
    containerAdapter := registry.NewContainerAdapter(s.container)
    
    // 2. 创建协议管理器（使用 dispose 体系的上下文）
    protocolMgr := protocol.NewProtocolManager(s.serviceManager.GetContext(), containerAdapter)
    
    // 3. 从全局注册表注册协议
    globalRegistry := protocols.GetGlobalRegistry()
    for _, protocolName := range globalRegistry.List() {
        proto, err := globalRegistry.Get(protocolName)
        if err != nil {
            return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to get protocol %s", protocolName)
        }
        if err := protocolMgr.RegisterProtocol(proto); err != nil {
            return coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to register protocol %s", protocolName)
        }
    }
    
    // 4. 获取启用的协议配置并转换
    enabledProtocols := s.getEnabledProtocols()
    configs := make(map[string]*registry.Config)
    for name, cfg := range enabledProtocols {
        configs[name] = &registry.Config{
            Name:    name,
            Enabled: cfg.Enabled,
            Host:    cfg.Host,
            Port:    cfg.Port,
            Options: make(map[string]interface{}),
        }
    }
    
    // 5. 初始化所有协议（自动处理依赖和初始化顺序）
    if err := protocolMgr.InitializeProtocols(configs); err != nil {
        return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to initialize protocols")
    }
    
    // 6. 保存协议管理器
    s.protocolMgr = protocolMgr
    
    // 7. 启动所有适配器
    return protocolMgr.StartAll()
}
```

#### 3.2 容器服务注册 (`internal/app/server/wiring.go`)

```go
// setupContainer 设置依赖注入容器
func (s *Server) setupContainer() {
    // 注册核心服务到容器
    s.container.RegisterSingleton("session_manager", func() (interface{}, error) {
        return s.session, nil
    })
    
    s.container.RegisterSingleton("http_server", func() (interface{}, error) {
        return s.apiServer, nil
    })
    
    s.container.RegisterSingleton("storage", func() (interface{}, error) {
        return s.storage, nil
    })
    
    // ... 其他服务
}
```

### Phase 4: 测试和清理（第4周）

#### 4.1 单元测试

- `internal/protocol/registry/registry_test.go`
- `internal/protocol/registry/topological_sort_test.go`
- `internal/protocol/registry/protocols/tcp_test.go`
- `internal/protocol/registry/protocols/udp_test.go`

#### 4.2 集成测试

- 验证协议注册和初始化
- 验证依赖解析
- 验证拓扑排序

#### 4.3 清理旧代码

- 标记 `ProtocolFactory` 为 deprecated
- 保留向后兼容性（可选）

## 代码质量要求

### 1. 类型安全

- ✅ 避免使用 `map[string]interface{}` 和 `interface{}`
- ✅ 使用 `ResolveTyped` 进行类型安全的依赖解析
- ✅ 协议配置使用强类型结构

### 2. Dispose 体系

- ✅ `ProtocolManager` 继承 `ManagerBase`
- ✅ 所有上下文从 dispose 体系分配
- ✅ 资源清理在 `onClose` 中实现

### 3. 错误处理

- ✅ 使用 `TypedError` 系统
- ✅ 使用 `utils.LogErrorf` 记录错误
- ✅ 提供详细的错误信息

### 4. 文件组织

```
internal/protocol/
├── manager.go                    # ProtocolManager（重构）
├── service.go                    # ProtocolService（现有）
└── registry/                     # 新增：协议注册框架
    ├── registry.go               # ProtocolRegistry
    ├── container.go              # Container 接口和适配器
    ├── topological_sort.go       # 拓扑排序算法
    └── protocols/                # 协议实现
        ├── init.go               # 协议注册
        ├── tcp.go                # TCP 协议
        ├── udp.go                # UDP 协议
        ├── websocket.go          # WebSocket 协议
        ├── quic.go               # QUIC 协议
        └── httppoll.go           # HTTP 长轮询协议（后续）
```

### 5. 依赖倒置

- ✅ 协议依赖 `Container` 接口，不依赖具体实现
- ✅ 协议依赖 `adapter.Adapter` 接口
- ✅ 使用接口而非具体类型

## 实施步骤

1. **Week 1**: 实现核心框架（Registry, Container, ProtocolManager 重构）
2. **Week 2**: 实现协议示例（TCP, UDP, WebSocket, QUIC）
3. **Week 3**: 集成到服务器，迁移现有代码
4. **Week 4**: 测试、文档、清理

## 风险控制

1. **向后兼容**: 保留 `ProtocolFactory`（标记为 deprecated）
2. **渐进式迁移**: 先实现框架，再逐步迁移协议
3. **充分测试**: 每个组件都要有单元测试
4. **类型安全**: 使用 `ResolveTyped` 确保类型安全

## 总结

本方案严格遵循 Tunnox 编码规范，确保：
- ✅ 代码质量高
- ✅ 类型安全
- ✅ 架构清晰
- ✅ 遵循 dispose 体系
- ✅ 依赖倒置原则
- ✅ 文件组织合理
- ✅ 测试覆盖完整

