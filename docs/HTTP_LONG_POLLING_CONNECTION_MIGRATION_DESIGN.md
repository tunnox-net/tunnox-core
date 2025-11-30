# HTTP Long Polling 连接迁移设计

## 问题背景

在 HTTP Long Polling 协议中，客户端首次连接时使用 `clientID=0` 进行握手，服务器分配实际 `clientID` 后，需要将临时连接迁移到实际 `clientID`。

当前问题：
- 迁移逻辑分散在 `handleHTTPPush` 和 `handleHTTPPoll` 中，是延迟的
- 客户端在握手后立即使用新 `clientID` 发送请求，可能找不到连接
- 架构上像是打补丁，不够优雅

## 设计目标

1. **适配层封装**：迁移逻辑完全封装在适配层（`ServerHTTPLongPollingConn`）内部
2. **自动触发**：握手成功后自动触发迁移，无需外部干预
3. **解耦设计**：适配层通过回调机制与连接管理器交互，保持职责清晰

## 架构设计

### 核心思想

```
┌─────────────────────────────────────────────────────────┐
│              SessionManager (协议无关层)                  │
│  - 处理握手、数据包路由等通用逻辑                          │
└─────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────┐
│         ServerHTTPLongPollingConn (适配层)               │
│  - 实现 net.Conn 接口                                    │
│  - 维护 Base64 编解码                                    │
│  - 维护连接状态（clientID）                              │
│  - 在 UpdateClientID 时自动触发迁移回调                   │
└─────────────────────────────────────────────────────────┘
                        ↓
┌─────────────────────────────────────────────────────────┐
│      httppollConnectionManager (连接管理层)               │
│  - 管理临时连接（clientID=0）和正式连接映射                │
│  - 提供迁移回调函数                                       │
│  - 在创建连接时注入回调                                   │
└─────────────────────────────────────────────────────────┘
```

### 关键组件

#### 1. ServerHTTPLongPollingConn（适配层）

```go
type ServerHTTPLongPollingConn struct {
    clientID int64
    
    // 迁移回调：当 clientID 从 0 变为非 0 时自动调用
    migrationCallback func(connID string, oldClientID, newClientID int64)
    
    // ... 其他字段
}

// UpdateClientID 更新客户端 ID（握手后调用）
func (c *ServerHTTPLongPollingConn) UpdateClientID(newClientID int64) {
    oldClientID := c.clientID
    c.clientID = newClientID
    
    // 自动触发迁移：从临时连接（0）迁移到正式连接（非0）
    if oldClientID == 0 && newClientID > 0 && c.migrationCallback != nil {
        // 获取 ConnectionID（通过某种方式，比如从 context 中获取）
        connID := c.getConnectionID()
        c.migrationCallback(connID, oldClientID, newClientID)
    }
}
```

#### 2. httppollConnectionManager（连接管理层）

```go
type httppollConnectionManager struct {
    // 临时连接映射（clientID=0）
    tempConnections map[string]*ServerHTTPLongPollingConn
    
    // 正式连接映射（clientID > 0）
    connections map[int64]*ServerHTTPLongPollingConn
    
    // IP 映射（用于 clientID=0 时匹配 push 和 poll）
    tempConnectionsByIP map[string]*ServerHTTPLongPollingConn
}

// createMigrationCallback 创建迁移回调函数
func (m *httppollConnectionManager) createMigrationCallback(connID string) func(string, int64, int64) {
    return func(actualConnID string, oldClientID, newClientID int64) {
        m.migrateTempToClientID(actualConnID, newClientID)
    }
}

// migrateTempToClientID 执行迁移
func (m *httppollConnectionManager) migrateTempToClientID(connID string, clientID int64) {
    // 从临时连接映射中移除
    conn, exists := m.tempConnections[connID]
    if !exists {
        return
    }
    
    delete(m.tempConnections, connID)
    
    // 添加到正式连接映射
    if clientID > 0 {
        if oldConn, exists := m.connections[clientID]; exists && oldConn != conn {
            oldConn.Close()
        }
        m.connections[clientID] = conn
    }
}
```

#### 3. 连接创建流程

