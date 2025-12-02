# HTTP 长轮询数据分片重组设计

## 1. 问题背景

### 1.1 当前问题
- HTTP 长轮询传输中，大数据包可能被分片传输
- MySQL 等协议对数据包完整性要求极高，分片可能导致协议解析失败
- 当前实现中，17字节的数据包可能被分成15字节+2字节，导致 MySQL 认证失败

### 1.2 设计目标
- 在 HTTP 长轮询层面实现透明的分片重组
- 保证数据包的完整性和顺序
- 最小化对现有代码的影响
- 支持大数据流的高效传输

## 2. 统一格式设计

### 2.1 设计原则

**核心原则**：HTTP Push Request Body 和 HTTP Poll Response Body 使用统一的 JSON 格式，所有数据都在 Body 中，由 JSON 返回，JSON 中带有对数据本身的自描述信息，`data` 字段是真正的字节流的 Base64 内容。

### 2.2 HTTP Push Request Body 格式

**POST /tunnox/v1/push**（客户端 → 服务端）

**分片数据示例**：
```json
{
  "fragment_group_id": "550e8400-e29b-41d4-a716-446655440000",
  "original_size": 98304,
  "fragment_size": 10240,
  "fragment_index": 0,
  "total_fragments": 10,
  "data": "base64_encoded_fragment_data",
  "timestamp": 1234567890
}
```

**完整数据示例**（不分片）：
```json
{
  "fragment_group_id": "single-uuid-or-empty",
  "original_size": 1024,
  "fragment_size": 1024,
  "fragment_index": 0,
  "total_fragments": 1,
  "data": "base64_encoded_complete_data",
  "timestamp": 1234567890
}
```

**注意**：Request 也会分片。当数据大小 > `FRAGMENT_THRESHOLD`（8KB）时，客户端会将数据分片，每个分片通过独立的 POST 请求发送。

### 2.3 HTTP Poll Response Body 格式

**GET /tunnox/v1/poll**（服务端 → 客户端）

**分片数据示例**：
```json
{
  "fragment_group_id": "550e8400-e29b-41d4-a716-446655440000",
  "original_size": 98304,
  "fragment_size": 10240,
  "fragment_index": 0,
  "total_fragments": 10,
  "data": "base64_encoded_fragment_data",
  "success": true,
  "timeout": false,
  "timestamp": 1234567890
}
```

**完整数据示例**（不分片）：
```json
{
  "fragment_group_id": "single-uuid-or-empty",
  "original_size": 1024,
  "fragment_size": 1024,
  "fragment_index": 0,
  "total_fragments": 1,
  "data": "base64_encoded_complete_data",
  "success": true,
  "timeout": false,
  "timestamp": 1234567890
}
```

**超时响应**（无数据）：
```json
{
  "success": true,
  "timeout": true,
  "timestamp": 1234567890
}
```

### 2.4 字段说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `fragment_group_id` | string (UUID) | 是 | 分片组唯一标识，同一组的所有分片共享此ID。完整数据（不分片）时可为空或使用单次UUID |
| `original_size` | int | 是 | 原始未分片的字节流总大小 |
| `fragment_size` | int | 是 | 当前分片的字节大小（Base64解码后） |
| `fragment_index` | int | 是 | 当前分片在组内的索引（从0开始） |
| `total_fragments` | int | 是 | 该分片组的总分片数 |
| `data` | string | 是 | Base64编码的字节流（真正的数据） |
| `success` | bool | 否 | 响应是否成功（仅 Response 使用） |
| `timeout` | bool | 否 | 是否超时（仅 Response 使用） |
| `timestamp` | int64 | 是 | 时间戳 |

### 2.5 分片判断逻辑

**移除 `is_fragment` 字段**：该字段是冗余的，可以通过以下方式判断：

```go
// 判断是否为分片数据
isFragment := (total_fragments > 1)

// 或者更严格的判断
isFragment := (total_fragments > 1) || (fragment_index > 0)

// 判断是否为完整数据
isComplete := (total_fragments == 1) && (fragment_index == 0)
```

**判断规则**：
- 如果 `total_fragments == 1` 且 `fragment_index == 0`，则为完整数据（不分片）
- 如果 `total_fragments > 1`，则为分片数据
- 如果 `fragment_index >= total_fragments`，则为无效分片（错误）

### 2.6 请求追踪

