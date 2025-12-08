# 协议注册框架设计文档

## 执行摘要

本文档设计了一个统一的协议注册框架，解决当前协议集成分散、需要在多个地方做 if/else 或 switch 判断的问题。通过插件化架构和依赖注入，实现协议的自动注册、初始化和生命周期管理。

**核心目标**：
- 统一协议注册：`Protocol.Register("udp", udpProtocol)`
- 协议自主初始化：每个协议有自己的初始化过程
- 依赖注入支持：协议可以通过容器查找依赖服务
- 自动集成：HTTP长轮询等协议可以自动向HTTP服务器注册路由

---

## 当前问题分析

### 问题1：协议注册分散

**现状**：
```go
// internal/app/server/services.go
func (pf *ProtocolFactory) CreateAdapter(protocolName string, ctx context.Context) (adapter.Adapter, error) {
    switch protocolName {
    case "tcp":
        return adapter.NewTcpAdapter(ctx, pf.session), nil
    case "udp":
        return adapter.NewUdpAdapter(ctx, pf.session), nil
    case "websocket":
        return adapter.NewWebSocketAdapter(ctx, pf.session), nil
    case "quic":
        return adapter.NewQuicAdapter(ctx, pf.session), nil
    default:
        return nil, fmt.Errorf("unsupported protocol: %s", protocolName)
    }
}
```

**问题**：
- 新增协议需要修改工厂代码
- 协议创建逻辑分散
- 无法支持协议特定的初始化逻辑

### 问题2：协议初始化分散

**现状**：
```go
// internal/app/server/wiring.go
func (s *Server) setupProtocolAdapters() error {
    for protocolName, config := range enabledProtocols {
        adapter, err := s.protocolFactory.CreateAdapter(protocolName, s.serviceManager.GetContext())
        adapter.SetAddr(addr)
        s.protocolMgr.Register(adapter)
    }
}
```

**问题**：
- 所有协议的初始化逻辑相同，无法定制
- 协议无法访问其他服务（如HTTP服务器）

### 问题3：协议依赖处理分散

**现状**：
- HTTP长轮询需要在 `ManagementAPIServer` 中手动注册路由
- 协议无法主动查找和集成依赖服务

**问题**：
- 协议与依赖服务耦合
- 无法实现协议的自主集成

---

## 架构设计

### 核心概念

#### 1. Protocol 接口

```go
// internal/protocol/registry/protocol.go

// Protocol 协议接口
type Protocol interface {
    // Name 返回协议名称（如 "tcp", "udp", "httppoll"）
    Name() string
    
    // Initialize 初始化协议
    // container: 依赖注入容器，用于查找依赖服务
    // config: 协议配置
    // 返回：协议适配器实例
    Initialize(ctx context.Context, container Container, config *ProtocolConfig) (adapter.Adapter, error)
    
    // Dependencies 返回协议依赖的服务名称列表
    // 例如：["http_server", "session_manager"]
    Dependencies() []string
    
    // ValidateConfig 验证配置
    ValidateConfig(config *ProtocolConfig) error
}

// ProtocolConfig 协议配置
type ProtocolConfig struct {
    Name     string                 // 协议名称
    Enabled  bool                   // 是否启用
    Host     string                 // 监听地址
    Port     int                    // 监听端口
    Options  map[string]interface{} // 协议特定选项
}
```

#### 2. Container 接口（扩展现有容器）

```go
// internal/protocol/registry/container.go

// Container 依赖注入容器接口
// 基于现有的 internal/cloud/container/container.go，但提供类型安全的接口
type Container interface {
    // Resolve 解析服务（类型安全版本）
    Resolve(name string) (interface{}, error)
    
    // ResolveTyped 解析指定类型的服务
    ResolveTyped(name string, target interface{}) error
    
    // HasService 检查服务是否存在
    HasService(name string) bool
    
    // ListServices 列出所有服务
    ListServices() []string
}

// HTTPRouter 接口（用于HTTP协议注册路由）
type HTTPRouter interface {
    // RegisterRoute 注册路由
    RegisterRoute(method, path string, handler http.HandlerFunc) error
    
    // RegisterRouteWithMiddleware 注册带中间件的路由
    RegisterRouteWithMiddleware(method, path string, handler http.HandlerFunc, middlewares ...func(http.Handler) http.Handler) error
}
```

#### 3. ProtocolRegistry 注册表

