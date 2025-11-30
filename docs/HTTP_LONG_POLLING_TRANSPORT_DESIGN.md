# HTTP 长轮询传输设计

## 1. 设计背景

### 1.1 问题分析

当前支持的连接协议（TCP、UDP、QUIC、WebSocket）存在以下问题：

1. **容易被封禁**：
   - TCP/UDP：非标准端口容易被防火墙识别
   - QUIC：特征明显，容易被深度包检测（DPI）识别
   - WebSocket：升级请求特征明显，部分防火墙会拦截

2. **网络环境限制**：
   - 企业网络：严格防火墙策略
   - 移动网络：运营商限制
   - 公共 Wi-Fi：端口和协议限制

### 1.2 HTTP 长轮询的优势

1. **伪装性强**：完全伪装成正常的 HTTP 请求
2. **穿透性好**：HTTP/HTTPS 端口（80/443）通常开放
3. **兼容性强**：所有网络环境都支持 HTTP
4. **隐蔽性好**：可以伪装成正常的 API 请求

### 1.3 设计目标

1. **完全兼容现有体系**：无缝集成到 `StreamProcessor` 体系
2. **高性能**：最小化延迟和开销
3. **高可靠性**：自动重连、数据缓存、顺序保证
4. **强伪装性**：完全伪装成正常的 HTTP API 请求

## 2. 协议设计

### 2.1 架构设计

HTTP 长轮询需要**两个 HTTP 连接**实现双向通信：

```
客户端                                   服务器
  |                                        |
  |  ┌─────────────────────────────────┐  |
  |  │  POST /api/push (发送数据)      │  |
  |  └─────────────────────────────────┘  |
  |              ↓                         |
  |  ┌─────────────────────────────────┐  |
  |  │  GET /api/poll (长轮询接收)     │  |
  |  │  (保持连接，等待服务器推送)      │  |
  |  └─────────────────────────────────┘  |
  |              ↓                         |
```

### 2.2 连接模式

#### 2.2.1 双连接模式（推荐）

- **上行连接（Push）**：客户端 → 服务器，使用 POST 请求发送数据
- **下行连接（Poll）**：客户端 ← 服务器，使用 GET 长轮询接收数据

**优点**：
- 双向通信，延迟低
- 实现简单，逻辑清晰

**缺点**：
- 需要维护两个连接
- 服务器资源占用稍高

#### 2.2.2 单连接模式（备选）

- 使用 POST 请求，在请求体中发送数据，响应体中接收数据

**优点**：
- 只需一个连接
- 资源占用少

**缺点**：
- 延迟较高（需要等待响应）
- 实现复杂

**选择**：采用**双连接模式**，平衡性能和复杂度。

### 2.3 API 端点设计

#### 2.3.1 上行端点（客户端 → 服务器）

```
POST /api/v1/tunnox/push
Content-Type: application/json
Authorization: Bearer <token>
X-Client-ID: <client_id>
X-Request-ID: <request_id>

{
  "data": "<base64_encoded_data>",
  "seq": <sequence_number>,
  "timestamp": <unix_timestamp>
}
```

**响应**：
```json
{
  "success": true,
  "ack": <acknowledged_sequence_number>,
  "timestamp": <unix_timestamp>
}
```

#### 2.3.2 下行端点（服务器 → 客户端）

```
GET /api/v1/tunnox/poll?timeout=30&since=<last_seq>
Authorization: Bearer <token>
X-Client-ID: <client_id>
X-Request-ID: <request_id>
```

**响应**（有数据时立即返回）：
```json
{
  "success": true,
  "data": "<base64_encoded_data>",
  "seq": <sequence_number>,
  "timestamp": <unix_timestamp>,
  "next_seq": <next_sequence_number>
}
```

**响应**（无数据时，30秒后返回）：
```json
{
  "success": true,
  "data": null,
  "timeout": true,
  "timestamp": <unix_timestamp>
}
```

### 2.4 数据格式

#### 2.4.1 数据编码

- **传输格式**：Base64 编码的二进制数据
- **数据内容**：`StreamProcessor` 序列化的 `TransferPacket`
- **大小限制**：单次请求最大 1MB（可配置）

#### 2.4.2 序列号管理

- **客户端序列号**：用于上行数据，确保顺序
- **服务器序列号**：用于下行数据，确保顺序
- **ACK 机制**：服务器返回已接收的最大序列号

### 2.5 伪装策略

#### 2.5.1 URL 伪装

- 使用常见的 API 路径：`/api/v1/tunnox/...`
- 可以进一步伪装：`/api/v1/notifications/push`、`/api/v1/messages/poll`

#### 2.5.2 请求头伪装

- 使用标准的 HTTP 头部
- 添加常见的业务头部：`X-Request-ID`、`X-Client-Version`
- 使用标准的认证方式：`Authorization: Bearer <token>`

#### 2.5.3 响应格式伪装

- 使用标准的 JSON 格式
- 添加业务字段：`success`、`message`、`data`
- 错误响应也使用标准格式

## 3. 实现设计

### 3.1 客户端实现

#### 3.1.1 HTTPLongPollingConn

实现 `net.Conn` 接口，包装 HTTP 长轮询连接：

