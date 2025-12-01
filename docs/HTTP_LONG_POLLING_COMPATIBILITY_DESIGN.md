# HTTP 长轮询协议适配设计

## 设计原则

**核心思想：复用现有 Tunnox 协议包结构，通过 HTTP Request/Response 适配层实现无状态通信**

### 关键要点

1. **复用现有协议**：直接使用现有的 `packet.Type`（Handshake, JsonCommand, CommandResp, TunnelOpen, TunnelOpenAck 等）
2. **复用现有概念**：ClientId, ConnectionId, MappingId, SecretKey 等标识符完全沿用
3. **包转换器**：`HTTPPacketConverter` 用于在 Tunnox 包（`TransferPacket`）和 HTTP Request/Response 之间转换
4. **StreamProcessor 适配**：`HTTPStreamProcessor` 实现 `stream.PackageStreamer` 接口，内部使用 `HTTPPacketConverter` 进行转换
5. **Header 传指令，Body 传数据**：所有指令包通过 Header 传递，Body 只传输数据流
6. **续连接机制**：通过 Poll 请求实现续连接，不需要额外的续连接指令
7. **ConnectionID 由服务端生成**：在握手 ACK 时返回，保证安全性和全局唯一性

## 网络结构

```
User(U) - Client1(C1) - Server(S) - Client2(C2) - Target(T)
         ------> 7788      <--------->                 x.x.x.x:3306
```

**通信方式变化**：从有状态的连接变成无状态的 Request/Response

## HTTP Header 结构

### 简化设计：只使用 X-Tunnel-Package（或 X-Packet-Data）

**核心原则**：所有信息都放在包内，与 Tunnox 协议包内容保持一致，不需要额外的 Header。

**请求头**：
- **X-Tunnel-Package**: 控制包数据（Base64+Gzip 编码的 JSON）
  - 包含 `ConnectionID`, `ClientID`, `MappingID`, `Type`, `Data` 等所有信息
  - 与 `TunnelPackage` 结构完全对应

**响应头**：
- **X-Tunnel-Package**: 响应包数据（Base64+Gzip 编码）
  - 包含 `ConnectionID`, `ClientID`, `Type`, `Data` 等所有信息
  - 与 `TunnelPackage` 结构完全对应

**优势**：
1. **简化设计**：只需要一个 Header，所有信息都在包里
2. **与协议一致**：完全复用 Tunnox 协议包结构，不需要额外映射
3. **易于维护**：新增字段只需在 `TunnelPackage` 中添加，不需要修改 Header 结构
4. **减少 Header 大小**：避免 Header 过大（某些代理服务器限制 Header 大小）

**性能考虑**：
- 虽然需要解码整个包才能获取 ConnectionID，但包很小（通常 < 1KB），解码开销可忽略
- 如果需要快速路由，可以在服务端缓存 ConnectionID 映射（解码后缓存）

**注意**：现有代码使用 `X-Tunnel-Package`，与 `TunnelPackage` 结构对应。如果统一命名，可以使用 `X-Packet-Data`，但建议保持 `X-Tunnel-Package` 以保持一致性。

## 包转换器设计

### 接口定义

**关键理解**：
- `HTTPPacketConverter` 是一个**转换器**，不是 `io.Reader/io.Writer`
- 它用于在 `TransferPacket` 和 `HTTP Request/Response` 之间进行转换
- 真正实现 `stream.PackageStreamer` 接口的是 `HTTPStreamProcessor`，它内部使用 `HTTPPacketConverter` 进行转换

```go
// HTTPPacketConverter HTTP 包转换器
// 用于在 Tunnox 包（TransferPacket）和 HTTP Request/Response 之间转换
// 注意：这不是 io.Reader/io.Writer，而是一个转换器
type HTTPPacketConverter interface {
    // WritePacket 将 Tunnox 包转换为 HTTP Request
    // 返回的 Request 包含所有必要的 Header 和 Body
    WritePacket(pkt *packet.TransferPacket) (*http.Request, error)
    
    // ReadPacket 从 HTTP Response 读取 Tunnox 包
    ReadPacket(resp *http.Response) (*packet.TransferPacket, error)
    
    // WriteData 将字节流写入 HTTP Request Body（Base64 编码）
    WriteData(data []byte) (io.Reader, error)
    
    // ReadData 从 HTTP Response Body 读取字节流（Base64 解码）
    ReadData(resp *http.Response) ([]byte, error)
}
```

### TunnelPackage 与 TransferPacket 的关系

**关键理解**：
- `TunnelPackage` 是 HTTP 层的包装，包含连接元数据和包内容
- `TransferPacket` 是协议层的包，包含包类型和数据
- 转换关系：`TransferPacket` ↔ `TunnelPackage` ↔ HTTP Header

**TunnelPackage 结构**：
```go
type TunnelPackage struct {
    ConnectionID string      `json:"connection_id"`  // 连接标识（必须）
    ClientID     int64       `json:"client_id,omitempty"`  // 客户端ID（可选）
    MappingID    string      `json:"mapping_id,omitempty"`  // 映射ID（可选）
    TunnelType   string      `json:"tunnel_type,omitempty"`  // "control" | "data"
    Type         string      `json:"type,omitempty"`  // 包类型字符串："Handshake", "JsonCommand" 等
    Data         interface{} `json:"data,omitempty"`  // 包数据（TransferPacket 的内容）
}
```

### 实现要点

