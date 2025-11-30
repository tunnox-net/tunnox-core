# HTTP Long Polling Base64 适配层设计方案

## 1. 问题分析

### 1.1 当前问题

1. **数据包分片问题**：
   - `StreamProcessor.WritePacket()` 会多次调用 `Write()`（包类型 1 字节、包体大小 4 字节、包体数据 N 字节）
   - 每次 `Write()` 都会触发 Base64 编码和 HTTP 请求，导致一个完整数据包被分成多个 HTTP 请求

2. **数据包边界问题**：
   - Base64 解码后的数据直接放入 channel
   - `StreamProcessor.ReadPacket()` 期望连续的字节流，但可能收到不完整的数据包

3. **Base64 填充字符混入**：
   - 客户端 `writeFlushLoop` 中出现 `invalid bodySize` 错误
   - 前 5 字节为 `43 43 43 43 43`（`+++++`），可能是 Base64 填充字符或错误数据

### 1.2 根本原因

**Base64 编码/解码与字节流处理的边界不清晰**：
- Base64 编码/解码在 HTTP 层完成
- 字节流处理在 `net.Conn` 层完成
- 两层之间缺少适配层来维护连续的字节流

## 2. 设计方案

### 2.1 核心思想

**在适配层（`net.Conn` 实现）中维护字节流缓冲区，隔离 Base64 编码/解码逻辑**

```
┌─────────────────────────────────────────────────────────┐
│                    StreamProcessor                       │
│  (期望连续的字节流，按包边界读取)                          │
└──────────────────────┬──────────────────────────────────┘
                       │ Read/Write (字节流)
┌──────────────────────▼──────────────────────────────────┐
│          HTTPLongPollingConn (适配层)                     │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Read 缓冲区：维护接收到的字节流                    │  │
│  │  - 从 HTTP Poll 接收 Base64 数据                  │  │
│  │  - Base64 解码后追加到 readBuffer                 │  │
│  │  - Read() 从 readBuffer 按需读取字节               │  │
│  └──────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────┐  │
│  │  Write 缓冲区：维护待发送的字节流                   │  │
│  │  - Write() 将数据写入 writeBuffer                 │  │
│  │  - writeFlushLoop 检测完整包后 Base64 编码发送     │  │
│  └──────────────────────────────────────────────────┘  │
└──────────────────────┬──────────────────────────────────┘
                       │ Base64 编码/解码
┌──────────────────────▼──────────────────────────────────┐
│              HTTP 请求/响应层                            │
│  - POST /tunnox/v1/push (发送 Base64 数据)              │
│  - GET /tunnox/v1/poll (接收 Base64 数据)                │
└─────────────────────────────────────────────────────────┘
```

### 2.2 数据流向

#### 2.2.1 接收数据（服务器 → 客户端）

```
HTTP Poll Response (Base64)
    ↓
Base64 解码
    ↓
追加到 readBuffer (字节流缓冲区)
    ↓
StreamProcessor.ReadPacket() 调用 Read()
    ↓
从 readBuffer 按字节读取
    ↓
解析完整数据包
```

#### 2.2.2 发送数据（客户端 → 服务器）

```
StreamProcessor.WritePacket() 调用 Write()
    ↓
写入 writeBuffer (字节流缓冲区)
    ↓
writeFlushLoop 检测完整包
    ↓
Base64 编码
    ↓
HTTP POST /tunnox/v1/push
```

### 2.3 关键设计点

#### 2.3.1 Read 缓冲区管理

```go
type HTTPLongPollingConn struct {
    // 读取缓冲区（字节流）
    readBuffer []byte
    readBufMu  sync.Mutex
    
    // 接收 Base64 数据的 channel
    base64DataChan chan []byte // Base64 编码的数据
}

// Read 实现：从 readBuffer 读取字节
func (c *HTTPLongPollingConn) Read(p []byte) (int, error) {
    c.readBufMu.Lock()
    defer c.readBufMu.Unlock()
    
    // 1. 如果 readBuffer 有数据，直接返回
    if len(c.readBuffer) > 0 {
        n := copy(p, c.readBuffer)
        c.readBuffer = c.readBuffer[n:]
        return n, nil
    }
    
    // 2. readBuffer 为空，从 base64DataChan 接收数据
    select {
    case base64Data := <-c.base64DataChan:
        // Base64 解码
        data, err := base64.StdEncoding.DecodeString(string(base64Data))
        if err != nil {
            return 0, err
        }
        
        // 追加到 readBuffer
        c.readBuffer = append(c.readBuffer, data...)
        
        // 从 readBuffer 读取
        n := copy(p, c.readBuffer)
        c.readBuffer = c.readBuffer[n:]
        return n, nil
    case <-c.Ctx().Done():
        return 0, c.Ctx().Err()
    }
}
```

#### 2.3.2 Write 缓冲区管理

