# 阶段四重构执行计划

> **创建日期**: 2025-12-31
> **预估工作量**: 2-3天（分6个子阶段）
> **风险级别**: 🔴 高

---

## 一、背景分析

### 当前问题

SessionManager存在"双架构"并存：

**新架构组件**（已创建，部分使用）：
- `clientRegistry` - 客户端注册表
- `tunnelRegistry` - 隧道注册表
- `packetRouter` - 数据包路由器

**旧架构字段**（直接在SessionManager中）：
- `connMap`, `controlConnMap`, `tunnelConnMap` 等map
- 55个SessionManager方法散落在5个文件中
- 37个packet handler方法需要提取为独立实现

### 统计数据

| 文件类别 | 文件数 | 代码行数 | SessionManager方法数 |
|---------|--------|----------|---------------------|
| Manager核心 | 3 | 606 | ~25 |
| 连接管理 | 2 | 603 | ~24 |
| Handler | 8 | 1,471 | ~37 |
| **总计** | **13** | **2,680** | **~86** |

---

## 二、重构目标

### 最终状态

```
SessionManager (< 300行)  # Facade/协调器
    ├── clientRegistry (ClientRegistry)
    ├── tunnelRegistry (TunnelRegistry)
    ├── packetRouter (PacketRouter)
    └── handlers (独立的 PacketHandler 实现)
        ├── HandshakeHandler
        ├── TunnelOpenHandler
        ├── TunnelBridgeHandler
        ├── SOCKS5Handler
        ├── HeartbeatHandler
        └── CommandHandler
```

### 设计原则

1. **Facade模式** - SessionManager保留为公共API，方法委托给子组件
2. **依赖注入** - Handlers通过构造函数接收依赖（registries, cloudControl等）
3. **渐进式迁移** - 保持向后兼容，分步骤重构
4. **类型安全** - 所有Handler使用强类型接口

---

## 三、执行计划（6个子阶段）

### 子阶段4.1: 连接管理委托（1天）

**目标**：将connection_lifecycle.go和control_connection_mgr.go的方法委托给registries

**修改文件**：
- connection_lifecycle.go (331行)
- control_connection_mgr.go (272行)

**策略**：
1. 在SessionManager中保留现有方法（Facade）
2. 方法内部委托给clientRegistry或tunnelRegistry
3. 逐步将旧架构map的使用替换为registry调用
4. 例如：
   ```go
   // 旧实现（直接操作map）
   func (s *SessionManager) RegisterControlConnection(conn *ControlConnection) {
       s.controlConnLock.Lock()
       s.controlConnMap[conn.ConnID] = conn
       s.controlConnLock.Unlock()
   }

   // 新实现（委托给registry）
   func (s *SessionManager) RegisterControlConnection(conn *ControlConnection) {
       s.clientRegistry.Register(conn)
       // 旧map保持同步（临时兼容）
       s.controlConnLock.Lock()
       s.controlConnMap[conn.ConnID] = conn
       s.controlConnLock.Unlock()
   }
   ```

**验收标准**：
- [ ] 所有connection/control方法调用registry
- [ ] 旧map暂时保持同步
- [ ] 测试通过

### 子阶段4.2: 提取HandshakeHandler（0.5天）

**目标**：创建第一个独立Handler实现

**创建文件**：
- handler/handshake.go (新建，~250行)

**从packet_handler_handshake.go提取**：
- handleHandshake
- pushConfigToClient
- sendHandshakeResponse

**Handler结构**：
```go
package handler

type HandshakeHandler struct {
    clientRegistry *registry.ClientRegistry
    tunnelRegistry *registry.TunnelRegistry
    cloudControl   CloudControlAPI
    authHandler    AuthHandler
    logger         Logger
}

func (h *HandshakeHandler) HandlePacket(pkt *types.StreamPacket) error {
    // 实现握手逻辑
}
```

**验收标准**：
- [ ] handler/handshake.go创建成功
- [ ] SessionManager.handleHandshake委托给HandshakeHandler
- [ ] 测试通过

