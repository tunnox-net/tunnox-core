# UDP 可靠分片传输设计

## 1. 问题分析

### 1.1 当前问题
- UDP 传输大数据包（> 65535 字节）直接失败
- UDP 可能丢包、乱序
- HTTP Poll 分片重组存在问题：分片组未收完就返回字节流

### 1.2 UDP 特性
- **无连接**：每个数据包独立传输
- **不可靠**：可能丢包、重复、乱序
- **无流控**：发送速度不受接收能力限制
- **MTU 限制**：通常 1500 字节（以太网），最大 65535 字节

### 1.3 设计目标
1. **可靠性**：保证数据完整性和顺序性
2. **高性能**：最小化延迟和开销
3. **自适应**：根据网络状况动态调整
4. **兼容性**：与现有 StreamProcessor 体系兼容

## 2. 分片格式设计

### 2.1 UDP 分片包格式

```
[Header: 24 bytes]
  - Magic: 2 bytes (0x55 0x4E) // "UN" = UDP Network
  - Version: 1 byte (0x01)
  - Flags: 1 byte
    - Bit 0: IsFragment (1=分片, 0=完整)
    - Bit 1: IsFirst (1=第一片)
    - Bit 2: IsLast (1=最后一片)
    - Bit 3: NeedACK (1=需要确认)
    - Bit 4-7: Reserved
  - FragmentGroupID: 8 bytes (uint64, 分片组唯一ID)
  - FragmentIndex: 2 bytes (uint16, 分片索引，0-based)
  - TotalFragments: 2 bytes (uint16, 总分片数)
  - OriginalSize: 4 bytes (uint32, 原始数据总大小)
  - FragmentSize: 2 bytes (uint16, 当前分片大小)
  - SequenceNum: 2 bytes (uint16, 分片序列号，用于重传检测)

[Payload: FragmentSize bytes]
  - 实际数据内容
```

**总开销**：24 字节头部 + 数据

### 2.2 ACK 包格式

```
[Header: 16 bytes]
  - Magic: 2 bytes (0x55 0x41) // "UA" = UDP ACK
  - Version: 1 byte (0x01)
  - Flags: 1 byte (全0)
  - FragmentGroupID: 8 bytes (uint64, 对应的分片组ID)
  - ReceivedBits: 2 bytes (uint16, 位图，表示已接收的分片)
  - LastReceivedIndex: 2 bytes (uint16, 最后接收的分片索引)
```

**位图编码**：每个 bit 表示一个分片是否已接收（最多支持 16 个分片，超过则使用扩展位图）

### 2.3 分片大小选择

```
UDP_MTU = 1500                    // 以太网 MTU
UDP_IP_HEADER = 20                // IP 头
UDP_UDP_HEADER = 8                // UDP 头
UDP_FRAGMENT_HEADER = 24          // 分片头
UDP_MAX_PAYLOAD = 1500 - 20 - 8  // 1472 字节

// 考虑安全边界和网络路径 MTU
UDP_FRAGMENT_SIZE = 1400          // 每个分片最大 1400 字节
UDP_FRAGMENT_THRESHOLD = 1200    // 超过 1200 字节才分片
```

## 3. 可靠传输机制

### 3.1 发送端设计

#### 3.1.1 分片发送缓冲区

```go
type UDPSendBuffer struct {
    groupID        uint64
    originalData   []byte
    fragments      []*UDPFragment
    totalFragments uint16
    sentFragments  map[uint16]time.Time  // 已发送的分片及发送时间
    ackedFragments map[uint16]bool       // 已确认的分片
    retryCount     map[uint16]int        // 重试次数
    mu             sync.RWMutex
    onComplete     func([]byte)          // 所有分片确认后的回调
    onTimeout      func()                // 超时回调
}
```

#### 3.1.2 发送流程

```
1. 检查数据大小
   ├─ <= UDP_FRAGMENT_THRESHOLD → 直接发送（不分片）
   └─ > UDP_FRAGMENT_THRESHOLD → 进入分片流程

2. 生成分片组ID（64位随机数或递增序列）

3. 计算分片参数
   ├─ totalFragments = (dataSize + UDP_FRAGMENT_SIZE - 1) / UDP_FRAGMENT_SIZE
   └─ 创建 UDPSendBuffer

4. 发送所有分片（并发发送，提高速度）
   ├─ 每个分片独立发送
   ├─ 记录发送时间
   └─ 设置重传定时器

5. 等待 ACK
   ├─ 收到 ACK → 更新 ackedFragments
   ├─ 检查是否全部确认
   │   ├─ 是 → 调用 onComplete
   │   └─ 否 → 继续等待
   └─ 超时 → 重传未确认的分片
```

#### 3.1.3 重传策略

**快速重传**：
- 收到重复 ACK 或部分 ACK 时，立即重传缺失分片
- 使用指数退避：初始 100ms，最大 1s

**超时重传**：
- 每个分片独立超时：RTT * 2（初始 RTT = 200ms）
- 最大重试次数：5 次
- 超过最大重试次数后，报告错误

### 3.2 接收端设计

#### 3.2.1 分片重组缓冲区

```go
type UDPReceiveBuffer struct {
    groupID        uint64
    originalSize   uint32
    totalFragments uint16
    fragments      map[uint16]*UDPFragment  // 按索引存储
    receivedCount  uint16
    receivedBits   []bool                  // 位图，标记已接收
    createdTime    time.Time
    lastActiveTime time.Time
    mu             sync.RWMutex
    onComplete     chan []byte             // 完成时发送重组数据
}
```

#### 3.2.2 接收流程

