# 隧道连接管理通用设计

> ✅ **状态**：统一接口已实现，位于 `internal/protocol/session/connection_interface.go`
> 
> 本文档说明如何通过统一接口抽象不同协议的连接管理差异，实现协议无关的连接状态管理、超时管理、错误处理和连接复用策略。

## 1. 设计原则

### 1.1 分层设计

隧道连接管理应该分为两层：

1. **通用层（Protocol-Agnostic）**：所有协议共享的概念和逻辑
   - TunnelID：逻辑隧道标识
   - MappingID：端口映射标识
   - 源端/目标端识别
   - 隧道生命周期管理
   - 桥接管理

2. **协议特定层（Protocol-Specific）**：各协议特有的实现
   - 连接标识方式（TCP 用 net.Conn，HTTP 长轮询用 ConnectionID）
   - 连接类型（HTTP 长轮询需要 control/data/keepalive，TCP 不需要）
   - 连接复用策略（TCP 可复用，HTTP 长轮询每次新建）
   - 连接状态管理

### 1.2 当前设计的问题

#### 问题 1：混入了协议特定概念
- **ConnectionID**：这是 HTTP 长轮询特有的，TCP/WebSocket/QUIC 不需要
- **TunnelType**：这是 HTTP 长轮询特有的（control/data/keepalive），TCP/WebSocket/QUIC 不需要

#### 问题 2：连接识别方式不统一
- **TCP/WebSocket/QUIC**：通过 `net.Conn` 直接识别，连接即标识
- **HTTP 长轮询**：通过 `ConnectionID` 识别，需要额外管理

#### 问题 3：连接复用策略不同
- **TCP/WebSocket/QUIC**：可以复用控制连接作为数据连接，或创建新连接
- **HTTP 长轮询**：每次隧道会话都创建新的数据连接

## 2. 通用层设计

### 2.1 核心标识符（所有协议通用）

#### TunnelID
- **作用**：唯一标识一个逻辑隧道（透传会话）
- **格式**：`{protocol}-tunnel-{timestamp}-{port}`
- **生命周期**：从隧道创建到隧道关闭
- **作用域**：全局唯一（跨 Server 节点）
- **协议无关**：所有协议都使用

#### MappingID
- **作用**：标识端口映射配置
- **格式**：`pmap_` + 随机字符串
- **生命周期**：配置存在期间
- **作用域**：全局唯一
- **协议无关**：所有协议都使用

### 2.2 连接角色识别（所有协议通用）

#### 源端连接（Source Connection）
- **定义**：Listen Client 到 Server 的数据连接
- **识别方式**：`connClientID == mapping.ListenClientID`
- **协议无关**：所有协议都通过 clientID 识别

#### 目标端连接（Target Connection）
- **定义**：Target Client 到 Server 的数据连接
- **识别方式**：`connClientID == mapping.TargetClientID`
- **协议无关**：所有协议都通过 clientID 识别

### 2.3 隧道桥接（所有协议通用）

隧道桥接使用 `TunnelConnectionInterface` 接口，实现协议无关的数据转发：

```go
// 通用隧道桥接接口
type TunnelBridge interface {
    // 设置源端连接（协议无关）
    SetSourceConnection(conn TunnelConnectionInterface)
    
    // 设置目标端连接（协议无关）
    SetTargetConnection(conn TunnelConnectionInterface)
    
    // 启动桥接
    Start() error
    
    // 关闭桥接
    Close() error
    
    // 获取隧道ID
    GetTunnelID() string
}
```

## 3. 协议特定层设计

### 3.1 TCP/WebSocket/QUIC 协议

#### 连接标识
- **方式**：直接使用 `net.Conn` 作为连接标识
- **特点**：
  - 连接即标识，不需要额外的 ConnectionID
  - 连接是持久的，可以复用
  - 连接状态通过 `net.Conn` 的状态管理

#### 连接类型
- **不需要区分**：TCP/WebSocket/QUIC 不需要区分 control/data/keepalive
- **控制连接**：通过握手和认证识别
- **数据连接**：通过 TunnelOpen 请求识别

#### 连接复用
- **可以复用**：控制连接可以用于数据连接
- **也可以新建**：每个隧道会话可以创建新连接
- **策略**：由客户端决定

#### 实现示例
```go
// TCP/WebSocket/QUIC 的连接管理
type TCPConnection struct {
    conn        net.Conn           // 连接即标识
    clientID    int64
    mappingID   string
    tunnelID    string
    stream      stream.PackageStreamer
}

// 识别连接
func (s *SessionManager) identifyTCPConnection(conn net.Conn, mappingID string) (isSource bool, err error) {
    // 通过 Stream 获取 clientID
    stream := getStreamFromConn(conn)
    clientID := stream.GetClientID()
    
    // 通过 mappingID 获取配置
    mapping := s.cloudControl.GetPortMapping(mappingID)
    
    // 判断是源端还是目标端
    isSource = (clientID == mapping.ListenClientID)
    return isSource, nil
}
```

### 3.2 HTTP 长轮询协议

#### 连接标识
- **方式**：使用 `ConnectionID` 作为连接标识
- **特点**：
  - 连接是无状态的，需要 ConnectionID 来标识
  - 每次请求都可能创建新连接
  - 连接状态通过 ConnectionID 管理

#### 连接类型
- **需要区分**：HTTP 长轮询需要区分 control/data/keepalive
- **control**：控制连接，用于握手、命令
- **data**：数据连接，用于隧道数据传输
- **keepalive**：保持连接请求

#### 连接复用
- **不能复用**：每次隧道会话都创建新的数据连接
- **原因**：HTTP 是无状态协议，连接是请求级别的
- **策略**：必须为每个隧道会话创建新连接

