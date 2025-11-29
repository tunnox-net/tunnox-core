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

### 3.2 服务器实现

#### 3.2.1 上行端点处理（Push）

```go
func (s *HTTPServer) handlePush(w http.ResponseWriter, r *http.Request) {
    // 1. 认证
    clientID, token, err := s.authenticateRequest(r)
    if err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }
    
    // 2. 解析请求
    var req struct {
        Data      string `json:"data"`
        Seq       uint64 `json:"seq"`
        Timestamp int64  `json:"timestamp"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }
    
    // 3. 解码数据
    data, err := base64.StdEncoding.DecodeString(req.Data)
    if err != nil {
        http.Error(w, "Invalid data", http.StatusBadRequest)
        return
    }
    
    // 4. 处理数据（转发到 StreamProcessor）
    sessionMgr := s.getSessionManager()
    conn := sessionMgr.GetControlConnection(clientID)
    if conn != nil {
        // 将数据写入连接的 StreamProcessor
        // 这里需要适配，将 HTTP 数据转换为 StreamProcessor 输入
    }
    
    // 5. 返回 ACK
    resp := map[string]interface{}{
        "success":   true,
        "ack":       req.Seq,
        "timestamp": time.Now().Unix(),
    }
    json.NewEncoder(w).Encode(resp)
}
```

#### 3.2.2 下行端点处理（Poll）

```go
func (s *HTTPServer) handlePoll(w http.ResponseWriter, r *http.Request) {
    // 1. 认证
    clientID, token, err := s.authenticateRequest(r)
    if err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }
    
    // 2. 解析参数
    timeout := 30 // 默认 30 秒
    if t := r.URL.Query().Get("timeout"); t != "" {
        timeout, _ = strconv.Atoi(t)
    }
    since := uint64(0)
    if s := r.URL.Query().Get("since"); s != "" {
        since, _ = strconv.ParseUint(s, 10, 64)
    }
    
    // 3. 获取客户端连接
    sessionMgr := s.getSessionManager()
    conn := sessionMgr.GetControlConnection(clientID)
    if conn == nil {
        // 客户端未连接，返回空响应
        resp := map[string]interface{}{
            "success":   true,
            "data":      nil,
            "timeout":   true,
            "timestamp": time.Now().Unix(),
        }
        json.NewEncoder(w).Encode(resp)
        return
    }
    
    // 4. 获取 HTTP 长轮询连接管理器
    httppollMgr := s.getHTTPLongPollingManager()
    pollConn := httppollMgr.GetOrCreatePollConnection(clientID, conn)
    
    // 5. 长轮询：等待数据或超时
    ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
    defer cancel()
    
    // 从轮询连接的输出队列获取数据
    dataChan := pollConn.GetOutputChannel()
    
    select {
    case <-ctx.Done():
        // 超时，返回空响应
        resp := map[string]interface{}{
            "success":   true,
            "data":      nil,
            "timeout":   true,
            "timestamp": time.Now().Unix(),
        }
        json.NewEncoder(w).Encode(resp)
        
    case data := <-dataChan:
        // 有数据，立即返回
        encoded := base64.StdEncoding.EncodeToString(data)
        resp := map[string]interface{}{
            "success":   true,
            "data":      encoded,
            "seq":       pollConn.GetNextSeq(),
            "timestamp": time.Now().Unix(),
        }
        json.NewEncoder(w).Encode(resp)
    }
}
```

#### 3.2.3 HTTP 长轮询连接管理器

需要创建一个管理器来管理 HTTP 长轮询连接的生命周期：

```go
type HTTPLongPollingManager struct {
    sessionMgr SessionManager
    
    // 客户端连接映射
    pushConnections map[int64]*HTTPPushConnection  // clientID -> push connection
    pollConnections map[int64]*HTTPPollConnection  // clientID -> poll connection
    
    mu sync.RWMutex
}

type HTTPPushConnection struct {
    clientID    int64
    sessionConn *session.ClientConnection
    inputChan   chan []byte  // 接收来自 HTTP 的数据
    seq         uint64
    mu          sync.Mutex
}

type HTTPPollConnection struct {
    clientID    int64
    sessionConn *session.ClientConnection
    outputChan  chan []byte  // 发送到 HTTP 的数据
    seq         uint64
    mu          sync.Mutex
}

func (m *HTTPLongPollingManager) GetOrCreatePushConnection(clientID int64, sessionConn *session.ClientConnection) *HTTPPushConnection {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if conn, exists := m.pushConnections[clientID]; exists {
        return conn
    }
    
    conn := &HTTPPushConnection{
        clientID:    clientID,
        sessionConn: sessionConn,
        inputChan:   make(chan []byte, 100),
    }
    
    m.pushConnections[clientID] = conn
    
    // 启动数据转发 goroutine
    go m.forwardPushData(conn)
    
    return conn
}

