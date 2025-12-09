# HTTP Poll 连接问题综合修复方案

## 问题总结

### 问题 1：连接重连后 Tunnel 绑定失效 ⚠️ 严重

**根本原因**：
- Listen Client 的控制连接超时后重新连接，获得新的 connectionID
- Tunnel (通过 TunnelOpen 建立) 仍然绑定到旧的 connectionID
- 数据被推送到旧连接的 `pollDataQueue`，但新连接的 poll 请求读不到数据

**复现条件**：
- 执行大查询（如 `SELECT * FROM table LIMIT 7000`）
- 数据传输时间超过 30-60 秒
- Listen Client 控制连接超时，重新连接
- 新的 poll 请求在新连接上，但数据在旧连接的队列中

**日志证据**：
```
# 旧连接（tunnel 绑定）
time="2025-12-09T18:23:08" msg="Tunnel[tcp-tunnel-...]: connID=conn_67fb198a"
time="2025-12-09T18:23:31" msg="WriteExact - connID=conn_67fb198a, sequenceNumber=26159"

# 新连接（poll 请求）
time="2025-12-09T18:27:33" msg="Client: ConnectionID=conn_5a75f407"
time="2025-12-09T18:27:28" msg="HandlePollRequest returned error: context deadline exceeded, connID=conn_c34c20ba"
```

### 问题 2：HTTP Server Timeout 配置冲突 ⚠️ 严重

**根本原因**：
- Management API 的 HTTP Server 配置：
  ```go
  ReadTimeout:  30 * time.Second
  WriteTimeout: 30 * time.Second
  IdleTimeout:  120 * time.Second
  ```
- Poll 请求故意阻塞最长 60 秒等待数据
- 但 `WriteTimeout = 30秒`，服务端写响应时被 HTTP Server 强制关闭

**影响**：
1. 空闲 5-10 分钟后查询卡住
2. Poll 请求返回 EOF，客户端 pollLoop 可能退出
3. 长时间传输的大数据包被中断

## 修复方案

### 修复 1：支持 Tunnel 连接迁移 🔧

**方案 A：当客户端重连时，自动迁移 Tunnel 绑定**

修改 `internal/protocol/session/manager.go` 的 `HandleHandshake` 方法：

```go
func (s *SessionManager) HandleHandshake(clientID int64, newConnID string, newConn *Connection) {
    // ... 现有逻辑 ...

    // ✅ 新增：迁移现有 tunnels 到新连接
    s.migrateTunnelsToNewConnection(clientID, newConnID, newConn)
}

func (s *SessionManager) migrateTunnelsToNewConnection(clientID int64, newConnID string, newConn *Connection) {
    s.bridgeLock.Lock()
    defer s.bridgeLock.Unlock()

    for tunnelID, bridge := range s.bridges {
        // 检查是否是该客户端的 source tunnel
        if bridge.listenClientID == clientID {
            oldConnID := bridge.GetSourceConnectionID()
            if oldConnID != "" && oldConnID != newConnID {
                utils.Infof("Migrating tunnel %s from old connection %s to new connection %s",
                    tunnelID, oldConnID, newConnID)

                // 获取旧连接的 stream processor
                oldConn := s.GetConnection(oldConnID)
                if oldConn != nil && oldConn.Stream != nil {
                    oldStream := oldConn.Stream

                    // 迁移 pollDataQueue 中的数据
                    if oldServerSP, ok := oldStream.(*httppoll.ServerStreamProcessor); ok {
                        if newServerSP, ok := newConn.Stream.(*httppoll.ServerStreamProcessor); ok {
                            migrateQueueData(oldServerSP, newServerSP)
                        }
                    }
                }

                // 更新 tunnel 的 source connection
                bridge.UpdateSourceConnection(newConn.Conn, newConn.Stream)
            }
        }
    }
}

func migrateQueueData(oldSP, newSP *httppoll.ServerStreamProcessor) {
    // 将旧连接队列中的数据迁移到新连接
    for {
        data, ok := oldSP.pollDataQueue.Pop()
        if !ok {
            break
        }
        newSP.pollDataQueue.Push(data)
    }

    // 通知新连接有数据
    select {
    case newSP.pollWaitChan <- struct{}{}:
    default:
    }
}
```

**方案 B：使用客户端 ID 而不是连接 ID 来路由数据**

修改数据推送逻辑，使用 `clientID` 而不是 `connectionID` 来查找目标队列：

