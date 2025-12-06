# Tunnox 核心术语定义

本文档定义了 Tunnox 项目中的核心概念和术语，用于统一代码、文档和 API 中的命名。

## 核心概念

### Mapping（映射）
**定义：** 用户配置的端口映射规则，对外称为"隧道"。

**特征：**
- 由用户创建和配置
- 包含源端口、目标地址、协议等信息
- 持久化存储（数据库或文件）
- 可以有多个运行时实例（Tunnel）

**示例：**
- 端口映射：`localhost:8080 -> remote:3306`
- 域名映射：`example.com -> remote:80`

**相关代码：**
- `internal/cloud/models/port_mapping.go`
- `internal/api/handlers_mapping.go`

---

### Tunnel（隧道）
**定义：** Mapping 的运行时实例，包含连接状态和活跃连接。

**特征：**
- 基于 Mapping 创建
- 包含运行时状态（连接数、流量统计等）
- 可以有多个活跃连接（Connection）
- 生命周期：创建 -> 运行 -> 关闭

**与 Mapping 的关系：**
- 一个 Mapping 可以有多个 Tunnel（多客户端场景）
- Tunnel 是 Mapping 的运行时表现

**相关代码：**
- `internal/cloud/models/tunnel.go`
- `internal/protocol/session/`

---

### Bridge（桥接）
**定义：** 跨节点的连接转发机制。

**特征：**
- 用于多节点部署场景
- 在不同节点间转发连接
- 支持负载均衡和故障转移

**使用场景：**
- 多服务器节点部署
- 跨地域连接转发

**相关代码：**
- `internal/cloud/bridge/`
- `internal/app/server/config.go` (BridgePoolConfig)

---

### Session（会话）
**定义：** 客户端与服务器的控制连接。

**特征：**
- 用于控制命令和状态同步
- 每个客户端有一个 Session
- 可以包含多个 Tunnel
- 生命周期：握手 -> 运行 -> 断开

**与 Tunnel 的关系：**
- Session 是控制层
- Tunnel 是数据层
- 一个 Session 可以管理多个 Tunnel

**相关代码：**
- `internal/protocol/session/`
- `internal/cloud/session/`

---

### Connection（连接）
**定义：** 具体的网络连接，可以是控制连接或数据连接。

**特征：**
- 控制连接：用于命令和状态同步
- 数据连接：用于实际数据传输
- 每个连接有唯一 ID
- 支持多种协议（TCP、WebSocket、QUIC、HTTP-Poll）

**类型：**
- **Control Connection：** 控制连接，属于 Session
- **Data Connection：** 数据连接，属于 Tunnel

**相关代码：**
- `internal/core/types/connection.go`
- `internal/protocol/adapter/`

---

### Node（节点）
**定义：** 服务器实例。

**特征：**
- 每个服务器是一个 Node
- 有唯一的 NodeID
- 可以管理多个客户端和隧道
- 支持多节点集群部署

**相关代码：**
- `internal/cloud/node/`
- `internal/app/server/config.go` (NodeID)

---

### Client（客户端）
**定义：** 连接到服务器的客户端实例。

**特征：**
- 每个客户端有唯一的 ClientID
- 可以创建多个 Tunnel
- 通过 Session 与服务器通信
- 支持多种连接协议

**类型：**
- **Target Client：** 目标客户端，提供目标服务
- **Listen Client：** 监听客户端，接收连接请求

**相关代码：**
- `internal/client/`
- `internal/cloud/models/client.go`

---

## 概念层级关系

```
Node (服务器节点)
  └── Session (客户端会话)
      ├── Control Connection (控制连接)
      └── Tunnel (隧道实例)
          ├── Data Connection (数据连接)
          └── Mapping (映射规则)
```

## 命名规范

### API 路径
- `/api/v1/mappings` - 映射管理
- `/api/v1/tunnels` - 隧道管理
- `/api/v1/sessions` - 会话管理
- `/api/v1/connections` - 连接管理

### 代码命名
- `PortMapping` - 端口映射模型
- `Tunnel` - 隧道模型
- `Session` - 会话模型
- `Connection` - 连接模型

### 存储键前缀
- `tunnox:mapping:` - 映射数据
- `tunnox:tunnel:` - 隧道数据
- `tunnox:session:` - 会话数据
- `tunnox:connection:` - 连接数据

---

## 历史遗留术语（已废弃）

以下术语已废弃，不应在新代码中使用：

- `PortMapping` → 使用 `Mapping`
- `TunnelMapping` → 使用 `Tunnel`
- `BridgeConnection` → 使用 `Bridge` 或 `Connection`

---

## 参考

- 架构设计文档：`docs/ARCHITECTURE_DESIGN_V2.2.md`
- 代码审查文档：`docs/chatgpt5_review.md`

