# 集群滚动更新快速恢复方案

## 问题场景

### 现状分析
1. **ClientA 双连接模型**
   - 指令通道（ControlConnection）→ Server-1
   - 映射通道（TunnelConnection）→ Server-2
   
2. **滚动更新过程**
   ```
   初始: Server-1, Server-2, Server-3 (全部运行)
   步骤1: 停止 Server-1 → ClientA 指令通道断开
   步骤2: 启动 Server-1'（新版本）
   步骤3: 停止 Server-2 → ClientA 映射通道断开
   步骤4: 启动 Server-2'（新版本）
   步骤5: 停止 Server-3
   步骤6: 启动 Server-3'（新版本）
   ```

3. **问题点**
   - 指令通道断开 → 无法接收配置推送
   - 映射通道断开 → 数据透传中断
   - 重连时间过长 → 服务恢复慢
   - 状态丢失 → 重连后需重新获取配置

## 快速恢复方案

### 方案A：服务端优雅关闭 + 客户端快速重连（推荐）

#### 1. 服务端优雅关闭增强

**现有机制**
- ✅ 已有：`GracefulShutdownTimeout = 30秒`
- ✅ 已有：Dispose 体系资源清理
- ❌ 缺失：主动通知客户端迁移

**增强措施**

##### 1.1 优雅关闭流程
```go
// 滚动更新触发优雅关闭
SIGTERM 信号 → 
    ↓
1. 停止接受新连接（Nginx健康检查失败）
    ↓
2. 发送 ServerShutdown 通知给所有控制连接
    ↓
3. 等待客户端重连到其他节点（最多10秒）
    ↓
4. 关闭剩余连接
    ↓
5. 清理资源
```

##### 1.2 Nginx 健康检查配置
```nginx
upstream tunnox_tcp {
    server server-1:8080 max_fails=1 fail_timeout=5s;
    server server-2:8080 max_fails=1 fail_timeout=5s;
    server server-3:8080 max_fails=1 fail_timeout=5s;
}

# 健康检查（需要nginx-plus或使用lua）
location /health {
    # Server在收到SIGTERM后返回503
    proxy_pass http://backend/health;
}
```

##### 1.3 新增 ServerShutdown 命令
```go
// internal/protocol/packet/command.go
const ServerShutdown CommandType = 8 // 服务器即将关闭

// 命令体
type ServerShutdownCommand struct {
    Reason        string `json:"reason"`         // "rolling_update"
    ReconnectTo   string `json:"reconnect_to"`   // "server-2:8080" (可选，负载均衡由nginx处理)
    GracePeriod   int    `json:"grace_period"`   // 10 (秒)
}
```

#### 2. 客户端快速重连机制

##### 2.1 重连策略（指数退避 + 快速失败检测）
```go
type ReconnectConfig struct {
    // 快速失败检测
    HealthCheckInterval time.Duration // 5秒（心跳间隔）
    FailureThreshold    int           // 2次失败后触发重连
    
    // 重连参数
    InitialDelay        time.Duration // 100ms（首次重连延迟）
    MaxDelay            time.Duration // 5s（最大延迟）
    BackoffMultiplier   float64       // 1.5（退避倍数）
    MaxRetries          int           // 无限重试（或30次）
    
    // 连接超时
    DialTimeout         time.Duration // 3秒
}

// 重连流程
失败检测 → 
    ↓
立即尝试重连（延迟100ms）→ 失败 →
    ↓
第2次重连（延迟150ms）→ 失败 →
    ↓
第3次重连（延迟225ms）→ 失败 →
    ↓
...
    ↓
第N次重连（延迟5s，达到上限）
```

##### 2.2 双连接独立重连
```go
// 指令通道和映射通道独立管理
type ClientConnectionManager struct {
    controlConn  *ControlChannelManager  // 指令通道管理器
    tunnelConns  *TunnelChannelPool      // 映射通道池
}

// 各自独立重连
- 指令通道断开 → 立即重连 → 恢复心跳和命令接收
- 映射通道断开 → 按需重建（当有数据需要传输时）
```

##### 2.3 状态持久化和恢复
```go
// 客户端本地缓存关键状态
type ClientState struct {
    ClientID       int64              `json:"client_id"`
    AuthToken      string             `json:"auth_token"`
    Mappings       []PortMapping      `json:"mappings"`        // 端口映射配置
    LastConfigHash string             `json:"last_config_hash"` // 配置版本
    
    // 不持久化：运行时状态
    // - 活跃连接（重连后重建）
    // - 统计数据（丢失可接受）
}

// 重连后立即恢复
1. 使用缓存的 AuthToken 认证
2. 对比 LastConfigHash，如不一致则拉取最新配置
3. 根据 Mappings 重建映射通道（按需）
```

#### 3. 跨服务器状态同步

##### 3.1 Redis 存储关键状态（已实现）
```
✅ ClientRuntimeState (Redis缓存)
  - NodeID: server-1 → server-2 (自动更新)
  - ConnID: conn_xxx → conn_yyy (自动更新)
  - Status: online
  - LastSeen: 2025-11-28 10:00:00

✅ ClientConfig (Redis持久化)
  - Mappings: [...] (不变)
  - AuthToken: xxx (不变)
```