```go
// internal/protocol/registry/registry.go

// ProtocolRegistry 协议注册表
type ProtocolRegistry struct {
    protocols map[string]Protocol
    mu        sync.RWMutex
}

// NewProtocolRegistry 创建协议注册表
func NewProtocolRegistry() *ProtocolRegistry {
    return &ProtocolRegistry{
        protocols: make(map[string]Protocol),
    }
}

// Register 注册协议
func (r *ProtocolRegistry) Register(protocol Protocol) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    name := protocol.Name()
    if name == "" {
        return fmt.Errorf("protocol name cannot be empty")
    }
    
    if _, exists := r.protocols[name]; exists {
        return fmt.Errorf("protocol %s already registered", name)
    }
    
    r.protocols[name] = protocol
    return nil
}

// Get 获取协议
func (r *ProtocolRegistry) Get(name string) (Protocol, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    protocol, exists := r.protocols[name]
    return protocol, exists
}

// List 列出所有已注册的协议
func (r *ProtocolRegistry) List() []string {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    names := make([]string, 0, len(r.protocols))
    for name := range r.protocols {
        names = append(names, name)
    }
    return names
}
```

#### 4. ProtocolManager（重构）

```go
// internal/protocol/registry/manager.go

// ProtocolManager 协议管理器（重构版）
type ProtocolManager struct {
    registry  *ProtocolRegistry
    container Container
    adapters  map[string]adapter.Adapter
    mu        sync.RWMutex
    ctx       context.Context
}

// NewProtocolManager 创建协议管理器
func NewProtocolManager(ctx context.Context, container Container) *ProtocolManager {
    return &ProtocolManager{
        registry:  NewProtocolRegistry(),
        container: container,
        adapters:  make(map[string]adapter.Adapter),
        ctx:       ctx,
    }
}

// RegisterProtocol 注册协议实现
func (pm *ProtocolManager) RegisterProtocol(protocol Protocol) error {
    return pm.registry.Register(protocol)
}

// InitializeProtocols 初始化所有启用的协议
func (pm *ProtocolManager) InitializeProtocols(configs map[string]*ProtocolConfig) error {
    // 1. 检查依赖
    if err := pm.validateDependencies(configs); err != nil {
        return fmt.Errorf("dependency validation failed: %w", err)
    }
    
    // 2. 按依赖顺序初始化
    initOrder, err := pm.resolveInitOrder(configs)
    if err != nil {
        return fmt.Errorf("failed to resolve init order: %w", err)
    }
    
    // 3. 依次初始化
    for _, protocolName := range initOrder {
        config := configs[protocolName]
        if !config.Enabled {
            continue
        }
        
        protocol, exists := pm.registry.Get(protocolName)
        if !exists {
            return fmt.Errorf("protocol %s not registered", protocolName)
        }
        
        // 验证配置
        if err := protocol.ValidateConfig(config); err != nil {
            return fmt.Errorf("invalid config for protocol %s: %w", protocolName, err)
        }
        
        // 初始化协议
        adapter, err := protocol.Initialize(pm.ctx, pm.container, config)
        if err != nil {
            return fmt.Errorf("failed to initialize protocol %s: %w", protocolName, err)
        }
        
        pm.adapters[protocolName] = adapter
    }
    
    return nil
}

// validateDependencies 验证依赖
func (pm *ProtocolManager) validateDependencies(configs map[string]*ProtocolConfig) error {
    for protocolName, config := range configs {
        if !config.Enabled {
            continue
        }
        
        protocol, exists := pm.registry.Get(protocolName)
        if !exists {
            continue // 未注册的协议跳过
        }
        
        deps := protocol.Dependencies()
        for _, depName := range deps {
            if !pm.container.HasService(depName) {
                return fmt.Errorf("protocol %s requires service %s, but it's not available", protocolName, depName)
            }
        }
    }
    return nil
}

// resolveInitOrder 解析初始化顺序（拓扑排序）
func (pm *ProtocolManager) resolveInitOrder(configs map[string]*ProtocolConfig) ([]string, error) {
    // 构建依赖图
    graph := make(map[string][]string)
    for protocolName, config := range configs {
        if !config.Enabled {
            continue
        }
        protocol, exists := pm.registry.Get(protocolName)
        if !exists {
            continue
        }
        graph[protocolName] = protocol.Dependencies()
    }
    
    // 拓扑排序
    return topologicalSort(graph)
}
```

---

## 协议实现示例

### 示例1：TCP 协议（简单协议）

