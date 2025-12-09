# 代码修复方案文档

本文档详细描述了代码中发现的问题及其修复方案。

## 目录

1. [多路复用连接 gRPC Stream 并发发送问题](#1-多路复用连接-grpc-stream-并发发送问题)
2. [自动连接器配置数据竞争问题](#2-自动连接器配置数据竞争问题)
3. [握手等待循环缺少超时机制](#3-握手等待循环缺少超时机制)
4. [连接池 DialTimeout 配置无效](#4-连接池-dialtimeout-配置无效)
5. [命令响应管理器 Channel 竞态风险](#5-命令响应管理器-channel-竞态风险)
6. [连接池未剔除已关闭连接](#6-连接池未剔除已关闭连接)
7. [HTTP 长轮询调度器性能问题](#7-http-长轮询调度器性能问题)
8. [日志体系统一化改造](#8-日志体系统一化改造)

---

## 1. 多路复用连接 gRPC Stream 并发发送问题

### 问题描述

**文件**: `internal/bridge/multiplexed_conn.go:86-141`

**严重程度**: 高

**问题**: 多路复用连接为每个 `ForwardSession` 启动一个 goroutine，直接并发调用同一个 gRPC client stream 的 `Send`。Go gRPC 明确要求同一 stream 的 `SendMsg` 需要串行化，否则会出现 data race、panic 或乱序数据。

**当前代码问题**:
- `RegisterSession` 为每个会话启动独立的 `sendLoopForSession` goroutine
- 多个 goroutine 同时调用 `m.stream.Send(packet)`，违反 gRPC 线程安全要求

### 修复方案

**方案**: 实现单写循环（fan-in 模式），所有会话的包通过 channel 汇聚到一个发送 goroutine。

**具体实现**:

1. **添加发送队列和锁**:
   ```go
   type grpcMultiplexedConn struct {
       // ... 现有字段
       sendQueue    chan *pb.ForwardPacket  // 统一的发送队列
       sendQueueMu  sync.Mutex              // 发送队列锁（可选，如果使用缓冲 channel 可不用）
   }
   ```

2. **修改初始化逻辑**:
   ```go
   func NewMultiplexedConn(...) {
       // ... 现有代码
       mc.sendQueue = make(chan *pb.ForwardPacket, 100) // 缓冲队列
       
       // 启动统一的发送循环
       go mc.sendLoop()
       go mc.receiveLoop()
   }
   ```

3. **实现单写循环**:
   ```go
   func (m *grpcMultiplexedConn) sendLoop() {
       for {
           select {
           case packet := <-m.sendQueue:
               if packet == nil {
                   return // 关闭信号
               }
               if err := m.stream.Send(packet); err != nil {
                   utils.Errorf("MultiplexedConn: failed to send packet: %v", err)
                   m.Close()
                   return
               }
               m.sessionsMu.Lock()
               m.lastActiveAt = time.Now()
               m.sessionsMu.Unlock()
           case <-m.Ctx().Done():
               return
           }
       }
   }
   ```

4. **修改会话发送逻辑**:
   ```go
   func (m *grpcMultiplexedConn) sendLoopForSession(session *ForwardSession) {
       sendChan := session.getSendChannel()
       
       for {
           select {
           case packet, ok := <-sendChan:
               if !ok {
                   return
               }
               
               m.closedMu.RLock()
               if m.closed {
                   m.closedMu.RUnlock()
                   return
               }
               m.closedMu.RUnlock()
               
               // 发送到统一队列而不是直接调用 stream.Send
               select {
               case m.sendQueue <- packet:
               case <-session.Ctx().Done():
                   return
               case <-m.Ctx().Done():
                   return
               }
           case <-session.Ctx().Done():
               return
           case <-m.Ctx().Done():
               return
           }
       }
   }
   ```

5. **关闭时清理**:
   ```go
   func (m *grpcMultiplexedConn) Close() error {
       // ... 现有代码
       close(m.sendQueue) // 关闭发送队列
       // ... 其他清理
   }
   ```

**替代方案（如果不想大改）**: 使用互斥锁保护 `stream.Send` 调用，但性能较差，不推荐。

---

## 2. 自动连接器配置数据竞争问题

### 问题描述

**文件**: `internal/client/auto_connector.go:153-170`

**严重程度**: 高

**问题**: 自动探测握手前直接修改共享的 `c.config.Server.Protocol/Address`，而多个探测 goroutine 可能同时成功并进入这一段，没有任何锁或副本，存在数据竞争且握手可能带着错误的地址/协议发送。

**当前代码问题**:
```go
// 临时设置协议和地址，以便握手时使用正确的协议
originalProtocol := ac.client.config.Server.Protocol
originalAddress := ac.client.config.Server.Address
ac.client.config.Server.Protocol = attempt.Endpoint.Protocol
ac.client.config.Server.Address = attempt.Endpoint.Address

// 执行握手（等待 ACK）
handshakeErr := ac.client.sendHandshakeOnStream(attempt.Stream, "control")

// 恢复原始配置
ac.client.config.Server.Protocol = originalProtocol
ac.client.config.Server.Address = originalAddress
```

### 修复方案

**方案**: 避免写全局配置，使用局部请求字段或加互斥保护。

**推荐方案（局部字段）**:

1. **修改 `sendHandshakeOnStream` 签名，接受协议和地址参数**:
   ```go
   func (c *TunnoxClient) sendHandshakeOnStream(
       stream stream.PackageStreamer, 
       connectionType string,
       protocol string,  // 新增
       address string,   // 新增
   ) error {
       // ... 现有代码
       req := &packet.HandshakeRequest{
           // ...
           Protocol: protocol,  // 使用传入的参数而不是 c.config.Server.Protocol
           // ...
       }
       // ...
   }
   ```

2. **修改 `auto_connector.go` 调用**:
   ```go
   // 执行握手（等待 ACK），传入协议和地址
   handshakeErr := ac.client.sendHandshakeOnStream(
       attempt.Stream, 
       "control",
       attempt.Endpoint.Protocol,  // 直接传入
       attempt.Endpoint.Address,   // 直接传入
   )
   // 不再需要临时修改和恢复配置
   ```

3. **如果 `sendHandshake` 也需要支持**:
   ```go
   func (c *TunnoxClient) sendHandshake() error {
       return c.sendHandshakeOnStream(
           c.controlStream, 
           "control",
           c.config.Server.Protocol,  // 使用配置值
           c.config.Server.Address,   // 使用配置值
       )
   }
   ```

**替代方案（互斥锁）**: 如果不想修改函数签名，可以在修改配置前后加锁，但需要确保所有访问 `config.Server` 的地方都加锁，风险较高。

---

## 3. 握手等待循环缺少超时机制

### 问题描述

**文件**: `internal/client/control_connection_handshake.go:95-123`

**严重程度**: 中

**问题**: 握手等待循环没有超时或上下文取消分支，若服务端无回应/掉线，该 goroutine 会永久阻塞，自动连接/重连可能因此卡死。

**当前代码问题**:
```go
// 等待握手响应（忽略心跳包）
var respPkt *packet.TransferPacket
for {
    pkt, _, err := stream.ReadPacket()
    if err != nil {
        return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to read handshake response")
    }
    // ... 处理逻辑
}
```

### 修复方案

**方案**: 在等待时使用 `select` 监听 `ctx`/超时并关闭连接。

**具体实现**:

1. **修改函数签名，接受 context**:
   ```go
   func (c *TunnoxClient) sendHandshakeOnStream(
       ctx context.Context,  // 新增
       stream stream.PackageStreamer, 
       connectionType string,
       protocol string,
       address string,
   ) error {
   ```

2. **添加超时 context**:
   ```go
   // 在函数开始处添加超时
   handshakeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
   defer cancel()
   ```

3. **修改等待循环**:
   ```go
   // 等待握手响应（忽略心跳包）
   var respPkt *packet.TransferPacket
   handshakeTimeout := time.NewTimer(10 * time.Second)
   defer handshakeTimeout.Stop()
   
   for {
       select {
       case <-handshakeCtx.Done():
           return coreErrors.Wrap(handshakeCtx.Err(), coreErrors.ErrorTypeNetwork, "handshake timeout")
       case <-handshakeTimeout.C:
           return coreErrors.New(coreErrors.ErrorTypeNetwork, "handshake timeout after 10s")
       default:
           // 非阻塞读取，需要 stream 支持 context 或超时
           // 如果 ReadPacket 不支持 context，需要包装
       }
       
       // 使用带超时的读取（如果 stream 支持）
       pkt, _, err := stream.ReadPacket()
       if err != nil {
           if err == context.DeadlineExceeded || err == context.Canceled {
               return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "handshake timeout")
           }
           return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to read handshake response")
       }
       
       if pkt == nil {
           // 检查超时
           select {
           case <-handshakeCtx.Done():
               return coreErrors.Wrap(handshakeCtx.Err(), coreErrors.ErrorTypeNetwork, "handshake timeout")
           default:
               continue
           }
       }
       
       // ... 现有处理逻辑
   }
   ```

**如果 `ReadPacket` 不支持 context**，需要：
- 在 `stream` 接口中添加 `ReadPacketWithContext(ctx context.Context)` 方法
- 或者使用 goroutine + channel 包装现有 `ReadPacket`

---

## 4. 连接池 DialTimeout 配置无效

### 问题描述

**文件**: `internal/bridge/node_pool.go:40-102`

**严重程度**: 中

**问题**: `NodePoolConfig` 暴露 `DialTimeout`，但 `createConnection` 始终硬编码 10s，初始化最小连接也用同样常量，配置项实质无效且无法调优拨号时长。

**当前代码问题**:
```go
type NodePoolConfig struct {
    // ...
    DialTimeout time.Duration  // 配置项存在
}

func (p *NodeConnectionPool) createConnection() (MultiplexedConn, error) {
    dialCtx, cancel := context.WithTimeout(p.Ctx(), 10*time.Second)  // 硬编码
    // ...
}
```

### 修复方案

**方案**: 使用配置值并在池级别统一控制超时。

**具体实现**:

1. **在 `NodeConnectionPool` 结构体中保存超时配置**:
   ```go
   type NodeConnectionPool struct {
       // ... 现有字段
       dialTimeout time.Duration  // 新增
   }
   ```

2. **在 `NewNodeConnectionPool` 中保存配置**:
   ```go
   func NewNodeConnectionPool(parentCtx context.Context, targetNodeID, targetAddr string, config *NodePoolConfig) (*NodeConnectionPool, error) {
       if config == nil {
           config = &NodePoolConfig{
               // ...
               DialTimeout: 10 * time.Second,  // 默认值
           }
       }
       
       pool := &NodeConnectionPool{
           // ... 现有字段
           dialTimeout: config.DialTimeout,  // 保存配置
       }
       // ...
   }
   ```

3. **在 `createConnection` 中使用配置值**:
   ```go
   func (p *NodeConnectionPool) createConnection() (MultiplexedConn, error) {
       dialCtx, cancel := context.WithTimeout(p.Ctx(), p.dialTimeout)  // 使用配置
       defer cancel()
       // ...
   }
   ```

4. **确保 `initializeMinConnections` 也使用相同超时**（已通过调用 `createConnection` 间接使用）。

---

## 5. 命令响应管理器 Channel 竞态风险

### 问题描述

**文件**: `internal/client/command_response_manager.go:36-105`

**严重程度**: 高

**问题**: 超时或调用 `UnregisterRequest` 会关闭 response channel，但 `HandleResponse` 可能在关闭后仍拿到旧的 channel 并执行 `responseChan <- resp`，会直接 panic（发送到已关闭的 channel 无保护）。

**当前代码问题**:
- `UnregisterRequest` 会 `close(ch)` 并 `delete(m.pendingRequests, commandID)`
- `HandleResponse` 在检查 `exists` 后，可能在解析响应过程中 channel 被关闭
- 虽然有二次检查 `stillExists`，但仍有时间窗口

### 修复方案

**方案**: 统一由单 goroutine 管理 channel 生命周期，或使用非关闭标记/缓冲 channel。

**推荐方案（使用 sync.Once 和状态标记）**:

1. **修改数据结构**:
   ```go
   type pendingRequest struct {
       responseChan chan *CommandResponse
       closed       bool
       mu           sync.Mutex
   }
   
   type CommandResponseManager struct {
       pendingRequests map[string]*pendingRequest  // 改为指针
       mu              sync.RWMutex
       timeout         time.Duration
   }
   ```

2. **修改 `RegisterRequest`**:
   ```go
   func (m *CommandResponseManager) RegisterRequest(commandID string) chan *CommandResponse {
       m.mu.Lock()
       defer m.mu.Unlock()
       
       pr := &pendingRequest{
           responseChan: make(chan *CommandResponse, 1),  // 缓冲 channel
           closed:       false,
       }
       m.pendingRequests[commandID] = pr
       return pr.responseChan
   }
   ```

3. **修改 `UnregisterRequest`**:
   ```go
   func (m *CommandResponseManager) UnregisterRequest(commandID string) {
       m.mu.Lock()
       defer m.mu.Unlock()
       
       if pr, exists := m.pendingRequests[commandID]; exists {
           pr.mu.Lock()
           if !pr.closed {
               pr.closed = true
               close(pr.responseChan)  // 关闭 channel
           }
           pr.mu.Unlock()
           delete(m.pendingRequests, commandID)
       }
   }
   ```

4. **修改 `HandleResponse`**:
   ```go
   func (m *CommandResponseManager) HandleResponse(pkt *packet.TransferPacket) bool {
       // ... 现有检查逻辑
       
       m.mu.RLock()
       pr, exists := m.pendingRequests[commandID]
       m.mu.RUnlock()
       
       if !exists {
           return false
       }
       
       // 解析响应
       // ... 现有解析逻辑
       
       // 发送前再次检查
       pr.mu.Lock()
       closed := pr.closed
       pr.mu.Unlock()
       
       if closed {
           return false
       }
       
       // 发送响应（使用 select 防止阻塞）
       select {
       case pr.responseChan <- resp:
           return true
       default:
           // Channel 已满或已关闭（虽然已检查，但双重保险）
           return false
       }
   }
   ```

**替代方案（不关闭 channel）**: 使用带缓冲的 channel，通过发送 `nil` 或特殊值表示"无响应"，但需要调用方处理。

---

## 6. 连接池未剔除已关闭连接

### 问题描述

**文件**: `internal/bridge/node_pool.go:115-140`

**严重程度**: 中

**问题**: 连接池不会剔除已关闭的连接。`connections` slice 里只按 `len(connections)` 计数；当 gRPC 连接意外关闭后仍占用名额，`currentCount < p.maxConns` 可能为假，新连接不会创建，导致池被死连接"卡满"，后续 session 全部失败。

**当前代码问题**:
```go
func (p *NodeConnectionPool) GetOrCreateSession(ctx context.Context, metadata *SessionMetadata) (*ForwardSession, error) {
    p.connsMu.RLock()
    
    // 优先从现有连接中查找可用的
    for _, conn := range p.connections {
        if conn.CanAcceptStream() {  // 可能返回 true 但连接已关闭
            // ...
        }
    }
    
    currentCount := int32(len(p.connections))  // 包含已关闭的连接
    // ...
}
```

### 修复方案

**方案**: 在获取/清理时过滤掉 `IsClosed()` 的连接，或在 Dial/Recv 错误后主动从池中删除并补齐。

**具体实现**:

1. **修改 `GetOrCreateSession`，过滤已关闭连接**:
   ```go
   func (p *NodeConnectionPool) GetOrCreateSession(ctx context.Context, metadata *SessionMetadata) (*ForwardSession, error) {
       p.connsMu.Lock()
       
       // 先清理已关闭的连接
       activeConns := make([]MultiplexedConn, 0, len(p.connections))
       for _, conn := range p.connections {
           if !conn.IsClosed() {
               activeConns = append(activeConns, conn)
           }
       }
       p.connections = activeConns
       p.connsMu.Unlock()
       
       // 重新加读锁查找可用连接
       p.connsMu.RLock()
       for _, conn := range p.connections {
           if conn.CanAcceptStream() {
               p.connsMu.RUnlock()
               session := NewForwardSession(ctx, conn, metadata)
               if session != nil {
                   return session, nil
               }
           }
       }
       
       currentCount := int32(len(p.connections))
       p.connsMu.RUnlock()
       
       // ... 后续逻辑
   }
   ```

2. **在 `receiveLoop` 错误时主动移除连接**（在 `multiplexed_conn.go`）:
   ```go
   func (m *grpcMultiplexedConn) receiveLoop() {
       // ...
       for {
           // ...
           packet, err := m.stream.Recv()
           if err != nil {
               // ... 现有错误处理
               m.Close()
               // 通知连接池移除此连接（需要回调或事件机制）
               return
           }
           // ...
       }
   }
   ```

3. **在连接池中添加移除连接的方法**:
   ```go
   func (p *NodeConnectionPool) removeConnection(conn MultiplexedConn) {
       p.connsMu.Lock()
       defer p.connsMu.Unlock()
       
       for i, c := range p.connections {
           if c == conn {
               // 从 slice 中移除
               p.connections = append(p.connections[:i], p.connections[i+1:]...)
               break
           }
       }
   }
   ```

4. **在 `NewMultiplexedConn` 中注册关闭回调**（如果可能）:
   ```go
   // 在 MultiplexedConn 接口中添加 OnClose 回调
   // 或在连接池创建连接时传入回调
   ```

5. **在 `cleanupIdleConnections` 中也检查已关闭连接**:
   ```go
   func (p *NodeConnectionPool) cleanupIdleConnections() {
       p.connsMu.Lock()
       defer p.connsMu.Unlock()
       
       activeConns := make([]MultiplexedConn, 0, len(p.connections))
       closedCount := 0
       
       for _, conn := range p.connections {
           // 先检查是否已关闭
           if conn.IsClosed() {
               closedCount++
               continue
           }
           
           // 保留最小连接数
           if int32(len(activeConns)) < p.minConns {
               activeConns = append(activeConns, conn)
               continue
           }
           
           // 关闭空闲连接
           if conn.IsIdle(p.maxIdleTime) {
               conn.Close()
               closedCount++
           } else {
               activeConns = append(activeConns, conn)
           }
       }
       
       p.connections = activeConns
       // ... 日志
   }
   ```

---

## 7. HTTP 长轮询调度器性能问题

### 问题描述

**文件**: `internal/protocol/session/httppoll_server_conn_poll.go:17-117`

**严重程度**: 中

**问题**: 长轮询调度器每 10ms tick 并在 Info 级别打印日志，生产环境会产生大量日志和 CPU 唤醒，即使队列为空也是 busy-ish loop。

**当前代码问题**:
```go
func (c *ServerHTTPLongPollingConn) pollDataScheduler() {
    ticker := time.NewTicker(10 * time.Millisecond)  // 每 10ms
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            // 定期检查队列
            for {
                // ...
                utils.Infof("HTTP long polling: [POLLDATA_SCHEDULER] pushed %d bytes...", ...)  // Info 级别
            }
        }
    }
}
```

### 修复方案

**方案**: 降低日志级别或把 tick 改为按队列事件驱动/更长间隔并用 jitter。

**具体实现**:

1. **降低日志级别**:
   ```go
   func (c *ServerHTTPLongPollingConn) pollDataScheduler() {
       ticker := time.NewTicker(10 * time.Millisecond)
       defer ticker.Stop()
       
       for {
           select {
           case <-ticker.C:
               for {
                   data, ok := c.pollDataQueue.Pop()
                   if !ok {
                       break
                   }
                   // ...
                   utils.Debugf("HTTP long polling: [POLLDATA_SCHEDULER] pushed %d bytes...", ...)  // 改为 Debug
               }
           }
       }
   }
   ```

2. **改为事件驱动 + 更长间隔**（推荐）:
   ```go
   func (c *ServerHTTPLongPollingConn) pollDataScheduler() {
       // 使用更长的 tick（100ms）作为兜底，主要靠事件驱动
       ticker := time.NewTicker(100 * time.Millisecond)
       defer ticker.Stop()
       
       // 队列有数据时通知调度器
       queueNotify := make(chan struct{}, 1)
       
       // 包装队列的 Push 方法，在 Push 时通知（需要修改队列实现）
       // 或者使用现有的 pollWaitChan 机制
       
       for {
           select {
           case <-c.Ctx().Done():
               return
           case <-ticker.C:
               // 兜底检查（100ms 一次）
               c.processQueue()
           case <-c.pollWaitChan:  // 或新的 queueNotify
               // 队列有数据，立即处理
               c.processQueue()
           }
       }
   }
   
   func (c *ServerHTTPLongPollingConn) processQueue() {
       for {
           data, ok := c.pollDataQueue.Pop()
           if !ok {
               break
           }
           select {
           case <-c.Ctx().Done():
               c.pollDataQueue.Push(data)
               return
           case c.pollDataChan <- data:
               utils.Debugf("HTTP long polling: [POLLDATA_SCHEDULER] pushed %d bytes...", ...)
               // 通知 PollData
               select {
               case c.pollWaitChan <- struct{}{}:
               default:
               }
           default:
               c.pollDataQueue.Push(data)
               return
           }
       }
   }
   ```

3. **添加 jitter（如果保留 ticker）**:
   ```go
   import "math/rand"
   
   func (c *ServerHTTPLongPollingConn) pollDataScheduler() {
       baseInterval := 50 * time.Millisecond
       jitter := time.Duration(rand.Intn(20)) * time.Millisecond  // 0-20ms jitter
       ticker := time.NewTicker(baseInterval + jitter)
       // ...
   }
   ```

**推荐**: 结合方案 1 和 2，降低日志级别 + 事件驱动 + 更长兜底间隔。

---

## 8. 日志体系统一化改造

### 问题描述

**文件**: 
- `internal/core/dispose/logger.go` - 使用标准库 `log` 实现简单分级
- `internal/utils/logger.go` - 使用 `logrus` 实现结构化日志
- 各模块中混用两种日志系统

**严重程度**: 中

**问题**: 
- 日志体系存在"两套性格"：`dispose/logger.go` 用标准库 `log`，`utils/logger.go` 用 `logrus`
- 大量 debug log 非常详细，但如果开在生产默认级别，很容易淹没关键信息
- 缺少统一的 Logger 接口和 level 控制
- 缺少带 request/connection/session ID 的结构化字段

### 修复方案

**方案**: 用一套统一 Logger 接口，实现 level 控制（默认 INFO，DEBUG 必须显式打开）和结构化字段（request/connection/session ID）。

**具体实现**:

1. **定义统一的 Logger 接口**:
   ```go
   // internal/core/logger/logger.go
   package logger
   
   type Logger interface {
       Debug(args ...interface{})
       Debugf(format string, args ...interface{})
       Info(args ...interface{})
       Infof(format string, args ...interface{})
       Warn(args ...interface{})
       Warnf(format string, args ...interface{})
       Error(args ...interface{})
       Errorf(format string, args ...interface{})
       
       WithField(key string, value interface{}) Logger
       WithFields(fields map[string]interface{}) Logger
       WithRequestID(requestID string) Logger
       WithConnectionID(connectionID string) Logger
       WithSessionID(sessionID string) Logger
       WithClientID(clientID string) Logger
   }
   ```

2. **实现基于 logrus 的 Logger**:
   ```go
   // internal/core/logger/logrus_logger.go
   package logger
   
   import "github.com/sirupsen/logrus"
   
   type LogrusLogger struct {
       entry *logrus.Entry
   }
   
   func NewLogrusLogger() *LogrusLogger {
       return &LogrusLogger{
           entry: logrus.NewEntry(logrus.StandardLogger()),
       }
   }
   
   func (l *LogrusLogger) WithField(key string, value interface{}) Logger {
       return &LogrusLogger{entry: l.entry.WithField(key, value)}
   }
   
   func (l *LogrusLogger) WithRequestID(requestID string) Logger {
       return l.WithField("request_id", requestID)
   }
   
   func (l *LogrusLogger) WithConnectionID(connectionID string) Logger {
       return l.WithField("connection_id", connectionID)
   }
   
   func (l *LogrusLogger) WithSessionID(sessionID string) Logger {
       return l.WithField("session_id", sessionID)
   }
   
   func (l *LogrusLogger) WithClientID(clientID string) Logger {
       return l.WithField("client_id", clientID)
   }
   
   func (l *LogrusLogger) Debug(args ...interface{}) {
       l.entry.Debug(args...)
   }
   
   func (l *LogrusLogger) Debugf(format string, args ...interface{}) {
       l.entry.Debugf(format, args...)
   }
   
   // ... 其他方法
   ```

3. **创建全局 Logger 实例**:
   ```go
   // internal/core/logger/global.go
   package logger
   
   var globalLogger Logger
   
   func init() {
       globalLogger = NewLogrusLogger()
   }
   
   func SetLogger(l Logger) {
       globalLogger = l
   }
   
   func GetLogger() Logger {
       return globalLogger
   }
   
   // 便捷函数
   func Debug(args ...interface{}) {
       globalLogger.Debug(args...)
   }
   
   func Debugf(format string, args ...interface{}) {
       globalLogger.Debugf(format, args...)
   }
   
   // ... 其他便捷函数
   ```

4. **迁移 `dispose/logger.go`**:
   ```go
   // internal/core/dispose/logger.go
   package dispose
   
   import "tunnox-core/internal/core/logger"
   
   // 保持向后兼容的包装函数
   func Debugf(format string, args ...interface{}) {
       logger.Debugf(format, args...)
   }
   
   func Infof(format string, args ...interface{}) {
       logger.Infof(format, args...)
   }
   
   func Warnf(format string, args ...interface{}) {
       logger.Warnf(format, args...)
   }
   
   func Errorf(format string, args ...interface{}) {
       logger.Errorf(format, args...)
   }
   
   func Warn(msg string) {
       logger.Warn(msg)
   }
   ```

5. **迁移 `utils/logger.go`**:
   ```go
   // internal/utils/logger.go
   package utils
   
   import "tunnox-core/internal/core/logger"
   
   // 保持向后兼容
   var Logger = logger.GetLogger()  // 类型可能需要适配
   
   // 或者直接使用 core/logger
   func Debugf(format string, args ...interface{}) {
       logger.Debugf(format, args...)
   }
   
   // ... 其他函数
   ```

6. **设置默认级别为 INFO**:
   ```go
   // 在 InitLogger 中
   func InitLogger(config *LogConfig) error {
       // ...
       if config.Level == "" {
           config.Level = "info"  // 默认 INFO
       }
       // ...
   }
   ```

7. **在关键位置使用结构化字段**:
   ```go
   // 示例：在 multiplexed_conn.go 中
   import "tunnox-core/internal/core/logger"
   
   func (m *grpcMultiplexedConn) sendLoopForSession(session *ForwardSession) {
       log := logger.GetLogger().
           WithConnectionID(m.targetNodeID).
           WithSessionID(session.streamID)
       
       // ...
       if err := m.stream.Send(packet); err != nil {
           log.Errorf("failed to send packet: %v", err)
           // ...
       }
   }
   ```

**迁移步骤**:
1. 创建 `internal/core/logger` 包和接口
2. 实现基于 logrus 的 Logger
3. 逐步迁移各模块使用新 Logger
4. 保持旧接口的向后兼容包装
5. 最终移除旧实现

---

## 修复优先级建议

1. **P0（立即修复）**:
   - 问题 1: 多路复用连接 gRPC Stream 并发发送（可能导致 panic）
   - 问题 2: 自动连接器配置数据竞争（可能导致错误握手）
   - 问题 5: 命令响应管理器 Channel 竞态（可能导致 panic）

2. **P1（尽快修复）**:
   - 问题 3: 握手等待循环缺少超时（可能导致卡死）
   - 问题 6: 连接池未剔除已关闭连接（可能导致连接失败）

3. **P2（计划修复）**:
   - 问题 4: 连接池 DialTimeout 配置无效（功能缺失）
   - 问题 7: HTTP 长轮询调度器性能问题（性能优化）
   - 问题 8: 日志体系统一化（代码质量）

---

## 测试建议

每个修复都应包含相应的测试：

1. **并发测试**: 对于并发问题（问题 1、2、5），使用 `go test -race` 验证
2. **超时测试**: 对于超时问题（问题 3），模拟服务端无响应场景
3. **集成测试**: 对于连接池问题（问题 4、6），测试连接创建和清理流程
4. **性能测试**: 对于性能问题（问题 7），测量 CPU 和日志输出量
5. **兼容性测试**: 对于日志改造（问题 8），确保现有代码仍能正常工作

---

## 注意事项

1. 所有修复都应保持向后兼容，除非明确需要破坏性变更
2. 修复后应更新相关文档和注释
3. 建议使用代码审查确保修复正确性
4. 考虑添加监控和告警，及时发现类似问题