##### 3.2 状态迁移流程
```
ClientA 从 Server-1 → Server-2

1. Server-1 收到 SIGTERM
   ↓
2. Server-1 发送 ServerShutdown 给 ClientA
   ↓
3. ClientA 断开连接，触发重连
   ↓
4. Nginx 将新连接路由到 Server-2
   ↓
5. ClientA 在 Server-2 认证成功
   ↓
6. Server-2 更新 Redis:
   - ClientRuntimeState.NodeID = "server-2"
   - ClientRuntimeState.ConnID = "conn_new"
   - ClientRuntimeState.LastSeen = now()
   ↓
7. Server-2 推送配置（如需要）
   ↓
8. 恢复完成（耗时 < 2秒）
```

### 方案B：连接迁移（预热新节点）

#### 流程
```
1. 新版本 Server-1' 启动
   ↓
2. Nginx 将其加入负载均衡池
   ↓
3. 等待 30秒（预热期，接收新连接）
   ↓
4. 向 Server-1 发送迁移信号
   ↓
5. Server-1 通知所有客户端 ServerShutdown
   ↓
6. 客户端重连到 Server-1' 或其他节点
   ↓
7. 所有客户端迁移完成后（或超时10秒）
   ↓
8. Server-1 优雅关闭
```

**优点**：
- 新节点预热，减少启动压力
- 客户端有更多时间迁移
- 可控的流量切换

**缺点**：
- 更新时间更长（每个节点 +30秒）
- 需要额外的协调机制

## 实施优先级

### Phase 1: 服务端优雅关闭增强（高优先级）
1. ✅ 已有：Dispose 体系
2. ✅ 已有：GracefulShutdownTimeout
3. ❌ **新增：ServerShutdown 命令**
4. ❌ **新增：健康检查端点**（返回 draining 状态）
5. ❌ **新增：优雅关闭流程**（通知客户端 → 等待迁移 → 关闭）

**预期效果**：
- 客户端提前感知服务器关闭
- 避免突然断线导致的重试延迟
- 恢复时间从 5-10秒 → **1-2秒**

### Phase 2: 客户端快速重连（中优先级）
1. ❌ **新增：ReconnectConfig 配置**
2. ❌ **新增：指数退避重连**
3. ❌ **新增：本地状态缓存**
4. ❌ **新增：快速失败检测**（心跳超时 → 立即重连）

**预期效果**：
- 重连延迟从秒级 → **毫秒级**（首次100ms）
- 失败检测时间从 30秒 → **10秒**（2次心跳失败）

### Phase 3: 高级优化（低优先级）
1. 连接预热（新节点启动后等待流量）
2. 连接池复用（映射通道复用）
3. 配置差量同步（仅同步变更部分）

## 关键指标

| 指标 | 当前 | 目标 | 方案 |
|------|------|------|------|
| 重连延迟 | 5-10秒 | < 2秒 | ServerShutdown + 快速重连 |
| 失败检测 | 30-90秒 | < 10秒 | 心跳超时清理（已实现） |
| 配置恢复 | 手动 | 自动 | 本地缓存 + 差量同步 |
| 数据丢失 | 有 | 无 | 映射通道独立重连 |
| 滚动更新时长 | 5分钟 | 3分钟 | 优雅关闭 + 快速重连 |

## 测试场景

### 1. 单节点滚动更新
```bash
1. 3节点集群运行中，100个客户端连接均匀分布
2. 执行滚动更新：
   - 停止 Server-1 (SIGTERM)
   - 等待 Server-1 客户端迁移完成
   - 启动 Server-1'
   - 重复 Server-2, Server-3
3. 验证指标：
   - ✅ 所有客户端在 2秒内恢复连接
   - ✅ 无数据丢失
   - ✅ 配置自动恢复
```

### 2. 跨节点映射通道
```bash
1. ClientA 指令通道 → Server-1
2. ClientA 映射通道 → Server-2
3. 停止 Server-2
4. 验证：
   - ✅ ClientA 映射通道重连到 Server-3
   - ✅ 数据传输恢复
   - ✅ 延迟 < 2秒
```

## 实施步骤

1. **设计方案**（已完成）✅
2. **实现 ServerShutdown 命令**
3. **实现健康检查端点**
4. **实现优雅关闭流程**
5. **客户端重连机制**（如客户端代码由你维护）
6. **E2E 测试**
7. **性能测试**（100+ 客户端滚动更新）

## 推荐方案

**采用方案A（优雅关闭 + 快速重连）**

理由：
1. ✅ 实现简单，改动小
2. ✅ 对现有架构影响最小
3. ✅ 性能提升显著（1-2秒恢复）
4. ✅ 适用于所有更新场景（不仅是滚动更新）
5. ✅ 客户端可独立升级重连逻辑

先实施服务端增强（Phase 1），立即见效。
客户端优化（Phase 2）可后续迭代。