```go
// internal/protocol/adapter/tcp_protocol.go

type TCPProtocol struct{}

func NewTCPProtocol() *TCPProtocol {
    return &TCPProtocol{}
}

func (p *TCPProtocol) Name() string {
    return "tcp"
}

func (p *TCPProtocol) Dependencies() []string {
    return []string{"session_manager"} // 需要 SessionManager
}

func (p *TCPProtocol) ValidateConfig(config *ProtocolConfig) error {
    if config.Port <= 0 {
        return fmt.Errorf("TCP port must be positive")
    }
    return nil
}

func (p *TCPProtocol) Initialize(ctx context.Context, container Container, config *ProtocolConfig) (adapter.Adapter, error) {
    // 1. 解析依赖
    var sessionMgr *session.SessionManager
    if err := container.ResolveTyped("session_manager", &sessionMgr); err != nil {
        return nil, fmt.Errorf("failed to resolve session_manager: %w", err)
    }
    
    // 2. 创建适配器
    adapter := adapter.NewTcpAdapter(ctx, sessionMgr)
    
    // 3. 配置地址
    addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
    adapter.SetAddr(addr)
    
    return adapter, nil
}
```

### 示例2：HTTP 长轮询协议（复杂协议，有依赖）

```go
// internal/protocol/adapter/httppoll_protocol.go

type HTTPPollProtocol struct{}

func NewHTTPPollProtocol() *HTTPPollProtocol {
    return &HTTPPollProtocol{}
}

func (p *HTTPPollProtocol) Name() string {
    return "httppoll"
}

func (p *HTTPPollProtocol) Dependencies() []string {
    return []string{"http_server", "session_manager"} // 需要 HTTP 服务器和 SessionManager
}

func (p *HTTPPollProtocol) ValidateConfig(config *ProtocolConfig) error {
    // HTTP 长轮询不需要监听端口（使用 HTTP 服务器的端口）
    return nil
}

func (p *HTTPPollProtocol) Initialize(ctx context.Context, container Container, config *ProtocolConfig) (adapter.Adapter, error) {
    // 1. 解析 SessionManager
    var sessionMgr *session.SessionManager
    if err := container.ResolveTyped("session_manager", &sessionMgr); err != nil {
        return nil, fmt.Errorf("failed to resolve session_manager: %w", err)
    }
    
    // 2. 解析 HTTP 服务器（如果存在）
    var httpRouter HTTPRouter
    if container.HasService("http_server") {
        var httpServer interface{}
        if err := container.Resolve("http_server", &httpServer); err == nil {
            // 尝试转换为 HTTPRouter
            if router, ok := httpServer.(HTTPRouter); ok {
                httpRouter = router
            } else if apiServer, ok := httpServer.(*api.ManagementAPIServer); ok {
                // 适配 ManagementAPIServer
                httpRouter = &httpServerAdapter{server: apiServer}
            }
        }
    }
    
    // 3. 注册 HTTP 路由（如果 HTTP 服务器存在）
    if httpRouter != nil {
        // 注册 Push 端点
        if err := httpRouter.RegisterRoute("POST", "/tunnox/v1/push", func(w http.ResponseWriter, r *http.Request) {
            // 委托给 ManagementAPIServer 的 handler
            // 这里需要访问 ManagementAPIServer，可以通过容器解析
            var apiServer *api.ManagementAPIServer
            if err := container.ResolveTyped("http_server", &apiServer); err == nil {
                apiServer.HandleHTTPPush(w, r)
            }
        }); err != nil {
            return nil, fmt.Errorf("failed to register push route: %w", err)
        }
        
        // 注册 Poll 端点
        if err := httpRouter.RegisterRoute("GET", "/tunnox/v1/poll", func(w http.ResponseWriter, r *http.Request) {
            var apiServer *api.ManagementAPIServer
            if err := container.ResolveTyped("http_server", &apiServer); err == nil {
                apiServer.HandleHTTPPoll(w, r)
            }
        }); err != nil {
            return nil, fmt.Errorf("failed to register poll route: %w", err)
        }
        
        utils.Infof("HTTP long polling: registered routes /tunnox/v1/push and /tunnox/v1/poll")
    } else {
        utils.Warnf("HTTP long polling: http_server not found, routes will not be registered")
    }
    
    // 4. 创建适配器（HTTP 长轮询可能不需要传统的 Adapter，返回 nil 或特殊适配器）
    // 注意：HTTP 长轮询的连接是通过 HTTP 请求建立的，不是通过 Listen/Accept
    return &httppollAdapter{
        sessionMgr: sessionMgr,
        httpRouter: httpRouter,
    }, nil
}

// httppollAdapter HTTP 长轮询适配器（特殊适配器）
type httppollAdapter struct {
    sessionMgr *session.SessionManager
    httpRouter HTTPRouter
}

func (a *httppollAdapter) Name() string {
    return "httppoll"
}

func (a *httppollAdapter) GetAddr() string {
    return "" // HTTP 长轮询使用 HTTP 服务器的地址
}

func (a *httppollAdapter) SetAddr(addr string) {
    // 忽略（使用 HTTP 服务器的地址）
}

func (a *httppollAdapter) ListenFrom(addr string) error {
    // HTTP 长轮询不需要监听（使用 HTTP 服务器的路由）
    return nil
}

func (a *httppollAdapter) Close() error {
    // 清理资源
    return nil
}
```