- **移除 `seq` 字段**：`seq` 字段是冗余的，客户端未使用 ACK 进行确认，数据流不需要顺序确认（TCP 层已保证）
- **使用 `RequestID`**：请求追踪通过 `X-Tunnel-Package` header 中的 `RequestID` 实现，用于匹配请求和响应（特别是控制包）

**注意**：
- Push Request 的 `X-Tunnel-Package` header 中可以包含 `RequestID`（可选，用于请求追踪）
- Poll Request 的 `X-Tunnel-Package` header 中必须包含 `RequestID`（用于匹配控制包的请求和响应）

## 3. 分片策略

### 3.1 分片触发条件

- **数据大小阈值**：当数据包大小 > `FRAGMENT_THRESHOLD`（建议 8KB）时触发分片
- **分片大小**：每个分片最大 `MAX_FRAGMENT_SIZE`（建议 10KB，Base64编码后约13.3KB）
- **最小分片**：如果数据 < `FRAGMENT_THRESHOLD`，直接传输，不分片

### 3.2 分片大小选择

```
FRAGMENT_THRESHOLD = 8 * 1024      // 8KB，超过此大小才分片
MAX_FRAGMENT_SIZE = 10 * 1024      // 10KB，每个分片最大大小
MIN_FRAGMENT_SIZE = 1 * 1024       // 1KB，最小分片大小（避免过度分片）
```

**考虑因素**：
- HTTP 请求体大小限制（通常 1-10MB）
- Base64 编码后大小增加约 33%
- 网络传输效率（分片过多增加开销）
- 重组缓冲区内存占用

### 3.3 分片计算逻辑

```go
func calculateFragments(dataSize int) (fragmentSize int, totalFragments int) {
    if dataSize <= FRAGMENT_THRESHOLD {
        return dataSize, 1  // 不分片
    }
    
    // 计算分片数（向上取整）
    totalFragments = (dataSize + MAX_FRAGMENT_SIZE - 1) / MAX_FRAGMENT_SIZE
    
    // 确保最后一片不会太小（如果小于MIN_FRAGMENT_SIZE，合并到前一片）
    lastFragmentSize := dataSize % MAX_FRAGMENT_SIZE
    if lastFragmentSize > 0 && lastFragmentSize < MIN_FRAGMENT_SIZE && totalFragments > 1 {
        totalFragments--
    }
    
    return MAX_FRAGMENT_SIZE, totalFragments
}
```

## 4. 发送端实现（服务端/客户端）

### 4.1 分片发送流程

```
1. 检查数据大小
   ├─ <= FRAGMENT_THRESHOLD → 直接发送（is_fragment=false）
   └─ > FRAGMENT_THRESHOLD → 进入分片流程

2. 生成分片组ID（UUID）

3. 计算分片参数
   ├─ totalFragments
   ├─ 每片大小（最后一片可能不同）
   └─ fragment_index (0..totalFragments-1)

4. 循环发送每个分片
   ├─ 提取分片数据
   ├─ Base64编码
   ├─ 构造分片响应JSON
   └─ 通过HTTP Push/Poll发送

5. 所有分片发送完成
```

### 4.2 关键实现点

**服务端（ServerStreamProcessor.WriteExact）**：
- 检测数据大小，决定是否分片
- 生成分片组ID（UUID）
- 分片并构造 `HTTPPollResponse` 格式的 JSON
- 逐个发送到 `pollDataQueue`（存储 JSON 字节）
- `HandlePollRequest` 返回 `HTTPPollResponse` 结构体（不是 JSON 字符串）
- `handleHTTPPoll` 直接序列化 `HTTPPollResponse` 为 JSON 返回

**客户端（HTTPLongPollingConn.Write）**：
- 检测数据大小，决定是否分片
- 生成分片组ID（UUID）
- 分片并构造 `HTTPPushRequest` 格式的 JSON
- 逐个通过 `sendFragment` 发送（POST /tunnox/v1/push）
- 在 `X-Tunnel-Package` header 中可以包含 `RequestID`（可选）

## 5. 接收端实现（客户端/服务端）

### 5.1 分片重组管理器