#### 实现示例
```go
// HTTP 长轮询的连接管理
type HTTPPollConnection struct {
    connectionID string              // 连接标识
    clientID     int64
    mappingID    string
    tunnelID     string
    tunnelType   string              // "control" | "data" | "keepalive"
    stream       stream.PackageStreamer
}

// 识别连接
func (s *SessionManager) identifyHTTPPollConnection(connID string, mappingID string) (isSource bool, err error) {
    // 通过 ConnectionID 查找连接
    conn := s.getConnectionByConnID(connID)
    if conn == nil {
        return false, fmt.Errorf("connection not found: %s", connID)
    }
    
    // 通过 Stream 获取 clientID
    stream := conn.Stream
    clientID := stream.GetClientID()
    
    // 通过 mappingID 获取配置
    mapping := s.cloudControl.GetPortMapping(mappingID)
    
    // 判断是源端还是目标端
    isSource = (clientID == mapping.ListenClientID)
    return isSource, nil
}
```

## 4. 统一接口设计

### 4.1 连接接口抽象

```go
// 通用连接接口（协议无关）
// 注意：实际代码中使用的是 TunnelConnectionInterface，与现有的 TunnelConnection 结构体不同
type TunnelConnectionInterface interface {
    // 获取连接标识（协议特定实现）
    GetConnectionID() string
    
    // 获取客户端ID（所有协议通用）
    GetClientID() int64
    
    // 获取映射ID（所有协议通用）
    GetMappingID() string
    
    // 获取隧道ID（所有协议通用）
    GetTunnelID() string
    
    // 获取流（所有协议通用）
    GetStream() stream.PackageStreamer
    
    // 获取底层连接（TCP/WebSocket/QUIC 返回 net.Conn，HTTP 长轮询返回 nil）
    GetNetConn() net.Conn
    
    // 连接状态管理（统一接口）
    ConnectionState() ConnectionStateManager
    ConnectionTimeout() ConnectionTimeoutManager
    ConnectionError() ConnectionErrorHandler
    ConnectionReuse() ConnectionReuseStrategy
    
    // 关闭连接（所有协议通用）
    Close() error
    IsClosed() bool
}

// TCP/WebSocket/QUIC 实现
type TCPTunnelConnection struct {
    connID    string              // 连接唯一标识
    conn      net.Conn
    clientID  int64
    mappingID string
    tunnelID  string
    stream    stream.PackageStreamer
    
    // 连接状态管理（统一接口）
    state    ConnectionStateManager
    timeout  ConnectionTimeoutManager
    error    ConnectionErrorHandler
    reuse    ConnectionReuseStrategy
}

func (c *TCPTunnelConnection) GetConnectionID() string {
    // TCP 使用连接的唯一标识（可以是 connID 或远程地址）
    // 实际实现中，应该使用 connID 而不是远程地址，因为远程地址可能重复
    return c.connID
}

func (c *TCPTunnelConnection) GetNetConn() net.Conn {
    return c.conn
}

// HTTP 长轮询实现
type HTTPPollTunnelConnection struct {
    connectionID string
    clientID     int64
    mappingID    string
    tunnelID     string
    stream       stream.PackageStreamer
    
    // 连接状态管理（统一接口）
    state    ConnectionStateManager
    timeout  ConnectionTimeoutManager
    error    ConnectionErrorHandler
    reuse    ConnectionReuseStrategy
}

func (c *HTTPPollTunnelConnection) GetConnectionID() string {
    return c.connectionID
}

func (c *HTTPPollTunnelConnection) GetNetConn() net.Conn {
    // HTTP 长轮询没有 net.Conn
    return nil
}

func (c *HTTPPollTunnelConnection) ConnectionState() ConnectionStateManager {
    return c.state
}

func (c *HTTPPollTunnelConnection) ConnectionTimeout() ConnectionTimeoutManager {
    return c.timeout
}

func (c *HTTPPollTunnelConnection) ConnectionError() ConnectionErrorHandler {
    return c.error
}

func (c *HTTPPollTunnelConnection) ConnectionReuse() ConnectionReuseStrategy {
    return c.reuse
}

func (c *HTTPPollTunnelConnection) IsClosed() bool {
    return c.state.IsClosed()
}
```

### 4.2 隧道桥接统一接口

```go
// 隧道桥接配置（协议无关）
type TunnelBridgeConfig struct {
    TunnelID       string
    MappingID      string
    SourceConn     TunnelConnectionInterface  // 使用统一接口
    BandwidthLimit int64
    CloudControl   CloudControlAPI
}

// 隧道桥接实现（协议无关）
type TunnelBridge struct {
    tunnelID    string
    mappingID   string
    sourceConn  TunnelConnectionInterface
    targetConn  TunnelConnectionInterface
    ready       chan struct{}
    // ... 其他字段 ...
}

func (b *TunnelBridge) SetSourceConnection(conn TunnelConnectionInterface) {
    b.sourceConn = conn
    // 协议无关的处理
}

func (b *TunnelBridge) SetTargetConnection(conn TunnelConnectionInterface) {
    b.targetConn = conn
    close(b.ready)
    // 协议无关的处理
}

func (b *TunnelBridge) Start() error {
    // 等待目标端连接建立
    <-b.ready
    
    // 使用统一接口进行数据转发
    // 通过 conn.GetStream() 或 conn.GetNetConn() 获取数据通道
    // 通过 conn.ConnectionState() 管理连接状态
    // 通过 conn.ConnectionTimeout() 管理超时
    // ...
}
```

## 5. 设计对比

### 5.1 当前设计（混合设计）

**优点**：
- 实现简单，直接针对 HTTP 长轮询优化
- 代码集中，易于理解

**缺点**：
- 混入了协议特定概念（ConnectionID, TunnelType）
- 其他协议需要适配这些概念
- 扩展性差，添加新协议需要修改通用逻辑

### 5.2 分层设计（推荐）

**优点**：
- 通用层协议无关，所有协议共享
- 协议特定层独立实现，互不影响
- 扩展性好，添加新协议只需实现协议特定层
- 代码清晰，职责分明

**缺点**：
- 实现复杂度稍高，需要抽象层
- 需要定义清晰的接口

## 6. 改进建议

### 6.1 重构方向

1. **提取通用层**：
   - 将 TunnelID、MappingID、源端/目标端识别等通用逻辑提取到通用层
   - 定义通用的连接接口和桥接接口

2. **协议特定实现**：
   - TCP/WebSocket/QUIC 实现基于 `net.Conn` 的连接管理
   - HTTP 长轮询实现基于 `ConnectionID` 的连接管理