```go
type HTTPLongPollingConn struct {
    // 写入缓冲区（字节流）
    writeBuffer bytes.Buffer
    writeBufMu  sync.Mutex
    writeFlush  chan struct{}
}

// Write 实现：写入 writeBuffer
func (c *HTTPLongPollingConn) Write(p []byte) (int, error) {
    c.writeBufMu.Lock()
    n, err := c.writeBuffer.Write(p)
    c.writeBufMu.Unlock()
    
    // 触发刷新检查
    select {
    case c.writeFlush <- struct{}{}:
    default:
    }
    
    return n, nil
}

// writeFlushLoop：检测完整包并发送
func (c *HTTPLongPollingConn) writeFlushLoop() {
    for {
        select {
        case <-c.writeFlush:
            // 检查是否有完整包
            c.writeBufMu.Lock()
            if c.writeBuffer.Len() >= 5 {
                bufData := c.writeBuffer.Bytes()
                
                // 解析包体大小
                bodySize := binary.BigEndian.Uint32(bufData[1:5])
                packetSize := 5 + int(bodySize)
                
                if c.writeBuffer.Len() >= packetSize {
                    // 提取完整包
                    data := make([]byte, packetSize)
                    copy(data, bufData[:packetSize])
                    c.writeBuffer.Next(packetSize)
                    c.writeBufMu.Unlock()
                    
                    // Base64 编码并发送
                    encoded := base64.StdEncoding.EncodeToString(data)
                    c.sendBase64Data(encoded)
                } else {
                    c.writeBufMu.Unlock()
                }
            } else {
                c.writeBufMu.Unlock()
            }
        }
    }
}
```

### 2.4 服务器端设计

服务器端采用相同的设计：

```go
type ServerHTTPLongPollingConn struct {
    // 读取缓冲区（字节流）
    readBuffer []byte
    readBufMu  sync.Mutex
    
    // 写入缓冲区（字节流）
    writeBuffer bytes.Buffer
    writeBufMu  sync.Mutex
    
    // Base64 数据通道（用于 HTTP 响应）
    base64DataChan chan []byte
}

// Read：从 readBuffer 读取字节
// Write：写入 writeBuffer，writeFlushLoop 检测完整包后 Base64 编码发送

// PushData：接收 Base64 数据，解码后追加到 readBuffer
func (c *ServerHTTPLongPollingConn) PushData(base64Data string) error {
    data, err := base64.StdEncoding.DecodeString(base64Data)
    if err != nil {
        return err
    }
    
    c.readBufMu.Lock()
    c.readBuffer = append(c.readBuffer, data...)
    c.readBufMu.Unlock()
    
    return nil
}

// PollData：从 writeBuffer 获取完整包，Base64 编码后返回
func (c *ServerHTTPLongPollingConn) PollData(ctx context.Context) (string, error) {
    // writeFlushLoop 检测完整包后，Base64 编码并发送到 base64DataChan
    select {
    case base64Data := <-c.base64DataChan:
        return string(base64Data), nil
    case <-ctx.Done():
        return "", ctx.Err()
    }
}
```

## 3. 优势

### 3.1 清晰的职责分离

- **HTTP 层**：只负责 Base64 编码/解码和 HTTP 请求/响应
- **适配层**：维护字节流缓冲区，确保 StreamProcessor 看到连续的字节流
- **StreamProcessor**：只关心字节流，不关心 Base64 编码

### 3.2 避免数据包分片

- `StreamProcessor.WritePacket()` 的多次 `Write()` 调用都写入同一个 `writeBuffer`
- `writeFlushLoop` 检测到完整包后才 Base64 编码发送
- 确保一个完整数据包对应一个 HTTP 请求

### 3.3 避免数据包边界问题

- Base64 解码后的数据追加到 `readBuffer`
- `StreamProcessor.ReadPacket()` 从 `readBuffer` 按字节读取
- 确保 `ReadPacket()` 看到连续的字节流

### 3.4 避免 Base64 填充字符混入

- Base64 编码/解码在适配层完成
- `writeBuffer` 和 `readBuffer` 只包含原始字节流
- 不会出现 Base64 填充字符（`+`、`/`、`=`）混入字节流的情况

## 4. 实现要点

### 4.1 客户端实现

1. **pollLoop**：接收 Base64 数据，解码后追加到 `readBuffer`
2. **Read**：从 `readBuffer` 按字节读取
3. **Write**：写入 `writeBuffer`
4. **writeFlushLoop**：检测完整包，Base64 编码后发送

### 4.2 服务器端实现

1. **handleHTTPPush**：接收 Base64 数据，调用 `PushData()` 解码后追加到 `readBuffer`
2. **Read**：从 `readBuffer` 按字节读取
3. **Write**：写入 `writeBuffer`
4. **writeFlushLoop**：检测完整包，Base64 编码后发送到 `base64DataChan`
5. **handleHTTPPoll**：从 `base64DataChan` 获取 Base64 数据并返回

## 5. 迁移建议

### 5.1 渐进式迁移

1. **第一阶段**：保持现有 HTTP 层逻辑不变，在 `net.Conn` 层添加缓冲区
2. **第二阶段**：将 Base64 编码/解码逻辑移到适配层
3. **第三阶段**：优化和测试

### 5.2 兼容性保证

- 保持 `net.Conn` 接口不变
- 保持 `StreamProcessor` 接口不变
- 只修改适配层内部实现

## 6. 总结

通过在适配层维护字节流缓冲区，可以：
1. **隔离 Base64 编码/解码逻辑**：StreamProcessor 只看到字节流
2. **避免数据包分片**：完整包才 Base64 编码发送
3. **避免数据包边界问题**：StreamProcessor 看到连续的字节流
4. **避免 Base64 填充字符混入**：缓冲区只包含原始字节流

这样设计既保持了架构的清晰性，又解决了当前的数据包分片和边界问题。