```go
// HTTPPacketConverter 需要维护连接状态
type HTTPPacketConverter struct {
    connectionID string
    clientID     int64
    mappingID    string
    tunnelType   string // "control" | "data"
}

// WritePacket 实现：TransferPacket -> TunnelPackage -> HTTP Request
func (c *HTTPPacketConverter) WritePacket(pkt *packet.TransferPacket) (*http.Request, error) {
    // 1. 提取包类型和数据
    packetType := pkt.PacketType
    var packetData interface{}
    
    switch {
    case packetType.IsJsonCommand() || packetType.IsCommandResp():
        // JsonCommand/CommandResp: 使用 CommandPacket
        packetData = pkt.CommandPacket
    default:
        // Handshake/HandshakeResp/TunnelOpen 等: 解析 Payload
        packetData = parsePayload(packetType, pkt.Payload)
    }
    
    // 2. 构建 TunnelPackage（所有信息都在这里）
    tunnelPkg := &httppoll.TunnelPackage{
        ConnectionID: c.connectionID,
        ClientID:     c.clientID,
        MappingID:    c.mappingID,
        TunnelType:   c.tunnelType,
        Type:         packetTypeToString(packetType),
        Data:         packetData,
    }
    
    // 3. 编码 TunnelPackage（JSON -> Gzip -> Base64）
    encoded, err := httppoll.EncodeTunnelPackage(tunnelPkg)
    if err != nil {
        return nil, err
    }
    
    // 4. 构建 HTTP Request（只设置 X-Tunnel-Package）
    req, _ := http.NewRequest("POST", "/tunnox/v1/push", nil)
    req.Header.Set("X-Tunnel-Package", encoded)
    
    return req, nil
}

// ReadPacket 实现：HTTP Response -> TunnelPackage -> TransferPacket
func (c *HTTPPacketConverter) ReadPacket(resp *http.Response) (*packet.TransferPacket, error) {
    // 1. 从 Header 读取 X-Tunnel-Package
    encoded := resp.Header.Get("X-Tunnel-Package")
    if encoded == "" {
        return nil, fmt.Errorf("missing X-Tunnel-Package header")
    }
    
    // 2. 解码 TunnelPackage
    tunnelPkg, err := httppoll.DecodeTunnelPackage(encoded)
    if err != nil {
        return nil, err
    }
    
    // 3. 更新连接状态（如果响应中包含新的 ConnectionID 等）
    if tunnelPkg.ConnectionID != "" {
        c.connectionID = tunnelPkg.ConnectionID
    }
    if tunnelPkg.ClientID > 0 {
        c.clientID = tunnelPkg.ClientID
    }
    if tunnelPkg.MappingID != "" {
        c.mappingID = tunnelPkg.MappingID
    }
    
    // 4. 转换为 TransferPacket
    return TunnelPackageToTransferPacket(tunnelPkg)
}

// 辅助函数：包类型字符串与字节的映射
func packetTypeToString(t packet.Type) string {
    baseType := t & 0x3F // 忽略压缩/加密标志
    switch baseType {
    case packet.Handshake:
        return "Handshake"
    case packet.HandshakeResp:
        return "HandshakeResponse"
    case packet.JsonCommand:
        return "JsonCommand"
    case packet.CommandResp:
        return "CommandResp"
    case packet.TunnelOpen:
        return "TunnelOpen"
    case packet.TunnelOpenAck:
        return "TunnelOpenAck"
    case packet.Heartbeat:
        return "Heartbeat"
    default:
        return fmt.Sprintf("Unknown_%d", baseType)
    }
}

func stringToPacketType(s string) packet.Type {
    switch s {
    case "Handshake":
        return packet.Handshake
    case "HandshakeResponse":
        return packet.HandshakeResp
    case "JsonCommand":
        return packet.JsonCommand
    case "CommandResp":
        return packet.CommandResp
    case "TunnelOpen":
        return packet.TunnelOpen
    case "TunnelOpenAck":
        return packet.TunnelOpenAck
    case "Heartbeat":
        return packet.Heartbeat
    default:
        return 0
    }
}

// 辅助函数：TunnelPackage -> TransferPacket
func TunnelPackageToTransferPacket(pkg *httppoll.TunnelPackage) (*packet.TransferPacket, error) {
    packetType := stringToPacketType(pkg.Type)
    if packetType == 0 {
        return nil, fmt.Errorf("unknown packet type: %s", pkg.Type)
    }
    
    var pkt *packet.TransferPacket
    switch {
    case packetType.IsJsonCommand() || packetType.IsCommandResp():
        // 从 Data 中提取 CommandPacket
        cmdPacket, ok := pkg.Data.(*packet.CommandPacket)
        if !ok {
            // 从 JSON 反序列化
            dataBytes, _ := json.Marshal(pkg.Data)
            cmdPacket = &packet.CommandPacket{}
            json.Unmarshal(dataBytes, cmdPacket)
        }
        pkt = &packet.TransferPacket{
            PacketType:    packetType,
            CommandPacket: cmdPacket,
        }
    default:
        // 对于 Handshake, HandshakeResp, TunnelOpen 等，序列化为 Payload
        payload, _ := json.Marshal(pkg.Data)
        pkt = &packet.TransferPacket{
            PacketType: packetType,
            Payload:    payload,
        }
    }
    
    return pkt, nil
}
```

## StreamProcessor 适配

### HTTPStreamProcessor

**关键理解**：
- `HTTPStreamProcessor` **实现** `stream.PackageStreamer` 接口
- 它内部使用 `HTTPPacketConverter` 进行 `TransferPacket` 和 HTTP Request/Response 之间的转换
- 上层代码调用 `ReadPacket()`/`WritePacket()` 等方法，内部转换为 HTTP 请求
- **注意**：HTTP 是无状态的，没有底层的 `io.Reader/io.Writer`，所以 `GetReader()` 和 `GetWriter()` 可能返回 `nil` 或空实现

**需要实现的接口方法**（`stream.PackageStreamer`）：
- `ReadPacket() (*packet.TransferPacket, int, error)` - 通过 HTTP Poll 实现
- `WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error)` - 通过 HTTP Push 实现
- `ReadExact(length int) ([]byte, error)` - 从数据流缓冲读取
- `WriteExact(data []byte) error` - 写入 HTTP Request Body
- `GetReader() io.Reader` - 可能返回 `nil`（HTTP 无状态）
- `GetWriter() io.Writer` - 可能返回 `nil`（HTTP 无状态）
- `Close()` - 关闭连接

```go
// HTTPStreamProcessor 实现 stream.PackageStreamer 接口
// 内部使用 HTTPPacketConverter 进行转换
// 注意：这是真正实现 PackageStreamer 接口的组件
type HTTPStreamProcessor struct {
    converter    HTTPPacketConverter
    httpClient   *http.Client
    pushURL      string
    pollURL      string
    
    // 连接信息
    connectionID string
    clientID     int64
    mappingID    string
    
    // 数据流缓冲
    dataBuffer   *bytes.Buffer
    packetQueue  chan *packet.TransferPacket
}

// ReadPacket 从 HTTP Poll 响应读取包
func (sp *HTTPStreamProcessor) ReadPacket() (*packet.TransferPacket, int, error) {
    // 1. 构建 Poll 请求的 TunnelPackage（续连接，只携带连接标识）
    pollPkg := &httppoll.TunnelPackage{
        ConnectionID: sp.connectionID,
        ClientID:     sp.clientID,
        MappingID:    sp.mappingID,
        // Type 为空，表示只是续连接，等待服务器响应
    }
    encoded, _ := httppoll.EncodeTunnelPackage(pollPkg)
    
    // 2. 发送 Poll 请求（只设置 X-Tunnel-Package）
    req, _ := http.NewRequest("GET", sp.pollURL, nil)
    req.Header.Set("X-Tunnel-Package", encoded)
    
    resp, err := sp.httpClient.Do(req)
    if err != nil {
        return nil, 0, err
    }
    defer resp.Body.Close()
    
    // 3. 检查是否有控制包（X-Tunnel-Package 中）
    if resp.Header.Get("X-Tunnel-Package") != "" {
        pkt, err := sp.converter.ReadPacket(resp)
        return pkt, 0, err
    }
    
    // 4. 读取 Body 数据流（如果有）
    if resp.ContentLength > 0 {
        data, _ := sp.converter.ReadData(resp)
        // 将数据放入缓冲，供 ReadExact 使用
        sp.dataBuffer.Write(data)
    }
    
    return nil, 0, nil
}

// WritePacket 通过 HTTP Push 发送包
func (sp *HTTPStreamProcessor) WritePacket(pkt *packet.TransferPacket, useCompression bool, rateLimitBytesPerSecond int64) (int, error) {
    // 1. 更新转换器的连接状态
    sp.converter.connectionID = sp.connectionID
    sp.converter.clientID = sp.clientID
    sp.converter.mappingID = sp.mappingID
    if sp.mappingID != "" {
        sp.converter.tunnelType = "data"
    } else {
        sp.converter.tunnelType = "control"
    }
    
    // 2. 转换为 HTTP Request（所有信息都在 X-Tunnel-Package 中）
    req, err := sp.converter.WritePacket(pkt)
    if err != nil {
        return 0, err
    }
    
    // 3. 发送请求
    resp, err := sp.httpClient.Do(req)
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()
    
    // 4. 处理响应（如果有控制包响应，在 X-Tunnel-Package 中）
    if resp.Header.Get("X-Tunnel-Package") != "" {
        respPkt, _ := sp.converter.ReadPacket(resp)
        // 将响应包放入队列，供后续读取
        if respPkt != nil {
            select {
            case sp.packetQueue <- respPkt:
            default:
                // 队列满，丢弃
            }
        }
    }
    
    return 0, nil
}

// WriteExact 将数据流写入 HTTP Request Body
func (sp *HTTPStreamProcessor) WriteExact(data []byte) error {
    // 1. Base64 编码数据
    bodyReader, _ := sp.converter.WriteData(data)
    
    // 2. 构建 HTTP Request
    // 数据流传输时，X-Tunnel-Package 只包含连接标识（用于路由）
    dataPkg := &httppoll.TunnelPackage{
        ConnectionID: sp.connectionID,
        ClientID:     sp.clientID,
        MappingID:    sp.mappingID,
        TunnelType:   "data",
        // Type 为空，表示这是数据流传输
    }
    encoded, _ := httppoll.EncodeTunnelPackage(dataPkg)
    
    req, _ := http.NewRequest("POST", sp.pushURL, bodyReader)
    req.Header.Set("X-Tunnel-Package", encoded)
    
    // 3. 发送请求
    resp, err := sp.httpClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    return nil
}

// ReadExact 从数据流缓冲读取指定长度
func (sp *HTTPStreamProcessor) ReadExact(length int) ([]byte, error) {
    // 从缓冲读取，如果不够则触发 Poll 请求获取更多数据
    for sp.dataBuffer.Len() < length {
        // 触发 Poll 获取更多数据
        sp.ReadPacket()
    }
    
    data := make([]byte, length)
    n, _ := sp.dataBuffer.Read(data)
    return data[:n], nil
}

// GetReader 获取底层 Reader（HTTP 无状态，返回 nil）
func (sp *HTTPStreamProcessor) GetReader() io.Reader {
    // HTTP 是无状态的，没有底层的 io.Reader
    // 返回 nil 或空实现，上层代码应该使用 ReadPacket() 和 ReadExact()
    return nil
}

// GetWriter 获取底层 Writer（HTTP 无状态，返回 nil）
func (sp *HTTPStreamProcessor) GetWriter() io.Writer {
    // HTTP 是无状态的，没有底层的 io.Writer
    // 返回 nil 或空实现，上层代码应该使用 WritePacket() 和 WriteExact()
    return nil
}

// Close 关闭连接
func (sp *HTTPStreamProcessor) Close() {
    // 关闭 HTTP 客户端连接
    // 清理资源
    close(sp.packetQueue)
    sp.dataBuffer.Reset()
}
```

