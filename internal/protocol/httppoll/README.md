# HTTP Long Polling 协议设计文档

## 概述

HTTP Long Polling 是 Tunnox 的核心协议之一，用于在不支持 WebSocket 或需要穿透防火墙的场景下建立双向通信。该协议通过 HTTP 请求/响应模拟持久连接，支持数据分片、重组、缓存和错误恢复。

## 目录

- [状态机](#状态机)
- [报文格式](#报文格式)
- [数据流链路](#数据流链路)
- [分片与重组](#分片与重组)
- [错误处理](#错误处理)

---

## 状态机

### 客户端状态机

```
[初始化]
    |
    v
[连接建立] --(发送 Push 请求)--> [等待响应]
    |                                    |
    |                                    v
    |                              [接收响应]
    |                                    |
    |                                    v
    +--(启动 Poll 循环)--> [Poll 等待] <--+
    |                                    |
    |                                    v
    |                              [处理数据]
    |                                    |
    |                                    v
    +--(触发立即 Poll)--> [立即 Poll] --+
    |                                    |
    v                                    |
[关闭] <---------------------------------+
```

**状态说明：**

1. **初始化**: 创建 `StreamProcessor`，设置连接信息
2. **连接建立**: 通过 Push 请求发送初始数据包（如 Handshake）
3. **Poll 等待**: 持续发送 Poll 请求，等待服务器响应
4. **接收响应**: 从 Poll 响应中提取数据包
5. **处理数据**: 解析数据包，处理分片重组
6. **立即 Poll**: 当有数据需要立即读取时触发
7. **关闭**: 清理资源，关闭连接

### 服务端状态机

```
[初始化]
    |
    v
[等待 Push/Poll] --(接收 Push)--> [处理 Push 数据]
    |                                    |
    |                                    v
    |                              [匹配控制包]
    |                                    |
    |                                    v
    |                              [发送响应]
    |                                    |
    |                                    |
    +--(接收 Poll)--> [等待数据] <-------+
    |                                    |
    |                                    v
    |                              [发送数据]
    |                                    |
    |                                    v
    +--(超时)--> [Poll 超时] ------------+
    |                                    |
    v                                    |
[关闭] <---------------------------------+
```

**状态说明：**

1. **初始化**: 创建 `ServerStreamProcessor`，设置连接信息
2. **等待 Push/Poll**: 同时处理 Push 和 Poll 请求
3. **处理 Push 数据**: 解析 Push 请求中的数据包
4. **匹配控制包**: 将控制包与等待的 Poll 请求匹配
5. **等待数据**: Poll 请求等待数据到达
6. **发送数据**: 将数据通过 Poll 响应返回
7. **Poll 超时**: Poll 请求超时，返回空响应
8. **关闭**: 清理资源，关闭连接

---

## 报文格式

### HTTP 请求格式

#### Push 请求（客户端 → 服务端）

```
POST /api/v1/tunnel/push
Headers:
  Authorization: Bearer <token>
  X-Client-ID: <clientID>
  X-Instance-ID: <instanceID>
  X-Connection-ID: <connectionID> (可选)
  X-Mapping-ID: <mappingID> (可选)
  Content-Type: application/json

Body:
{
  "connection_id": "<connectionID>",
  "client_id": <clientID>,
  "mapping_id": "<mappingID>",
  "tunnel_type": "control" | "data",
  "type": "<packetType>",
  "request_id": "<requestID>", (可选)
  "data": <packetData> | <FragmentResponse>
}
```

#### Poll 请求（客户端 → 服务端）

```
GET /api/v1/tunnel/poll?request_id=<requestID>
Headers:
  Authorization: Bearer <token>
  X-Client-ID: <clientID>
  X-Instance-ID: <instanceID>
  X-Connection-ID: <connectionID>
  X-Mapping-ID: <mappingID> (可选)

Response (立即响应):
{
  "success": true,
  "data": <FragmentResponse> | <TunnelPackage>
}

Response (超时):
{
  "success": false,
  "timeout": true
}
```

### 数据包格式

#### TunnelPackage（完整数据包）

```json
{
  "connection_id": "conn-123",
  "client_id": 456,
  "mapping_id": "mapping-789",
  "tunnel_type": "control" | "data",
  "type": "handshake" | "tunnel_open" | "data" | ...,
  "request_id": "req-abc", (可选)
  "data": { ... }
}
```

#### FragmentResponse（分片数据包）

```json
{
  "fragment_group_id": "uuid-xxx",
  "original_size": 50000,
  "fragment_size": 10000,
  "fragment_index": 0,
  "total_fragments": 5,
  "sequence_number": 12345,
  "data": "base64-encoded-data",
  "timestamp": 1234567890,
  "success": true, (仅 Response)
  "timeout": false (仅 Response)
}
```

**分片判断规则：**
- `total_fragments > 1` → 分片数据
- `total_fragments == 1` → 完整数据

---

## 数据流链路

### 客户端发送数据（Push）

```
[WritePacket]
    |
    v
[PacketConverter.WritePacket]
    | (转换为 TunnelPackage)
    |
    v
[HTTP POST /push]
    | (包含 TunnelPackage JSON)
    |
    v
[服务端接收]
    |
    v
[ServerStreamProcessor.HandlePushRequest]
    | (解析 TunnelPackage)
    |
    v
[匹配等待的 Poll 请求]
    | (控制包) 或 [加入数据队列] (数据包)
    |
    v
[返回 HTTP 200]
```

### 客户端接收数据（Poll）

```
[Poll 循环启动]
    |
    v
[发送 Poll 请求]
    | (GET /poll?request_id=xxx)
    |
    v
[等待响应] (最长 30 秒)
    |
    v
[接收响应]
    | (包含 FragmentResponse 或 TunnelPackage)
    |
    v
[检查是否为分片]
    | (total_fragments > 1)
    |
    v
[分片处理]
    | [单分片] → 直接处理
    | [多分片] → 重组后处理
    |
    v
[解析为 TransferPacket]
    |
    v
[加入 packetQueue]
    |
    v
[ReadPacket 读取]
```

### 服务端发送数据

```
[WriteExact/WritePacket]
    |
    v
[检查数据大小]
    | (> 8KB 需要分片)
    |
    v
[分片处理]
    | [小数据] → 直接发送
    | [大数据] → 分片发送
    |
    v
[等待 Poll 请求]
    | (控制包立即匹配，数据包加入队列)
    |
    v
[匹配 Poll 请求]
    | (通过 requestID 或 connectionID)
    |
    v
[发送响应]
    | (包含 FragmentResponse 或 TunnelPackage)
    |
    v
[返回 HTTP 200]
```

---

## 分片与重组

### 分片策略

**触发条件：**
- 数据大小 > `FragmentThreshold` (8KB)
- 单个分片大小：`MinFragmentSize` (1KB) ~ `MaxFragmentSize` (10KB)

**分片流程：**

```
[原始数据] (50KB)
    |
    v
[计算分片数] (50KB / 10KB = 5 片)
    |
    v
[生成 GroupID] (UUID)
    |
    v
[生成 SequenceNumber] (保证顺序)
    |
    v
[创建分片]
    Fragment 0: [0-10KB]
    Fragment 1: [10-20KB]
    Fragment 2: [20-30KB]
    Fragment 3: [30-40KB]
    Fragment 4: [40-50KB]
    |
    v
[Base64 编码]
    |
    v
[发送分片] (按顺序发送)
```

### 重组流程

```
[接收分片 0]
    |
    v
[FragmentReassembler.AddFragment]
    | (创建 FragmentGroup)
    |
    v
[接收分片 1, 2, 3, 4]
    |
    v
[检查完整性]
    | (ReceivedCount == TotalFragments)
    |
    v
[按索引排序]
    | (Fragment 0, 1, 2, 3, 4)
    |
    v
[拼接数据]
    | (Fragment 0 + 1 + 2 + 3 + 4)
    |
    v
[验证大小]
    | (len(result) == OriginalSize)
    |
    v
[返回重组数据]
```

**重组器特性：**

- **并发安全**: 使用 `sync.RWMutex` 保护
- **超时清理**: 30 秒未完成的分片组自动清理
- **容量限制**: 最多 100 个分片组，单个分片组最大 10MB
- **幂等性**: 重复接收的分片会被忽略

---

## 错误处理

### 客户端错误处理

1. **网络错误**: 自动重试（最多 3 次）
2. **超时**: Poll 请求超时后重新发送
3. **分片丢失**: 分片组超时后清理，等待重传
4. **数据损坏**: 验证失败后丢弃，记录错误日志

### 服务端错误处理

1. **Push 失败**: 返回 HTTP 错误码，客户端重试
2. **Poll 超时**: 返回 `{"success": false, "timeout": true}`
3. **分片重组失败**: 记录错误，丢弃分片组
4. **队列满**: 拒绝新请求，返回错误

### 响应缓存

**目的**: 解决 Poll 请求重试时的重复响应问题

**机制**:
- 缓存最近 1000 个响应（RequestID → Response）
- TTL: 60 秒
- FIFO 淘汰策略

**使用场景**:
- 客户端 Poll 请求超时后重试
- 网络抖动导致重复请求
- 确保幂等性

---

## 关键组件

### StreamProcessor（客户端）

- **职责**: 客户端数据流处理
- **功能**: 
  - Push 请求发送
  - Poll 循环管理
  - 分片重组
  - 响应缓存

### ServerStreamProcessor（服务端）

- **职责**: 服务端数据流处理
- **功能**:
  - Push 请求处理
  - Poll 请求匹配
  - 数据队列管理
  - 分片发送

### FragmentReassembler（分片重组器）

- **职责**: 分片数据的重组管理
- **功能**:
  - 分片接收与存储
  - 完整性检查
  - 数据重组
  - 超时清理

### PacketConverter（包转换器）

- **职责**: Tunnox 包与 HTTP 请求/响应的转换
- **功能**:
  - `TransferPacket` → `TunnelPackage` → HTTP Request
  - HTTP Response → `TunnelPackage` → `TransferPacket`

---

## 性能优化

1. **分片阈值**: 8KB，平衡网络效率和分片开销
2. **响应缓存**: 减少重复数据传输
3. **立即 Poll**: 有数据时立即触发，减少延迟
4. **并发处理**: 使用 channel 和 goroutine 实现异步处理
5. **容量限制**: 防止内存泄漏和资源耗尽

---

## 参考

- 实现文件: `internal/protocol/httppoll/`
- 测试文件: `internal/protocol/httppoll/*_test.go`
- 相关协议: WebSocket, QUIC, TCP