```go
type FragmentReassembler struct {
    groups map[string]*FragmentGroup  // fragment_group_id -> FragmentGroup
    mu     sync.RWMutex
    maxAge time.Duration              // 分片组最大存活时间（超时清理）
}

type FragmentGroup struct {
    groupID        string
    originalSize   int
    totalFragments int
    fragments      []*Fragment        // 按index排序
    receivedCount  int
    completeTime   time.Time
    mu             sync.Mutex
}

type Fragment struct {
    index    int
    size     int
    data     []byte
    received time.Time
}
```

### 5.2 重组流程

```
1. 接收分片数据（Request 或 Response）
   ├─ 解析JSON
   ├─ 判断是否为分片：total_fragments > 1
   │   ├─ false (total_fragments == 1) → 直接返回完整数据
   │   └─ true (total_fragments > 1) → 进入重组流程

2. 查找或创建分片组
   ├─ 根据 fragment_group_id 查找
   ├─ 不存在 → 创建新组
   └─ 存在 → 添加到现有组

3. 验证分片有效性
   ├─ fragment_index 范围检查 (0..totalFragments-1)
   ├─ fragment_size 一致性检查
   ├─ original_size 一致性检查
   └─ total_fragments 一致性检查

4. 存储分片
   ├─ 按 index 插入到 fragments 数组
   ├─ 更新 receivedCount
   └─ 检查是否完整

5. 检查完整性
   ├─ receivedCount == totalFragments → 重组
   └─ 否则 → 等待更多分片

6. 重组数据
   ├─ 按 index 顺序拼接所有分片
   ├─ 验证总大小 == original_size
   ├─ 返回完整数据
   └─ 清理分片组
```

### 5.3 超时和错误处理

**超时机制**：
- 每个分片组设置超时时间（建议 30秒）
- 超时后清理不完整的分片组
- 记录警告日志

**错误处理**：
- 重复分片：忽略重复的 fragment_index
- 大小不匹配：记录错误，清理分片组
- 索引越界：记录错误，清理分片组
- 分片丢失：超时后清理

### 5.4 内存管理

- 限制最大分片组数量（建议 100）
- 限制单个分片组最大大小（建议 10MB）
- 定期清理超时的分片组
- 使用对象池复用 Fragment 对象

## 6. 集成点

### 6.1 服务端集成

**修改点**：
1. `ServerStreamProcessor.WriteExact`：
   - 检测数据大小，决定是否分片
   - 生成分片组ID，构造 `HTTPPollResponse` 格式（不包含 `is_fragment` 字段）
   - 分片并逐个发送到 `pollDataQueue`（存储 JSON 字节）
2. `ServerStreamProcessor.HandlePollRequest`：
   - 从 `pollDataQueue` 取出 JSON 字节
   - 解析为 `HTTPPollResponse` 结构体
   - 返回 `HTTPPollResponse` 结构体（不是 JSON 字符串）
3. `handleHTTPPoll`：
   - 接收 `HTTPPollResponse` 结构体
   - 直接序列化为 JSON 返回（统一格式）
4. `ServerStreamProcessor.ReadAvailable`：
   - 接收分片并重组，返回完整数据
   - 通过 `PushData` 接收 Base64 数据，解码后追加到 `readBuffer`
5. `handleHTTPPush`：
   - 接收 `HTTPPushRequest` 格式的 JSON
   - 判断是否为分片：`total_fragments > 1`
   - 如果是分片，添加到 `FragmentReassembler` 并重组
   - 如果是完整数据，直接推送到流处理器

**新增组件**：
- `FragmentReassembler`：分片重组管理器（共享组件）
- `HTTPPollResponse`：统一的 Response Body 格式结构体（移除 `is_fragment` 字段）

### 6.2 客户端集成

**修改点**：
1. `HTTPLongPollingConn.Write`：
   - 检测数据大小，决定是否分片
   - 生成分片组ID，构造 `HTTPPushRequest` 格式（不包含 `is_fragment` 字段）
   - 分片并逐个通过 `sendFragment` 发送
2. `HTTPLongPollingConn.sendFragment`：
   - 构造 `HTTPPushRequest` 格式的 JSON
   - 通过 POST /tunnox/v1/push 发送
   - 在 `X-Tunnel-Package` header 中可以包含 `RequestID`（可选）
3. `HTTPLongPollingConn.pollLoop`：
   - 接收 `HTTPPollResponse` 格式的 JSON
   - 判断是否为分片：`total_fragments > 1`
   - 如果是分片，添加到 `FragmentReassembler` 并重组
   - 如果是完整数据，直接发送到 `base64DataChan`
   - 重组完成后，将 Base64 数据发送到 `base64DataChan`