### 示例3：UDP 协议

```go
// internal/protocol/adapter/udp_protocol.go

type UDPProtocol struct{}

func NewUDPProtocol() *UDPProtocol {
    return &UDPProtocol{}
}

func (p *UDPProtocol) Name() string {
    return "udp"
}

func (p *UDPProtocol) Dependencies() []string {
    return []string{"session_manager"}
}

func (p *UDPProtocol) ValidateConfig(config *ProtocolConfig) error {
    if config.Port <= 0 {
        return fmt.Errorf("UDP port must be positive")
    }
    return nil
}

func (p *UDPProtocol) Initialize(ctx context.Context, container Container, config *ProtocolConfig) (adapter.Adapter, error) {
    var sessionMgr *session.SessionManager
    if err := container.ResolveTyped("session_manager", &sessionMgr); err != nil {
        return nil, fmt.Errorf("failed to resolve session_manager: %w", err)
    }
    
    adapter := adapter.NewUdpAdapter(ctx, sessionMgr)
    addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
    adapter.SetAddr(addr)
    
    return adapter, nil
}
```

---

## 使用方式

### 1. 协议注册（在 init 函数中）

```go
// internal/protocol/adapter/init.go

func init() {
    // 注册所有协议
    protocolRegistry := protocol.GetGlobalRegistry()
    
    protocolRegistry.Register(NewTCPProtocol())
    protocolRegistry.Register(NewUDPProtocol())
    protocolRegistry.Register(NewWebSocketProtocol())
    protocolRegistry.Register(NewQUICProtocol())
    protocolRegistry.Register(NewHTTPPollProtocol())
}
```

### 2. 服务器启动（重构后）

```go
// internal/app/server/wiring.go

func (s *Server) setupProtocolAdapters() error {
    // 1. 创建协议管理器（使用容器）
    protocolMgr := protocol.NewProtocolManager(s.serviceManager.GetContext(), s.container)
    
    // 2. 注册协议（从 init 函数自动注册，或手动注册）
    // 协议已经在 init 函数中注册到全局注册表
    // 这里只需要从全局注册表复制到本地管理器
    globalRegistry := protocol.GetGlobalRegistry()
    for _, protocolName := range globalRegistry.List() {
        proto, _ := globalRegistry.Get(protocolName)
        protocolMgr.RegisterProtocol(proto)
    }
    
    // 3. 获取启用的协议配置
    enabledProtocols := s.getEnabledProtocols()
    
    // 4. 转换为 ProtocolConfig
    configs := make(map[string]*protocol.ProtocolConfig)
    for name, cfg := range enabledProtocols {
        configs[name] = &protocol.ProtocolConfig{
            Name:    name,
            Enabled: cfg.Enabled,
            Host:    cfg.Host,
            Port:    cfg.Port,
            Options: make(map[string]interface{}),
        }
    }
    
    // 5. 初始化所有协议（自动处理依赖和初始化顺序）
    if err := protocolMgr.InitializeProtocols(configs); err != nil {
        return fmt.Errorf("failed to initialize protocols: %w", err)
    }
    
    // 6. 启动所有适配器
    s.protocolMgr = protocolMgr
    return protocolMgr.StartAll()
}
```

### 3. 容器服务注册

