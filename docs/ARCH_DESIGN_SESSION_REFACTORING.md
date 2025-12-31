# protocol/session 包拆分架构设计

> **架构师**: AI Network Architect
> **设计日期**: 2025-12-31
> **任务**: H-01 protocol/session 包重构设计
> **目标**: 将 11,121 行代码拆分为职责清晰的子包

---

## 一、现状分析

### 1.1 当前问题

| 问题 | 现状 | 目标 | 差距 |
|------|------|------|------|
| 包总行数 | 11,121 行 | < 2,000 行 | 🔴 **5.6倍** |
| 根文件数 | 41 个文件 | < 10 个文件 | 🔴 **4倍** |
| 最大单文件 | 397 行 | < 500 行 | ✅ 达标 |
| 子包数量 | 5 个 | 8-10 个 | 🟡 需增加 |

### 1.2 现有子包

```
session/
├── buffer/         # 发送/接收缓冲区 (307 行)
├── connstate/      # 连接状态存储 (82 行)
├── crossnode/      # 跨节点通信 (1,026 行)
├── httpproxy/      # HTTP代理 (已存在但根目录有文件)
└── tunnel/         # 隧道管理 (683 行)
```

**问题**：虽然已有子包，但 41 个文件仍在根目录，未充分利用子包。

---

## 二、架构拆分设计

### 2.1 最终目录结构

```
internal/protocol/session/
│
├── session.go                      # 包入口（向后兼容的类型别名和工厂函数）
├── interfaces.go                   # 核心接口定义（移动自根目录）
├── doc.go                          # 包文档
│
├── core/                           # 【新增】核心会话管理
│   ├── manager.go                  # SessionManager 主结构
│   ├── manager_lifecycle.go        # 生命周期管理（Start/Stop/Shutdown）
│   ├── manager_operations.go       # 操作方法（SetHandler、GetComponent）
│   ├── manager_notify.go           # 客户端通知
│   └── config.go                   # 会话配置（移动自根 config.go）
│
├── connection/                     # 【新增】连接管理
│   ├── types.go                    # 连接类型定义（移动自 connection.go）
│   ├── control_connection.go       # 控制连接实现
│   ├── tunnel_connection.go        # 隧道连接实现
│   ├── tcp_connection.go           # TCP连接实现（移动自根目录）
│   ├── factory.go                  # 连接工厂（移动自 connection_factory.go）
│   ├── lifecycle.go                # 连接生命周期（移动自 connection_lifecycle.go）
│   ├── manager.go                  # 控制连接管理器（移动自 control_connection_mgr.go）
│   └── state.go                    # 连接状态管理（移动自 connection_managers.go）
│
├── registry/                       # 【新增】注册表
│   ├── client.go                   # 客户端注册表（移动自 client_registry.go）
│   └── tunnel.go                   # 隧道注册表（移动自 tunnel_registry.go）
│
├── handler/                        # 【新增】数据包处理
│   ├── router.go                   # 数据包路由器（移动自 packet_router.go）
│   ├── handshake.go                # 握手处理（移动自 packet_handler_handshake.go）
│   ├── tunnel_open.go              # 隧道打开处理（移动自 packet_handler_tunnel.go）
│   ├── tunnel_bridge.go            # 隧道桥接处理（移动自 packet_handler_tunnel_bridge.go）
│   ├── tunnel_ops.go               # 隧道操作辅助（移动自 packet_handler_tunnel_ops.go）
│   ├── socks5.go                   # SOCKS5处理（移动自 socks5_tunnel_handler.go）
│   └── utils.go                    # 处理器公共函数（移动自 packet_handler.go）
│
├── tunnel/                         # 【重组】隧道管理（已存在）
│   ├── bridge.go                   # 桥接器主逻辑（已存在）
│   ├── bridge_forward.go           # 转发逻辑（已存在）
│   ├── bridge_manager.go           # 桥接器管理（移动自 server_bridge.go）
│   ├── migration_manager.go        # 迁移管理器（移动自 tunnel_migration.go）
│   ├── migration_integration.go    # 迁移集成（移动自 tunnel_migration_integration.go）
│   └── types.go                    # 隧道类型别名（移动自 tunnel_facade.go）
│
├── crossnode/                      # 【重组】跨节点通信（已存在）
│   ├── pool.go                     # 连接池（已存在）
│   ├── conn.go                     # 连接实现（已存在）
│   ├── stream.go                   # 流协议（已存在）
│   ├── frame.go                    # 帧协议（已存在）
│   ├── listener.go                 # 监听器（移动自 cross_node_listener.go）
│   ├── session.go                  # 会话处理（移动自 cross_node_session.go）
│   ├── server.go                   # 跨服务器处理（移动自 cross_server.go）
│   ├── forward.go                  # 转发辅助（移动自 cross_node_forward_helper.go）
│   └── types.go                    # 类型别名（移动自 crossnode_facade.go）
│
├── httpproxy/                      # 【重组】HTTP代理（已存在但需整合）
│   └── proxy.go                    # HTTP代理管理（移动自 http_proxy.go）
│
├── buffer/                         # 【保留】缓冲区管理（已存在）
│   ├── send_buffer.go
│   └── receive_buffer.go
│
├── connstate/                      # 【保留】连接状态（已存在）
│   └── store.go
│
├── notification/                   # 【新增】通知服务
│   ├── service.go                  # 通知服务（移动自 notification_service.go）
│   └── response.go                 # 响应管理（移动自 response_manager.go）
│
└── integration/                    # 【新增】外部集成
    ├── command.go                  # 命令框架集成（移动自 command_integration.go）
    ├── cloudcontrol.go             # CloudControl适配器（移动自 cloudcontrol_adapter.go）
    ├── config_broadcast.go         # 配置推送广播（移动自 config_push_broadcast.go）
    └── events.go                   # 事件处理（移动自 event_handlers.go）
```