## 匿名客户端连接流程

### 1.1 指令通道握手

**ConnectionID 生成策略**：
- **服务端生成**：服务端在握手 ACK 时生成并返回 ConnectionID，保证安全性和全局唯一性
- **首次握手**：客户端不提供 ConnectionID（或提供临时 ID 仅用于请求匹配）
- **后续请求**：客户端使用服务端分配的 ConnectionID

**客户端流程：**

```go
// 1. 创建 HTTPStreamProcessor（匿名连接）
sp := NewHTTPStreamProcessor(httpClient, pushURL, pollURL)
sp.connectionID = "" // 首次握手时为空，等待服务端分配
sp.clientID = 0 // 匿名客户端，clientID=0

// 2. 构建 Handshake 包（复用现有 packet.HandshakeRequest）
handshakeReq := &packet.HandshakeRequest{
    ClientID:       0, // 匿名客户端
    Token:          fmt.Sprintf("anonymous:%s", deviceID),
    Version:        "2.0",
    Protocol:       "httppoll",
    ConnectionType: "control",
}

// 3. 转换为 TransferPacket
pkt := &packet.TransferPacket{
    PacketType: packet.Handshake,
    Payload:    marshalJSON(handshakeReq),
}

// 4. 构建 TunnelPackage（首次握手，ConnectionID 为空）
tunnelPkg := &httppoll.TunnelPackage{
    ConnectionID: "", // 首次握手为空，等待服务端分配
    ClientID:     0,
    TunnelType:   "control",
    Type:         "Handshake",
    Data:         handshakeReq,
}
encoded, _ := httppoll.EncodeTunnelPackage(tunnelPkg)

// 5. 发送请求（所有信息都在 X-Tunnel-Package 中）
req, _ := http.NewRequest("POST", "/tunnox/v1/push", nil)
req.Header.Set("X-Tunnel-Package", encoded)
resp, _ := httpClient.Do(req)
```

**服务端流程：**

```go
// 1. 接收 HTTP Request
func handleHTTPPush(w http.ResponseWriter, r *http.Request) {
    // 2. 从 X-Tunnel-Package 解码（所有信息都在这里）
    encoded := r.Header.Get("X-Tunnel-Package")
    if encoded == "" {
        s.respondError(w, http.StatusBadRequest, "missing X-Tunnel-Package header")
        return
    }
    
    tunnelPkg, err := httppoll.DecodeTunnelPackage(encoded)
    if err != nil {
        s.respondError(w, http.StatusBadRequest, fmt.Sprintf("failed to decode tunnel package: %v", err))
        return
    }
    
    // 3. 转换为 TransferPacket
    pkt, err := TunnelPackageToTransferPacket(tunnelPkg)
    if err != nil {
        s.respondError(w, http.StatusBadRequest, fmt.Sprintf("failed to convert tunnel package: %v", err))
        return
    }
    
    // 4. 如果是握手请求，进入标准握手流程
    if tunnelPkg.Type == "Handshake" && tunnelPkg.ConnectionID == "" {
        // 5. 解析 HandshakeRequest
        handshakeReq, ok := tunnelPkg.Data.(*packet.HandshakeRequest)
        if !ok {
            // 从 JSON 反序列化
            dataBytes, _ := json.Marshal(tunnelPkg.Data)
            handshakeReq = &packet.HandshakeRequest{}
            json.Unmarshal(dataBytes, handshakeReq)
        }
        
        // 6. 进入标准 Tunnox 握手处理（复用现有逻辑）
        handshakeResp := handleHandshake(handshakeReq)
        
        // 7. 生成全局唯一的 ConnectionID（服务端生成，保证安全性和唯一性）
        connectionID := s.generateConnectionID()
        
        // 8. 登记连接
        clientAddr := r.RemoteAddr // IP:Port
        clientID := handshakeResp.ClientID
        secretKey := handshakeResp.SecretKey
        
        connectionMgr.Register(connectionID, &ConnectionInfo{
            ConnectionID: connectionID,
            ClientAddr:   clientAddr,
            ClientID:     clientID,
            SecretKey:    secretKey,
            ChannelType:  "control",
            CreatedAt:    time.Now(),
        })
        
        // 9. 在 HandshakeResponse 中添加 ConnectionID
        handshakeResp.ConnectionID = connectionID
        
        // 10. 构建响应 TunnelPackage（所有信息都在这里）
        respPkg := &httppoll.TunnelPackage{
            ConnectionID: connectionID,
            ClientID:     clientID,
            TunnelType:   "control",
            Type:         "HandshakeResponse",
            Data:         handshakeResp,
        }
        
        // 11. 通过 HTTP Response Header 返回（只设置 X-Tunnel-Package）
        encodedResp, _ := httppoll.EncodeTunnelPackage(respPkg)
        w.Header().Set("X-Tunnel-Package", encodedResp)
        
        w.WriteHeader(http.StatusOK)
    }
}
```

### 1.2 客户端收到响应