3. **统一接口**：
   - 定义 `TunnelConnectionInterface` 接口，各协议实现
   - 定义 `TunnelBridge` 接口，使用 `TunnelConnectionInterface` 接口

### 6.2 实施步骤

1. **定义接口**：
   - 定义 `TunnelConnectionInterface` 及其子接口
   - 定义 `TunnelBridge` 接口

2. **实现协议特定层**：
   - TCP/WebSocket/QUIC 实现 `TunnelConnectionInterface`
   - HTTP 长轮询实现 `TunnelConnectionInterface`

3. **迁移通用层**：
   - `TunnelBridge` 使用 `TunnelConnectionInterface`
   - `SessionManager` 使用 `TunnelConnectionInterface`

4. **测试覆盖**：
   - 确保各协议的测试通过
   - 添加接口测试

## 7. 统一抽象接口设计（✅ 已实现）

### 7.1 连接状态管理（已统一）

**统一接口**：`ConnectionStateManager`
- **TCP/WebSocket/QUIC**：通过 `net.Conn` 的状态判断
- **HTTP 长轮询**：通过显式状态管理

**实现位置**：`internal/protocol/session/connection_interface.go`

```go
// 统一接口
type ConnectionStateManager interface {
    IsConnected() bool                    // 连接是否活跃
    IsClosed() bool                       // 连接是否已关闭
    GetState() ConnectionStateType        // 获取当前状态
    SetState(state ConnectionStateType)   // 设置状态
    UpdateActivity()                      // 更新活跃时间
    GetLastActiveTime() time.Time         // 获取最后活跃时间
    IsStale(timeout time.Duration) bool   // 检查是否超时失效
}

// 使用方式（协议无关）
func checkConnectionHealth(conn TunnelConnectionInterface) bool {
    state := conn.ConnectionState()
    return state.IsConnected() && !state.IsStale(60*time.Second)
}
```

**协议实现**：
- `TCPConnectionState`：基于 `net.Conn` 的状态管理
- `HTTPPollConnectionState`：显式状态管理

### 7.2 连接复用策略（已统一）

**统一接口**：`ConnectionReuseStrategy`
- **TCP/WebSocket/QUIC**：可以复用连接
- **HTTP 长轮询**：不能复用连接

**实现位置**：`internal/protocol/session/connection_interface.go`

```go
// 统一接口
type ConnectionReuseStrategy interface {
    CanReuse(conn TunnelConnectionInterface, tunnelID string) bool  // 检查是否可以复用
    ShouldCreateNew(tunnelID string) bool                            // 检查是否应该创建新连接
    MarkAsUsed(conn TunnelConnectionInterface, tunnelID string)      // 标记连接已使用
    GetReuseCount(conn TunnelConnectionInterface) int                 // 获取复用次数
}

// 使用方式（协议无关）
func getOrCreateConnection(tunnelID string, conn TunnelConnectionInterface) TunnelConnectionInterface {
    reuse := conn.ConnectionReuse()
    if reuse.ShouldCreateNew(tunnelID) {
        return createNewConnection()
    }
    // 查找可复用的连接
    if existing := findReusableConnection(reuse); existing != nil {
        if reuse.CanReuse(existing, tunnelID) {
            reuse.MarkAsUsed(existing, tunnelID)
            return existing
        }
    }
    return createNewConnection()
}
```

**协议实现**：
- `TCPConnectionReuse`：支持连接复用，可配置最大复用次数
- `HTTPPollConnectionReuse`：不支持连接复用，每次创建新连接

### 7.3 错误处理（已统一）

**统一接口**：`ConnectionErrorHandler`
- **TCP/WebSocket/QUIC**：通过 `net.Error` 判断
- **HTTP 长轮询**：通过 HTTP 状态码和超时判断

**实现位置**：`internal/protocol/session/connection_interface.go`

```go
// 统一接口
type ConnectionErrorHandler interface {
    HandleError(err error) error          // 处理错误
    IsRetryable(err error) bool           // 检查是否可重试
    ShouldClose(err error) bool           // 检查是否应该关闭连接
    ClassifyError(err error) ErrorType    // 分类错误类型
}

// 使用方式（协议无关）
func handleConnectionError(conn TunnelConnectionInterface, err error) {
    errorHandler := conn.ConnectionError()
    
    if errorHandler.ShouldClose(err) {
        conn.Close()
        return
    }
    
    if errorHandler.IsRetryable(err) {
        // 重试逻辑
        retryConnection(conn)
    } else {
        // 不可重试错误
        logError(errorHandler.ClassifyError(err))
    }
}
```

**协议实现**：
- `TCPConnectionError`：基于 `net.Error` 的错误处理
- `HTTPPollConnectionError`：基于 HTTP 状态码和超时的错误处理

### 7.4 超时管理（已统一）

**统一接口**：`ConnectionTimeoutManager`
- **TCP/WebSocket/QUIC**：通过 `net.Conn.SetDeadline` 管理
- **HTTP 长轮询**：通过 HTTP 请求超时管理

**实现位置**：`internal/protocol/session/connection_interface.go`

```go
// 统一接口
type ConnectionTimeoutManager interface {
    SetReadDeadline(t time.Time) error    // 设置读取超时
    SetWriteDeadline(t time.Time) error   // 设置写入超时
    SetDeadline(t time.Time) error        // 设置读写超时
    IsReadTimeout(err error) bool         // 检查是否是读取超时
    IsWriteTimeout(err error) bool        // 检查是否是写入超时
    ResetDeadline() error                 // 重置超时
}

// 使用方式（协议无关）
func readWithTimeout(conn TunnelConnectionInterface, timeout time.Duration) ([]byte, error) {
    timeoutMgr := conn.ConnectionTimeout()
    deadline := time.Now().Add(timeout)
    
    if err := timeoutMgr.SetReadDeadline(deadline); err != nil {
        return nil, err
    }
    
    data, err := readData(conn)
    
    if timeoutMgr.IsReadTimeout(err) {
        // 超时处理
        return nil, fmt.Errorf("read timeout: %w", err)
    }
    
    return data, err
}
```

**协议实现**：
- `TCPConnectionTimeout`：基于 `net.Conn.SetDeadline` 的超时管理
- `HTTPPollConnectionTimeout`：基于 HTTP 请求超时的超时管理