```go
type HTTPLongPollingConn struct {
    baseURL      string
    clientID     int64
    token        string
    
    // 上行连接（发送数据）
    pushURL      string
    pushClient   *http.Client
    pushSeq      uint64
    pushMu       sync.Mutex
    
    // 下行连接（接收数据）
    pollURL      string
    pollClient   *http.Client
    pollSeq      uint64
    pollMu       sync.Mutex
    
    // 数据通道
    readChan     chan []byte
    writeChan    chan []byte
    
    // 控制
    ctx          context.Context
    cancel       context.CancelFunc
    closed       bool
    closeOnce    sync.Once
}

func NewHTTPLongPollingConn(baseURL string, clientID int64, token string) (*HTTPLongPollingConn, error) {
    ctx, cancel := context.WithCancel(context.Background())
    
    conn := &HTTPLongPollingConn{
        baseURL:    baseURL,
        clientID:   clientID,
        token:      token,
        pushURL:    baseURL + "/api/v1/tunnox/push",
        pollURL:    baseURL + "/api/v1/tunnox/poll",
        pushClient: &http.Client{Timeout: 30 * time.Second},
        pollClient: &http.Client{Timeout: 60 * time.Second}, // 长轮询超时
        readChan:   make(chan []byte, 100),
        writeChan:  make(chan []byte, 100),
        ctx:        ctx,
        cancel:     cancel,
    }
    
    // 启动接收循环
    go conn.pollLoop()
    
    return conn, nil
}
```

#### 3.1.2 Write 实现（上行）

```go
func (c *HTTPLongPollingConn) Write(p []byte) (int, error) {
    if c.closed {
        return 0, io.EOF
    }
    
    // 序列号管理
    c.pushMu.Lock()
    seq := c.pushSeq
    c.pushSeq++
    c.pushMu.Unlock()
    
    // 编码数据
    data := base64.StdEncoding.EncodeToString(p)
    
    // 构造请求
    reqBody := map[string]interface{}{
        "data":      data,
        "seq":       seq,
        "timestamp": time.Now().Unix(),
    }
    reqJSON, _ := json.Marshal(reqBody)
    
    // 发送 POST 请求
    req, _ := http.NewRequestWithContext(c.ctx, "POST", c.pushURL, bytes.NewReader(reqJSON))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+c.token)
    req.Header.Set("X-Client-ID", strconv.FormatInt(c.clientID, 10))
    req.Header.Set("X-Request-ID", generateRequestID())
    
    resp, err := c.pushClient.Do(req)
    if err != nil {
        return 0, fmt.Errorf("push request failed: %w", err)
    }
    defer resp.Body.Close()
    
    // 检查响应
    if resp.StatusCode != http.StatusOK {
        return 0, fmt.Errorf("push request failed: status %d", resp.StatusCode)
    }
    
    // 解析 ACK
    var ackResp struct {
        Success bool  `json:"success"`
        Ack     uint64 `json:"ack"`
    }
    json.NewDecoder(resp.Body).Decode(&ackResp)
    
    return len(p), nil
}
```

#### 3.1.3 Read 实现（下行）

```go
func (c *HTTPLongPollingConn) Read(p []byte) (int, error) {
    if c.closed {
        return 0, io.EOF
    }
    
    select {
    case <-c.ctx.Done():
        return 0, c.ctx.Err()
    case data := <-c.readChan:
        n := copy(p, data)
        return n, nil
    }
}

// pollLoop 长轮询循环
func (c *HTTPLongPollingConn) pollLoop() {
    for {
        select {
        case <-c.ctx.Done():
            return
        default:
        }
        
        // 构造 GET 请求
        url := fmt.Sprintf("%s?timeout=30&since=%d", c.pollURL, c.pollSeq)
        req, _ := http.NewRequestWithContext(c.ctx, "GET", url, nil)
        req.Header.Set("Authorization", "Bearer "+c.token)
        req.Header.Set("X-Client-ID", strconv.FormatInt(c.clientID, 10))
        req.Header.Set("X-Request-ID", generateRequestID())
        
        // 发送长轮询请求
        resp, err := c.pollClient.Do(req)
        if err != nil {
            // 连接错误，等待后重试
            time.Sleep(1 * time.Second)
            continue
        }
        
        // 解析响应
        var pollResp struct {
            Success bool   `json:"success"`
            Data    string `json:"data"`
            Seq     uint64 `json:"seq"`
            Timeout bool   `json:"timeout"`
        }
        json.NewDecoder(resp.Body).Decode(&pollResp)
        resp.Body.Close()
        
        // 处理数据
        if pollResp.Data != "" {
            data, err := base64.StdEncoding.DecodeString(pollResp.Data)
            if err == nil {
                // 更新序列号
                c.pollMu.Lock()
                c.pollSeq = pollResp.Seq + 1
                c.pollMu.Unlock()
                
                // 发送到读取通道
                select {
                case c.readChan <- data:
                case <-c.ctx.Done():
                    return
                }
            }
        }
        
        // 如果是超时，立即发起下一个请求
        if pollResp.Timeout {
            continue
        }
    }
}
```