```go
// 1. 接收 HTTP Response
resp, _ := httpClient.Do(req)

// 2. 从 X-Tunnel-Package 读取响应包（所有信息都在这里）
respPkt, _ := sp.converter.ReadPacket(resp)

// 3. 解析 HandshakeResponse
var handshakeResp packet.HandshakeResponse
if respPkt.PacketType == packet.HandshakeResp {
    json.Unmarshal(respPkt.Payload, &handshakeResp)
}

// 4. 保存分配的凭据和 ConnectionID（从 HandshakeResponse 中获取）
sp.clientID = handshakeResp.ClientID
sp.secretKey = handshakeResp.SecretKey
sp.connectionID = handshakeResp.ConnectionID // 服务端分配的 ConnectionID

// 5. 更新转换器的连接状态
sp.converter.connectionID = sp.connectionID
sp.converter.clientID = sp.clientID

// 6. 启动 Poll 循环（续连接机制）
// LongPoll 的续连接机制：定期发送 Poll 请求，等待服务器响应
// 不需要额外的续连接指令，Poll 请求本身就是续连接的机制
go sp.startPollLoop()
```

**关键点**：
- **ConnectionID 由服务端分配**：在握手 ACK 时返回，客户端保存并使用
- **续连接机制**：通过 Poll 请求实现，不需要额外的续连接指令
- **Poll 循环**：客户端定期发送 Poll 请求（携带 ConnectionID），等待服务器响应

## 续连接机制分析

### 当前实现分析

**现状**：
- 当前代码中**没有明确的续连接指令**
- LongPoll 的 Poll 请求本身就起到了"续连接"的作用
- 客户端定期发送 Poll 请求（携带 ConnectionID），等待服务器响应

**续连接指令的必要性**：

**方案A：不需要续连接指令（推荐）**
- **理由**：
  - Poll 请求本身就是续连接的机制
  - 客户端定期发送 Poll 请求（无控制包，只有连接标识），等待服务器响应
  - 服务器在 Poll 响应中返回待处理的指令或数据
  - 更简单，符合 LongPoll 的标准模式

**方案B：保留续连接指令（备选）**
- **用途**：
  - 用于连接保活（heartbeat）
  - 用于连接状态同步
  - 用于接收服务器主动下发的指令（如配置更新、通知等）
- **实现**：
  ```go
  // 在 packet/packet.go 中新增
  const (
      // ... 现有指令 ...
      ResumeConnection Type = 0x24 // 续连接（LongPoll 专用）
  )
  
  // ResumeConnectionRequest 续连接请求
  type ResumeConnectionRequest struct {
      ConnectionID string `json:"connection_id"`
      ClientID     int64  `json:"client_id"`
  }
  
  // ResumeConnectionResponse 续连接响应
  type ResumeConnectionResponse struct {
      ConnectionID string `json:"connection_id"`
      Success      bool   `json:"success"`
  }
  ```

**推荐**：采用方案A，不需要续连接指令。Poll 请求本身就是续连接的机制。

## 隧道连接处理

### 处理流程

隧道连接的处理方式与指令通道相似，关键点：

1. **复用 Tunnox 包结构**：使用 `TunnelOpen` 和 `TunnelOpenAck`
2. **连接管理**：服务器在 HTTP LongPoll 连接管理器中登记，记录 connectionID 与 clientAddr 的映射
3. **响应路由**：根据 connectionID 查找对应的 clientAddr，将 Response 发送到正确的连接

### 隧道数据分片

**问题**：HTTP 请求的 Body 有大小限制，需要处理分片

**解决方案**：

```go
// 1. 每个分片设置最大大小（如 1MB）
const MaxChunkSize = 1024 * 1024

// 2. 发送数据时分片
func (sp *HTTPStreamProcessor) WriteExact(data []byte) error {
    for len(data) > 0 {
        chunkSize := Min(len(data), MaxChunkSize)
        chunk := data[:chunkSize]
        data = data[chunkSize:]
        
        // 3. 构建分片请求
        // 分片信息放在单独的 Header 中（仅用于数据流分片）
        // X-Tunnel-Package 只包含连接标识（用于路由）
        dataPkg := &httppoll.TunnelPackage{
            ConnectionID: sp.connectionID,
            ClientID:     sp.clientID,
            MappingID:    sp.mappingID,
            TunnelType:   "data",
            // Type 为空，表示这是数据流传输
        }
        encoded, _ := httppoll.EncodeTunnelPackage(dataPkg)
        
        req, _ := http.NewRequest("POST", sp.pushURL, encodeBase64(chunk))
        req.Header.Set("X-Tunnel-Package", encoded)
        // 分片信息放在单独的 Header 中（数据流传输的细节）
        req.Header.Set("X-Chunk-Index", strconv.Itoa(chunkIndex))
        req.Header.Set("X-Chunk-Total", strconv.Itoa(totalChunks))
        req.Header.Set("X-Chunk-Size", strconv.Itoa(chunkSize))
        
        // 4. 发送分片
        httpClient.Do(req)
    }
    return nil
}

// 5. 接收端按 connectionID 分组，按顺序合并
func handleHTTPPush(w http.ResponseWriter, r *http.Request) {
    // 从 X-Tunnel-Package 中提取 ConnectionID
    encoded := r.Header.Get("X-Tunnel-Package")
    tunnelPkg, _ := httppoll.DecodeTunnelPackage(encoded)
    connectionID := tunnelPkg.ConnectionID
    
    // 从单独的 Header 中获取分片信息（数据流传输的细节）
    chunkIndex := parseInt(r.Header.Get("X-Chunk-Index"))
    chunkTotal := parseInt(r.Header.Get("X-Chunk-Total"))
    
    // 从 Body 读取分片数据
    chunkData, _ := decodeBase64(r.Body)
    
    // 按 connectionID 分组存储
    chunkBuffer := getChunkBuffer(connectionID)
    chunkBuffer[chunkIndex] = chunkData
    
    // 如果所有分片都到达，按顺序合并
    if len(chunkBuffer) == chunkTotal {
        mergedData := mergeChunks(chunkBuffer)
        // 将合并后的数据交给上层处理
        processTunnelData(connectionID, mergedData)
    }
}
```

**关键点**：
- HTTP 基于 TCP，包是顺序到达的
- 只需要在对端按 connectionID 分组取出后，转成字节流合并即可
- 不需要复杂的重排序逻辑

## 服务器端连接管理

### 连接管理器

**关键理解**：HTTP 是无状态的，不需要通过 clientAddr 查找 net.Conn。

**正确的连接管理方式**：
- ConnectionID -> 连接对象（在内存中）
- 连接对象包含数据队列和状态信息
- Poll 请求时，从队列中取出数据，通过当前的 ResponseWriter 返回
- **不需要"回写 Response 到原有连接"**，因为响应就是针对当前 Poll 请求的

```go
type HTTPLongPollingConnectionManager struct {
    mu sync.RWMutex
    
    // connectionID -> ServerHTTPLongPollingConn（连接对象）
    connections map[string]*session.ServerHTTPLongPollingConn
}

type ConnectionInfo struct {
    ConnectionID string
    ClientID     int64
    SecretKey    string
    MappingID    string // 隧道连接才有
    ChannelType  string // "control" | "data"
    CreatedAt    time.Time
    LastActivity time.Time
}

// Register 登记连接
func (mgr *HTTPLongPollingConnectionManager) Register(connID string, conn *session.ServerHTTPLongPollingConn) {
    mgr.mu.Lock()
    defer mgr.mu.Unlock()
    mgr.connections[connID] = conn
}

// GetByConnectionID 根据 connectionID 获取连接对象
func (mgr *HTTPLongPollingConnectionManager) GetByConnectionID(connectionID string) *session.ServerHTTPLongPollingConn {
    mgr.mu.RLock()
    defer mgr.mu.RUnlock()
    return mgr.connections[connectionID]
}

// Remove 移除连接
func (mgr *HTTPLongPollingConnectionManager) Remove(connectionID string) {
    mgr.mu.Lock()
    defer mgr.mu.Unlock()
    delete(mgr.connections, connectionID)
}
```