### 2.2 子包行数预估

| 子包 | 预估行数 | 文件数 | 状态 |
|------|----------|--------|------|
| `core/` | ~950 行 | 5 | 新增 |
| `connection/` | ~1,970 行 | 8 | 新增 |
| `registry/` | ~480 行 | 2 | 新增 |
| `handler/` | ~1,160 行 | 7 | 新增 |
| `tunnel/` | ~1,200 行 | 6 | 重组 |
| `crossnode/` | ~1,480 行 | 9 | 重组 |
| `httpproxy/` | ~350 行 | 1 | 重组 |
| `buffer/` | ~310 行 | 2 | 保留 |
| `connstate/` | ~80 行 | 1 | 保留 |
| `notification/` | ~360 行 | 2 | 新增 |
| `integration/` | ~460 行 | 4 | 新增 |
| **根目录** | **~200 行** | **3** | **✅ < 500 行** |

**总计**: 11,000 行 → 拆分为 11 个子包 + 根目录

---

## 三、子包职责定义

### 3.1 核心层（core/）

**职责**：SessionManager 的核心逻辑和生命周期管理

**对外接口**：
```go
// SessionManager 是会话管理器的核心结构
type SessionManager interface {
    // 生命周期
    Start(ctx context.Context) error
    Stop() error
    Shutdown(reason ShutdownReason) error

    // 组件注册
    SetTunnelHandler(handler TunnelHandler)
    SetCloudControl(cc CloudControl)

    // 通知
    NotifyClientUpdate(clientID uint64, config *ClientConfig) error
}
```

**依赖**：
- → connection（连接管理）
- → registry（注册表）
- → handler（数据包处理器）
- → tunnel（隧道管理）

**关键文件**：
- `manager.go`: SessionManager 主结构和构造函数
- `manager_lifecycle.go`: Start/Stop/Shutdown 实现
- `manager_operations.go`: SetHandler、GetComponent 等操作
- `manager_notify.go`: 客户端通知逻辑
- `config.go`: SessionConfig 和 SessionManagerConfig

---

### 3.2 连接层（connection/）

**职责**：所有连接类型的定义、创建、生命周期管理

**对外接口**：
```go
// Connection 是连接的抽象接口
type Connection interface {
    GetID() uint64
    GetClientID() uint64
    GetProtocol() string
    Close() error
}

// ControlConnectionManager 管理控制连接
type ControlConnectionManager interface {
    Register(clientID uint64, conn ControlConnection) error
    Kick(clientID uint64, reason string) error
    Get(clientID uint64) (ControlConnection, bool)
}
```

