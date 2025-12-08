# UDP 握手机制设计

## 问题分析

### 当前问题
1. **时序竞争**：客户端在服务端 `Accept()` 之前发送数据，导致数据丢失
2. **无连接特性**：UDP 无连接，服务端需要等待第一个数据报才能确定远程地址和 SessionID
3. **数据丢失**：即使服务端先 `Accept()`，数据仍可能丢失（测试验证）

### 根本原因
- UDP 是无连接协议，没有内置的握手机制
- 客户端使用 `DialUDP` 创建已连接的 socket，数据立即发送
- 服务端使用 `ListenUDP` + `Accept()`，需要等待第一个数据报
- 存在时序窗口：客户端发送时，服务端可能尚未准备好接收

## 解决方案：SYN/ACK 握手机制

### 设计原则
1. **符合 UDP 可靠传输协议惯例**：QUIC、DTLS、KCP 等都有握手机制
2. **利用现有设计**：设计文档中已预留 `FlagSYN`
3. **最小化延迟**：握手过程应尽可能快速
4. **可靠性**：确保服务端准备好接收数据后再发送

### 握手机制设计

#### 方案1：完整三次握手（推荐）
```
客户端                          服务端
  |                               |
  |-- SYN (FlagSYN) ------------->|
  |                               | Accept() 收到 SYN
  |                               | 创建 Transport
  |                               | 启动 Receiver
  |<-- SYN-ACK (FlagSYN|FlagACK) -|
  |                               |
  |-- ACK (FlagACK) ------------->|
  |                               |
  |-- 数据 (正常数据包) --------->|
  |                               |
```

**优点**：
- 完全解决时序竞争问题
- 符合 TCP 三次握手的成熟模式
- 确保双方都准备好

**缺点**：
- 增加 1 RTT 延迟
- 需要实现握手状态机

#### 方案2：简化握手（快速模式）
```
客户端                          服务端
  |                               |
  |-- SYN (FlagSYN) ------------->|
  |                               | Accept() 收到 SYN
  |                               | 创建 Transport
  |                               | 启动 Receiver
  |                               | 立即发送 SYN-ACK
  |<-- SYN-ACK (FlagSYN|FlagACK) -|
  |                               |
  |-- 数据 (正常数据包) --------->|
  |                               |
```

**优点**：
- 减少 1 RTT（客户端不需要发送 ACK）
- 实现简单

**缺点**：
- 客户端无法确认服务端是否收到 SYN-ACK
- 如果 SYN-ACK 丢失，客户端会重传 SYN

#### 方案3：服务端主动 ACK（当前建议）
```
客户端                          服务端
  |                               |
  |-- 数据 (正常数据包) --------->|
  |                               | Accept() 收到数据
  |                               | 创建 Transport
  |                               | 启动 Receiver
  |                               | 立即发送 ACK
  |<-- ACK (FlagACK) -------------|
  |                               |
  |-- 继续发送数据 --------------->|
  |                               |
```

**优点**：
- 实现最简单
- 不需要修改客户端逻辑
- 服务端收到数据后立即确认

**缺点**：
- 如果第一个数据包丢失，客户端需要重传
- 仍然存在时序竞争（但可以通过重传解决）

## 推荐方案：完整三次握手

### 理由
1. **符合设计文档**：设计文档中已预留 `FlagSYN`
2. **符合行业惯例**：QUIC、DTLS、KCP 等都有类似机制
3. **完全解决时序竞争**：确保双方都准备好
4. **可靠性最高**：双方都确认连接建立

### 实现细节

#### 客户端流程
1. `dialUDP()` 创建 `Transport`
2. 发送 SYN 包（`FlagSYN`，`PacketSeq=0`）
3. 等待 SYN-ACK（`FlagSYN|FlagACK`）
4. 收到 SYN-ACK 后，发送 ACK（`FlagACK`，`AckSeq=1`）
5. 握手完成，开始发送数据

#### 服务端流程
1. `Accept()` 等待第一个数据报
2. 收到 SYN 包（`FlagSYN`）
3. 创建 `Transport`，启动 `Receiver`
4. 发送 SYN-ACK（`FlagSYN|FlagACK`，`AckSeq=1`）
5. 等待 ACK（`FlagACK`）
6. 握手完成，开始接收数据

### 与现有设计的兼容性

#### 设计文档中的 FlagSYN
```go
const (
    FlagACK      uint8 = 0x01
    FlagSYN      uint8 = 0x02  // 已预留
    FlagFIN      uint8 = 0x04
    FlagRetrans  uint8 = 0x08
)
```

#### 当前实现状态
- `FlagSYN` 已定义但未使用
- `FlagACK` 已实现（用于数据包确认）
- 需要添加握手状态机

### 实现步骤

1. **修改 `Transport.NewTransport()`**：
   - 客户端：发送 SYN 包，等待 SYN-ACK
   - 服务端：收到 SYN 后发送 SYN-ACK，等待 ACK

2. **修改 `Sender.SendLogicalPacket()`**：
   - 检查握手状态，未完成时不允许发送数据

3. **修改 `Receiver.handleDatagram()`**：
   - 处理 SYN 包
   - 处理 SYN-ACK 包
   - 处理 ACK 包

4. **添加握手状态机**：
   - `HandshakeState` 枚举：`Init`, `SynSent`, `SynReceived`, `Established`
   - 状态转换逻辑

### 性能考虑

1. **延迟**：增加 1 RTT（通常 < 10ms 本地，< 100ms 远程）
2. **带宽**：每个连接增加 3 个小包（约 96 字节）
3. **可靠性**：完全解决时序竞争问题

### 与 QUIC 的对比

QUIC 的握手过程：
1. Client → Server: Initial packet (包含 ClientHello)
2. Server → Client: Handshake packet (包含 ServerHello)
3. Client → Server: Handshake packet (包含 Finished)
4. 开始数据传输

TUTP 的握手过程（简化版）：
1. Client → Server: SYN
2. Server → Client: SYN-ACK
3. Client → Server: ACK
4. 开始数据传输

**相似性**：都是三次握手，确保双方准备好
**差异**：TUTP 更简单，不需要 TLS 协商

## 结论

**推荐使用完整三次握手**，理由：
1. ✅ 完全解决时序竞争问题
2. ✅ 符合 UDP 可靠传输协议惯例
3. ✅ 利用现有设计（FlagSYN 已预留）
4. ✅ 可靠性最高
5. ✅ 延迟可接受（1 RTT）

**这不是"不符合 UDP 习惯"**，而是**UDP 可靠传输协议的标准做法**。