### 3.2 服务器实现（统一架构设计）

#### 3.2.1 核心设计原则

**关键原则**：服务器端也应该像客户端一样，将 HTTP 请求/响应转换为 `net.Conn` 接口，然后统一使用 `StreamProcessor`。

**架构对比**：

```
其他协议（TCP/WebSocket/QUIC/UDP）：
  客户端: net.Conn → StreamProcessor ✅
  服务器: AcceptConnection(net.Conn) → StreamProcessor ✅

HTTP Long Polling（当前实现）：
  客户端: HTTPLongPollingConn(net.Conn) → StreamProcessor ✅
  服务器: HTTPLongPollingManager + 特殊转发逻辑 ❌

HTTP Long Polling（目标实现）：
  客户端: HTTPLongPollingConn(net.Conn) → StreamProcessor ✅
  服务器: HTTPLongPollingConn(net.Conn) → StreamProcessor ✅
```

#### 3.2.2 服务器端 HTTPLongPollingConn 设计

服务器端也需要实现一个 `HTTPLongPollingConn`，实现 `net.Conn` 接口：

```go
// internal/protocol/session/httppoll_server_conn.go
package session

import (
    "io"
    "net"
    "sync"
    "time"
)

// ServerHTTPLongPollingConn 服务器端 HTTP 长轮询连接
// 实现 net.Conn 接口，将 HTTP 请求/响应转换为双向流
type ServerHTTPLongPollingConn struct {
    clientID int64
    
    // 上行数据（客户端 → 服务器）
    pushDataChan chan []byte  // 从 HTTP POST 接收的数据
    pushSeq      uint64
    
    // 下行数据（服务器 → 客户端）
    pollDataChan chan []byte  // 发送到 HTTP GET 响应的数据
    pollSeq      uint64
    
    // 控制
    mu           sync.RWMutex
    closed       bool
    closeOnce    sync.Once
    
    // 地址信息
    localAddr    net.Addr
    remoteAddr   net.Addr
}

// NewServerHTTPLongPollingConn 创建服务器端 HTTP 长轮询连接
func NewServerHTTPLongPollingConn(clientID int64) *ServerHTTPLongPollingConn {
    return &ServerHTTPLongPollingConn{
        clientID:     clientID,
        pushDataChan: make(chan []byte, 100),
        pollDataChan: make(chan []byte, 100),
        localAddr:    &httppollServerAddr{addr: "server"},
        remoteAddr:   &httppollServerAddr{addr: fmt.Sprintf("client-%d", clientID)},
    }
    }
    
// Read 实现 io.Reader（从 HTTP POST 读取数据）
func (c *ServerHTTPLongPollingConn) Read(p []byte) (int, error) {
    c.mu.RLock()
    closed := c.closed
    c.mu.RUnlock()
    
    if closed {
        return 0, io.EOF
    }
    
    // 从 pushDataChan 读取数据（由 handleHTTPPush 写入）
    select {
    case data, ok := <-c.pushDataChan:
        if !ok {
            return 0, io.EOF
        }
        n := copy(p, data)
        return n, nil
    case <-time.After(30 * time.Second):
        return 0, io.EOF // 超时
    }
}

// Write 实现 io.Writer（通过 HTTP GET 响应发送数据）
func (c *ServerHTTPLongPollingConn) Write(p []byte) (int, error) {
    c.mu.RLock()
    closed := c.closed
    c.mu.RUnlock()
    
    if closed {
        return 0, io.ErrClosedPipe
    }
    
    // 将数据发送到 pollDataChan（由 handleHTTPPoll 读取）
    select {
    case c.pollDataChan <- p:
        return len(p), nil
    default:
        return 0, io.ErrShortWrite // 通道满
    }
}

// Close 实现 io.Closer
func (c *ServerHTTPLongPollingConn) Close() error {
    var err error
    c.closeOnce.Do(func() {
        c.mu.Lock()
        c.closed = true
        c.mu.Unlock()
        
        close(c.pushDataChan)
        close(c.pollDataChan)
    })
    return err
    }
    
// LocalAddr, RemoteAddr, SetDeadline 等实现 net.Conn 接口
func (c *ServerHTTPLongPollingConn) LocalAddr() net.Addr  { return c.localAddr }
func (c *ServerHTTPLongPollingConn) RemoteAddr() net.Addr { return c.remoteAddr }
func (c *ServerHTTPLongPollingConn) SetDeadline(t time.Time) error {
    // HTTP 长轮询的 deadline 由 HTTP 请求控制
    return nil
}
func (c *ServerHTTPLongPollingConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *ServerHTTPLongPollingConn) SetWriteDeadline(t time.Time) error { return nil }
    
// PushData 从 HTTP POST 请求接收数据（由 handleHTTPPush 调用）
func (c *ServerHTTPLongPollingConn) PushData(data []byte) error {
    c.mu.RLock()
    closed := c.closed
    c.mu.RUnlock()
    
    if closed {
        return io.ErrClosedPipe
    }
    
    select {
    case c.pushDataChan <- data:
        return nil
    default:
        return io.ErrShortWrite
    }
}

// PollData 等待数据用于 HTTP GET 响应（由 handleHTTPPoll 调用）
func (c *ServerHTTPLongPollingConn) PollData(ctx context.Context) ([]byte, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    case data, ok := <-c.pollDataChan:
        if !ok {
            return nil, io.EOF
        }
        return data, nil
    }
}
```