**性能考虑**：
- 使用 `map[string]*ServerHTTPLongPollingConn`，O(1) 查找时间复杂度
- 使用 `sync.RWMutex` 保证并发安全
- 不需要通过 clientAddr 查找，直接通过 ConnectionID 查找连接对象

### 响应路由机制

**重要说明**：HTTP 是无状态的，服务器**不能主动**向客户端发送 Response。

**关键理解**：
- ❌ **错误理解**：客户端没有 request，服务端也能发 response 给客户端
- ❌ **错误理解**：需要通过 clientAddr 查找 net.Conn，然后"回写 Response 到原有连接"
- ✅ **正确理解**：服务端只能等待客户端的 request，然后在 response 中返回数据
- ✅ **正确理解**：响应就是针对当前 Poll 请求的，通过当前的 ResponseWriter 返回，不需要"回写"

**正确的流程**：

1. **服务器有数据要发送时**：放入该 connectionID 的连接对象的数据队列
2. **客户端定期发送 Poll 请求**：携带 connectionID（在 `X-Tunnel-Package` 中），等待服务器响应
3. **服务器根据 ConnectionID 查找连接对象**：O(1) 时间复杂度，直接 map 查找
4. **服务器从连接对象的数据队列取出数据**：在 Poll 响应中返回（通过 `X-Tunnel-Package` 和 Body）
5. **响应通过当前的 ResponseWriter 返回**：不需要"回写"，响应就是针对当前 Poll 请求的

**续连接机制**：
- 客户端定期发送 Poll 请求（续连接）
- 服务端在 Poll 响应中返回待处理的数据
- 不需要额外的续连接指令，Poll 请求本身就是续连接的机制

**性能优化**：
- 使用 `map[string]*ServerHTTPLongPollingConn`，O(1) 查找
- 不需要通过 clientAddr 查找，直接通过 ConnectionID 查找
- 连接对象包含数据队列，Poll 请求时直接从队列取出数据返回

**实现示例**：

```go
// 当服务器需要向客户端发送指令时
func (s *Server) SendCommandToClient(connectionID string, cmd *packet.JsonCommand) error {
    // 1. 查找连接信息
    connInfo := s.connMgr.GetByConnectionID(connectionID)
    if connInfo == nil {
        return fmt.Errorf("connection not found: %s", connectionID)
    }
    
    // 2. 构建响应包
    respPkt := &packet.TransferPacket{
        PacketType:    packet.JsonCommand,
        CommandPacket: &packet.CommandPacket{
            CommandType: cmd.CommandType,
            CommandBody: marshalJSON(cmd),
        },
    }
    
    // 3. 将响应包放入该连接的响应队列
    // 注意：服务器不能主动发送，只能等待客户端的 Poll 请求
    s.getConnectionResponseQueue(connectionID).Enqueue(respPkt)
    
    // 4. 通知等待的 Poll 请求（如果有）
    s.notifyPollRequest(connectionID)
    
    return nil
}

// 处理 Poll 请求
func (s *Server) handleHTTPPoll(w http.ResponseWriter, r *http.Request) {
    // 1. 从 X-Tunnel-Package 中提取 ConnectionID（所有信息都在包里）
    packageHeader := r.Header.Get("X-Tunnel-Package")
    if packageHeader == "" {
        s.respondError(w, http.StatusBadRequest, "missing X-Tunnel-Package header")
        return
    }
    
    // 2. 解码包（获取 ConnectionID）
    pkg, err := httppoll.DecodeTunnelPackage(packageHeader)
    if err != nil {
        s.respondError(w, http.StatusBadRequest, fmt.Sprintf("failed to decode tunnel package: %v", err))
        return
    }
    
    connectionID := pkg.ConnectionID
    
    // 3. 根据 ConnectionID 查找连接对象（O(1) 查找，直接 map 查找）
    conn := s.connMgr.GetByConnectionID(connectionID)
    if conn == nil {
        s.respondError(w, http.StatusNotFound, "connection not found")
        return
    }
    
    // 4. 设置超时（长轮询）
    timeout := 30 * time.Second
    ctx, cancel := context.WithTimeout(r.Context(), timeout)
    defer cancel()
    
    // 5. 从连接对象的数据队列获取待发送的数据（阻塞等待，直到有数据或超时）
    // 注意：响应通过当前的 ResponseWriter 返回，不需要"回写"
    base64Data, err := conn.PollData(ctx)
    if err == context.DeadlineExceeded {
        // 超时，返回空响应（客户端会立即发送下一个 Poll 请求）
        resp := HTTPPollResponse{
            Success:   true,
            Timeout:   true,
            Timestamp: time.Now().Unix(),
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(resp)
        return
    }
    if err != nil {
        s.respondError(w, http.StatusInternalServerError, err.Error())
        return
    }
    
    // 6. 检查是否有控制包响应（从连接对象获取）
    if respPkt := conn.GetPendingPacket(); respPkt != nil {
        respPkg := &httppoll.TunnelPackage{
            ConnectionID: connectionID,
            ClientID:     conn.GetClientID(),
            Type:         packetTypeToString(respPkt.PacketType),
            Data:         extractPacketData(respPkt),
        }
        encoded, _ := httppoll.EncodeTunnelPackage(respPkg)
        w.Header().Set("X-Tunnel-Package", encoded)
    }
    
    // 7. 如果有数据流，从 Body 返回
    if base64Data != "" {
        resp := HTTPPollResponse{
            Success:   true,
            Data:      base64Data,
            Timeout:   false,
            Timestamp: time.Now().Unix(),
        }
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(resp)
        return
    }
    
    // 8. 返回空响应
    w.WriteHeader(http.StatusOK)
}
```

**关键点**：
- 服务器**被动响应**：只能等待客户端的 Poll 请求
- **ConnectionID -> 连接对象**：直接通过 ConnectionID 查找连接对象（O(1) 查找）
- **连接对象包含数据队列**：服务器将待发送的数据放入连接对象的数据队列
- **响应通过当前 ResponseWriter 返回**：不需要"回写 Response 到原有连接"，响应就是针对当前 Poll 请求的
- 长轮询：Poll 请求会阻塞等待，直到有数据或超时
- 超时处理：超时是正常情况，客户端立即发送下一个 Poll 请求

**性能优化**：
- 使用 `map[string]*ServerHTTPLongPollingConn`，O(1) 查找时间复杂度
- 不需要通过 clientAddr 查找，直接通过 ConnectionID 查找
- 连接对象包含数据队列，Poll 请求时直接从队列取出数据返回
- 不需要维护 clientAddr 映射，简化连接管理

**行业最佳实践**：
- **直接通过 ConnectionID 查找连接对象**：O(1) 查找，性能最优
- **连接对象包含数据队列**：数据在连接对象中排队，Poll 请求时取出
- **响应通过当前 ResponseWriter 返回**：HTTP 无状态特性，响应就是针对当前请求的
- **不需要维护 clientAddr 映射**：简化连接管理，避免性能问题

## 潜在问题与改进建议

### 1. 续连接机制的必要性

**问题**：续连接指令（ResumeConnection）的作用不够明确。

**分析**：
- LongPoll 的 Poll 请求本身就可以形成 request，等待服务器响应
- 续连接指令是多余的，Poll 请求本身就是续连接的机制

**结论**：
- ✅ **不需要续连接指令**：直接使用 Poll 请求
  - 客户端定期发送 Poll 请求（携带 ConnectionID，无控制包）
  - 服务器在 Poll 响应中返回待处理的指令或数据
  - 更简单，符合 LongPoll 的标准模式

### 2. ConnectionID 生成策略（重要安全考虑）