```go
// internal/app/server/wiring.go

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

---

## 架构优势

### 1. 统一注册机制
- ✅ 所有协议通过 `Protocol.Register()` 注册
- ✅ 无需修改工厂代码
- ✅ 支持自动发现

### 2. 协议自主初始化
- ✅ 每个协议有自己的初始化逻辑
- ✅ 协议可以访问容器查找依赖
- ✅ 支持协议特定的配置验证

### 3. 依赖注入支持
- ✅ 协议声明依赖，系统自动验证
- ✅ 支持依赖顺序初始化（拓扑排序）
- ✅ 类型安全的依赖解析

### 4. 自动集成
- ✅ HTTP 长轮询可以自动注册路由
- ✅ 协议可以主动集成其他服务
- ✅ 减少手动配置代码

### 5. 可扩展性
- ✅ 新增协议只需实现 `Protocol` 接口
- ✅ 无需修改核心代码
- ✅ 支持协议插件化

---

## 迁移计划

### Phase 1: 基础框架（2周）

1. **Week 1**：实现核心框架
   - [ ] 定义 `Protocol` 接口
   - [ ] 实现 `ProtocolRegistry`
   - [ ] 实现 `ProtocolManager`（重构版）
   - [ ] 实现依赖解析和拓扑排序

2. **Week 2**：实现协议示例
   - [ ] 实现 `TCPProtocol`
   - [ ] 实现 `UDPProtocol`
   - [ ] 实现 `HTTPPollProtocol`
   - [ ] 编写单元测试

### Phase 2: 集成迁移（2周）

1. **Week 3**：集成到服务器
   - [ ] 重构 `setupProtocolAdapters`
   - [ ] 设置容器服务注册
   - [ ] 迁移现有协议

2. **Week 4**：测试和优化
   - [ ] 集成测试
   - [ ] 性能测试
   - [ ] 文档更新

### Phase 3: 清理和优化（1周）

- [ ] 删除旧的 `ProtocolFactory` switch-case
- [ ] 清理重复代码
- [ ] 代码审查

---

## 设计细节

### 1. 依赖解析算法（拓扑排序）

```go
// internal/protocol/registry/topological_sort.go

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
        return nil, fmt.Errorf("circular dependency detected")
    }
    
    return result, nil
}
```

### 2. HTTP 服务器适配器

```go
// internal/protocol/registry/http_adapter.go

// httpServerAdapter 适配 ManagementAPIServer 为 HTTPRouter
type httpServerAdapter struct {
    server *api.ManagementAPIServer
}

func (a *httpServerAdapter) RegisterRoute(method, path string, handler http.HandlerFunc) error {
    // 使用反射或接口方法注册路由
    // 需要 ManagementAPIServer 提供注册方法
    return a.server.RegisterRoute(method, path, handler)
}

func (a *httpServerAdapter) RegisterRouteWithMiddleware(method, path string, handler http.HandlerFunc, middlewares ...func(http.Handler) http.Handler) error {
    return a.server.RegisterRouteWithMiddleware(method, path, handler, middlewares...)
}
```

### 3. 全局注册表（可选）

```go
// internal/protocol/registry/global.go

var (
    globalRegistry *ProtocolRegistry
    globalOnce     sync.Once
)

// GetGlobalRegistry 获取全局协议注册表
func GetGlobalRegistry() *ProtocolRegistry {
    globalOnce.Do(func() {
        globalRegistry = NewProtocolRegistry()
    })
    return globalRegistry
}
```

---

## 风险控制

### 技术风险

1. **循环依赖检测**
   - 使用拓扑排序检测
   - 初始化时验证

2. **类型安全**
   - 使用 `ResolveTyped` 确保类型安全
   - 编译时检查

3. **向后兼容**
   - 保留旧的 `ProtocolFactory`（标记为 deprecated）
   - 渐进式迁移

### 业务风险

1. **初始化顺序**
   - 自动解析依赖顺序
   - 支持手动指定优先级

2. **错误处理**
   - 详细的错误信息
   - 依赖缺失时明确提示

---

## 总结

### 核心价值

1. **统一注册**：`Protocol.Register("udp", udpProtocol)`
2. **自主初始化**：协议自己负责初始化逻辑
3. **依赖注入**：通过容器查找依赖服务
4. **自动集成**：HTTP 长轮询自动注册路由

### 架构原则

1. **开闭原则**：对扩展开放，对修改封闭
2. **依赖倒置**：协议依赖抽象（Container），不依赖具体实现
3. **单一职责**：每个协议只负责自己的初始化
4. **接口隔离**：协议接口精简，只包含必要方法

### 实施建议

1. **渐进式迁移**：先实现框架，再逐步迁移协议
2. **保持兼容**：保留旧代码，标记为 deprecated
3. **充分测试**：每个协议都要有完整的测试
4. **文档完善**：提供协议开发指南

---

**文档版本**：v1.0  
**最后更新**：2024  
**维护者**：架构团队