#### 3.2.3 统一的连接创建流程

服务器端处理 HTTP 请求时，创建统一的连接：

```go
// internal/api/handlers_httppoll.go

// handleHTTPPush 处理客户端推送数据
func (s *ManagementAPIServer) handleHTTPPush(w http.ResponseWriter, r *http.Request) {
    // 1. 获取 ClientID
    clientID, err := s.getClientIDFromRequest(r)
    if err != nil {
        s.respondError(w, http.StatusBadRequest, err.Error())
        return
    }
    
    // 2. 解析请求数据
    var req HTTPPushRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        s.respondError(w, http.StatusBadRequest, "Invalid request")
        return
    }
    
    // 3. 解码数据
    data, err := base64.StdEncoding.DecodeString(req.Data)
    if err != nil {
        s.respondError(w, http.StatusBadRequest, "Invalid data")
        return
    }
    
    // 4. 获取或创建 HTTP 长轮询连接
    httppollConn := s.getOrCreateHTTPLongPollingConn(clientID)
    
    // 5. 将数据推送到连接（触发 Read()）
    if err := httppollConn.PushData(data); err != nil {
        s.respondError(w, http.StatusServiceUnavailable, "Connection closed")
        return
    }
    
    // 6. 返回 ACK
    s.respondJSON(w, http.StatusOK, HTTPPushResponse{
        Success:   true,
        Ack:       req.Seq,
        Timestamp: time.Now().Unix(),
    })
}

// handleHTTPPoll 处理客户端长轮询
func (s *ManagementAPIServer) handleHTTPPoll(w http.ResponseWriter, r *http.Request) {
    // 1. 获取 ClientID
    clientID, err := s.getClientIDFromRequest(r)
    if err != nil {
        s.respondError(w, http.StatusBadRequest, err.Error())
        return
    }
    
    // 2. 获取或创建 HTTP 长轮询连接
    httppollConn := s.getOrCreateHTTPLongPollingConn(clientID)
    
    // 3. 解析超时参数
    timeout := 30 * time.Second
    if t := r.URL.Query().Get("timeout"); t != "" {
        if parsed, err := strconv.Atoi(t); err == nil {
            timeout = time.Duration(parsed) * time.Second
        }
    }
    
    // 4. 长轮询：等待数据（触发 Write()）
    ctx, cancel := context.WithTimeout(r.Context(), timeout)
    defer cancel()
    
    data, err := httppollConn.PollData(ctx)
    if err == context.DeadlineExceeded {
        // 超时，返回空响应
        s.respondJSON(w, http.StatusOK, HTTPPollResponse{
            Success:   true,
            Timeout:   true,
            Timestamp: time.Now().Unix(),
        })
        return
    }
    if err != nil {
        s.respondError(w, http.StatusInternalServerError, err.Error())
        return
    }
    
    // 5. 有数据，立即返回
    encoded := base64.StdEncoding.EncodeToString(data)
    s.respondJSON(w, http.StatusOK, HTTPPollResponse{
        Success:   true,
        Data:      encoded,
        Seq:       httppollConn.GetNextSeq(),
        Timestamp: time.Now().Unix(),
    })
}

// getOrCreateHTTPLongPollingConn 获取或创建 HTTP 长轮询连接
func (s *ManagementAPIServer) getOrCreateHTTPLongPollingConn(clientID int64) *session.ServerHTTPLongPollingConn {
    // 1. 检查是否已存在连接
    if conn := s.getHTTPLongPollingConn(clientID); conn != nil {
        return conn
    }
    
    // 2. 创建新的 HTTP 长轮询连接（实现 net.Conn）
    httppollConn := session.NewServerHTTPLongPollingConn(clientID)
    
    // 3. ✅ 统一使用 CreateConnection，就像其他协议一样
    conn, err := s.sessionMgr.CreateConnection(httppollConn, httppollConn)
    if err != nil {
        // 处理错误
        return nil
    }
    
    // 4. 设置协议类型
    conn.Protocol = "httppoll"
    
    // 5. 保存连接映射（用于后续查找）
    s.registerHTTPLongPollingConn(clientID, httppollConn, conn)
    
    return httppollConn
}
```

#### 3.2.4 架构优势

**统一性**：
- 所有协议都使用相同的 `CreateConnection` → `StreamProcessor` 流程
- 不需要协议特定的管理器或转发逻辑
- 代码更简洁，维护更容易

**清晰性**：
- 协议适配器只负责：实现 `net.Conn` 接口
- `StreamProcessor` 负责：数据包读写
- 职责分离，层次清晰

**可扩展性**：
- 新增协议只需实现 `net.Conn` 接口
- 不需要修改核心逻辑
- 易于测试和维护

### 3.3 与现有体系集成（统一架构）

#### 3.3.1 客户端集成（已完成 ✅）

客户端已经正确实现：
- `HTTPLongPollingConn` 实现 `net.Conn` 接口
- 直接传给 `StreamProcessor`
- 与其他协议完全一致