## 8. 统一接口的优势

### 8.1 代码复用

**之前**（协议特定）：
```go
// TCP 处理
if tcpConn != nil {
    if err := tcpConn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
        return err
    }
}

// HTTP 长轮询处理
if httpConn != nil {
    // 不同的超时处理方式
    httpConn.SetReadTimeout(timeout)
}
```

**现在**（统一接口）：
```go
// 所有协议统一处理
timeoutMgr := conn.ConnectionTimeout()
if err := timeoutMgr.SetReadDeadline(time.Now().Add(timeout)); err != nil {
    return err
}
```

### 8.2 易于扩展

添加新协议时，只需实现接口：
```go
// 新协议实现
type NewProtocolConnection struct {
    // ... 字段 ...
    state    ConnectionStateManager
    timeout  ConnectionTimeoutManager
    error    ConnectionErrorHandler
    reuse    ConnectionReuseStrategy
}

func (c *NewProtocolConnection) ConnectionState() ConnectionStateManager {
    return c.state
}

func (c *NewProtocolConnection) ConnectionTimeout() ConnectionTimeoutManager {
    return c.timeout
}

func (c *NewProtocolConnection) ConnectionError() ConnectionErrorHandler {
    return c.error
}

func (c *NewProtocolConnection) ConnectionReuse() ConnectionReuseStrategy {
    return c.reuse
}

// ... 其他接口实现 ...
```

### 8.3 测试友好

可以轻松创建 Mock 实现：
```go
type MockConnectionState struct {
    connected bool
    closed    bool
}

func (m *MockConnectionState) IsConnected() bool {
    return m.connected
}

func (m *MockConnectionState) IsClosed() bool {
    return m.closed
}

// ... 测试代码 ...
```

## 9. 实现位置

所有统一接口定义在：`internal/protocol/session/connection_interface.go`

- `TunnelConnectionInterface`：隧道连接主接口
- `ConnectionStateManager`：连接状态管理接口
- `ConnectionTimeoutManager`：超时管理接口
- `ConnectionErrorHandler`：错误处理接口
- `ConnectionReuseStrategy`：连接复用策略接口

各协议的实现：
- TCP：`TCPConnectionState`, `TCPConnectionTimeout`, `TCPConnectionError`, `TCPConnectionReuse`
- HTTP 长轮询：`HTTPPollConnectionState`, `HTTPPollConnectionTimeout`, `HTTPPollConnectionError`, `HTTPPollConnectionReuse`
- WebSocket/QUIC：类似实现

## 10. 新协议实现指南

### 10.1 实现步骤

添加新协议时，只需按照以下步骤实现：

#### 步骤 1：实现 `TunnelConnectionInterface`

创建新协议的连接结构体，实现 `TunnelConnectionInterface` 接口：

```go
// 新协议连接实现
type NewProtocolConnection struct {
    // 基础字段
    connID    string
    clientID  int64
    mappingID string
    tunnelID  string
    protocol  string
    
    // 协议特定字段
    // ... 根据协议特点添加字段 ...
    
    // 连接状态管理（统一接口）
    state    ConnectionStateManager
    timeout  ConnectionTimeoutManager
    error    ConnectionErrorHandler
    reuse    ConnectionReuseStrategy
    
    // 流和连接
    stream   stream.PackageStreamer
    netConn  net.Conn  // 如果有的话
}

// 实现基础信息方法
func (c *NewProtocolConnection) GetConnectionID() string {
    return c.connID
}

func (c *NewProtocolConnection) GetClientID() int64 {
    return c.clientID
}

func (c *NewProtocolConnection) GetMappingID() string {
    return c.mappingID
}

func (c *NewProtocolConnection) GetTunnelID() string {
    return c.tunnelID
}

func (c *NewProtocolConnection) GetProtocol() string {
    return c.protocol
}

// 实现流接口
func (c *NewProtocolConnection) GetStream() stream.PackageStreamer {
    return c.stream
}

func (c *NewProtocolConnection) GetNetConn() net.Conn {
    return c.netConn  // 如果没有，返回 nil
}

// 实现连接状态管理接口
func (c *NewProtocolConnection) ConnectionState() ConnectionStateManager {
    return c.state
}

func (c *NewProtocolConnection) ConnectionTimeout() ConnectionTimeoutManager {
    return c.timeout
}

func (c *NewProtocolConnection) ConnectionError() ConnectionErrorHandler {
    return c.error
}

func (c *NewProtocolConnection) ConnectionReuse() ConnectionReuseStrategy {
    return c.reuse
}

// 实现生命周期方法
func (c *NewProtocolConnection) Close() error {
    // 关闭连接和流
    if c.netConn != nil {
        c.netConn.Close()
    }
    if c.stream != nil {
        c.stream.Close()
    }
    // 更新状态
    c.state.SetState(StateClosed)
    return nil
}

func (c *NewProtocolConnection) IsClosed() bool {
    return c.state.IsClosed()
}
```

#### 步骤 2：实现连接状态管理器

根据协议特点实现 `ConnectionStateManager`：

```go
// 新协议连接状态管理器
type NewProtocolConnectionState struct {
    connectionID string
    state        ConnectionStateType
    createdAt    time.Time
    lastActive   time.Time
    closed       bool
    // 协议特定状态字段 ...
}

func NewNewProtocolConnectionState(connID string) *NewProtocolConnectionState {
    return &NewProtocolConnectionState{
        connectionID: connID,
        state:        StateConnected,
        createdAt:    time.Now(),
        lastActive:   time.Now(),
        closed:       false,
    }
}

// 实现 ConnectionStateManager 接口的所有方法
func (s *NewProtocolConnectionState) IsConnected() bool {
    return !s.closed && s.state != StateClosed
}

func (s *NewProtocolConnectionState) IsClosed() bool {
    return s.closed || s.state == StateClosed
}

func (s *NewProtocolConnectionState) GetState() ConnectionStateType {
    return s.state
}

func (s *NewProtocolConnectionState) SetState(state ConnectionStateType) {
    s.state = state
    if state == StateStreaming || state == StateConnected {
        s.lastActive = time.Now()
    }
    if state == StateClosed {
        s.closed = true
    }
}

func (s *NewProtocolConnectionState) UpdateActivity() {
    s.lastActive = time.Now()
}

func (s *NewProtocolConnectionState) GetLastActiveTime() time.Time {
    return s.lastActive
}

func (s *NewProtocolConnectionState) GetCreatedTime() time.Time {
    return s.createdAt
}

func (s *NewProtocolConnectionState) IsStale(timeout time.Duration) bool {
    return time.Since(s.lastActive) > timeout
}
```