```go
// 在 SessionManager 中维护 clientID -> 最新 connectionID 的映射
type SessionManager struct {
    // ...
    clientConnections map[int64]string // clientID -> latest connectionID
    clientConnMu      sync.RWMutex
}

// 当客户端重连时更新映射
func (s *SessionManager) HandleHandshake(clientID int64, newConnID string, newConn *Connection) {
    s.clientConnMu.Lock()
    s.clientConnections[clientID] = newConnID
    s.clientConnMu.Unlock()
    // ...
}

// 推送数据时使用最新连接
func (bridge *TunnelBridge) writeToListenClient(data []byte) error {
    // 获取最新的连接 ID
    latestConnID := bridge.sessionMgr.GetLatestConnectionID(bridge.listenClientID)
    latestConn := bridge.sessionMgr.GetConnection(latestConnID)

    if latestConn != nil && latestConn.Stream != nil {
        return latestConn.Stream.WriteExact(data)
    }
    return errors.New("no active connection for client")
}
```

### 修复 2：调整 HTTP Server Timeout 配置 🔧

修改 `internal/api/server.go` 的 HTTP Server 配置：

```go
func StartManagementAPI(ctx context.Context, config *config.ServerConfig, ...) error {
    // ...

    server := &http.Server{
        Addr:    config.ManagementAPI.ListenAddr,
        Handler: router,

        // ✅ 修复：增加超时时间，适配 HTTP Long Polling
        ReadTimeout:  90 * time.Second,  // 从 30s 增加到 90s（大于 poll 等待时间 60s + 传输时间）
        WriteTimeout: 90 * time.Second,  // 从 30s 增加到 90s
        IdleTimeout:  300 * time.Second, // 从 120s 增加到 300s（5分钟）

        // ✅ 新增：设置更大的 header 和 body 大小限制
        MaxHeaderBytes: 1 << 20, // 1 MB
    }

    // ...
}
```

**注意**：如果不想全局增加超时，可以为 `/tunnox/v1/poll` 和 `/tunnox/v1/push` 路由使用独立的 handler：

```go
// 创建专门用于 HTTP Poll 的子 server
pollRouter := chi.NewRouter()
pollRouter.Post("/tunnox/v1/poll", handlers.HandleHTTPPoll)
pollRouter.Post("/tunnox/v1/push", handlers.HandleHTTPPush)

pollServer := &http.Server{
    Addr:         config.ManagementAPI.ListenAddr,
    Handler:      pollRouter,
    ReadTimeout:  90 * time.Second,
    WriteTimeout: 90 * time.Second,
}

// 主 API server 保持较短的超时
mainServer := &http.Server{
    Addr:         config.ManagementAPI.ListenAddr,
    Handler:      mainRouter,
    ReadTimeout:  30 * time.Second,
    WriteTimeout: 30 * time.Second,
}
```

### 修复 3：增强客户端 pollLoop 恢复机制 🔧

修改 `internal/protocol/httppoll/stream_processor_poll.go`：

```go
func (sp *StreamProcessor) pollLoop(pollIndex int) {
    defer func() {
        utils.Infof("HTTPStreamProcessor: pollLoop %d exiting, clientID=%d", pollIndex, sp.clientID)

        // ✅ 新增：如果 pollLoop 异常退出，自动重启
        if r := recover(); r != nil {
            utils.Errorf("HTTPStreamProcessor: pollLoop %d panic: %v, restarting...", pollIndex, r)
            time.Sleep(1 * time.Second)
            go sp.pollLoop(pollIndex)
        }
    }()

    // 错开启动时间
    time.Sleep(time.Duration(pollIndex) * 50 * time.Millisecond)

    utils.Infof("HTTPStreamProcessor: pollLoop %d started, clientID=%d", pollIndex, sp.clientID)

    consecutiveErrors := 0
    maxConsecutiveErrors := 10

    for {
        select {
        case <-sp.ctx.Done():
            return
        default:
        }

        // 发送 poll 请求
        err := sp.sendPollRequest(sp.ctx, pollIndex)

        if err != nil {
            if err == io.EOF {
                consecutiveErrors++
                utils.Warnf("HTTPStreamProcessor: pollLoop %d received EOF (consecutive: %d/%d), connection may be closed",
                    pollIndex, consecutiveErrors, maxConsecutiveErrors)

                // ✅ 修复：EOF 后等待一段时间再重试
                backoff := time.Duration(consecutiveErrors) * 500 * time.Millisecond
                if backoff > 5*time.Second {
                    backoff = 5 * time.Second
                }
                time.Sleep(backoff)

                // ✅ 如果连续错误过多，触发重连
                if consecutiveErrors >= maxConsecutiveErrors {
                    utils.Errorf("HTTPStreamProcessor: pollLoop %d too many consecutive errors, may need reconnection", pollIndex)
                    // 触发上层重连逻辑（通过关闭 context 或发送信号）
                    sp.cancel() // 这会触发 controlConnection 重连
                    return
                }
                continue
            }

            // 其他错误
            consecutiveErrors++
            utils.Errorf("HTTPStreamProcessor: pollLoop %d error: %v (consecutive: %d)", pollIndex, err, consecutiveErrors)
            time.Sleep(time.Duration(consecutiveErrors) * 100 * time.Millisecond)
        } else {
            // 成功，重置错误计数
            consecutiveErrors = 0
        }
    }
}
```

