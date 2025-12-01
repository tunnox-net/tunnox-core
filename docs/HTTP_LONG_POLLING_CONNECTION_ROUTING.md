# HTTP 长轮询连接路由机制

## 核心问题

**问题**：ConnectionID 如何与连接关联？如何保证响应路由到正确的客户端？

## HTTP 长轮询的特殊性

### 关键理解

**HTTP 是无状态的**：
- 每个 HTTP 请求都是独立的
- 服务器不能主动向客户端发送数据
- 只能等待客户端的请求，然后在响应中返回数据

**不需要"回写 Response 到原有连接"**：
- 响应就是针对当前 Poll 请求的
- 通过当前的 ResponseWriter 返回即可
- 不需要通过 clientAddr 查找 net.Conn

## 连接管理机制

### 正确的连接管理方式

**ConnectionID -> 连接对象（在内存中）**：
- ConnectionID 是逻辑连接的唯一标识
- 连接对象（`ServerHTTPLongPollingConn`）在内存中维护
- 连接对象包含数据队列和状态信息

**数据流转流程**：
```
服务器有数据要发送
  ↓
放入连接对象的数据队列（conn.pollDataQueue）
  ↓
客户端发送 Poll 请求（携带 ConnectionID）
  ↓
服务器根据 ConnectionID 查找连接对象（O(1) 查找）
  ↓
从连接对象的数据队列取出数据
  ↓
通过当前的 ResponseWriter 返回（响应就是针对当前 Poll 请求的）
```

### 实现示例

```go
// 连接管理器（简化版）
type HTTPLongPollingConnectionManager struct {
    mu sync.RWMutex
    // connectionID -> 连接对象（O(1) 查找）
    connections map[string]*session.ServerHTTPLongPollingConn
}

// 根据 ConnectionID 查找连接对象
func (mgr *HTTPLongPollingConnectionManager) GetByConnectionID(connectionID string) *session.ServerHTTPLongPollingConn {
    mgr.mu.RLock()
    defer mgr.mu.RUnlock()
    return mgr.connections[connectionID] // O(1) 查找
}

// 处理 Poll 请求
func (s *Server) handleHTTPPoll(w http.ResponseWriter, r *http.Request) {
    // 1. 从 X-Tunnel-Package 提取 ConnectionID
    tunnelPkg, _ := httppoll.DecodeTunnelPackage(r.Header.Get("X-Tunnel-Package"))
    connectionID := tunnelPkg.ConnectionID
    
    // 2. 根据 ConnectionID 查找连接对象（O(1) 查找）
    conn := s.connMgr.GetByConnectionID(connectionID)
    if conn == nil {
        s.respondError(w, http.StatusNotFound, "connection not found")
        return
    }
    
    // 3. 从连接对象的数据队列取出数据（阻塞等待）
    data, err := conn.PollData(ctx)
    
    // 4. 通过当前的 ResponseWriter 返回（响应就是针对当前 Poll 请求的）
    w.Header().Set("X-Tunnel-Package", encodedResponse)
    w.Write(data)
}
```

## 性能分析

### 查找性能

**方案A：通过 ConnectionID 查找（推荐）**
- 时间复杂度：O(1)
- 实现：`map[string]*ServerHTTPLongPollingConn`
- 性能：最优，直接 map 查找

**方案B：通过 clientAddr 查找（不推荐）**
- 时间复杂度：O(1)（但需要维护额外的映射）
- 实现：`map[string]string` (clientAddr -> connectionID)
- 问题：
  - 需要维护额外的映射
  - clientAddr 可能变化（NAT、代理等）
  - 增加复杂度，没有性能优势

**推荐**：采用方案A，直接通过 ConnectionID 查找连接对象。

### 响应路由

**关键理解**：
- HTTP 响应就是针对当前请求的
- 通过当前的 ResponseWriter 返回即可
- 不需要"回写 Response 到原有连接"

**实现**：
```go
// 服务器有数据要发送时
func (s *Server) SendDataToClient(connectionID string, data []byte) error {
    // 1. 根据 ConnectionID 查找连接对象
    conn := s.connMgr.GetByConnectionID(connectionID)
    if conn == nil {
        return fmt.Errorf("connection not found")
    }
    
    // 2. 将数据放入连接对象的数据队列
    conn.EnqueueData(data)
    
    // 3. 通知等待的 Poll 请求（如果有）
    conn.NotifyPollRequest()
    
    return nil
}

// 处理 Poll 请求时
func (s *Server) handleHTTPPoll(w http.ResponseWriter, r *http.Request) {
    // ... 查找连接对象 ...
    
    // 从连接对象的数据队列取出数据
    data, err := conn.PollData(ctx)
    
    // 通过当前的 ResponseWriter 返回（响应就是针对当前 Poll 请求的）
    w.Write(data)
}
```

## 行业最佳实践

### 1. 直接通过 ConnectionID 查找连接对象

**优势**：
- O(1) 查找时间复杂度
- 简单直接，不需要额外的映射
- 性能最优

**实现**：
```go
connections map[string]*ServerHTTPLongPollingConn
conn := connections[connectionID] // O(1) 查找
```

### 2. 连接对象包含数据队列

**优势**：
- 数据在连接对象中排队
- Poll 请求时直接从队列取出
- 不需要额外的响应队列管理

**实现**：
```go
type ServerHTTPLongPollingConn struct {
    // 数据队列
    pollDataQueue *PriorityQueue
    pollDataChan  chan []byte
    // ...
}

// Poll 请求时
data, err := conn.PollData(ctx) // 从队列取出数据
```

### 3. 响应通过当前 ResponseWriter 返回

**优势**：
- 符合 HTTP 无状态特性
- 不需要"回写 Response 到原有连接"
- 简单直接

**实现**：
```go
// Poll 请求时
w.Header().Set("X-Tunnel-Package", encodedResponse)
w.Write(data) // 通过当前的 ResponseWriter 返回
```

### 4. 不需要维护 clientAddr 映射

**理由**：
- HTTP 是无状态的，不需要通过 clientAddr 查找连接
- 直接通过 ConnectionID 查找即可
- 避免维护额外的映射，简化连接管理

## 总结

**正确的连接管理方式**：
1. **ConnectionID -> 连接对象**：直接通过 ConnectionID 查找连接对象（O(1) 查找）
2. **连接对象包含数据队列**：数据在连接对象中排队，Poll 请求时取出
3. **响应通过当前 ResponseWriter 返回**：不需要"回写 Response 到原有连接"
4. **不需要维护 clientAddr 映射**：简化连接管理，避免性能问题

**性能优化**：
- 使用 `map[string]*ServerHTTPLongPollingConn`，O(1) 查找
- 连接对象包含数据队列，Poll 请求时直接从队列取出数据返回
- 不需要通过 clientAddr 查找，直接通过 ConnectionID 查找

**行业最佳实践**：
- 直接通过 ConnectionID 查找连接对象
- 连接对象包含数据队列
- 响应通过当前 ResponseWriter 返回
- 不需要维护 clientAddr 映射