```go
// internal/client/control_connection.go
case "httppoll", "http-long-polling", "httplp":
    conn, err = dialHTTPLongPolling(c.Ctx(), c.config.Server.Address, clientID, token)
    // conn 是 net.Conn，直接传给 StreamProcessor
    c.controlStream = streamFactory.CreateStreamProcessor(conn, conn)
```

#### 3.3.2 服务器集成（需要重构）

**当前问题**：
- 服务器端使用了 `HTTPLongPollingManager` 和特殊转发逻辑
- 没有统一使用 `CreateConnection` → `StreamProcessor`

**目标架构**：
- 服务器端也实现 `ServerHTTPLongPollingConn`（实现 `net.Conn`）
- 统一使用 `CreateConnection(httppollConn, httppollConn)` → `StreamProcessor`
- 移除 `HTTPLongPollingManager` 和特殊转发逻辑

**实现步骤**：

**步骤 1**：创建 `internal/protocol/session/httppoll_server_conn.go`

```go
package session

// ServerHTTPLongPollingConn 服务器端 HTTP 长轮询连接
// 实现 net.Conn 接口，统一使用 StreamProcessor
type ServerHTTPLongPollingConn struct {
    // 实现 net.Conn 接口
    // Read: 从 HTTP POST 读取数据
    // Write: 通过 HTTP GET 响应发送数据
}
```

**步骤 2**：修改 `internal/api/handlers_httppoll.go`

```go
func (s *ManagementAPIServer) handleHTTPPush(w http.ResponseWriter, r *http.Request) {
    // 1. 获取或创建 ServerHTTPLongPollingConn
    httppollConn := s.getOrCreateHTTPLongPollingConn(clientID)
    
    // 2. 将 HTTP POST 数据推送到连接（触发 Read()）
    httppollConn.PushData(data)
    
    // 3. StreamProcessor 会自动从 Read() 读取数据
}

func (s *ManagementAPIServer) handleHTTPPoll(w http.ResponseWriter, r *http.Request) {
    // 1. 获取 ServerHTTPLongPollingConn
    httppollConn := s.getOrCreateHTTPLongPollingConn(clientID)
    
    // 2. 等待数据（触发 Write()）
    data := httppollConn.PollData(ctx)
    
    // 3. StreamProcessor 会自动通过 Write() 写入数据
}
```

**步骤 3**：统一连接创建

```go
func (s *ManagementAPIServer) getOrCreateHTTPLongPollingConn(clientID int64) *session.ServerHTTPLongPollingConn {
    // 1. 创建 ServerHTTPLongPollingConn（实现 net.Conn）
    httppollConn := session.NewServerHTTPLongPollingConn(clientID)
    
    // 2. ✅ 统一使用 CreateConnection，就像 TCP/WebSocket/QUIC 一样
    conn, err := s.sessionMgr.CreateConnection(httppollConn, httppollConn)
    if err != nil {
        return nil
    }
    
    // 3. 设置协议类型
    conn.Protocol = "httppoll"
    
    // 4. 后续握手、数据包处理都通过 StreamProcessor 统一处理
    return httppollConn
}
```

**步骤 4**：移除 `HTTPLongPollingManager`

- 删除 `internal/protocol/session/httppoll_manager.go`
- 删除 `forwardPushData`/`forwardPollData` 逻辑
- 删除临时连接、poll reader 等复杂概念

#### 3.3.3 数据流转换（统一架构）

**关键点**：HTTP 长轮询的数据流与其他协议完全一致，都通过 `StreamProcessor` 处理。

**数据流**：

```
客户端：
  StreamProcessor.WritePacket()
    ↓
  HTTPLongPollingConn.Write() → HTTP POST (Base64编码)
    ↓
  服务器接收

服务器：
  HTTP POST → Base64解码 → ServerHTTPLongPollingConn.PushData()
    ↓
  ServerHTTPLongPollingConn.Read() → StreamProcessor.ReadPacket()
    ↓
  统一处理（握手、命令、数据包）

服务器：
  StreamProcessor.WritePacket()
    ↓
  ServerHTTPLongPollingConn.Write() → pollDataChan
    ↓
  ServerHTTPLongPollingConn.PollData() → HTTP GET 响应 (Base64编码)

客户端：
  HTTP GET 响应 → Base64解码 → HTTPLongPollingConn.pollLoop()
    ↓
  HTTPLongPollingConn.Read() → StreamProcessor.ReadPacket()
    ↓
  统一处理
```

**优势**：
- 数据格式完全统一：所有协议都使用 `StreamProcessor` 的序列化格式
- 不需要特殊的数据转换逻辑
- 代码路径统一，易于调试和维护

## 4. 性能优化

### 4.1 连接复用

- **HTTP 客户端复用**：使用 `http.Client` 的连接池
- **Keep-Alive**：启用 HTTP Keep-Alive，复用 TCP 连接
- **连接池大小**：根据并发连接数调整

### 4.2 批量传输

- **批量推送**：多个小数据包合并为一个 HTTP 请求
- **批量轮询**：一次轮询返回多个数据包
- **减少 HTTP 头部开销**：合并请求减少头部重复