4. `HTTPLongPollingConn.Read`：
   - 从 `base64DataChan` 接收 Base64 数据
   - 解码后追加到 `readBuffer`
   - 从 `readBuffer` 读取数据返回

**新增组件**：
- `FragmentReassembler`：分片重组管理器（与服务端共享逻辑）
- `HTTPPushRequest`：统一的 Request Body 格式结构体（移除 `seq` 和 `is_fragment` 字段）

## 7. 实现策略

### 7.1 统一格式

- **Request/Response Body 统一**：
  - HTTP Push Request Body 和 HTTP Poll Response Body 使用相同的分片元数据字段
  - 都包含 `fragment_group_id`, `original_size`, `fragment_size`, `fragment_index`, `total_fragments`, `data`
  - `data` 字段是 Base64 编码的字节流（真正的数据）
  - 差异仅在于状态字段（Push Request 有 `timestamp`，Poll Response 有 `success`/`timeout`/`timestamp`）

- **移除冗余字段**：
  - 移除 `seq` 字段（冗余，未使用，客户端未使用 ACK 进行确认）
  - 移除 `is_fragment` 字段（冗余，可通过 `total_fragments > 1` 判断）
  - 使用 `X-Tunnel-Package` header 中的 `RequestID` 进行请求追踪

- **分片判断**：
  - 完整数据：`total_fragments == 1` 且 `fragment_index == 0`
  - 分片数据：`total_fragments > 1`
  - 判断逻辑：`isFragment := (total_fragments > 1)`

- **Request 和 Response 都支持分片**：
  - Request（POST /tunnox/v1/push）：客户端发送数据时会分片（数据 > 8KB）
  - Response（GET /tunnox/v1/poll）：服务端发送数据时会分片（数据 > 8KB）

- **所有数据**：统一使用分片格式，小数据包 `total_fragments=1`
- **简化实现**：不需要格式检测和兼容逻辑
- **直接替换**：替换现有的数据传输格式

### 7.2 实现步骤

1. **阶段1**：实现分片发送和重组核心逻辑
2. **阶段2**：集成到服务端和客户端
3. **阶段3**：测试和优化
4. **阶段4**：移除旧的数据传输代码

## 8. 性能考虑

### 8.1 开销分析

- **内存开销**：每个分片组需要额外内存存储元数据
- **CPU开销**：分片计算、Base64编码/解码、重组拼接
- **网络开销**：分片元数据增加约 200-300 字节/分片

### 8.2 优化策略

- **批量发送**：同一分片组的分片可以批量发送（如果支持）
- **压缩**：大数据可以考虑压缩后再分片
- **异步重组**：重组过程异步进行，不阻塞发送

## 9. 测试策略

### 9.1 单元测试

- 分片计算逻辑测试
- 分片重组逻辑测试
- 边界情况测试（单分片、最大分片、超时等）

### 9.2 集成测试

- MySQL 大数据传输测试
- 分片丢失恢复测试
- 并发分片组测试

### 9.3 压力测试

- 大量分片组并发
- 大数据流传输
- 内存泄漏检测

## 10. 配置参数

```go
const (
    // 分片阈值：超过此大小才分片
    FragmentThreshold = 8 * 1024  // 8KB
    
    // 最大分片大小
    MaxFragmentSize = 10 * 1024   // 10KB
    
    // 最小分片大小
    MinFragmentSize = 1 * 1024    // 1KB
    
    // 分片组超时时间
    FragmentGroupTimeout = 30 * time.Second
    
    // 最大分片组数量
    MaxFragmentGroups = 100
    
    // 单个分片组最大大小
    MaxFragmentGroupSize = 10 * 1024 * 1024  // 10MB
)
```

## 11. 日志和监控

### 11.1 关键日志

- 分片发送：记录分片组ID、分片索引、分片大小
- 分片接收：记录分片组ID、接收进度
- 重组完成：记录分片组ID、总大小、耗时
- 错误情况：记录分片丢失、超时、大小不匹配等

### 11.2 监控指标

- 分片发送数量
- 分片重组成功率
- 分片重组平均耗时
- 分片组超时数量
- 内存使用情况

## 12. 风险评估

### 12.1 潜在风险