**当前实现**：
- **客户端生成**：客户端在创建连接时生成 ConnectionID（UUID）
- **服务端验证**：服务端接收并验证 ConnectionID 格式
- **问题**：客户端生成 ConnectionID 存在安全风险

**安全风险分析**：
1. **客户端可能伪造 ConnectionID**：恶意客户端可能使用已存在的 ConnectionID，导致连接冲突
2. **ConnectionID 碰撞**：多个客户端可能生成相同的 ConnectionID（虽然概率低）
3. **无法保证全局唯一性**：客户端生成的 ConnectionID 无法保证在服务器集群中的全局唯一性

**推荐方案：服务端生成 ConnectionID**

**理由**：
1. **安全性**：服务端生成可以保证 ConnectionID 的唯一性和不可伪造性
2. **全局唯一性**：服务端可以保证 ConnectionID 在集群中的全局唯一性
3. **统一管理**：服务端可以统一管理 ConnectionID 的分配和回收

**实现方案**：

**方案A：在握手 ACK 时返回 ConnectionID（推荐）**

```go
// 客户端流程
// 1. 首次握手时，不提供 ConnectionID（或提供临时 ID）
handshakeReq := &packet.HandshakeRequest{
    ClientID: 0,
    Token:    "anonymous:device123",
    // 不提供 ConnectionID，或提供临时 ID 用于匹配请求
}

// 2. 发送握手请求
req.Header.Set("X-Packet-Type", strconv.Itoa(int(packet.Handshake)))
req.Header.Set("X-Packet-Data", encodePacketData(handshakeReq))
// 可选：提供临时 ID 用于匹配请求
req.Header.Set("X-Temp-Connection-ID", generateTempID())

// 3. 接收握手响应
resp, _ := httpClient.Do(req)

// 4. 从响应 Header 获取服务端分配的 ConnectionID
connectionID := resp.Header.Get("X-Connection-ID")
if connectionID == "" {
    // 从响应包中获取
    respPkg, _ := converter.ReadPacket(resp)
    if respPkg.PacketType == packet.HandshakeResp {
        var handshakeResp packet.HandshakeResponse
        json.Unmarshal(respPkg.Payload, &handshakeResp)
        connectionID = handshakeResp.ConnectionID // 需要在 HandshakeResponse 中添加
    }
}

// 5. 保存 ConnectionID，后续所有请求都使用这个 ID
sp.connectionID = connectionID
```

```go
// 服务端流程
func (s *Server) handleHandshake(w http.ResponseWriter, r *http.Request) {
    // 1. 解析握手请求
    handshakeReq := parseHandshakeRequest(r)
    
    // 2. 处理握手（生成 clientID, secretKey 等）
    handshakeResp := s.processHandshake(handshakeReq)
    
    // 3. 生成全局唯一的 ConnectionID
    connectionID := s.generateConnectionID() // 服务端生成，保证全局唯一
    
    // 4. 登记连接
    connectionMgr.Register(connectionID, &ConnectionInfo{
        ConnectionID: connectionID,
        ClientID:     handshakeResp.ClientID,
        SecretKey:    handshakeResp.SecretKey,
        // ...
    })
    
    // 5. 在响应中返回 ConnectionID
    w.Header().Set("X-Connection-ID", connectionID)
    
    // 6. 在 HandshakeResponse 中也包含 ConnectionID（双重保障）
    handshakeResp.ConnectionID = connectionID
    
    // 7. 返回响应
    respPkt := &packet.TransferPacket{
        PacketType: packet.HandshakeResp,
        Payload:    marshalJSON(handshakeResp),
    }
    encoded, _ := encodePacketData(respPkt.Payload)
    w.Header().Set("X-Packet-Type", strconv.Itoa(int(packet.HandshakeResp)))
    w.Header().Set("X-Packet-Data", encoded)
    
    w.WriteHeader(http.StatusOK)
}
```

**需要修改的数据结构**：

```go
// packet/packet.go
type HandshakeResponse struct {
    Success      bool   `json:"success"`
    Error        string `json:"error,omitempty"`
    Message      string `json:"message,omitempty"`
    SessionToken string `json:"session_token,omitempty"`
    ClientID     int64  `json:"client_id,omitempty"`
    SecretKey    string `json:"secret_key,omitempty"`
    ConnectionID string `json:"connection_id,omitempty"` // 新增：服务端分配的 ConnectionID
}
```

**方案B：在首次 Push 请求时返回 ConnectionID（备选）**

如果握手是异步的，可以在首次 Push 请求的响应中返回 ConnectionID。

**推荐**：采用方案A，在握手 ACK 时返回 ConnectionID，保证安全性和全局唯一性。

### 3. 连接超时和断开处理

**问题**：文档中没有明确说明如何处理连接超时和断开。

**需要补充**：
- **超时机制**：
  - Poll 请求超时（如 30 秒）后，客户端立即发送下一个 Poll 请求
  - 服务端检测连接超时（如 60 秒无请求），清理连接资源

- **断开检测**：
  - 客户端主动断开：发送 Disconnect 包，服务端清理连接
  - 服务端检测断开：超时无请求，清理连接
  - 网络异常：客户端重连时使用相同的 ConnectionID，服务端恢复连接状态

- **连接恢复**：
  - 客户端重连时，如果 ConnectionID 已存在，服务端恢复连接状态
  - 如果连接已过期，服务端返回错误，客户端重新握手

### 4. 分片丢失处理

**问题**：如果某个分片丢失，如何保证数据完整性？

**当前设计假设**：
- HTTP 基于 TCP，包是顺序到达的
- 不需要复杂的重排序逻辑

**实际情况**：
- HTTP 请求可能失败（网络错误、超时等）
- 分片可能丢失，需要重传机制

**建议**：
- **方案A**：简单重传
  - 如果某个分片超时未到达，服务端返回错误
  - 客户端重新发送整个数据块（重新分片）

- **方案B**：分片确认机制
  - 每个分片发送后，等待服务端确认
  - 如果某个分片未确认，客户端重传该分片
  - 服务端按顺序合并，如果发现分片缺失，请求重传

**推荐**：对于小数据块（<10MB），采用方案A；对于大数据块，采用方案B。

### 5. Header 大小限制

**设计决策**：不需要处理 Header 大小限制的退避方案。

**理由**：
- 控制包通常很小（< 1KB），压缩后更小
- 不会有大 Header 的场景
- 不需要写多余的代码处理边界情况

**实现**：
- 所有控制包通过 `X-Tunnel-Package` 传输
- 使用 Gzip 压缩 + Base64 编码
- 如果确实超过限制（极罕见），返回错误，客户端重新请求

### 6. 数据流缓冲管理

**问题**：`ReadExact` 实现中，数据流缓冲可能导致内存问题。

**当前设计问题**：
```go
func (sp *HTTPStreamProcessor) ReadExact(length int) ([]byte, error) {
    // 从缓冲读取，如果不够则触发 Poll 请求获取更多数据
    for sp.dataBuffer.Len() < length {
        // 触发 Poll 获取更多数据
        sp.ReadPacket()
    }
    // ...
}
```

**问题**：
- 如果 `length` 很大，可能导致大量 Poll 请求
- 缓冲可能无限增长，导致内存问题

**建议**：
- **限制缓冲大小**：设置最大缓冲大小（如 1MB）
- **流式读取**：如果数据量大，使用流式读取，而不是全部缓冲
- **背压机制**：如果缓冲满，暂停 Poll 请求

### 7. 响应路由的准确性

**问题**：文档中提到"通过 connectionID 路由 Response 到正确的连接"，但 HTTP 是无状态的。