#### 步骤 3：实现超时管理器

根据协议特点实现 `ConnectionTimeoutManager`：

```go
// 新协议超时管理器
type NewProtocolConnectionTimeout struct {
    // 如果有 net.Conn，使用它
    conn         net.Conn
    readTimeout  time.Duration
    writeTimeout time.Duration
    idleTimeout  time.Duration
    lastRead     time.Time
    lastWrite    time.Time
}

func NewNewProtocolConnectionTimeout(conn net.Conn, readTimeout, writeTimeout, idleTimeout time.Duration) *NewProtocolConnectionTimeout {
    return &NewProtocolConnectionTimeout{
        conn:         conn,
        readTimeout:  readTimeout,
        writeTimeout: writeTimeout,
        idleTimeout:  idleTimeout,
        lastRead:     time.Now(),
        lastWrite:    time.Now(),
    }
}

// 实现 ConnectionTimeoutManager 接口的所有方法
func (t *NewProtocolConnectionTimeout) SetReadDeadline(deadline time.Time) error {
    if t.conn != nil {
        return t.conn.SetReadDeadline(deadline)
    }
    // 如果没有 net.Conn，使用协议特定的超时机制
    t.readTimeout = time.Until(deadline)
    return nil
}

func (t *NewProtocolConnectionTimeout) SetWriteDeadline(deadline time.Time) error {
    if t.conn != nil {
        return t.conn.SetWriteDeadline(deadline)
    }
    t.writeTimeout = time.Until(deadline)
    return nil
}

func (t *NewProtocolConnectionTimeout) SetDeadline(deadline time.Time) error {
    if t.conn != nil {
        return t.conn.SetDeadline(deadline)
    }
    timeout := time.Until(deadline)
    t.readTimeout = timeout
    t.writeTimeout = timeout
    return nil
}

func (t *NewProtocolConnectionTimeout) GetReadTimeout() time.Duration {
    return t.readTimeout
}

func (t *NewProtocolConnectionTimeout) GetWriteTimeout() time.Duration {
    return t.writeTimeout
}

func (t *NewProtocolConnectionTimeout) GetIdleTimeout() time.Duration {
    return t.idleTimeout
}

func (t *NewProtocolConnectionTimeout) IsReadTimeout(err error) bool {
    if err == nil {
        return false
    }
    // 根据协议特点判断超时错误
    // 例如：TCP 使用 net.Error，HTTP 使用 context deadline
    netErr, ok := err.(net.Error)
    return ok && netErr.Timeout()
}

func (t *NewProtocolConnectionTimeout) IsWriteTimeout(err error) bool {
    return t.IsReadTimeout(err)
}

func (t *NewProtocolConnectionTimeout) IsIdleTimeout() bool {
    return time.Since(t.lastRead) > t.idleTimeout || time.Since(t.lastWrite) > t.idleTimeout
}

func (t *NewProtocolConnectionTimeout) ResetReadDeadline() error {
    if t.conn != nil && t.readTimeout > 0 {
        return t.conn.SetReadDeadline(time.Now().Add(t.readTimeout))
    }
    t.lastRead = time.Now()
    return nil
}

func (t *NewProtocolConnectionTimeout) ResetWriteDeadline() error {
    if t.conn != nil && t.writeTimeout > 0 {
        return t.conn.SetWriteDeadline(time.Now().Add(t.writeTimeout))
    }
    t.lastWrite = time.Now()
    return nil
}

func (t *NewProtocolConnectionTimeout) ResetDeadline() error {
    if t.conn != nil && t.idleTimeout > 0 {
        return t.conn.SetDeadline(time.Now().Add(t.idleTimeout))
    }
    t.lastRead = time.Now()
    t.lastWrite = time.Now()
    return nil
}
```

#### 步骤 4：实现错误处理器

根据协议特点实现 `ConnectionErrorHandler`：

```go
// 新协议错误处理器
type NewProtocolConnectionError struct {
    lastError error
}

func NewNewProtocolConnectionError() *NewProtocolConnectionError {
    return &NewProtocolConnectionError{}
}

// 实现 ConnectionErrorHandler 接口的所有方法
func (e *NewProtocolConnectionError) HandleError(err error) error {
    if err == nil {
        return nil
    }
    e.lastError = err
    return err
}

func (e *NewProtocolConnectionError) IsRetryable(err error) bool {
    if err == nil {
        return false
    }
    // 根据协议特点判断是否可重试
    // 例如：网络错误、超时错误可重试
    netErr, ok := err.(net.Error)
    return ok && (netErr.Timeout() || netErr.Temporary())
}

func (e *NewProtocolConnectionError) ShouldClose(err error) bool {
    if err == nil {
        return false
    }
    // EOF 和连接关闭错误应该关闭连接
    if err == io.EOF {
        return true
    }
    netErr, ok := err.(net.Error)
    if !ok {
        return false
    }
    // 非临时错误应该关闭连接
    return !netErr.Temporary()
}

func (e *NewProtocolConnectionError) IsTemporary(err error) bool {
    if err == nil {
        return false
    }
    netErr, ok := err.(net.Error)
    return ok && netErr.Temporary()
}

func (e *NewProtocolConnectionError) ClassifyError(err error) ErrorType {
    if err == nil {
        return ErrorNone
    }
    if err == io.EOF {
        return ErrorClosed
    }
    netErr, ok := err.(net.Error)
    if !ok {
        return ErrorUnknown
    }
    if netErr.Timeout() {
        return ErrorTimeout
    }
    if netErr.Temporary() {
        return ErrorNetwork
    }
    return ErrorUnknown
}

func (e *NewProtocolConnectionError) GetLastError() error {
    return e.lastError
}

func (e *NewProtocolConnectionError) ClearError() {
    e.lastError = nil
}
```