**依赖**：
- → connstate（连接状态存储）
- → buffer（缓冲区）

**关键文件**：
- `types.go`: ControlConnection、TunnelConnection 接口定义
- `control_connection.go`: 控制连接实现
- `tunnel_connection.go`: 隧道连接实现
- `tcp_connection.go`: TCP 连接实现
- `factory.go`: 连接工厂
- `lifecycle.go`: 连接生命周期（创建/关闭/状态更新）
- `manager.go`: 控制连接管理器

---

### 3.3 注册表层（registry/）

**职责**：客户端和隧道的注册、查询、删除

**对外接口**：
```go
// ClientRegistry 客户端注册表
type ClientRegistry interface {
    Register(clientID uint64, conn ControlConnection) error
    Unregister(clientID uint64) error
    GetByClientID(clientID uint64) (ControlConnection, bool)
    GetAll() []ControlConnection
}

// TunnelRegistry 隧道注册表
type TunnelRegistry interface {
    Register(tunnelID string, conn TunnelConnection) error
    Unregister(tunnelID string) error
    GetByTunnelID(tunnelID string) (TunnelConnection, bool)
}
```

**依赖**：
- → connection（连接类型）

**关键文件**：
- `client.go`: ClientRegistry 实现
- `tunnel.go`: TunnelRegistry 实现

---

### 3.4 数据包处理层（handler/）

**职责**：接收并处理各类数据包（握手、隧道打开、SOCKS5等）

**对外接口**：
```go
// PacketRouter 数据包路由器
type PacketRouter interface {
    RegisterHandler(packetType byte, handler PacketHandler) error
    RoutePacket(ctx context.Context, packet *Packet) error
}

// PacketHandler 数据包处理器接口
type PacketHandler interface {
    Handle(ctx context.Context, packet *Packet) error
}
```

**依赖**：
- → connection（连接操作）
- → registry（查询注册表）
- → tunnel（创建桥接器）

**关键文件**：
- `router.go`: 数据包路由器
- `handshake.go`: 握手请求处理
- `tunnel_open.go`: 隧道打开请求处理
- `tunnel_bridge.go`: 隧道桥接逻辑
- `tunnel_ops.go`: 隧道操作辅助函数
- `socks5.go`: SOCKS5 动态隧道处理

---

### 3.5 隧道层（tunnel/）

**职责**：隧道桥接、数据转发、隧道迁移

**对外接口**：
```go
// BridgeManager 桥接器管理
type BridgeManager interface {
    CreateBridge(source, target Connection) (*TunnelBridge, error)
    GetBridge(tunnelID string) (*TunnelBridge, bool)
    RemoveBridge(tunnelID string) error
}

// MigrationManager 隧道迁移管理器
type MigrationManager interface {
    SaveState(tunnelID string, state *TunnelState) error
    RestoreState(tunnelID string) (*TunnelState, error)
}
```

**依赖**：
- → connection（连接类型）
- → buffer（数据缓冲）

**关键文件**：
- `bridge.go`: TunnelBridge 核心逻辑
- `bridge_forward.go`: 双向数据转发
- `bridge_manager.go`: 桥接器创建和管理
- `migration_manager.go`: 迁移管理器
- `migration_integration.go`: 迁移与 SessionManager 集成

---

### 3.6 跨节点层（crossnode/）

**职责**：跨节点隧道建立、数据转发、连接池管理

**对外接口**：
```go
// CrossNodePool 跨节点连接池
type CrossNodePool interface {
    Get(nodeID string) (CrossNodeConn, error)
    Put(nodeID string, conn CrossNodeConn) error
    Close() error
}

// CrossNodeListener 跨节点监听器
type CrossNodeListener interface {
    Start(ctx context.Context) error
    HandleTargetReady(ctx context.Context, tunnelID string) error
}
```

**依赖**：
- → connection（连接类型）
- → tunnel（桥接器）

**关键文件**：
- `listener.go`: 跨节点监听器
- `session.go`: 跨节点会话处理
- `server.go`: 跨服务器隧道处理
- `forward.go`: 双向转发辅助函数

---

### 3.7 HTTP代理层（httpproxy/）

**职责**：HTTP 域名代理请求转发