func (m *HTTPLongPollingManager) forwardPushData(conn *HTTPPushConnection) {
    // 从 HTTP 接收数据，转发到 StreamProcessor
    for data := range conn.inputChan {
        // 将数据写入 StreamProcessor
        // 这里需要适配，将 HTTP 数据转换为 StreamProcessor 输入
        stream := conn.sessionConn.Stream
        // 构造 TransferPacket 并写入
    }
}
```

### 3.3 与现有体系集成

#### 3.3.1 客户端集成

**步骤 1**：在 `internal/client/control_connection.go` 中添加协议支持：

```go
case "httppoll", "http-long-polling", "httplp":
    conn, err = dialHTTPLongPolling(c.Ctx(), c.config.Server.Address, c.config.ClientID, c.config.Token)
```

**步骤 2**：创建 `internal/client/transport_httppoll.go`：

```go
package client

import (
    "context"
    "net"
)

func dialHTTPLongPolling(ctx context.Context, baseURL string, clientID int64, token string) (net.Conn, error) {
    return NewHTTPLongPollingConn(baseURL, clientID, token)
}
```

**步骤 3**：在 `internal/client/auto_connector.go` 中添加默认端点：

```go
var DefaultServerEndpoints = []ServerEndpoint{
    {Protocol: "tcp", Address: "gw.tunnox.net:8000"},
    {Protocol: "udp", Address: "gw.tunnox.net:8000"},
    {Protocol: "quic", Address: "gw.tunnox.net:443"},
    {Protocol: "websocket", Address: "https://gw.tunnox.net/_tunnox"},
    {Protocol: "httppoll", Address: "https://gw.tunnox.net"},  // 新增
}
```

**步骤 4**：在 `tryConnect` 方法中添加：

```go
case "httppoll", "http-long-polling", "httplp":
    conn, err = dialHTTPLongPolling(timeoutCtx, endpoint.Address, 0, "")
```

#### 3.3.2 服务器集成

**步骤 1**：在 `internal/api/server.go` 的 `registerRoutes` 方法中添加路由：

```go
// HTTP 长轮询端点（用于客户端连接）
api.HandleFunc("/tunnox/push", s.handleHTTPPush).Methods("POST")
api.HandleFunc("/tunnox/poll", s.handleHTTPPoll).Methods("GET")
```

**步骤 2**：创建 `internal/api/handlers_httppoll.go`：

```go
package api

import (
    "encoding/base64"
    "encoding/json"
    "net/http"
    "strconv"
    "time"
    
    "tunnox-core/internal/protocol/session"
)