**澄清**：
- 服务器**不能主动**向客户端发送 Response
- 服务器只能**等待**客户端的 Poll 请求，然后在 Response 中返回数据
- 正确的流程是：
  1. 服务器有数据要发送时，放入该 connectionID 的响应队列
  2. 客户端发送 Poll 请求（携带 connectionID）
  3. 服务器从响应队列取出数据，在 Poll 响应中返回

**建议**：修正文档中的描述，明确说明响应路由的实际机制。

### 8. 错误处理和重试机制

**问题**：文档中没有详细说明错误处理和重试机制。

**需要补充**：
- **网络错误**：请求失败时，客户端重试（指数退避）
- **服务端错误**：根据 HTTP 状态码决定是否重试
- **连接错误**：连接不存在时，客户端重新握手
- **超时处理**：Poll 请求超时是正常情况，客户端立即发送下一个请求

## 实现要点总结

### 1. 包转换器

- ✅ `HTTPPacketConverter` 是一个转换器，用于在 `TransferPacket` 和 HTTP Request/Response 之间转换
- ✅ 提供 `WritePacket`：将 `TransferPacket` 转换为 HTTP Request
- ✅ 提供 `ReadPacket`：从 HTTP Response 读取 `TransferPacket`
- ✅ 复用现有的包类型和结构
- ✅ **注意**：这不是 `io.Reader/io.Writer`，而是一个转换器接口

### 2. StreamProcessor 适配

- ✅ `HTTPStreamProcessor` **实现** `stream.PackageStreamer` 接口（这是真正实现接口的组件）
- ✅ 内部使用 `HTTPPacketConverter` 进行转换
- ✅ `ReadPacket` 通过 HTTP Poll 实现
- ✅ `WritePacket` 通过 HTTP Push 实现
- ✅ `WriteExact/ReadExact` 处理数据流
- ✅ `GetReader()/GetWriter()` 返回 `nil`（HTTP 无状态，没有底层流）
- ⚠️ **需要处理**：数据流缓冲管理，避免内存问题

### 3. 连接管理

- ✅ 服务器登记 connectionID 与连接信息的映射
- ✅ 通过响应队列机制，在 Poll 响应中返回数据
- ✅ 支持匿名客户端连接（clientID=0）
- ✅ **统一基于 ConnectionID 寻址**：所有连接查找都使用 ConnectionID
- ⚠️ **需要处理**：连接超时和断开检测，连接恢复机制
- ⚠️ **需要改进**：ConnectionID 由服务端生成（安全改进）

### 4. 续连接机制

- ✅ **已明确**：不需要续连接指令，Poll 请求本身就是续连接的机制
- ✅ **实现**：客户端定期发送 Poll 请求（携带 ConnectionID），等待服务器响应
- ✅ **优势**：简单、符合 LongPoll 标准模式

### 5. 隧道分片

- ✅ 支持大数据流分片传输
- ✅ 按 connectionID 分组，按顺序合并
- ⚠️ **需要处理**：分片丢失和重传机制
- ⚠️ **需要明确**：分片确认机制（对于大数据块）

## 与现有架构的集成

### StreamProcessor 替换

**关键点**：
- `HTTPStreamProcessor` 实现 `stream.PackageStreamer` 接口
- 在创建连接时，使用 `HTTPStreamProcessor` 而不是 `StreamProcessor`
- 上层代码无需修改，因为接口相同

**实现示例**：
```go
// 创建 HTTP 长轮询连接时
httppollConn := NewServerHTTPLongPollingConn(ctx, clientID)
streamProcessor := NewHTTPStreamProcessor(httppollConn, pushURL, pollURL)

// 使用统一的 CreateConnection
conn, err := sessionMgr.CreateConnection(streamProcessor, streamProcessor)
// 上层代码调用 conn.Stream.ReadPacket() / WritePacket()，无需修改
```

### 包类型映射

**需要实现的函数**：
```go
// packetTypeToString: packet.Type (byte) -> string
func packetTypeToString(t packet.Type) string {
    baseType := t & 0x3F // 忽略压缩/加密标志
    switch baseType {
    case packet.Handshake:
        return "Handshake"
    case packet.HandshakeResp:
        return "HandshakeResponse"
    case packet.JsonCommand:
        return "JsonCommand"
    case packet.CommandResp:
        return "CommandResp"
    case packet.TunnelOpen:
        return "TunnelOpen"
    case packet.TunnelOpenAck:
        return "TunnelOpenAck"
    case packet.Heartbeat:
        return "Heartbeat"
    case packet.TunnelData:
        return "TunnelData"
    case packet.TunnelClose:
        return "TunnelClose"
    default:
        return fmt.Sprintf("Unknown_%d", baseType)
    }
}

// stringToPacketType: string -> packet.Type (byte)
func stringToPacketType(s string) packet.Type {
    switch s {
    case "Handshake":
        return packet.Handshake
    case "HandshakeResponse":
        return packet.HandshakeResp
    case "JsonCommand":
        return packet.JsonCommand
    case "CommandResp":
        return packet.CommandResp
    case "TunnelOpen":
        return packet.TunnelOpen
    case "TunnelOpenAck":
        return packet.TunnelOpenAck
    case "Heartbeat":
        return packet.Heartbeat
    case "TunnelData":
        return packet.TunnelData
    case "TunnelClose":
        return packet.TunnelClose
    default:
        return 0
    }
}
```

## 错误处理和重试机制

### 网络错误处理

```go
// 客户端重试策略（指数退避）
func (sp *HTTPStreamProcessor) sendWithRetry(req *http.Request, maxRetries int) (*http.Response, error) {
    var lastErr error
    for i := 0; i < maxRetries; i++ {
        resp, err := sp.httpClient.Do(req)
        if err == nil {
            return resp, nil
        }
        lastErr = err
        
        // 指数退避
        backoff := time.Duration(1<<uint(i)) * time.Second
        time.Sleep(backoff)
    }
    return nil, lastErr
}
```

### HTTP 状态码处理

```go
// 根据状态码决定是否重试
func shouldRetry(statusCode int) bool {
    switch statusCode {
    case http.StatusBadRequest:      // 400: 不重试
        return false
    case http.StatusUnauthorized:    // 401: 不重试，重新握手
        return false
    case http.StatusNotFound:        // 404: 不重试，重新握手
        return false
    case http.StatusInternalServerError: // 500: 重试
        return true
    case http.StatusServiceUnavailable:  // 503: 重试
        return true
    default:
        return false
    }
}
```

### 连接错误处理

```go
// 连接不存在时的处理
if statusCode == http.StatusNotFound {
    // 连接不存在，重新握手
    sp.connectionID = ""
    sp.clientID = 0
    return sp.reconnect()
}
```

### 超时处理

```go
// Poll 请求超时是正常情况
if err == context.DeadlineExceeded {
    // 立即发送下一个 Poll 请求
    return sp.ReadPacket()
}
```

## 连接恢复机制

### 客户端重连

```go
// 客户端重连时使用相同的 ConnectionID
func (sp *HTTPStreamProcessor) reconnect() error {
    if sp.connectionID == "" {
        // 没有 ConnectionID，重新握手
        return sp.handshake()
    }
    
    // 有 ConnectionID，尝试恢复连接
    // 发送 Poll 请求，如果连接已过期，服务端返回 404
    resp, err := sp.sendPollRequest()
    if err != nil {
        return err
    }
    
    if resp.StatusCode == http.StatusNotFound {
        // 连接已过期，重新握手
        sp.connectionID = ""
        return sp.handshake()
    }
    
    return nil
}
```

### 服务端检测断开