1. **内存泄漏**：分片组未正确清理
   - **缓解**：定期清理、超时机制、限制数量

2. **分片丢失**：网络问题导致分片丢失
   - **缓解**：超时检测、错误日志、重传机制（可选）

3. **性能下降**：分片重组增加延迟
   - **缓解**：异步重组、优化算法、合理分片大小

4. **兼容性问题**：新旧版本混用
   - **缓解**：格式检测、渐进式迁移

### 12.2 回滚方案

- Git 版本控制，可以快速回滚到之前的版本
- 监控告警，及时发现问题
- 充分测试后再部署

## 13. 实现优先级

### 13.1 第一阶段（核心功能）

1. 分片发送逻辑（服务端和客户端）
2. 分片重组逻辑（服务端和客户端）
3. 基本错误处理
4. 单元测试

### 13.2 第二阶段（优化和稳定性）

1. 超时和清理机制
2. 内存管理优化
3. 性能优化
4. 集成测试

### 13.3 第三阶段（监控和运维）

1. 日志完善
2. 监控指标
3. 压力测试
4. 文档完善

## 14. 数据流示例

### 14.1 客户端发送数据（监听端口 → 服务端）

```
1. 监听端口收到字节流（如 MySQL 数据包）
   ↓
2. HTTPLongPollingConn.Write() 被调用
   ↓
3. 检测数据大小，决定是否分片
   ├─ <= 8KB → 构造单个 HTTPPushRequest（is_fragment=false）
   └─ > 8KB → 分片，构造多个 HTTPPushRequest（is_fragment=true）
   ↓
4. 每个分片通过 POST /tunnox/v1/push 发送
   - Request Body: HTTPPushRequest JSON
   - Header: X-Tunnel-Package (包含 ConnectionID, ClientID, MappingID, RequestID)
   ↓
5. 服务端 handleHTTPPush 接收
   - 解析 HTTPPushRequest JSON
   - 如果是分片，添加到 FragmentReassembler
   - 重组完成后，通过 PushData 推送到流处理器
```

### 14.2 服务端发送数据（服务端 → 监听端口）

```
1. 服务端收到目标服务器数据（如 MySQL 响应）
   ↓
2. ServerStreamProcessor.WriteExact() 被调用
   ↓
3. 检测数据大小，决定是否分片
   ├─ <= 8KB → 构造单个 HTTPPollResponse（total_fragments=1）
   └─ > 8KB → 分片，构造多个 HTTPPollResponse（total_fragments>1）
   ↓
4. 分片 JSON 字节推送到 pollDataQueue
   ↓
5. HandlePollRequest 从队列取出
   - 解析 JSON 为 HTTPPollResponse 结构体
   - 返回 HTTPPollResponse 结构体
   ↓
6. handleHTTPPoll 序列化并返回
   - Response Body: HTTPPollResponse JSON（包含分片元数据和 data 字段）
   - Header: X-Tunnel-Package (控制包，如果有)
   ↓
7. 客户端 pollLoop 接收
   - 解析 HTTPPollResponse JSON
   - 判断是否为分片：total_fragments > 1
   ├─ 是分片 → 添加到 FragmentReassembler，等待重组
   └─ 完整数据 → 直接发送到 base64DataChan
   ↓
8. 分片重组完成（如果分片）
   - FragmentReassembler 检测到所有分片已接收
   - 按 fragment_index 顺序重组数据
   - 验证总大小 == original_size
   - Base64 数据发送到 base64DataChan
   ↓
9. HTTPLongPollingConn.Read() 读取
   - 从 base64DataChan 接收 Base64 数据
   - 解码后追加到 readBuffer
   - 返回给监听端口
```

## 15. 类型移除和替换

### 15.1 需要移除的字段

由于没有兼容包袱，以下字段需要从代码中完全移除：

#### 15.1.1 `seq` 相关字段

**移除位置**：
- `HTTPPushRequest.Seq` (uint64) - `internal/api/handlers_httppoll.go`
- `HTTPPushResponse.Ack` (uint64) - `internal/api/handlers_httppoll.go`
- `HTTPLongPollingConn.pushSeq` (uint64) - `internal/client/transport_httppoll.go`
- `HTTPLongPollingConn.pollSeq` (uint64) - `internal/client/transport_httppoll.go`