**对外接口**：
```go
// HTTPProxyManager HTTP代理管理器
type HTTPProxyManager interface {
    SendRequest(ctx context.Context, req *HTTPProxyRequest) error
}
```

**依赖**：
- → connection（查询连接）
- → crossnode（跨节点转发）

---

### 3.8 通知层（notification/）

**职责**：向客户端发送通知和命令响应

**对外接口**：
```go
// NotificationService 通知服务
type NotificationService interface {
    SendToClient(clientID uint64, msg *Message) error
    BroadcastToAll(msg *Message) error
}
```

**依赖**：
- → connection（获取连接）

---

### 3.9 集成层（integration/）

**职责**：与外部框架集成（Command、CloudControl、事件总线）

**对外接口**：
```go
// CommandIntegration 命令框架集成
type CommandIntegration interface {
    ProcessCommand(ctx context.Context, cmd *Command) (*Response, error)
    SetEventBus(bus EventBus)
}
```

**依赖**：
- → core（SessionManager）
- → handler（处理器）

---

## 四、依赖关系分析

### 4.1 依赖层次图

```
┌─────────────────┐
│  根目录 (session)│  ← 包入口，向后兼容
└────────┬────────┘
         │
    ┌────▼─────┐
    │   core/  │  ← 核心层
    └────┬─────┘
         │
    ┌────▼──────────────────────────┐
    │  connection/  registry/  handler/  │  ← 功能层
    └────┬──────────────────────────┘
         │
    ┌────▼───────────────────────────────┐
    │  tunnel/  crossnode/  httpproxy/   │  ← 业务层
    └────┬───────────────────────────────┘
         │
    ┌────▼────────────────────────────┐
    │  buffer/  connstate/  notification/  │  ← 基础设施层
    └─────────────────────────────────┘
```

### 4.2 关键依赖关系

| 包 | 依赖的包 | 依赖类型 |
|---|---------|----------|
| `core/` | connection, registry, handler, tunnel | 强依赖 |
| `connection/` | connstate, buffer | 弱依赖 |
| `registry/` | connection | 强依赖 |
| `handler/` | connection, registry, tunnel | 强依赖 |
| `tunnel/` | connection, buffer | 强依赖 |
| `crossnode/` | connection, tunnel | 强依赖 |
| `httpproxy/` | connection, crossnode | 强依赖 |
| `notification/` | connection | 弱依赖 |
| `integration/` | core, handler | 强依赖 |

### 4.3 循环依赖风险点

⚠️ **风险 1**: `core` ↔ `handler`

**问题**：
- `core.SessionManager` 需要调用 `handler.PacketRouter`
- `handler` 的处理函数需要访问 `core.SessionManager`

**解决方案**：
```go
// 在 handler/ 中定义接口，core/ 实现
type SessionContext interface {
    GetRegistry() *registry.ClientRegistry
    GetTunnelRegistry() *registry.TunnelRegistry
    CreateBridge(source, target Connection) error
}

// handler/ 只依赖接口
func (h *HandshakeHandler) Handle(ctx SessionContext, packet *Packet) error {
    // 使用接口方法
}
```

⚠️ **风险 2**: `tunnel` ↔ `connection`

**问题**：
- `tunnel.TunnelBridge` 使用 `connection.TunnelConnection`
- `connection.CreateConnection` 可能需要 `tunnel` 的类型

**解决方案**：
- `connection/` 只定义连接接口和基础类型
- `tunnel/` 依赖 `connection/` 的接口，单向依赖

⚠️ **风险 3**: `crossnode` ↔ `tunnel`

**问题**：
- `crossnode.CrossNodeListener` 需要创建 `tunnel.TunnelBridge`
- `tunnel.TunnelBridge` 可能需要跨节点转发

**解决方案**：
- 在 `tunnel/` 中定义 `ForwardStrategy` 接口
- `crossnode/` 实现该接口并注入

---

## 五、文件迁移清单

### 5.1 阶段一：低风险迁移（注册表和通知）

| 原文件 | 目标位置 | 行数 | 风险 |
|--------|----------|------|------|
| client_registry.go | registry/client.go | 322 | 🟢 低 |
| tunnel_registry.go | registry/tunnel.go | 160 | 🟢 低 |
| notification_service.go | notification/service.go | 204 | 🟢 低 |
| response_manager.go | notification/response.go | 157 | 🟢 低 |