### 4.3 压缩优化

- **数据压缩**：对 Base64 编码前的数据进行压缩（gzip）
- **响应压缩**：服务器启用 gzip 压缩响应
- **减少传输量**：压缩后 Base64 编码，减少传输字节数

### 4.4 缓存优化

- **客户端缓存**：未确认的数据缓存，支持重传
- **服务器缓存**：未推送的数据缓存，支持重连后推送
- **序列号管理**：确保数据顺序和完整性

## 5. 可靠性设计

### 5.1 自动重连

- **连接断开检测**：HTTP 请求失败时自动重连
- **指数退避**：重连间隔指数增长（1s, 2s, 4s, 8s, max 30s）
- **最大重试次数**：可配置，默认无限重试

### 5.2 数据可靠性

- **序列号保证**：确保数据顺序
- **ACK 机制**：确认数据接收
- **重传机制**：未确认的数据自动重传

### 5.3 错误处理

- **网络错误**：自动重试
- **认证错误**：返回错误，不重试
- **服务器错误**：根据错误码决定是否重试

## 6. 安全性设计

### 6.1 认证机制

- **Token 认证**：使用 Bearer Token
- **客户端 ID**：验证客户端身份
- **请求签名**：可选，增加安全性

### 6.2 数据加密

- **HTTPS 传输**：强制使用 HTTPS
- **数据加密**：可选，对数据进行端到端加密
- **TLS 配置**：支持 TLS 1.2+

### 6.3 防重放攻击

- **时间戳验证**：验证请求时间戳
- **序列号验证**：防止重放攻击
- **请求 ID**：唯一请求 ID，防止重复处理

## 7. 配置参数

### 7.1 客户端配置

```go
type HTTPLongPollingConfig struct {
    BaseURL          string        // 服务器基础 URL
    PushTimeout      time.Duration // 推送超时（默认 30s）
    PollTimeout      time.Duration // 轮询超时（默认 30s）
    MaxRetries       int           // 最大重试次数（默认无限）
    RetryInterval    time.Duration // 重试间隔（默认 1s）
    EnableCompression bool         // 启用压缩（默认 true）
    BatchSize        int           // 批量大小（默认 10）
}
```

### 7.2 服务器配置

```go
type HTTPLongPollingServerConfig struct {
    PollTimeout      time.Duration // 轮询超时（默认 30s）
    MaxPollTimeout   time.Duration // 最大轮询超时（默认 60s）
    EnableCompression bool         // 启用压缩（默认 true）
    MaxRequestSize   int64         // 最大请求大小（默认 1MB）
    ConnectionPoolSize int         // 连接池大小（默认 100）
}
```

## 8. 性能指标

### 8.1 延迟

- **上行延迟**：< 100ms（正常网络）
- **下行延迟**：< 轮询超时（30s）+ 处理时间（< 100ms）
- **平均延迟**：< 500ms（正常网络）

### 8.2 吞吐

- **单连接吞吐**：> 1MB/s（正常网络）
- **并发连接**：支持 1000+ 并发连接
- **服务器吞吐**：> 100MB/s（取决于服务器性能）

### 8.3 资源占用

- **客户端内存**：< 10MB（每个连接）
- **服务器内存**：< 50MB（每个连接）
- **CPU 开销**：< 5%（正常负载）

## 9. 测试策略

### 9.1 功能测试

- 连接建立和断开
- 数据发送和接收
- 自动重连
- 错误处理

### 9.2 性能测试

- 延迟测试
- 吞吐测试
- 并发测试
- 压力测试

### 9.3 兼容性测试

- 不同网络环境
- 不同防火墙配置
- 不同代理配置

## 10. 实现步骤

### 阶段 1：基础实现

1. 实现 `HTTPLongPollingConn`（客户端）
2. 实现 HTTP 端点处理（服务器）
3. 基础功能测试

### 阶段 2：集成

1. 集成到客户端连接体系
2. 集成到服务器路由
3. 集成测试

### 阶段 3：优化

1. 性能优化
2. 可靠性优化
3. 安全性优化

### 阶段 4：测试和调优

1. 全面测试
2. 性能调优
3. 生产环境验证

## 11. 关键设计决策

### 11.1 为什么选择双连接模式

**优点**：
- 双向通信，延迟低
- 实现简单，逻辑清晰
- 可以独立控制上行和下行

**缺点**：
- 需要维护两个连接
- 服务器资源占用稍高

**权衡**：选择双连接模式，因为性能更重要。

### 11.2 为什么使用 Base64 编码

**原因**：
- HTTP 请求体是文本格式，二进制数据需要编码
- Base64 是标准编码，兼容性好
- 实现简单，性能可接受

**优化**：
- 可以先压缩再编码，减少传输量
- 可以使用更高效的编码方式（如 Base85）

### 11.3 为什么使用长轮询而不是短轮询

**长轮询**：
- 服务器有数据时立即返回
- 无数据时等待（最多 30 秒）
- 减少无效请求，降低服务器负载

**短轮询**：
- 客户端定期请求（如每秒一次）
- 即使无数据也返回
- 增加服务器负载和网络开销