#### 步骤 5：实现连接复用策略

根据协议特点实现 `ConnectionReuseStrategy`：

```go
// 新协议连接复用策略
type NewProtocolConnectionReuse struct {
    reuseCounts map[string]int
    maxReuse    int
}

func NewNewProtocolConnectionReuse(maxReuse int) *NewProtocolConnectionReuse {
    return &NewProtocolConnectionReuse{
        reuseCounts: make(map[string]int),
        maxReuse:    maxReuse,
    }
}

// 实现 ConnectionReuseStrategy 接口的所有方法
func (r *NewProtocolConnectionReuse) CanReuse(conn TunnelConnectionInterface, tunnelID string) bool {
    // 根据协议特点判断是否可以复用
    // 例如：TCP 可以复用，HTTP 长轮询不能复用
    if conn == nil {
        return false
    }
    connID := conn.GetConnectionID()
    count := r.reuseCounts[connID]
    return count < r.maxReuse && !conn.IsClosed()
}

func (r *NewProtocolConnectionReuse) ShouldCreateNew(tunnelID string) bool {
    // 根据协议特点判断是否应该创建新连接
    // 例如：HTTP 长轮询必须创建新连接
    return false  // 默认可以复用
}

func (r *NewProtocolConnectionReuse) MarkAsReusable(conn TunnelConnectionInterface) {
    // 标记连接可复用
}

func (r *NewProtocolConnectionReuse) MarkAsUsed(conn TunnelConnectionInterface, tunnelID string) {
    connID := conn.GetConnectionID()
    r.reuseCounts[connID]++
}

func (r *NewProtocolConnectionReuse) Release(conn TunnelConnectionInterface) {
    // 释放连接（可复用）
}

func (r *NewProtocolConnectionReuse) GetReuseCount(conn TunnelConnectionInterface) int {
    connID := conn.GetConnectionID()
    return r.reuseCounts[connID]
}

func (r *NewProtocolConnectionReuse) GetMaxReuseCount() int {
    return r.maxReuse
}
```

#### 步骤 6：创建连接工厂函数

```go
// 创建新协议连接
func NewNewProtocolConnection(
    connID string,
    clientID int64,
    mappingID string,
    tunnelID string,
    stream stream.PackageStreamer,
    netConn net.Conn,  // 如果有的话
) *NewProtocolConnection {
    // 创建状态管理器
    state := NewNewProtocolConnectionState(connID)
    
    // 创建超时管理器
    timeout := NewNewProtocolConnectionTimeout(
        netConn,
        30*time.Second,  // readTimeout
        30*time.Second,  // writeTimeout
        60*time.Second,  // idleTimeout
    )
    
    // 创建错误处理器
    errorHandler := NewNewProtocolConnectionError()
    
    // 创建复用策略
    reuse := NewNewProtocolConnectionReuse(10)  // maxReuse
    
    return &NewProtocolConnection{
        connID:    connID,
        clientID:  clientID,
        mappingID: mappingID,
        tunnelID:  tunnelID,
        protocol:  "newprotocol",
        stream:    stream,
        netConn:   netConn,
        state:     state,
        timeout:   timeout,
        error:     errorHandler,
        reuse:     reuse,
    }
}
```

### 10.2 实现检查清单

实现新协议时，请确保完成以下检查：

#### ✅ 基础接口实现
- [ ] 实现 `GetConnectionID()` 方法
- [ ] 实现 `GetClientID()` 方法
- [ ] 实现 `GetMappingID()` 方法
- [ ] 实现 `GetTunnelID()` 方法
- [ ] 实现 `GetProtocol()` 方法
- [ ] 实现 `GetStream()` 方法
- [ ] 实现 `GetNetConn()` 方法（如果没有，返回 nil）

#### ✅ 连接状态管理
- [ ] 实现 `ConnectionState()` 方法
- [ ] 实现 `ConnectionStateManager` 接口的所有方法
- [ ] 正确管理连接状态（Connecting, Connected, Streaming, Closing, Closed）
- [ ] 正确更新活跃时间

#### ✅ 超时管理
- [ ] 实现 `ConnectionTimeout()` 方法
- [ ] 实现 `ConnectionTimeoutManager` 接口的所有方法
- [ ] 正确处理读取/写入/空闲超时
- [ ] 正确判断超时错误

#### ✅ 错误处理
- [ ] 实现 `ConnectionError()` 方法
- [ ] 实现 `ConnectionErrorHandler` 接口的所有方法
- [ ] 正确分类错误类型（Network, Timeout, Protocol, Auth, Closed）
- [ ] 正确判断错误是否可重试

#### ✅ 连接复用策略
- [ ] 实现 `ConnectionReuse()` 方法
- [ ] 实现 `ConnectionReuseStrategy` 接口的所有方法
- [ ] 根据协议特点决定是否支持复用
- [ ] 正确管理复用计数

#### ✅ 生命周期管理
- [ ] 实现 `Close()` 方法
- [ ] 实现 `IsClosed()` 方法
- [ ] 正确清理资源
- [ ] 正确更新连接状态

#### ✅ 测试
- [ ] 单元测试覆盖所有接口方法
- [ ] 集成测试验证协议功能
- [ ] 测试连接状态管理
- [ ] 测试超时管理
- [ ] 测试错误处理
- [ ] 测试连接复用策略

### 10.3 协议特性对照表

根据协议特性选择合适的实现方式：

| 特性 | TCP/WebSocket/QUIC | HTTP 长轮询 | 新协议参考 |
|------|-------------------|-------------|-----------|
| **连接标识** | `net.Conn` | `ConnectionID` | 根据协议选择 |
| **连接持久性** | 持久连接 | 无状态连接 | 根据协议选择 |
| **连接复用** | 支持复用 | 不支持复用 | 根据协议选择 |
| **状态管理** | 基于 `net.Conn` | 显式状态管理 | 根据协议选择 |
| **超时管理** | `net.Conn.SetDeadline` | HTTP 请求超时 | 根据协议选择 |
| **错误处理** | `net.Error` | HTTP 状态码 | 根据协议选择 |
| **数据通道** | `net.Conn` | `stream.PackageStreamer` | 根据协议选择 |