**移除原因**：
- `seq` 字段未被使用，客户端未使用 ACK 进行确认
- 数据流不需要顺序确认（TCP 层已保证）
- 请求追踪通过 `X-Tunnel-Package` header 中的 `RequestID` 实现

**影响范围**：
- `handleHTTPPush` 中不再返回 `Ack` 字段
- `sendFragment` 中不再生成和发送 `seq`
- `HTTPPushResponse` 结构体可以简化，只保留 `Success` 和 `Timestamp`

#### 15.1.2 `is_fragment` 字段

**移除位置**：
- `HTTPPushRequest.IsFragment` (bool) - `internal/api/handlers_httppoll.go`
- `HTTPPollResponse.IsFragment` (bool) - `internal/api/handlers_httppoll.go`
- `FragmentResponse.IsFragment` (bool) - `internal/protocol/httppoll/fragment_format.go`

**移除原因**：
- 该字段是冗余的，可以通过 `total_fragments > 1` 判断
- 判断逻辑：`isFragment := (total_fragments > 1)`

**影响范围**：
- `CreateFragmentResponse` 中不再设置 `IsFragment` 字段
- `CreateCompleteResponse` 中不再设置 `IsFragment` 字段
- `handleHTTPPush` 中判断分片改为：`if pushReq.TotalFragments > 1`
- `pollLoop` 中判断分片改为：`if pollResp.TotalFragments > 1`
- `ReadAvailable` 中判断分片改为：`if fragment.TotalFragments > 1`

### 15.2 需要统一/替换的类型

#### 15.2.1 统一 Request/Response 结构体

**当前问题**：
- `HTTPPushRequest` 和 `HTTPPollResponse` 重复定义了分片元数据字段
- `FragmentResponse` 也定义了相同的字段，但包含额外的 `Success` 和 `Timeout` 字段

**建议方案**：

**方案A：统一使用 `FragmentResponse` 作为基础结构**

```go
// FragmentResponse 统一的分片格式（用于 Request 和 Response）
type FragmentResponse struct {
    FragmentGroupID string `json:"fragment_group_id"`
    OriginalSize    int    `json:"original_size"`
    FragmentSize    int    `json:"fragment_size"`
    FragmentIndex   int    `json:"fragment_index"`
    TotalFragments  int    `json:"total_fragments"`
    Data            string `json:"data"`
    Timestamp       int64  `json:"timestamp"`
    
    // 仅 Response 使用
    Success         bool   `json:"success,omitempty"`
    Timeout         bool   `json:"timeout,omitempty"`
}

// HTTPPushRequest 直接使用 FragmentResponse（或类型别名）
type HTTPPushRequest = FragmentResponse

// HTTPPollResponse 直接使用 FragmentResponse（或类型别名）
type HTTPPollResponse = FragmentResponse
```

**方案B：保留独立结构体，但统一字段定义**

```go
// FragmentBase 分片基础字段（共享）
type FragmentBase struct {
    FragmentGroupID string `json:"fragment_group_id"`
    OriginalSize    int    `json:"original_size"`
    FragmentSize    int    `json:"fragment_size"`
    FragmentIndex   int    `json:"fragment_index"`
    TotalFragments  int    `json:"total_fragments"`
    Data            string `json:"data"`
    Timestamp       int64  `json:"timestamp"`
}

// HTTPPushRequest 嵌入 FragmentBase
type HTTPPushRequest struct {
    FragmentBase
}

// HTTPPollResponse 嵌入 FragmentBase，添加 Response 特有字段
type HTTPPollResponse struct {
    FragmentBase
    Success bool `json:"success,omitempty"`
    Timeout bool `json:"timeout,omitempty"`
}
```

**推荐方案A**：更简洁，减少重复代码，统一使用 `FragmentResponse`。

#### 15.2.2 简化 `HTTPPushResponse`

**当前结构**：
```go
type HTTPPushResponse struct {
    Success   bool   `json:"success"`
    Ack       uint64 `json:"ack"`       // 需要移除
    Timestamp int64  `json:"timestamp"`
}
```

**简化后**：
```go
type HTTPPushResponse struct {
    Success   bool   `json:"success"`
    Timestamp int64  `json:"timestamp"`
}
```

### 15.3 需要移除的代码逻辑

#### 15.3.1 序列号管理逻辑

**移除位置**：
- `HTTPLongPollingConn.sendFragment` 中的 `pushSeq` 递增逻辑
- `HTTPLongPollingConn.pollLoop` 中的 `pollSeq` 使用（如果存在）