**选择**：长轮询，平衡延迟和资源占用。

## 12. 风险评估

### 12.1 技术风险

- **延迟较高**：HTTP 长轮询延迟高于直接连接（30s 轮询超时）
- **资源占用**：需要维护多个 HTTP 连接和 goroutine
- **实现复杂度**：需要处理连接管理、重连、数据转换等
- **数据转换开销**：Base64 编码/解码增加 CPU 开销

### 12.2 业务风险

- **被识别风险**：虽然伪装成 HTTP，但仍可能被深度检测识别
- **性能影响**：延迟和吞吐可能不如直接连接
- **兼容性问题**：某些代理可能不支持长轮询

### 12.3 缓解措施

- **性能优化**：通过批量传输、压缩等优化性能
- **连接复用**：使用 HTTP Keep-Alive 减少连接数
- **渐进实现**：分阶段实现，逐步优化
- **降级方案**：HTTP 长轮询作为其他协议的降级方案
- **智能选择**：根据网络状况自动选择最佳传输方式

## 13. 与其他协议对比

| 特性 | TCP | UDP | QUIC | WebSocket | HTTP 长轮询 |
|------|-----|-----|------|-----------|-------------|
| 伪装性 | ⭐⭐ | ⭐ | ⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 穿透性 | ⭐⭐ | ⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| 延迟 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| 吞吐 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| 可靠性 | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| 实现复杂度 | ⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ |

**结论**：HTTP 长轮询在伪装性和穿透性方面最优，适合被严格限制的网络环境。

## 14. 使用场景

### 14.1 适合场景

- ✅ **企业网络**：严格防火墙策略
- ✅ **移动网络**：运营商限制
- ✅ **公共 Wi-Fi**：端口和协议限制
- ✅ **代理环境**：只能使用 HTTP/HTTPS
- ✅ **降级方案**：其他协议失败时的备选

### 14.2 不适合场景

- ❌ **低延迟要求**：对延迟敏感的应用（如游戏）
- ❌ **高吞吐要求**：需要高吞吐的场景
- ❌ **实时性要求**：需要实时双向通信的场景

## 15. 实现优先级

### 15.1 第一阶段（核心功能）

1. 客户端 HTTP 长轮询连接实现
2. 服务器 HTTP 端点实现
3. 基础数据收发功能
4. 与 StreamProcessor 集成

### 15.2 第二阶段（可靠性）

1. 自动重连机制
2. 数据缓存和重传
3. 序列号管理
4. 错误处理

### 15.3 第三阶段（性能优化）

1. 批量传输
2. 数据压缩
3. 连接复用
4. 性能调优

### 15.4 第四阶段（高级特性）

1. 智能路由
2. 多路径传输
3. HTTP/2 支持
4. 监控和统计

## 12. 数据流详细设计（统一架构）

### 12.1 客户端数据流（上行）

```
应用程序数据
    ↓
StreamProcessor.WritePacket()
    ↓
TransferPacket 序列化（二进制）
    ↓
HTTPLongPollingConn.Write() 
    ↓
writeFlushLoop 检测完整包
    ↓
Base64 编码
    ↓
HTTP POST /tunnox/v1/push
    ↓
服务器接收
```

### 12.2 服务器数据流（上行）

```
HTTP POST /tunnox/v1/push
    ↓
Base64 解码
    ↓
ServerHTTPLongPollingConn.PushData()
    ↓
ServerHTTPLongPollingConn.Read() (被 StreamProcessor 调用)
    ↓
StreamProcessor.ReadPacket()
    ↓
统一处理（握手、命令、数据包）
```

### 12.3 服务器数据流（下行）

```
StreamProcessor.WritePacket()
    ↓
TransferPacket 序列化（二进制）
    ↓
ServerHTTPLongPollingConn.Write() (被 StreamProcessor 调用)
    ↓
写入 pollDataChan
    ↓
ServerHTTPLongPollingConn.PollData() (handleHTTPPoll 调用)
    ↓
Base64 编码
    ↓
HTTP GET /tunnox/v1/poll 响应
```

### 12.4 客户端数据流（下行）

```
HTTP GET /tunnox/v1/poll 响应
    ↓
Base64 解码
    ↓
HTTPLongPollingConn.pollLoop()
    ↓
写入 readChan
    ↓
HTTPLongPollingConn.Read() (被 StreamProcessor 调用)
    ↓
StreamProcessor.ReadPacket()
    ↓
统一处理
```

### 12.5 关键设计点

**统一性**：
- 所有数据都通过 `StreamProcessor` 处理
- 不需要特殊的数据转换逻辑
- HTTP 层只负责 Base64 编码/解码

**职责分离**：
- `HTTPLongPollingConn` / `ServerHTTPLongPollingConn`：实现 `net.Conn`，处理 HTTP 请求/响应
- `StreamProcessor`：处理数据包序列化/反序列化
- `SessionManager`：统一管理所有连接

## 13. 连接生命周期管理（统一架构）

### 13.1 客户端连接管理

- **连接建立**：`dialHTTPLongPolling` → `HTTPLongPollingConn` → `StreamProcessor`
- **连接保持**：通过长轮询保持连接活跃
- **连接断开**：HTTP 请求超时或错误时断开
- **自动重连**：检测到断开后自动重连