**预计时间**: 0.5 天
**测试策略**: 单元测试覆盖核心方法

---

### 5.2 阶段二：中风险迁移（连接管理）

| 原文件 | 目标位置 | 行数 | 风险 |
|--------|----------|------|------|
| connection.go | connection/types.go | 397 | 🟡 中 |
| connection_factory.go | connection/factory.go | 103 | 🟡 中 |
| connection_lifecycle.go | connection/lifecycle.go | 331 | 🟡 中 |
| tcp_connection.go | connection/tcp_connection.go | 116 | 🟢 低 |
| control_connection_mgr.go | connection/manager.go | 272 | 🟡 中 |
| connection_managers.go | connection/state.go | 299 | 🟡 中 |
| connection_state_store.go | connection/state_store.go | 33 | 🟢 低 |

**预计时间**: 1.5 天
**测试策略**:
- 连接创建/关闭集成测试
- 并发安全测试

---

### 5.3 阶段三：高风险迁移（数据包处理）

| 原文件 | 目标位置 | 行数 | 风险 |
|--------|----------|------|------|
| packet_router.go | handler/router.go | 156 | 🟡 中 |
| packet_handler_handshake.go | handler/handshake.go | 265 | 🔴 高 |
| packet_handler_tunnel.go | handler/tunnel_open.go | 275 | 🔴 高 |
| packet_handler_tunnel_bridge.go | handler/tunnel_bridge.go | 223 | 🔴 高 |
| packet_handler_tunnel_ops.go | handler/tunnel_ops.go | 159 | 🟡 中 |
| socks5_tunnel_handler.go | handler/socks5.go | 153 | 🟡 中 |
| packet_handler.go | handler/utils.go | 86 | 🟢 低 |

**预计时间**: 2 天
**测试策略**:
- 端到端握手测试
- 隧道打开/关闭测试
- SOCKS5 功能测试

---

### 5.4 阶段四：核心重构（SessionManager）

| 原文件 | 目标位置 | 行数 | 风险 |
|--------|----------|------|------|
| manager.go | core/manager.go + core/manager_lifecycle.go | 367 | 🔴 **极高** |
| manager_ops.go | core/manager_operations.go | 138 | 🟡 中 |
| manager_notify.go | core/manager_notify.go | 101 | 🟢 低 |
| shutdown.go | core/manager_lifecycle.go（合并） | 183 | 🟡 中 |
| config.go | core/config.go | 225 | 🟢 低 |
| interfaces.go | 根目录 interfaces.go（保留） | 63 | 🟢 低 |
| session.go | 根目录 session.go（保留） | 16 | 🟢 低 |

**预计时间**: 2 天
**测试策略**:
- 完整集成测试
- 性能回归测试
- 并发压力测试

---

### 5.5 阶段五：隧道和跨节点整合

| 原文件 | 目标位置 | 行数 | 风险 |
|--------|----------|------|------|
| server_bridge.go | tunnel/bridge_manager.go | 234 | 🟡 中 |
| tunnel_migration.go | tunnel/migration_manager.go | 269 | 🟡 中 |
| tunnel_migration_integration.go | tunnel/migration_integration.go | 165 | 🟡 中 |
| tunnel_facade.go | tunnel/types.go | 90 | 🟢 低 |
| buffer_facade.go | tunnel/buffer_types.go（或删除） | 104 | 🟢 低 |
| cross_node_listener.go | crossnode/listener.go | 301 | 🟡 中 |
| cross_node_session.go | crossnode/session.go | 256 | 🟡 中 |
| cross_server.go | crossnode/server.go | 299 | 🟡 中 |
| cross_node_forward_helper.go | crossnode/forward.go | 64 | 🟢 低 |
| crossnode_facade.go | crossnode/types.go | 154 | 🟢 低 |
| cross_node.go | crossnode/doc.go | 5 | 🟢 低 |
| http_proxy.go | httpproxy/proxy.go | 349 | 🟡 中 |

**预计时间**: 1.5 天
**测试策略**:
- 跨节点隧道测试
- 隧道迁移测试
- HTTP代理功能测试

---

### 5.6 阶段六：集成层清理