### 10.4 快速开始模板

可以使用以下模板快速开始新协议的实现：

```go
package session

import (
    "net"
    "time"
    "tunnox-core/internal/stream"
)

// 1. 定义连接结构体
type NewProtocolConnection struct {
    // 基础字段
    connID    string
    clientID  int64
    mappingID string
    tunnelID  string
    protocol  string
    
    // 协议特定字段
    // TODO: 添加协议特定字段
    
    // 连接状态管理
    state    ConnectionStateManager
    timeout  ConnectionTimeoutManager
    error    ConnectionErrorHandler
    reuse    ConnectionReuseStrategy
    
    // 流和连接
    stream   stream.PackageStreamer
    netConn  net.Conn
}

// 2. 实现 TunnelConnectionInterface
// TODO: 实现所有接口方法

// 3. 实现状态管理器
type NewProtocolConnectionState struct {
    // TODO: 实现 ConnectionStateManager
}

// 4. 实现超时管理器
type NewProtocolConnectionTimeout struct {
    // TODO: 实现 ConnectionTimeoutManager
}

// 5. 实现错误处理器
type NewProtocolConnectionError struct {
    // TODO: 实现 ConnectionErrorHandler
}

// 6. 实现复用策略
type NewProtocolConnectionReuse struct {
    // TODO: 实现 ConnectionReuseStrategy
}

// 7. 创建工厂函数
func NewNewProtocolConnection(...) *NewProtocolConnection {
    // TODO: 创建并初始化连接
}
```

### 10.5 常见问题

#### Q1: 如果协议没有 `net.Conn` 怎么办？
**A**: `GetNetConn()` 返回 `nil`，数据通过 `GetStream()` 获取的 `stream.PackageStreamer` 传输。

#### Q2: 如果协议不支持连接复用怎么办？
**A**: 在 `ConnectionReuseStrategy` 的 `CanReuse()` 和 `ShouldCreateNew()` 方法中返回 `false` 和 `true`。

#### Q3: 如何判断协议的超时错误？
**A**: 在 `ConnectionTimeoutManager.IsReadTimeout()` 和 `IsWriteTimeout()` 方法中，根据协议特点判断错误类型。

#### Q4: 如何管理协议特定的状态？
**A**: 在 `ConnectionStateManager` 实现中添加协议特定的状态字段，通过 `SetState()` 和 `GetState()` 管理。

#### Q5: 如何集成到现有系统？
**A**: 在 `SessionManager` 中注册新协议的连接创建逻辑，确保新协议连接实现 `TunnelConnectionInterface` 接口。

### 10.6 辅助工具和最佳实践

#### 辅助函数：创建默认管理器

可以创建辅助函数来简化管理器的创建：

```go
// 创建默认的连接状态管理器（适用于大多数协议）
func NewDefaultConnectionState(connID string) *DefaultConnectionState {
    return &DefaultConnectionState{
        connectionID: connID,
        state:        StateConnected,
        createdAt:    time.Now(),
        lastActive:   time.Now(),
        closed:       false,
    }
}

// 创建默认的超时管理器（适用于有 net.Conn 的协议）
func NewDefaultConnectionTimeout(
    conn net.Conn,
    readTimeout, writeTimeout, idleTimeout time.Duration,
) *DefaultConnectionTimeout {
    return &DefaultConnectionTimeout{
        conn:         conn,
        readTimeout:  readTimeout,
        writeTimeout: writeTimeout,
        idleTimeout:  idleTimeout,
        lastRead:     time.Now(),
        lastWrite:    time.Now(),
    }
}

// 创建默认的错误处理器（适用于大多数协议）
func NewDefaultConnectionError() *DefaultConnectionError {
    return &DefaultConnectionError{}
}

// 创建默认的复用策略（可配置是否支持复用）
func NewDefaultConnectionReuse(supportReuse bool, maxReuse int) *DefaultConnectionReuse {
    return &DefaultConnectionReuse{
        supportReuse: supportReuse,
        reuseCounts:  make(map[string]int),
        maxReuse:     maxReuse,
    }
}
```

#### 基类模式：复用通用实现

对于相似的协议，可以创建基类来复用代码：

```go
// 基础连接实现（适用于有 net.Conn 的协议）
type BaseNetConnConnection struct {
    connID    string
    clientID  int64
    mappingID string
    tunnelID  string
    protocol  string
    conn      net.Conn
    stream    stream.PackageStreamer
    
    state    ConnectionStateManager
    timeout  ConnectionTimeoutManager
    error    ConnectionErrorHandler
    reuse    ConnectionReuseStrategy
}

// 实现通用方法
func (c *BaseNetConnConnection) GetConnectionID() string {
    return c.connID
}

func (c *BaseNetConnConnection) GetClientID() int64 {
    return c.clientID
}

func (c *BaseNetConnConnection) GetMappingID() string {
    return c.mappingID
}

func (c *BaseNetConnConnection) GetTunnelID() string {
    return c.tunnelID
}

func (c *BaseNetConnConnection) GetProtocol() string {
    return c.protocol
}

func (c *BaseNetConnConnection) GetStream() stream.PackageStreamer {
    return c.stream
}

func (c *BaseNetConnConnection) GetNetConn() net.Conn {
    return c.conn
}

func (c *BaseNetConnConnection) ConnectionState() ConnectionStateManager {
    return c.state
}

func (c *BaseNetConnConnection) ConnectionTimeout() ConnectionTimeoutManager {
    return c.timeout
}

func (c *BaseNetConnConnection) ConnectionError() ConnectionErrorHandler {
    return c.error
}

func (c *BaseNetConnConnection) ConnectionReuse() ConnectionReuseStrategy {
    return c.reuse
}

func (c *BaseNetConnConnection) Close() error {
    if c.conn != nil {
        c.conn.Close()
    }
    if c.stream != nil {
        c.stream.Close()
    }
    c.state.SetState(StateClosed)
    return nil
}

func (c *BaseNetConnConnection) IsClosed() bool {
    return c.state.IsClosed()
}

// 新协议可以嵌入基类
type NewProtocolConnection struct {
    *BaseNetConnConnection
    // 协议特定字段
    // ...
}
```