// handleHTTPPush 处理客户端推送数据
func (s *ManagementAPIServer) handleHTTPPush(w http.ResponseWriter, r *http.Request) {
    // 1. 认证
    clientID, err := s.authenticateHTTPRequest(r)
    if err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }
    
    // 2. 获取 SessionManager
    if s.sessionMgr == nil {
        http.Error(w, "SessionManager not available", http.StatusInternalServerError)
        return
    }
    
    // 3. 获取客户端连接
    connInterface := s.sessionMgr.GetControlConnectionInterface(clientID)
    if connInterface == nil {
        http.Error(w, "Client not connected", http.StatusNotFound)
        return
    }
    
    // 4. 类型断言
    conn, ok := connInterface.(*session.ClientConnection)
    if !ok {
        http.Error(w, "Invalid connection type", http.StatusInternalServerError)
        return
    }
    
    // 5. 解析请求
    var req struct {
        Data      string `json:"data"`
        Seq       uint64 `json:"seq"`
        Timestamp int64  `json:"timestamp"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }
    
    // 6. 解码数据
    data, err := base64.StdEncoding.DecodeString(req.Data)
    if err != nil {
        http.Error(w, "Invalid data", http.StatusBadRequest)
        return
    }
    
    // 7. 将数据写入 StreamProcessor
    // 这里需要将 HTTP 数据转换为 TransferPacket
    // 可以通过创建一个适配器来实现
    
    // 8. 返回 ACK
    resp := map[string]interface{}{
        "success":   true,
        "ack":       req.Seq,
        "timestamp": time.Now().Unix(),
    }
    json.NewEncoder(w).Encode(resp)
}

// handleHTTPPoll 处理客户端长轮询
func (s *ManagementAPIServer) handleHTTPPoll(w http.ResponseWriter, r *http.Request) {
    // 实现长轮询逻辑
    // ...
}
```

**步骤 3**：创建 HTTP 长轮询适配器，将 HTTP 数据转换为 StreamProcessor 输入：

```go
// internal/protocol/adapter/httppoll_adapter.go
package adapter

import (
    "io"
    "net"
    "tunnox-core/internal/protocol/session"
    "tunnox-core/internal/stream"
)

// HTTPPollAdapter HTTP 长轮询适配器
// 将 HTTP 长轮询连接包装成 net.Conn，供 StreamProcessor 使用
type HTTPPollAdapter struct {
    BaseAdapter
    pushConn *HTTPPushConn
    pollConn *HTTPPollConn
}

// HTTPPushConn HTTP 推送连接（客户端 -> 服务器）
type HTTPPushConn struct {
    sessionConn *session.ClientConnection
    inputChan   chan []byte
    closed      bool
}

// HTTPPollConn HTTP 轮询连接（服务器 -> 客户端）
type HTTPPollConn struct {
    sessionConn *session.ClientConnection
    outputChan  chan []byte
    closed      bool
}

func (c *HTTPPushConn) Read(p []byte) (int, error) {
    data := <-c.inputChan
    if data == nil {
        return 0, io.EOF
    }
    return copy(p, data), nil
}

func (c *HTTPPushConn) Write(p []byte) (int, error) {
    // 数据已经通过 HTTP POST 发送，这里不需要实现
    return len(p), nil
}

// ... 实现其他 net.Conn 方法 ...
```

#### 3.3.3 数据流转换

**关键点**：HTTP 长轮询的数据需要与 `StreamProcessor` 的数据格式兼容。

**方案**：
1. HTTP 传输的是 Base64 编码的 `TransferPacket` 序列化数据
2. 客户端：`StreamProcessor` → 序列化 → Base64 → HTTP POST
3. 服务器：HTTP POST → Base64 解码 → 反序列化 → `StreamProcessor`
4. 服务器：`StreamProcessor` → 序列化 → Base64 → HTTP GET 响应
5. 客户端：HTTP GET 响应 → Base64 解码 → 反序列化 → `StreamProcessor`

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

## 12. 数据流详细设计

### 12.1 客户端数据流

```
应用程序数据
    ↓
StreamProcessor.WritePacket()
    ↓
TransferPacket 序列化（二进制）
    ↓
Base64 编码
    ↓
HTTP POST /api/v1/tunnox/push
    ↓
服务器接收
```

### 12.2 服务器数据流

```
StreamProcessor 输出
    ↓
TransferPacket 序列化（二进制）
    ↓
Base64 编码
    ↓
缓存到输出队列
    ↓
HTTP GET /api/v1/tunnox/poll (长轮询)
    ↓
返回 JSON 响应
```

### 12.3 数据格式转换

**关键实现**：需要创建一个适配层，将 HTTP 请求/响应与 StreamProcessor 连接。

```go
// HTTPLongPollingStreamAdapter 适配器
type HTTPLongPollingStreamAdapter struct {
    pushConn *HTTPPushConn
    pollConn *HTTPPollConn
    stream   stream.PackageStreamer
}

// 从 HTTP POST 接收数据，写入 StreamProcessor
func (a *HTTPLongPollingStreamAdapter) handlePushData(data []byte) {
    // Base64 解码
    decoded, _ := base64.StdEncoding.DecodeString(string(data))
    
    // 反序列化为 TransferPacket
    pkt := deserializeTransferPacket(decoded)
    
    // 写入 StreamProcessor
    a.stream.WritePacket(pkt, false, 0)
}

// 从 StreamProcessor 读取数据，通过 HTTP GET 返回
func (a *HTTPLongPollingStreamAdapter) handlePollRequest() []byte {
    // 从 StreamProcessor 读取
    pkt, _, err := a.stream.ReadPacket()
    if err != nil {
        return nil
    }
    
    // 序列化 TransferPacket
    serialized := serializeTransferPacket(pkt)
    
    // Base64 编码
    encoded := base64.StdEncoding.EncodeToString(serialized)
    
    return []byte(encoded)
}
```

## 13. 连接生命周期管理

### 13.1 客户端连接管理

- **连接建立**：首次 HTTP POST/GET 请求时建立
- **连接保持**：通过长轮询保持连接活跃
- **连接断开**：HTTP 请求超时或错误时断开
- **自动重连**：检测到断开后自动重连

### 13.2 服务器连接管理

- **连接注册**：HTTP 请求到达时注册连接
- **连接超时**：30 秒无活动后清理连接
- **数据缓存**：连接断开时缓存未推送的数据
- **重连恢复**：客户端重连后推送缓存的数据

## 14. 与 SessionManager 集成

### 14.1 连接注册

当客户端通过 HTTP 长轮询连接时，需要在 `SessionManager` 中注册：

```go
// 在 handlePush 或 handlePoll 中
sessionMgr.RegisterHTTPLongPollingConnection(clientID, httppollConn)
```

### 14.2 数据路由

`SessionManager` 需要能够将数据路由到 HTTP 长轮询连接：

```go
// 当有数据需要发送给客户端时
if httppollConn := sessionMgr.GetHTTPLongPollingConnection(clientID); httppollConn != nil {
    httppollConn.SendData(data)
}
```

## 15. 后续优化方向

1. **HTTP/2 Server Push**：利用 HTTP/2 的服务器推送功能，减少轮询延迟
2. **WebSocket 降级**：HTTP 长轮询作为 WebSocket 的降级方案
3. **多路径传输**：同时使用多个 HTTP 连接提高可靠性
4. **智能路由**：根据网络状况自动选择最佳传输方式
5. **压缩优化**：对数据进行压缩后再 Base64 编码，减少传输量
6. **批量传输**：合并多个小数据包，减少 HTTP 请求次数