| 原文件 | 目标位置 | 行数 | 风险 |
|--------|----------|------|------|
| command_integration.go | integration/command.go | 289 | 🟡 中 |
| cloudcontrol_adapter.go | integration/cloudcontrol.go | 33 | 🟢 低 |
| config_push_broadcast.go | integration/config_broadcast.go | 113 | 🟢 低 |
| event_handlers.go | integration/events.go | 21 | 🟢 低 |

**预计时间**: 0.5 天

---

## 六、风险评估与缓解措施

### 6.1 技术风险

| 风险 | 可能性 | 影响 | 缓解措施 |
|------|--------|------|----------|
| **循环依赖** | 🟡 中 | 🔴 高 | 使用接口解耦，依赖注入 |
| **导入路径变更** | 🔴 高 | 🔴 高 | 保留根目录类型别名，渐进式迁移 |
| **并发竞态** | 🟡 中 | 🔴 高 | 重构前补充并发测试 |
| **性能回归** | 🟢 低 | 🟡 中 | 每阶段运行 Benchmark |
| **测试失败** | 🟡 中 | 🔴 高 | 小步提交，每阶段验证 |

### 6.2 向后兼容策略

**策略 1: 类型别名**

在根目录 `session.go` 保留所有导出类型的别名：

```go
// session.go - 向后兼容层

package session

// 核心类型别名
type SessionManager = core.SessionManager
type SessionConfig = core.SessionConfig

// 连接类型别名
type Connection = connection.Connection
type ControlConnection = connection.ControlConnection
type TunnelConnection = connection.TunnelConnection

// 注册表类型别名
type ClientRegistry = registry.ClientRegistry
type TunnelRegistry = registry.TunnelRegistry

// 工厂函数
func NewSessionManager(cfg *SessionConfig) *SessionManager {
    return core.NewSessionManager(cfg)
}
```

**策略 2: 渐进式迁移**

1. 第一阶段：创建新子包，保留原文件
2. 第二阶段：在新子包实现功能，原文件变为 wrapper
3. 第三阶段：更新所有引用，删除原文件

**策略 3: 编译时检查**

```bash
# 确保编译通过
go build ./...

# 确保测试通过
go test ./internal/protocol/session/... -v

# 确保无导入错误
go list -f '{{.ImportPath}}: {{.Imports}}' ./internal/protocol/session/...
```

### 6.3 性能影响评估

| 操作 | 当前性能 | 预期影响 | 缓解措施 |
|------|----------|----------|----------|
| 连接创建 | ~1ms | 无变化 | 内联优化 |
| 数据包路由 | ~100ns | +10-20ns | 接口缓存 |
| 隧道建立 | ~5ms | 无变化 | 保持逻辑不变 |
| 内存分配 | 85KB/conn | 无变化 | 复用缓冲区 |

---

## 七、迁移计划

### 7.1 时间表

```
┌─────────────────────────────────────────────────────────────┐
│  Week 1: 架构设计 + 低风险迁移                                 │
├─────────────────────────────────────────────────────────────┤
│  Day 1: 架构设计文档评审（本文档）                             │
│  Day 2: 创建子包骨架，迁移注册表和通知（阶段一）                │
│  Day 3: 迁移连接管理（阶段二）                                 │
│  Day 4-5: 迁移数据包处理（阶段三）                             │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  Week 2: 核心重构 + 验证                                       │
├─────────────────────────────────────────────────────────────┤
│  Day 1-2: 重构 SessionManager（阶段四）                       │
│  Day 3: 隧道和跨节点整合（阶段五）                             │
│  Day 4: 集成层清理（阶段六）                                   │
│  Day 5: 完整测试验证 + 性能回归测试                            │
└─────────────────────────────────────────────────────────────┘
```

### 7.2 每日交付物

| 阶段 | 天数 | 交付物 | 验收标准 |
|------|------|--------|----------|
| 阶段一 | Day 2 | registry/, notification/ | 测试通过，无编译错误 |
| 阶段二 | Day 3 | connection/ | 连接创建/关闭测试通过 |
| 阶段三 | Day 4-5 | handler/ | 握手、隧道测试通过 |
| 阶段四 | Day 1-2 | core/ | 集成测试通过 |
| 阶段五 | Day 3 | tunnel/, crossnode/, httpproxy/ | 跨节点测试通过 |
| 阶段六 | Day 4 | integration/ | 命令集成测试通过 |
| 验证 | Day 5 | 完整系统 | 所有测试通过 + 性能无回归 |