### 子阶段4.3: 提取TunnelOpenHandler（0.5天）

**目标**：提取隧道打开处理逻辑

**创建文件**：
- handler/tunnel_open.go (新建，~260行)

**从packet_handler_tunnel.go提取**：
- handleTunnelOpen
- setMappingIDOnConnection
- 其他相关方法

**验收标准**：
- [ ] handler/tunnel_open.go创建成功
- [ ] SessionManager委托给TunnelOpenHandler
- [ ] 测试通过

### 子阶段4.4: 提取TunnelBridgeHandler（0.5天）

**目标**：提取隧道桥接逻辑

**创建文件**：
- handler/tunnel_bridge.go (新建，~220行)

**从packet_handler_tunnel_bridge.go和packet_handler_tunnel_ops.go提取**：
- handleExistingBridge
- handleSourceBridge
- sendTunnelOpenResponse
- 其他隧道操作

**验收标准**：
- [ ] handler/tunnel_bridge.go创建成功
- [ ] SessionManager委托给TunnelBridgeHandler
- [ ] 测试通过

### 子阶段4.5: 提取其他Handlers（0.5天）

**目标**：提取剩余的Handler实现

**创建文件**：
- handler/socks5.go (从socks5_tunnel_handler.go，~150行)
- handler/heartbeat.go (从command_integration.go的handleHeartbeat，~50行)
- handler/command.go (从command_integration.go的其他方法，~200行)
- handler/event.go (从event_handlers.go，~20行)

**验收标准**：
- [ ] 所有handler文件创建成功
- [ ] SessionManager委托给对应Handler
- [ ] 测试通过

### 子阶段4.6: 简化SessionManager（0.5天）

**目标**：清理SessionManager，移除旧架构代码

**修改文件**：
- manager.go
- connection_lifecycle.go（删除或大幅简化）
- control_connection_mgr.go（删除或大幅简化）
- packet_handler*.go（删除，已提取到handler/）

**清理内容**：
1. 移除旧架构map（controlConnMap, tunnelConnMap等）
2. 移除临时同步代码
3. SessionManager保留：
   - 组件引用（clientRegistry, tunnelRegistry, handlers）
   - Facade方法（委托给子组件）
   - 初始化和资源清理逻辑

**目标代码量**：
- manager.go: < 300行
- manager_ops.go: < 100行
- manager_notify.go: < 100行

**验收标准**：
- [ ] SessionManager < 500行（3个文件总和）
- [ ] 无旧架构map
- [ ] 所有方法委托给子组件
- [ ] 测试通过（包括集成测试）

---

## 四、风险控制

### 高风险点

1. **大规模方法迁移** - 86个SessionManager方法需要重构
   - 缓解：分6个子阶段，每次只处理一部分
   - 验证：每个子阶段完成后运行测试

2. **依赖关系复杂** - Handler之间可能相互依赖
   - 缓解：通过依赖注入明确依赖关系
   - 验证：静态分析+集成测试

3. **测试覆盖不足** - connection和handler包缺少单元测试
   - 缓解：依赖现有集成测试
   - 计划：在阶段六补充单元测试

### 回退策略

如果阶段四遇到blocker：
1. 保留旧架构代码（通过类型别名兼容）
2. 新旧并存，逐步迁移调用方
3. 在阶段六清理旧代码

---

## 五、技术债务

### 新增临时方案

1. **双写模式**（子阶段4.1）：
   ```go
   // 同时写入registry和旧map
   s.clientRegistry.Register(conn)
   s.controlConnMap[conn.ConnID] = conn  // 临时兼容
   ```
   - 清理时机：子阶段4.6

2. **Facade方法**（所有子阶段）：
   ```go
   // SessionManager保留方法，委托给handler
   func (s *SessionManager) handleHandshake(pkt *types.StreamPacket) error {
       return s.handshakeHandler.HandlePacket(pkt)
   }
   ```
   - 保留时机：永久（作为公共API）

---

## 六、依赖关系

### Handler依赖图