```go
// 服务端检测连接超时
func (mgr *HTTPLongPollingConnectionManager) cleanupExpiredConnections() {
    now := time.Now()
    for connID, connInfo := range mgr.connections {
        if now.Sub(connInfo.LastActivity) > 60*time.Second {
            // 连接超时，清理
            delete(mgr.connections, connID)
        }
    }
}
```

## 优势

1. **完全复用现有协议**：不需要重新定义包结构
2. **统一接口**：通过 StreamProcessor 适配，上层代码无需修改
3. **无状态通信**：HTTP Request/Response 模式，适合 LongPoll
4. **易于扩展**：新增包类型只需在转换器中添加处理逻辑
5. **设计简化**：只使用 `X-Tunnel-Package`，所有信息都在包里
6. **安全性**：ConnectionID 由服务端生成，保证唯一性和不可伪造性

## 心跳包处理

### 处理方式

**方案A：心跳包通过 X-Tunnel-Package 传输**
```go
heartbeatPkg := &httppoll.TunnelPackage{
    ConnectionID: connectionID,
    ClientID:     clientID,
    Type:         "Heartbeat",
    Data:         nil, // 心跳包没有数据
}
```

**方案B：心跳包通过空的 Poll 请求实现（推荐）**
- 客户端定期发送 Poll 请求（只携带 ConnectionID，Type 为空），等待服务器响应
- 如果服务器没有数据，返回空响应，这本身就是心跳
- 更简单，符合 LongPoll 的标准模式

**推荐**：采用方案B，不需要特殊处理心跳包。

## 数据流传输细节

### Body 编码格式

- **编码方式**：Base64 编码（与现有实现一致）
- **同时传输**：可以，`X-Tunnel-Package` 中放控制包，Body 中放数据流
- **数据流格式**：纯字节流，Base64 编码

### 示例

```go
// 控制包 + 数据流同时传输
req.Header.Set("X-Tunnel-Package", encodedControlPkg) // 控制包
req.Body = base64EncodedDataStream // 数据流（Base64 编码）
```

## 待完善事项

1. ✅ **Header 简化**：已明确只使用 `X-Tunnel-Package`，所有信息都在包里
2. ✅ **续连接机制**：已明确不需要续连接指令，Poll 请求本身就是续连接的机制
3. ✅ **包转换器实现**：已修正，统一使用 `X-Tunnel-Package`
4. ✅ **TunnelPackage 与 TransferPacket 关系**：已明确转换关系
5. ✅ **ConnectionID 生成策略**：已明确服务端生成，在握手 ACK 时返回
6. ⚠️ **连接寻址统一**：**重要** - 统一基于 ConnectionID 寻址，移除基于 ClientID 的查找
7. **完善连接超时和断开处理**：添加超时检测、断开检测、连接恢复机制
8. **完善分片丢失处理**：添加分片确认和重传机制
9. **处理 Header 大小限制**：大包使用 Body 传输
10. **完善错误处理和重试机制**：添加详细的错误处理和重试策略
11. **优化数据流缓冲管理**：限制缓冲大小，添加背压机制
12. **包类型映射函数**：实现 `packetTypeToString` 和 `stringToPacketType`

## 关键改进建议

### 1. ConnectionID 由服务端生成（高优先级 - 安全）

**当前问题**：
- 客户端生成 ConnectionID 存在安全风险
- 无法保证全局唯一性
- 可能被恶意客户端伪造

**改进方案**：
1. **服务端生成 ConnectionID**：在握手 ACK 时返回
2. **修改 HandshakeResponse**：添加 `ConnectionID` 字段
3. **客户端使用服务端分配的 ConnectionID**：后续所有请求都使用这个 ID
4. **服务端验证**：确保 ConnectionID 的唯一性和有效性

**实现优先级**：**高** - 这是重要的安全改进

### 2. 连接寻址统一基于 ConnectionID（高优先级 - 架构改进）

**当前问题**：
- 部分代码基于 ClientID 寻址，部分基于 ConnectionID
- 导致连接查找逻辑不统一，容易出错

**改进方案**：
1. **统一基于 ConnectionID 寻址**：所有连接查找都使用 ConnectionID
2. **移除基于 ClientID 的查找**：ClientID 只用于业务逻辑，不用于连接寻址
3. **ConnectionID 作为唯一标识**：连接创建时就确定，不会改变
4. **ClientID 可以更新**：握手后更新 ClientID，但 ConnectionID 保持不变

**实现要点**：
```go
// 统一使用 ConnectionID 查找连接
conn, exists := sessionMgr.GetConnection(connectionID)

// 不再使用 ClientID 查找连接
// ❌ conn := sessionMgr.GetConnectionByClientID(clientID)
// ✅ conn := sessionMgr.GetConnection(connectionID)
```

**实现优先级**：**高** - 这是重要的架构改进，与原有架构更兼容

## 设计文档审查总结

### 已修正的问题

1. ✅ **包转换器实现与简化设计一致**：统一使用 `X-Tunnel-Package`，移除所有额外的 Header
2. ✅ **明确 TunnelPackage 与 TransferPacket 的关系**：添加转换函数和详细说明
3. ✅ **修正握手流程**：明确首次握手时 ConnectionID 由服务端生成
4. ✅ **补充心跳包处理**：说明心跳包通过 Poll 请求实现
5. ✅ **明确分片处理**：分片信息放在单独的 Header 中（数据流传输细节）
6. ✅ **补充数据流传输细节**：说明 Body 编码格式和同时传输机制
7. ✅ **补充错误处理和重试机制**：详细说明各种错误场景的处理
8. ✅ **补充连接恢复机制**：详细说明连接恢复的流程
9. ✅ **补充包类型映射**：添加字符串与字节的映射函数
10. ✅ **补充与现有架构的集成**：说明如何替换现有 StreamProcessor

### 仍需完善的问题

1. ⚠️ **连接寻址统一**：需要统一基于 ConnectionID 寻址，移除基于 ClientID 的查找
2. ⚠️ **连接超时和断开处理**：需要实现超时检测、断开检测、连接恢复机制
3. ⚠️ **分片丢失处理**：需要实现分片确认和重传机制（对于大数据块）
4. ⚠️ **数据流缓冲管理**：需要实现缓冲大小限制和背压机制

### 设计完整性检查

**已完善**：
- ✅ Header 结构设计
- ✅ 包转换器设计
- ✅ StreamProcessor 适配设计
- ✅ 握手流程设计
- ✅ 续连接机制设计
- ✅ 隧道连接处理设计
- ✅ 连接管理设计
- ✅ 错误处理设计
- ✅ 连接恢复设计

**待完善**：
- ⚠️ 分片丢失处理（实现细节）
- ⚠️ 数据流缓冲管理（实现细节）
- ⚠️ 连接寻址统一（架构改进）

### 关键设计决策

1. **只使用 X-Tunnel-Package**：所有信息都在包里，简化设计
2. **服务端生成 ConnectionID**：保证安全性和全局唯一性
3. **统一基于 ConnectionID 寻址**：简化连接管理逻辑
4. **Poll 请求作为续连接机制**：不需要额外的续连接指令
5. **完全复用现有协议**：不需要重新定义包结构

### 实现建议

**Phase 1：核心功能**
1. 实现包转换器（统一使用 `X-Tunnel-Package`）
2. 实现 `HTTPStreamProcessor`（实现 `stream.PackageStreamer` 接口）
3. 实现握手流程（服务端生成 ConnectionID）
4. 实现连接管理（基于 ConnectionID 寻址）

**Phase 2：完善功能**
5. 实现错误处理和重试机制
6. 实现连接超时和断开检测
7. 实现连接恢复机制

**Phase 3：优化功能**
8. 实现分片丢失处理（对于大数据块）
9. 实现数据流缓冲管理优化