### 修复 4：清理长时间空闲后的资源状态 🔧

修改 `internal/protocol/httppoll/server_stream_processor.go`：

```go
func NewServerStreamProcessor(ctx context.Context, connID string, clientID int64, mappingID string) *ServerStreamProcessor {
    sp := &ServerStreamProcessor{
        // ... 现有初始化 ...
    }

    // ✅ 新增：启动空闲检测和清理
    go sp.idleCleanupLoop()

    return sp
}

func (sp *ServerStreamProcessor) idleCleanupLoop() {
    ticker := time.NewTicker(2 * time.Minute)
    defer ticker.Stop()

    lastActivity := time.Now()

    for {
        select {
        case <-sp.Ctx().Done():
            return
        case <-ticker.C:
            // 检查是否长时间没有活动
            if time.Since(lastActivity) > 10*time.Minute {
                utils.Infof("ServerStreamProcessor[%s]: detected long idle (>10min), cleaning up stale state", sp.connectionID)

                // 清理可能残留的 fragments
                sp.fragmentReassembler.Cleanup()

                // 清理过期的待重组分片组
                sp.fragmentReassembler.RemoveStaleGroups(5 * time.Minute)

                lastActivity = time.Now()
            }
        }
    }
}

// 在 FragmentReassembler 中添加清理方法
func (r *FragmentReassembler) Cleanup() {
    r.mu.Lock()
    defer r.mu.Unlock()

    // 清理超过 5 分钟未完成的分片组
    now := time.Now()
    for groupID, group := range r.groups {
        if now.Sub(group.lastUpdate) > 5*time.Minute {
            utils.Warnf("FragmentReassembler: removing stale fragment group %s (age: %v)",
                groupID, now.Sub(group.lastUpdate))
            delete(r.groups, groupID)
        }
    }
}
```

## 测试验证

### 测试场景 1：重连后数据传输

```bash
# 1. 启动测试环境
./start_test.sh

# 2. 执行大查询
mysql -h 127.0.0.1 -P 7788 -u root -p -e "SELECT * FROM log.log_db_record LIMIT 7000"

# 3. 在查询过程中，手动重启 Listen Client
# （模拟连接超时重连）

# 4. 验证查询能否完成，数据是否正确
```

### 测试场景 2：空闲后恢复

```bash
# 1. 启动测试环境
./start_test.sh

# 2. 等待 10 分钟（模拟长时间空闲）

# 3. 执行查询
mysql -h 127.0.0.1 -P 7788 -u root -p -e "SELECT * FROM log.log_db_record LIMIT 7000"

# 4. 验证查询能否成功
```

### 测试场景 3：并发大查询

```bash
# 运行并发测试脚本
python3 test_concurrent_10_queries.py

# 检查是否所有查询都成功完成
```

## 日志检查点

修复后需要在日志中确认：

1. **连接迁移日志**：
   ```
   Migrating tunnel tcp-tunnel-... from old connection conn_xxx to new connection conn_yyy
   ```

2. **无超时错误**：
   ```
   # 应该消失
   HandlePollRequest returned error: context deadline exceeded
   ```

3. **pollLoop 恢复**：
   ```
   HTTPStreamProcessor: pollLoop X received EOF, restarting...
   HTTPStreamProcessor: pollLoop X started after recovery
   ```

4. **资源清理**：
   ```
   ServerStreamProcessor: detected long idle, cleaning up stale state
   FragmentReassembler: removing stale fragment group
   ```

## 实施优先级

1. **P0 - 立即修复**：
   - 修复 2：调整 HTTP Server Timeout（5 分钟工作量）

2. **P1 - 本周完成**：
   - 修复 1-A：Tunnel 连接迁移（1-2 天工作量）
   - 修复 3：增强 pollLoop 恢复（半天工作量）

3. **P2 - 下周完成**：
   - 修复 4：空闲清理（半天工作量）
   - 修复 1-B：基于 clientID 路由（2-3 天工作量，需要重构）

## 风险评估

- **修复 2（Timeout）**：低风险，直接配置修改
- **修复 3（pollLoop）**：低风险，只影响客户端
- **修复 1-A（迁移）**：中等风险，需要仔细测试数据一致性
- **修复 4（清理）**：低风险，防御性代码
- **修复 1-B（路由）**：高风险，需要大量测试