```
1. 接收 UDP 数据包
   ├─ 解析头部
   ├─ 检查 Magic 和 Version
   └─ 判断包类型
       ├─ 分片包 → 进入重组流程
       └─ ACK 包 → 更新发送缓冲区

2. 分片重组流程
   ├─ 查找或创建 UDPReceiveBuffer
   ├─ 验证分片有效性
   │   ├─ FragmentIndex 范围检查
   │   ├─ FragmentSize 一致性检查
   │   └─ OriginalSize 一致性检查
   ├─ 存储分片（去重）
   ├─ 更新 receivedBits
   ├─ 发送 ACK（快速确认）
   └─ 检查完整性
       ├─ 完整 → 重组并返回
       └─ 不完整 → 等待更多分片

3. 重组数据
   ├─ 按 FragmentIndex 顺序拼接
   ├─ 验证总大小 == OriginalSize
   └─ 返回完整数据
```

#### 3.2.3 ACK 策略

**立即 ACK**：
- 收到每个分片后立即发送 ACK
- 使用位图编码已接收的分片

**批量 ACK**（可选优化）：
- 如果连续收到多个分片，可以批量 ACK
- 减少 ACK 包数量

**选择性 ACK (SACK)**：
- 对于超过 16 个分片的大包，使用扩展位图
- 明确告知发送端哪些分片已接收

### 3.3 乱序处理

**接收端**：
- 使用 `map[uint16]*UDPFragment` 存储分片，不依赖接收顺序
- 重组时按 `FragmentIndex` 排序拼接

**发送端**：
- 不依赖接收顺序，只关心 ACK
- 每个分片独立确认

## 4. 性能优化

### 4.1 快速路径（无丢包场景）

**发送端**：
- 并发发送所有分片（使用 goroutine pool）
- 批量发送，减少系统调用

**接收端**：
- 使用无锁数据结构（如 lock-free map）
- 预分配缓冲区，避免频繁分配

### 4.2 内存管理

- 使用 `sync.Pool` 复用分片缓冲区
- 及时释放已重组的分片组
- 限制最大并发分片组数量（防止内存爆炸）

### 4.3 网络优化

- **MTU 发现**：动态调整分片大小
- **拥塞控制**：根据丢包率调整发送速率
- **RTT 估算**：动态调整超时时间

## 5. 与现有体系集成

### 5.1 StreamProcessor 集成

**发送端**（`udpStreamConn.Write`）：
```go
func (c *udpStreamConn) Write(p []byte) (int, error) {
    if len(p) <= UDP_FRAGMENT_THRESHOLD {
        // 小包直接发送
        return c.conn.Write(p)
    }
    
    // 大包分片发送
    return c.sendFragmented(p)
}
```

**接收端**（`udpStreamConn.Read`）：
```go
func (c *udpStreamConn) Read(p []byte) (int, error) {
    // 从重组缓冲区读取完整数据
    return c.receiveBuffer.Read(p)
}
```

### 5.2 与 HTTP 分片的区别

| 特性 | HTTP 分片 | UDP 分片 |
|------|----------|----------|
| 可靠性 | HTTP 层保证（TCP） | 需要应用层保证 |
| 顺序性 | TCP 保证 | 需要应用层处理 |
| ACK | 不需要 | 必须 |
| 重传 | TCP 处理 | 应用层处理 |
| 乱序 | 不会发生 | 可能发生 |

## 6. 实现要点

### 6.1 关键组件

1. **UDPFragmentSender**：发送端分片管理器
2. **UDPFragmentReceiver**：接收端重组管理器
3. **UDPACKManager**：ACK 处理管理器
4. **UDPRetransmissionTimer**：重传定时器

### 6.2 状态管理

**发送端状态**：
- Pending: 等待发送
- Sent: 已发送，等待 ACK
- Acked: 已确认
- Retransmitting: 重传中
- Failed: 失败（超过最大重试）

**接收端状态**：
- Receiving: 接收中
- Complete: 完整，等待读取
- Timeout: 超时，清理

### 6.3 错误处理

- **分片丢失**：重传机制处理
- **分片损坏**：校验和（可选）或依赖上层校验
- **重组超时**：清理不完整的分片组
- **内存不足**：拒绝新分片组，清理旧组

## 7. 配置参数

```go
const (
    // 分片参数
    UDPFragmentSize     = 1400    // 每个分片最大大小
    UDPFragmentThreshold = 1200   // 分片阈值
    
    // 超时参数
    UDPInitialRTT       = 200 * time.Millisecond
    UDPMaxRTT           = 2000 * time.Millisecond
    UDPRetryTimeout     = 100 * time.Millisecond
    UDPMaxRetries       = 5
    
    // 缓冲区参数
    UDPMaxSendBuffers   = 100     // 最大发送缓冲区数
    UDPMaxReceiveBuffers = 100   // 最大接收缓冲区数
    UDPBufferTimeout    = 30 * time.Second
    
    // 性能参数
    UDPConcurrentSends  = 10      // 并发发送数
    UDPACKDelay         = 10 * time.Millisecond  // ACK 延迟（批量 ACK）
)
```

## 8. 测试策略

1. **单元测试**：分片、重组、ACK 逻辑
2. **集成测试**：端到端传输
3. **压力测试**：大量并发分片组
4. **网络模拟**：丢包、乱序、延迟场景

## 9. 与 HTTP 分片的修复

**问题**：HTTP Poll 在分片组未收完时就返回数据

**修复方案**：
- 确保 `ReadAvailable` 和 `ReadPacket` 只在分片组完整时才返回数据
- 使用 channel 或 callback 机制，等待重组完成
- 添加超时机制，防止永久阻塞