### 13.2 服务器连接管理（统一流程）

- **连接建立**：`handleHTTPPush`/`handleHTTPPoll` → `ServerHTTPLongPollingConn` → `CreateConnection` → `StreamProcessor`
- **连接注册**：通过 `CreateConnection` 自动注册到 `SessionManager`
- **连接超时**：由 `SessionManager` 统一管理
- **数据路由**：通过 `StreamProcessor` 统一处理，不需要特殊路由逻辑

### 13.3 与 SessionManager 集成（统一接口）

**连接创建**：
```go
// 服务器端（统一流程）
httppollConn := session.NewServerHTTPLongPollingConn(clientID)
conn, err := s.sessionMgr.CreateConnection(httppollConn, httppollConn)
// 后续握手、数据包处理都通过 StreamProcessor 统一处理
```

**数据路由**：
```go
// 当有数据需要发送给客户端时
// 通过 StreamProcessor.WritePacket() 统一处理
// ServerHTTPLongPollingConn.Write() 会自动将数据发送到 pollDataChan
// handleHTTPPoll 会从 pollDataChan 读取并返回给客户端
```

**优势**：
- 不需要 `RegisterHTTPLongPollingConnection` 等特殊方法
- 不需要 `GetHTTPLongPollingConnection` 等特殊查找逻辑
- 所有协议都使用相同的 `CreateConnection` 接口

## 15. 架构对比与重构方案

### 15.1 当前架构问题

**问题 1：服务器端混层**
- 使用了 `HTTPLongPollingManager` 特殊管理器
- 需要 `forwardPushData`/`forwardPollData` 转发逻辑
- 需要临时连接、poll reader 等复杂概念
- 与其他协议不一致

**问题 2：职责不清**
- 协议适配器应该只负责实现 `net.Conn`
- 但当前实现把数据转发、连接管理都混在一起

**问题 3：代码复杂**
- `httppoll_manager.go` 有 600+ 行代码
- 包含大量特殊处理逻辑
- 难以维护和扩展

### 15.2 目标架构

**统一原则**：
```
所有协议都遵循相同的模式：
  协议适配器（实现 net.Conn） → CreateConnection → StreamProcessor → 统一处理
```

**架构对比**：

| 协议 | 客户端 | 服务器端 |
|------|--------|----------|
| TCP | `net.Dial` → `StreamProcessor` ✅ | `AcceptConnection` → `StreamProcessor` ✅ |
| WebSocket | `websocketStreamConn` → `StreamProcessor` ✅ | `AcceptConnection` → `StreamProcessor` ✅ |
| QUIC | `quicStreamConn` → `StreamProcessor` ✅ | `AcceptConnection` → `StreamProcessor` ✅ |
| UDP | `udpStreamConn` → `StreamProcessor` ✅ | `AcceptConnection` → `StreamProcessor` ✅ |
| HTTP Long Polling | `HTTPLongPollingConn` → `StreamProcessor` ✅ | `ServerHTTPLongPollingConn` → `StreamProcessor` ✅ |

### 15.3 重构步骤

**阶段 1：创建 ServerHTTPLongPollingConn**
1. 创建 `internal/protocol/session/httppoll_server_conn.go`
2. 实现 `net.Conn` 接口
3. 实现 `PushData()` 和 `PollData()` 方法

**阶段 2：修改 HTTP 处理器**
1. 修改 `handleHTTPPush`：使用 `ServerHTTPLongPollingConn.PushData()`
2. 修改 `handleHTTPPoll`：使用 `ServerHTTPLongPollingConn.PollData()`
3. 统一使用 `CreateConnection` 创建连接

**阶段 3：移除特殊逻辑**
1. 删除 `HTTPLongPollingManager`
2. 删除 `forwardPushData`/`forwardPollData`
3. 删除临时连接、poll reader 等概念
4. 简化 `handlers_httppoll.go`

**阶段 4：测试验证**
1. 测试握手流程
2. 测试数据收发
3. 测试连接管理
4. 性能对比测试

### 15.4 预期收益

**代码简化**：
- 删除 `httppoll_manager.go`（~600 行）
- 简化 `handlers_httppoll.go`（减少 ~200 行）
- 总代码量减少 ~40%

**架构统一**：
- 所有协议使用相同的连接创建流程
- 所有协议使用相同的 `StreamProcessor` 处理
- 代码路径统一，易于调试

**维护性提升**：
- 职责清晰，易于理解
- 新增协议只需实现 `net.Conn`
- 减少特殊逻辑，降低 bug 风险

## 16. 后续优化方向

1. **HTTP/2 Server Push**：利用 HTTP/2 的服务器推送功能，减少轮询延迟
2. **WebSocket 降级**：HTTP 长轮询作为 WebSocket 的降级方案
3. **多路径传输**：同时使用多个 HTTP 连接提高可靠性
4. **智能路由**：根据网络状况自动选择最佳传输方式
5. **压缩优化**：对数据进行压缩后再 Base64 编码，减少传输量
6. **批量传输**：合并多个小数据包，减少 HTTP 请求次数