#### 配置化实现：通过配置决定行为

可以通过配置来决定连接的行为：

```go
// 连接配置
type ConnectionConfig struct {
    // 超时配置
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
    IdleTimeout  time.Duration
    
    // 复用配置
    SupportReuse bool
    MaxReuse      int
    
    // 协议特定配置
    // ...
}

// 使用配置创建连接
func NewProtocolConnectionWithConfig(
    connID string,
    clientID int64,
    mappingID string,
    tunnelID string,
    stream stream.PackageStreamer,
    netConn net.Conn,
    config *ConnectionConfig,
) *NewProtocolConnection {
    // 使用配置创建管理器
    state := NewDefaultConnectionState(connID)
    timeout := NewDefaultConnectionTimeout(
        netConn,
        config.ReadTimeout,
        config.WriteTimeout,
        config.IdleTimeout,
    )
    errorHandler := NewDefaultConnectionError()
    reuse := NewDefaultConnectionReuse(config.SupportReuse, config.MaxReuse)
    
    return &NewProtocolConnection{
        // ...
        state:   state,
        timeout: timeout,
        error:   errorHandler,
        reuse:   reuse,
    }
}
```

#### 测试辅助工具

提供测试辅助工具，简化测试编写：

```go
// Mock 连接（用于测试）
type MockTunnelConnection struct {
    ConnID    string
    ClientID  int64
    MappingID string
    TunnelID  string
    Protocol  string
    Stream    stream.PackageStreamer
    NetConn   net.Conn
    Closed    bool
    
    MockState    ConnectionStateManager
    MockTimeout  ConnectionTimeoutManager
    MockError    ConnectionErrorHandler
    MockReuse    ConnectionReuseStrategy
}

func (m *MockTunnelConnection) GetConnectionID() string {
    return m.ConnID
}

// ... 实现其他方法 ...

// 创建测试连接
func NewMockTunnelConnection() *MockTunnelConnection {
    return &MockTunnelConnection{
        ConnID:    "mock-conn-1",
        ClientID:  1,
        MappingID: "mock-mapping-1",
        TunnelID:  "mock-tunnel-1",
        Protocol:  "mock",
        Closed:    false,
        MockState: NewMockConnectionState(),
        // ...
    }
}
```

### 10.7 实现示例对比

#### 示例 1：TCP 协议（有 net.Conn，支持复用）

```go
// TCP 连接实现
type TCPTunnelConnection struct {
    *BaseNetConnConnection
}

func NewTCPTunnelConnection(...) *TCPTunnelConnection {
    // 使用 TCP 特定的配置
    config := &ConnectionConfig{
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  60 * time.Second,
        SupportReuse: true,
        MaxReuse:     10,
    }
    
    base := &BaseNetConnConnection{
        // ... 初始化基础字段 ...
        state:   NewTCPConnectionState(conn),
        timeout: NewTCPConnectionTimeout(conn, config.ReadTimeout, config.WriteTimeout, config.IdleTimeout),
        error:   NewTCPConnectionError(),
        reuse:   NewTCPConnectionReuse(config.MaxReuse),
    }
    
    return &TCPTunnelConnection{BaseNetConnConnection: base}
}
```

#### 示例 2：HTTP 长轮询（无 net.Conn，不支持复用）

```go
// HTTP 长轮询连接实现
type HTTPPollTunnelConnection struct {
    connectionID string
    clientID     int64
    mappingID    string
    tunnelID     string
    stream       stream.PackageStreamer
    
    state    ConnectionStateManager
    timeout  ConnectionTimeoutManager
    error    ConnectionErrorHandler
    reuse    ConnectionReuseStrategy
}

func NewHTTPPollTunnelConnection(...) *HTTPPollTunnelConnection {
    return &HTTPPollTunnelConnection{
        connectionID: connID,
        clientID:     clientID,
        mappingID:    mappingID,
        tunnelID:     tunnelID,
        stream:       stream,
        state:        NewHTTPPollConnectionState(connID),
        timeout:      NewHTTPPollConnectionTimeout(30*time.Second, 30*time.Second, 60*time.Second),
        error:        NewHTTPPollConnectionError(),
        reuse:        NewHTTPPollConnectionReuse(),  // 不支持复用
    }
}

func (c *HTTPPollTunnelConnection) GetNetConn() net.Conn {
    return nil  // HTTP 长轮询没有 net.Conn
}
```

### 10.8 代码生成工具（可选）

可以创建代码生成工具来自动生成协议实现的骨架：

```bash
# 使用工具生成新协议实现骨架
go run tools/protocol_generator.go \
    --protocol=newprotocol \
    --has-net-conn=true \
    --support-reuse=true \
    --output=internal/protocol/session/newprotocol_connection.go
```

## 11. 总结

### 10.1 设计评估

**分层设计（最优方案）**：
- ✅ 通用层协议无关，所有协议共享
- ✅ 协议特定层独立实现，互不影响
- ✅ 扩展性好，添加新协议只需实现协议特定层
- ✅ 代码清晰，职责分明
- ✅ 易于测试，可以独立测试各层
- ⚠️ 实现复杂度稍高，需要抽象层

### 10.2 核心优势

**采用分层设计**，理由：
1. **更好的扩展性**：添加新协议只需实现 `TunnelConnectionInterface` 及其子接口
2. **更清晰的职责**：通用逻辑和协议特定逻辑完全分离
3. **更好的可维护性**：修改协议特定逻辑不影响其他协议
4. **更好的可测试性**：可以独立测试各层，易于 Mock
5. **更好的代码复用**：通用逻辑只需实现一次

### 10.3 设计要点

1. **统一接口抽象**：所有协议通过 `TunnelConnectionInterface` 统一管理
2. **协议特定实现**：各协议独立实现状态管理、超时管理、错误处理、复用策略
3. **桥接统一化**：`TunnelBridge` 使用 `TunnelConnectionInterface`，实现协议无关的数据转发
4. **生命周期管理**：通过统一接口管理连接的创建、使用、关闭