```
HandshakeHandler
    ├── ClientRegistry (认证后注册)
    ├── CloudControl (获取配置)
    └── AuthHandler (认证逻辑)

TunnelOpenHandler
    ├── TunnelRegistry (注册隧道)
    ├── CloudControl (获取映射配置)
    └── BridgeManager (跨节点路由)

TunnelBridgeHandler
    ├── TunnelRegistry (查找隧道)
    ├── BridgeManager (建立桥接)
    └── TunnelRoutingTable (路由表)

SOCKS5Handler
    ├── TunnelRegistry
    └── CloudControl

HeartbeatHandler
    ├── ClientRegistry (更新活跃时间)
    └── TunnelRegistry (更新隧道状态)

CommandHandler
    ├── CommandRegistry (查找handler)
    └── CommandExecutor (执行命令)
```

### 注入方式

所有Handler通过构造函数接收依赖：

```go
func NewHandshakeHandler(
    clientRegistry *registry.ClientRegistry,
    cloudControl CloudControlAPI,
    authHandler AuthHandler,
    logger Logger,
) *HandshakeHandler {
    return &HandshakeHandler{
        clientRegistry: clientRegistry,
        cloudControl:   cloudControl,
        authHandler:    authHandler,
        logger:         logger,
    }
}
```

---

## 七、测试策略

### 每个子阶段

1. **单元测试**（如有）：
   ```bash
   go test ./internal/protocol/session/... -v
   ```

2. **集成测试**：
   ```bash
   cd tests
   python3 -m scenarios.tcp_sql --skip-build
   ```

3. **编译验证**：
   ```bash
   go build ./...
   go vet ./...
   ```

### 阶段四完成后

1. **完整测试套件**：
   ```bash
   go test ./... -v
   go test -race ./...
   ```

2. **性能基准**（如有）：
   ```bash
   go test -bench=. -benchmem ./internal/protocol/session/...
   ```

3. **端到端测试**：
   ```bash
   ./start_test.sh
   ```

---

## 八、成功标准

### 代码质量

- [ ] SessionManager < 500行（manager.go + manager_ops.go + manager_notify.go）
- [ ] 每个Handler文件 < 300行
- [ ] 无弱类型（interface{}, any, map[string]interface{}）
- [ ] 所有方法 < 100行

### 架构质量

- [ ] SessionManager纯Facade，无业务逻辑
- [ ] Handler独立，依赖注入清晰
- [ ] 无循环依赖
- [ ] Registry负责状态管理

### 测试质量

- [ ] 所有现有测试通过（16个单元测试）
- [ ] 集成测试通过（tcp_sql等）
- [ ] 无竞态条件（go test -race）

---

## 九、执行时间表

| 子阶段 | 任务 | 预估时间 | 依赖 |
|--------|------|----------|------|
| 4.1 | 连接管理委托 | 1天 | - |
| 4.2 | HandshakeHandler | 0.5天 | 4.1 |
| 4.3 | TunnelOpenHandler | 0.5天 | 4.1 |
| 4.4 | TunnelBridgeHandler | 0.5天 | 4.1, 4.3 |
| 4.5 | 其他Handlers | 0.5天 | 4.1-4.4 |
| 4.6 | 简化SessionManager | 0.5天 | 4.1-4.5 |

**总计**: 3.5天（考虑调试和测试时间）

---

## 十、下一步行动

### 立即执行（子阶段4.1）

1. **分析connection_lifecycle.go的方法依赖**：
   - 哪些方法依赖controlConnMap/tunnelConnMap
   - 如何委托给clientRegistry/tunnelRegistry

2. **修改RegisterControlConnection**：
   - 调用clientRegistry.Register
   - 保持旧map同步（临时）

3. **修改GetControlConnectionByClientID**：
   - 优先从clientRegistry查询
   - fallback到旧map（临时）

4. **验证**：
   ```bash
   go test ./internal/protocol/session/... -v
   ```

---

**计划创建时间**: 2025-12-31
**下次更新**: 子阶段4.1完成后