**移除代码示例**：
```go
// 移除前
c.pushMu.Lock()
seq := c.pushSeq
c.pushSeq++
c.pushMu.Unlock()
// ... 在请求中使用 seq

// 移除后
// 直接发送，不需要 seq
```

#### 15.3.2 ACK 处理逻辑

**移除位置**：
- `handleHTTPPush` 中构造 `HTTPPushResponse` 时的 `Ack` 字段设置
- `sendFragment` 中解析 ACK 响应的逻辑（如果存在）

**移除代码示例**：
```go
// 移除前
resp := HTTPPushResponse{
    Success:   true,
    Ack:       pushReq.Seq,  // 移除
    Timestamp: time.Now().Unix(),
}

// 移除后
resp := HTTPPushResponse{
    Success:   true,
    Timestamp: time.Now().Unix(),
}
```

### 15.4 类型替换总结

| 类型/字段 | 操作 | 原因 | 影响文件 |
|----------|------|------|---------|
| `HTTPPushRequest.Seq` | 移除 | 冗余，未使用 | `internal/api/handlers_httppoll.go` |
| `HTTPPushRequest.IsFragment` | 移除 | 可通过 `total_fragments > 1` 判断 | `internal/api/handlers_httppoll.go` |
| `HTTPPollResponse.IsFragment` | 移除 | 可通过 `total_fragments > 1` 判断 | `internal/api/handlers_httppoll.go` |
| `FragmentResponse.IsFragment` | 移除 | 可通过 `total_fragments > 1` 判断 | `internal/protocol/httppoll/fragment_format.go` |
| `HTTPPushResponse.Ack` | 移除 | `seq` 移除后不再需要 | `internal/api/handlers_httppoll.go` |
| `HTTPLongPollingConn.pushSeq` | 移除 | `seq` 移除后不再需要 | `internal/client/transport_httppoll.go` |
| `HTTPLongPollingConn.pollSeq` | 移除 | `seq` 移除后不再需要 | `internal/client/transport_httppoll.go` |
| `HTTPPushRequest` | 统一 | 建议统一使用 `FragmentResponse` | `internal/api/handlers_httppoll.go` |
| `HTTPPollResponse` | 统一 | 建议统一使用 `FragmentResponse` | `internal/api/handlers_httppoll.go` |

### 15.5 迁移步骤

1. **第一步**：移除 `IsFragment` 字段
   - 从所有结构体中移除 `IsFragment` 字段
   - 将所有 `if xxx.IsFragment` 改为 `if xxx.TotalFragments > 1`

2. **第二步**：移除 `seq` 相关字段
   - 移除 `HTTPPushRequest.Seq`
   - 移除 `HTTPPushResponse.Ack`
   - 移除 `HTTPLongPollingConn.pushSeq` 和 `pollSeq`
   - 移除序列号管理逻辑

3. **第三步**：统一结构体
   - 统一 `HTTPPushRequest` 和 `HTTPPollResponse` 使用 `FragmentResponse`
   - 或使用嵌入结构体方式统一字段定义

4. **第四步**：更新测试
   - 更新所有单元测试和集成测试
   - 移除对 `seq` 和 `is_fragment` 的断言

## 16. 总结

本设计通过引入分片重组机制，解决了 HTTP 长轮询中大数据包传输的问题。关键设计点：

1. **统一格式**：HTTP Push Request Body 和 HTTP Poll Response Body 使用统一的 JSON 格式，包含分片元数据和 Base64 编码的数据
2. **透明分片**：对上层协议（如MySQL）透明，保证数据包完整性
3. **完整性保证**：通过分片组ID和索引保证数据完整性
4. **简化设计**：
   - 移除冗余的 `seq` 字段（未使用）
   - 移除冗余的 `is_fragment` 字段（可通过 `total_fragments > 1` 判断）
   - 统一 Request/Response 结构体，减少重复代码
   - 使用 `X-Tunnel-Package` header 中的 `RequestID` 进行请求追踪
5. **双向分片**：Request 和 Response 都支持分片，当数据 > 8KB 时自动分片
6. **错误处理**：完善的超时、清理、错误处理机制
7. **性能优化**：合理的分片大小和重组策略

通过渐进式实现和充分测试，可以安全地部署到生产环境。