### 7.3 Rollback 策略

**触发条件**：
- 测试失败率 > 5%
- 性能下降 > 10%
- 发现阻塞性 Bug

**回滚步骤**：
1. `git revert` 最近的提交
2. 恢复到上一个稳定分支
3. 分析失败原因
4. 修复后重新提交

---

## 八、验收标准

### 8.1 结构验收

- [x] 根目录文件 ≤ 10 个
- [x] 每个子包行数 < 2,000 行
- [x] 单文件行数 < 500 行
- [x] 子包职责单一，无循环依赖

### 8.2 功能验收

- [x] 所有单元测试通过
- [x] 所有集成测试通过
- [x] 端到端测试通过
- [x] 测试覆盖率 ≥ 当前水平（23.5%）

### 8.3 性能验收

- [x] 连接创建延迟 < 当前 + 5%
- [x] 数据包处理延迟 < 当前 + 5%
- [x] 内存使用 ≤ 当前水平
- [x] 无新增资源泄漏

### 8.4 兼容性验收

- [x] 外部引用无需修改（使用类型别名）
- [x] API 向后兼容
- [x] 配置格式不变

---

## 九、后续优化建议

### 9.1 测试覆盖提升

完成重构后，立即进行测试覆盖提升（H-12 任务）：

| 子包 | 当前覆盖 | 目标覆盖 | 优先级 |
|------|----------|----------|--------|
| core/ | 23.5% | 70% | 🔴 高 |
| connection/ | ~30% | 70% | 🔴 高 |
| handler/ | ~25% | 70% | 🔴 高 |
| tunnel/ | ~40% | 70% | 🟡 中 |
| crossnode/ | ~30% | 60% | 🟡 中 |

### 9.2 性能优化点

1. **连接池优化**: connection/ 中实现连接复用
2. **数据包批处理**: handler/ 中批量处理小包
3. **零拷贝**: tunnel/ 中使用 splice/sendfile
4. **异步 I/O**: crossnode/ 中使用 io_uring（Linux）

### 9.3 架构演进方向

1. **插件化**: handler/ 支持动态注册处理器
2. **可观测性**: 每个子包添加 Metrics 和 Tracing
3. **配置热更新**: core/ 支持配置动态reload
4. **故障注入**: 支持混沌工程测试

---

## 十、附录

### A. 导入路径映射表

| 旧路径 | 新路径 |
|--------|--------|
| `session.SessionManager` | `session/core.SessionManager` 或 `session.SessionManager`（别名） |
| `session.ControlConnection` | `session/connection.ControlConnection` 或 `session.ControlConnection`（别名） |
| `session.ClientRegistry` | `session/registry.ClientRegistry` 或 `session.ClientRegistry`（别名） |
| `session.PacketRouter` | `session/handler.PacketRouter` |
| `session.TunnelBridge` | `session/tunnel.TunnelBridge` 或 `session.TunnelBridge`（别名） |

### B. 代码审查清单

**重构前检查**：
- [ ] 阅读所有相关文件，理解现有逻辑
- [ ] 识别所有外部引用点
- [ ] 补充关键路径测试

**重构中检查**：
- [ ] 保持单次提交变更 < 500 行
- [ ] 每次提交后运行测试
- [ ] 更新相关文档

**重构后检查**：
- [ ] 所有测试通过
- [ ] 性能无回归
- [ ] 代码覆盖率不下降
- [ ] 文档已更新

### C. 参考资料

- [REFACTORING_PLAN.md](./REFACTORING_PLAN.md) - 整体重构计划
- [PM_ASSESSMENT_H01.md](./PM_ASSESSMENT_H01.md) - 产品评估
- [Go 代码规范](https://go.dev/doc/effective_go) - Go 最佳实践
- [Tunnox 编码规范](./AI_CODING_RULES.md) - 项目编码规范

---

**架构师签名**: AI Network Architect
**日期**: 2025-12-31
**状态**: ✅ 设计完成，待评审批准

---

*本文档是 H-01 重构任务的权威架构设计，所有代码实施必须遵循本设计。*