```go
func (s *ManagementAPIServer) getOrCreateHTTPLongPollingConn(...) *ServerHTTPLongPollingConn {
    // 1. 创建连接
    httppollConn := session.NewServerHTTPLongPollingConn(serverCtx, clientID)
    
    // 2. 创建连接（通过 SessionManager）
    conn, err := sessionMgrWithConn.CreateConnection(httppollConn, httppollConn)
    
    // 3. 如果是临时连接（clientID=0），注册并注入迁移回调
    if clientID == 0 {
        s.httppollConnMgr.registerTemp(conn.ID, httppollConn)
        
        // 注入迁移回调：当 clientID 更新时自动触发迁移
        migrationCallback := s.httppollConnMgr.createMigrationCallback(conn.ID)
        httppollConn.SetMigrationCallback(migrationCallback)
    }
    
    return httppollConn
}
```

#### 4. 握手完成后的自动迁移

```go
// 在 SessionManager.handleHandshake 中
// 当握手成功且 clientID > 0 时，调用 UpdateClientID
// UpdateClientID 会自动触发迁移回调

if isControlConnection && clientConn.Authenticated && clientConn.ClientID > 0 {
    conn := s.getConnectionByConnID(connPacket.ConnectionID)
    if conn != nil {
        if sp, ok := conn.Stream.(*stream.StreamProcessor); ok {
            reader := sp.GetReader()
            // 检查是否是 HTTP 长轮询连接
            if httppollConn, ok := reader.(interface{ UpdateClientID(int64) }); ok {
                // 更新 clientID，自动触发迁移
                httppollConn.UpdateClientID(clientConn.ClientID)
            }
        }
    }
}
```

## 优势

1. **职责清晰**：
   - 适配层负责连接状态管理和迁移触发
   - 连接管理层负责映射关系维护
   - 协议层（SessionManager）只负责通用逻辑

2. **自动迁移**：
   - 握手成功后自动触发，无需外部干预
   - 客户端立即使用新 clientID 也能找到连接

3. **解耦设计**：
   - 适配层通过回调与连接管理器交互
   - 不依赖具体的连接管理器实现

4. **易于扩展**：
   - 其他协议如果需要类似机制，可以实现相同的接口
   - 迁移逻辑可以独立测试

## 实现细节

### 1. 如何获取 ConnectionID？

`ServerHTTPLongPollingConn` 需要知道自己的 `ConnectionID` 才能调用迁移回调。

**方案A**：在创建连接时传入 ConnectionID
```go
httppollConn := session.NewServerHTTPLongPollingConn(serverCtx, clientID)
conn, _ := sessionMgrWithConn.CreateConnection(httppollConn, httppollConn)
httppollConn.SetConnectionID(conn.ID) // 设置 ConnectionID
```

**方案B**：从 context 中获取（如果 ConnectionID 存储在 context 中）

**推荐方案A**：简单直接，在创建连接后立即设置。

### 2. 迁移回调的线程安全

迁移回调在 `UpdateClientID` 中调用，而 `UpdateClientID` 可能在不同 goroutine 中调用，需要确保线程安全。

`httppollConnectionManager.migrateTempToClientID` 已经使用锁保护，是线程安全的。

### 3. 错误处理

如果迁移失败（比如 ConnectionID 不存在），应该：
- 记录错误日志
- 不阻塞后续流程
- 允许连接继续工作（降级处理）

## 迁移流程图

```
客户端握手请求 (clientID=0)
    ↓
服务器创建临时连接 (clientID=0)
    ↓
注册到 tempConnections 和 tempConnectionsByIP
    ↓
注入迁移回调
    ↓
握手成功，分配 clientID
    ↓
调用 UpdateClientID(newClientID)
    ↓
检测到 oldClientID=0 && newClientID>0
    ↓
自动调用迁移回调
    ↓
从 tempConnections 移除
    ↓
添加到 connections[clientID]
    ↓
客户端使用新 clientID 发送请求
    ↓
通过 connections[clientID] 找到连接 ✅
```

## 总结

这个设计将迁移逻辑完全封装在适配层，通过回调机制实现自动迁移，保持了架构的清晰性和可扩展性。

