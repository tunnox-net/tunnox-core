# Tunnox UDP 传输层（TUTP）设计文档

> 目标：让 Cursor 能够 **按步骤推进实现**，并在现有 Tunnox 结构下，补齐一个可用的 UDP 传输协议层。  
> 要求：**尽量不改现有公共接口**，新增代码清晰分层，方便你手动 review 与调优。

---

## 0. 总体目标与阶段划分

### 0.1 功能目标（MVP）

在 `internal/protocol` 下新增一个基于 UDP 的传输协议层（Tunnox UDP Transport，简称 **TUTP**），作为新的协议选项（`"udp"`），满足：

1. 通过 `net.UDPConn` 在 Client ↔ Server 之间传输数据。
2. 在 UDP 之上实现**可靠有序的数据流**（`reliable-ordered` 模式）：
   - 自定义报文头（Version/Flags/SessionID/PacketSeq...）。
   - 分片和重组（FragmentGroup）。
   - 按逻辑包序号做 ACK / 重传 / 窗口控制。
3. 对上层暴露一个类似 TCP 的 `io.ReadWriteCloser`，供 `StreamProcessor` 使用。
4. 复用现有：
   - `internal/stream`（压缩/加密/限速/封包）
   - `internal/security`（会话 ID、密钥）
   - `internal/core/dispose`（资源管理）

> MVP 不要求多 stream、复杂拥塞控制，只要能稳定跑业务流量。

### 0.2 实现阶段（给 Cursor 的步骤）

**阶段 1：基础结构 & 报文头 & 分片**

1. 新建 `internal/protocol/udp/` 目录及基础文件。
2. 实现 `TUTPHeader` 结构及 Encode/Decode。
3. 实现 `FragmentGroup`（分片聚合/重组）。

**阶段 2：Session 状态 & Sender/Receiver**

4. 实现 `SessionState` 结构（窗口、重试、统计）。
5. 实现 `Sender`（发送缓存、ACK 处理、重传）。
6. 实现 `Receiver`（分片收集、重组、ACK 生成）。

**阶段 3：Transport 封装 & 适配器**

7. 实现 `Transport`：封装 `*net.UDPConn`，对上提供 `io.ReadWriteCloser`。
8. 实现 `udp_adapter.go`：接入现有 `Adapter` / `ProtocolManager`。

**阶段 4：基础测试**

9. 为 `header` / `fragment_group` / `sender` / `receiver` 写单元测试。
10. 为 `transport` 写简单集成测试（loopback UDP + 简单读写）。

---

## 1. 目录结构与文件职责

在现有仓库中新增如下文件（保持与当前结构风格一致）：

```text
internal/
  protocol/
    udp/
      config.go           // 常量与配置项（MTU、窗口大小、超时等）
      header.go           // TUTP 报文头定义与编解码
      fragment_group.go   // 分片聚合与重组
      session.go          // 单个 UDP 会话状态（窗口、重传统计）
      sender.go           // 发送端逻辑：窗口、ACK、重传
      receiver.go         // 接收端逻辑：分片收集、重组、ACK 生成
      transport.go        // 封装 UDPConn，对上提供 Read/Write 接口
    adapter/
      udp_adapter.go      // 注册 “udp” 协议，桥接到 ProtocolManager
```

> Cursor：  
> - 请在创建文件前，先搜索 `internal/protocol/adapter` 下的现有 adapter（如 `tcp_adapter.go`），对齐接口与命名风格。  
> - 所有新类型/函数使用已有 logger、dispose、context 习惯。

---

## 2. 配置与常量（`config.go`）

用途：集中定义 UDP 协议相关的默认参数，避免 magic number 写死在各文件。

```go
package udp

import "time"

const (
    // 协议版本号
    TUTPVersion uint8 = 1

    // Flag 位（位运算）
    FlagACK      uint8 = 0x01
    FlagSYN      uint8 = 0x02
    FlagFIN      uint8 = 0x04
    FlagRetrans  uint8 = 0x08
    FlagUnreliable uint8 = 0x10 // 预留

    // MTU 相关（字节）
    MaxUDPPayloadSize   = 1200 // UDP payload 上限（减去 IP/UDP 头后）
    MaxHeaderSize       = 32   // TUTPHeader 最大长度估算
    MaxDataPerDatagram  = MaxUDPPayloadSize - MaxHeaderSize

    // 窗口 & 重传
    DefaultSendWindowSize    = 64
    DefaultRecvWindowSize    = 64
    DefaultMaxRetransmit     = 5
    DefaultRetransmitTimeout = 500 * time.Millisecond

    // FragmentGroup
    DefaultFragmentGroupTTL      = 10 * time.Second
    DefaultMaxFragmentGroupsPerSession = 1024

    // Session 管理
    DefaultSessionIdleTimeout = 60 * time.Second
)
```

> Cursor：请根据实际场景调整数值，但先用上述默认值方便调试。

---

## 3. 报文头（`header.go`）

### 3.1 结构定义

```go
package udp

// TUTPHeader 定义了每个 UDP datagram 前的自定义头部。
// 注意：实际编码为大端字节序。
type TUTPHeader struct {
    Version    uint8  // 固定为 TUTPVersion
    Flags      uint8  // ACK/SYN/FIN/...

    SessionID  uint32 // 服务器与客户端协商的 Session ID
    StreamID   uint32 // 预留，目前可固定为 0

    PacketSeq  uint32 // 逻辑包序号，用于可靠传输与乱序重排
    FragSeq    uint16 // 当前分片序号：0..FragCount-1
    FragCount  uint16 // 分片总数：1 表示未分片

    AckSeq     uint32 // 累积 ACK：表示 <= AckSeq 的包已被确认
    WindowSize uint16 // 接收端通告窗口大小
    Reserved   uint16 // 保留字段

    Timestamp  uint32 // 发送时间戳（毫秒），用于 RTT 估算
}
```

### 3.2 接口签名

```go
// HeaderLength 返回固定头长度（单位：字节）。
func HeaderLength() int

// Encode 将头部编码到 buf 中，buf 必须至少为 HeaderLength() 大小。
// 返回写入的字节数和错误。
func (h *TUTPHeader) Encode(buf []byte) (int, error)

// DecodeHeader 从 buf 中解析 TUTPHeader。
// 返回解析出的头、消耗的长度、错误。
func DecodeHeader(buf []byte) (*TUTPHeader, int, error)
```

**实现要点：**

- 使用 `binary.BigEndian` 写/读。
- Decode 时校验：
  - `len(buf) >= HeaderLength()`；
  - `Version == TUTPVersion`；
  - `FragCount >= 1`。
- 不在这里做 Session 有效性校验。

---

## 4. 分片聚合（`fragment_group.go`）

### 4.1 key 与结构体

```go
package udp

import "time"

type FragmentGroupKey struct {
    SessionID uint32
    StreamID  uint32
    PacketSeq uint32
}

// FragmentGroup 负责管理某个 PacketSeq 的所有分片。
type FragmentGroup struct {
    Key            FragmentGroupKey
    TotalFragments int
    ReceivedCount  int
    OriginalSize   int

    Fragments      [][]byte   // len == TotalFragments，按 FragSeq 下标存储
    CreatedAt      time.Time
    LastAccessTime time.Time
}
```

### 4.2 API

```go
func NewFragmentGroup(key FragmentGroupKey, totalFragments int, originalSize int) *FragmentGroup

// AddFragment 写入一个分片。
// - fragSeq: 当前分片序号（0..TotalFragments-1）
// - data: 该分片数据（调用方需要自行拷贝或保证后续不修改）
func (g *FragmentGroup) AddFragment(fragSeq int, data []byte) error

func (g *FragmentGroup) IsComplete() bool

// Reassemble 在完整时按 FragSeq 顺序拼接为原始 payload。
// 长度不符 OriginalSize 时返回 error。
func (g *FragmentGroup) Reassemble() ([]byte, error)
```

> Cursor：  
> - `AddFragment` 中要更新 `ReceivedCount`、`LastAccessTime`，重复分片要忽略。  
> - 不在这里做 TTL/清理逻辑，TTL 在上层 `Receiver` 管理。

---

## 5. Session 状态（`session.go`）

### 5.1 SessionKey 与 SessionState

```go
package udp

import (
    "sync"
    "time"
)

// SessionKey 标识一个 UDP 逻辑会话。
type SessionKey struct {
    SessionID uint32
    StreamID  uint32 // 目前可固定为 0，为未来多 stream 留扩展点
}

type SessionState struct {
    Key SessionKey

    // 发送侧窗口状态
    sendMutex   sync.Mutex
    sendBase    uint32 // 最早未确认的 PacketSeq
    nextSeq     uint32 // 下一个将要发送的 PacketSeq
    sendWindow  uint16 // 当前窗口大小（包数）
    maxWindow   uint16 // 最大窗口大小
    rto         time.Duration // 当前重传超时
    inFlight    map[uint32]*SendPacketState

    // 接收侧状态
    recvMutex   sync.Mutex
    recvBase    uint32 // 最后一个按序递交给上层的 PacketSeq
    fragments   map[FragmentGroupKey]*FragmentGroup

    // 限制 & 清理
    lastActive  time.Time
}
```

### 5.2 SendPacketState

```go
// SendPacketState 记录某个 PacketSeq 的发送状态。
type SendPacketState struct {
    Seq         uint32
    Payload     []byte   // 完整逻辑包的 payload（由 Sender 管理生命周期）
    LastSend    time.Time
    Retries     int
    FragCount   int
}
```

> Cursor：  
> - `SessionState` 需要提供基本方法：创建、更新 lastActive、获取 inFlight 数量等。  
> - 不在 `session.go` 做网络 IO，所有 IO 放在 `sender.go` / `receiver.go` / `transport.go`。

---

## 6. 发送端（`sender.go`）

### 6.1 Sender 结构

```go
package udp

import (
    "net"
    "sync"
    "time"
)

type Sender struct {
    conn      *net.UDPConn
    session   *SessionState
    cfg       *Config // 可选，如需将 config.go 参数封装为结构体

    closeCh   chan struct{}
    wg        sync.WaitGroup
}

// Config 可选：把 config 常量包装为配置结构，避免硬编码。
type Config struct {
    SendWindowSize    uint16
    MaxRetransmit     int
    RetransmitTimeout time.Duration
}
```

> 如果不想引入 `Config` 结构，可以直接用 `config.go` 常量。

### 6.2 核心方法签名

```go
// NewSender 创建 Sender 并初始化 SessionState 窗口参数。
func NewSender(conn *net.UDPConn, session *SessionState) *Sender

// SendLogicalPacket 将一整个“逻辑包”（已经由 StreamProcessor 封装好的数据）发送出去。
// 1. 拆分为多个 fragment（基于 MaxDataPerDatagram）。
// 2. 为该包分配 PacketSeq。
// 3. 填充并发送多个 datagram。
// 4. 更新 inFlight 状态。
func (s *Sender) SendLogicalPacket(payload []byte) error

// HandleAck 处理从 Receiver 解析出的 AckSeq 和 WindowSize。
// - 移除 <= AckSeq 的 inFlight 状态
// - 更新 sendBase
// - 调整窗口大小等
func (s *Sender) HandleAck(ackSeq uint32, windowSize uint16)

// StartRetransmitLoop 启动重传检测循环，在独立 goroutine 运行。
// 周期性扫描 inFlight，发现超时包则重发，超过 MaxRetransmit 则返回错误（上层关闭会话）。
func (s *Sender) StartRetransmitLoop()

// Close 停止重传循环，释放资源。
func (s *Sender) Close() error
```

### 6.3 SendLogicalPacket 大致逻辑（伪代码）

```text
func (s *Sender) SendLogicalPacket(payload []byte):

  lock session.sendMutex

  if inFlight count >= sendWindow:
      阻塞等待（或返回错误，先简单阻塞）

  seq := session.nextSeq
  session.nextSeq++

  // 注册 inFlight 状态
  state := &SendPacketState{
      Seq:       seq,
      Payload:   payload (或拷贝),
      LastSend:  now,
      Retries:   0,
      FragCount: 计算分片数量,
  }
  inFlight[seq] = state

  unlock session.sendMutex

  按 payload 长度拆分为 N 个 fragment：
      对每个 fragment:
          构造 TUTPHeader：
              Version = TUTPVersion
              Flags   = 0
              SessionID, StreamID 来自 session.Key
              PacketSeq = seq
              FragSeq, FragCount
              AckSeq = 0
              WindowSize = session.recvWindow（可选）
              Timestamp = now
          编码 header + payload fragment 到 buffer
          conn.WriteToUDP(buffer, 远端地址)

  state.LastSend = now
```

重传循环 `StartRetransmitLoop()`：

- 每隔 `RetransmitTimeout / 2` 左右扫描一次 `inFlight`；
- 对每个 state：
  - 若 `now - LastSend > RetransmitTimeout`：
    - 若 `Retries >= MaxRetransmit`：
      - 记录错误，通知上层关闭；
    - 否则 重发所有 fragment，`Retries++`，`LastSend = now`。

---

## 7. 接收端（`receiver.go`）

### 7.1 Receiver 结构

```go
package udp

import (
    "net"
    "sync"
    "time"
)

type Receiver struct {
    conn     *net.UDPConn
    session  *SessionState

    // 将完整逻辑包交给上层的回调
    onPacket func(payload []byte) error

    closeCh  chan struct{}
    wg       sync.WaitGroup
}
```

### 7.2 核心方法签名

```go
func NewReceiver(conn *net.UDPConn, session *SessionState, onPacket func([]byte) error) *Receiver

// StartReadLoop 启动 UDP 读循环：
// - 从 conn.ReadFromUDP 读 datagram；
// - Decode header；
// - 交给 handleDatagram。
func (r *Receiver) StartReadLoop()

// handleDatagram 处理单个 datagram：
// - 校验 SessionID/StreamID（不匹配则丢弃）。
// - 若 Flags 带 ACK，则调用 Sender.HandleAck。
// - 否则，将 fragment 存入 FragmentGroup；
//   如果完整，则重组并按模式（reliable-ordered）决定是否立即交给 onPacket。
func (r *Receiver) handleDatagram(header *TUTPHeader, payload []byte)

// Close 停止读循环，释放资源。
func (r *Receiver) Close() error
```

### 7.3 乱序 & 按序交付逻辑

`reliable-ordered` 模式下：

- `session.recvBase` 表示最后一个已按序交付给上层的 PacketSeq。
- 当一个新的 PacketSeq 完成重组时：
  - 若 `PacketSeq == recvBase + 1`：
    - 立即交付给 `onPacket`，`recvBase++`；
    - 若队列中存在 `recvBase+1, recvBase+2...` 的已完成包，可继续顺序交付；
  - 若 `PacketSeq > recvBase + 1`：
    - 标记为“已完成但待交付”，等待前面的包到齐。

ACK 策略：

- 每完成一个新的连续 PacketSeq，可以更新 `AckSeq = recvBase`；
- 根据策略：
  - 要么立即发 ACK datagram；
  - 要么按固定时间/批量 ACK，以减少 ACK 带宽。

---

## 8. Transport：对上提供 io.ReadWriteCloser（`transport.go`）

### 8.1 设计思路

- 上层（`StreamProcessor`）使用的是一个“字节流”接口（`io.Reader` / `io.Writer`）。
- UDP 层内部按“逻辑包”运作（一次发送/接收一个完整 payload）。
- 方案：
  - 在 `Transport` 内部维护一个 `bytes.Buffer` 或 channel，缓存重组好的 payload；
  - `Read(p []byte)` 从缓存中按顺序读出；
  - `Write(p []byte)` 将调用拆分为一个或多个逻辑包（可直接按 `p` 为一包）。

### 8.2 结构

```go
package udp

import (
    "io"
    "net"
    "sync"
)

type Transport struct {
    conn     *net.UDPConn
    session  *SessionState
    sender   *Sender
    receiver *Receiver

    // 上层读取缓冲区（按顺序存放完整 payload）
    readBufMu sync.Mutex
    readBuf   []byte
    readCond  *sync.Cond

    closed   bool
    closeMu  sync.Mutex
}
```

### 8.3 构造函数与接口

```go
// NewTransport 创建 UDP 传输层，并启动 Receiver 读循环与 Sender 重传循环。
// - conn: 一个已连接到对端的 UDPConn（建议使用 net.DialUDP 返回的）
func NewTransport(conn *net.UDPConn, session *SessionState) *Transport

// Write 将数据写入 UDP 会话。
// 简化起见，可以把一次 Write 作为一个逻辑包发送。
func (t *Transport) Write(p []byte) (int, error)

// Read 从内部缓冲区中读取数据。
// Receiver 每重组一个 payload，会 append 到 readBuf 并唤醒等待。
func (t *Transport) Read(p []byte) (int, error)

// Close 关闭 Sender/Receiver 和底层 conn。
func (t *Transport) Close() error
```

**实现要点：**

- `NewTransport` 内：
  - 创建 SessionState；
  - 构造 Sender/Receiver；
  - 设置 Receiver.onPacket 回调：  
    - 把 payload append 到 `readBuf` 并调用 `readCond.Signal()`；
  - 启动：
    - `receiver.StartReadLoop()`（goroutine）
    - `sender.StartRetransmitLoop()`（goroutine）

- `Read`：
  - 若 `readBuf` 为空且未关闭：`readCond.Wait()`；
  - 从 `readBuf` 拷贝数据到 `p`，移除已读部分；
  - 若 `closed` 且 `readBuf` 为空：返回 `io.EOF`。

- `Write`：
  - 可以简单实现为：对 `p` 拷贝一份，作为一个逻辑包传给 `sender.SendLogicalPacket`；
  - 返回 `len(p)` 或错误。

> Cursor：  
> - 请参考现有 `StreamProcessor` 是如何使用底层连接的（`net.Conn` / `io.ReadWriteCloser`），对接口对齐。  
> - 若现有 adapter 抽象要求实现某个接口（比如 `adapter.Connection`），请保证 `Transport` 满足该接口。

---

## 9. UDP Adapter 接入（`adapter/udp_adapter.go`）

### 9.1 目标

- 在 `internal/protocol/adapter` 中新增一个 `UDPAdapter`，实现现有的 `Adapter` 接口（Cursor 需自行搜索确定接口）。
- 适配流程：
  1. 根据配置创建 `*net.UDPConn`（DialUDP 到 server 或 ListenUDP）。
  2. 为每个逻辑会话构造一个 `Transport` 实例。
  3. 把 `Transport` 暴露给上层 `StreamProcessor`。

### 9.2 结构示意

```go
package adapter

import (
    "context"
    "net"

    "github.com/your/module/internal/protocol/udp"
    "github.com/your/module/internal/core/dispose"
)

type UDPAdapter struct {
    *dispose.ManagerBase

    ctx    context.Context
    cancel context.CancelFunc

    // 根据项目实际情况，可能还需要:
    // config, logger, metrics, etc.
}
```

### 9.3 核心方法（接口示例）

> 注意：以下方法名根据实际 `Adapter` 接口调整，Cursor 必须先搜索现有 adapter 实现（如 `tcp_adapter.go`）并对齐签名。

```go
// NewUDPAdapter 创建一个 UDPAdapter 实例。
func NewUDPAdapter(parentCtx context.Context /* + config 参数 */) *UDPAdapter

// Start/Stop 若 Adapter 接口中有类似方法，则在 Start 中建立 UDP 监听/连接资源，在 Stop 中释放。
func (a *UDPAdapter) Start() error
func (a *UDPAdapter) Stop() error

// OpenSession 为某个上层 session/mapping 创建一个 UDP 传输连接，返回 io.ReadWriteCloser。
// 内部流程：
// 1. 创建 UDPConn（DialUDP）。
// 2. 创建 SessionState。
// 3. 调用 udp.NewTransport(conn, sessionState)。
// 4. 返回 Transport 给上层。
func (a *UDPAdapter) OpenSession(/* 上层需要的参数，如 remote addr, sessionID, etc. */) (io.ReadWriteCloser, error)
```

> Cursor：  
> - 请精确对齐现有 adapter 的接口定义，不要凭空造。  
> - 在管理生命周期时，使用 `dispose.ManagerBase` 记录所有 `Transport` 实例，保证 Close 时整体释放。

---

## 10. 测试计划

### 10.1 单元测试

- `header_test.go`：
  - Encode/Decode 对称性；
  - Version/FragCount 非法值校验。

- `fragment_group_test.go`：
  - 多片分片 AddFragment + Reassemble；
  - 乱序插入情况下 Reassemble 的正确性；
  - 重复分片的处理。

- `sender_receiver_test.go`：
  - 可建立 fake UDPConn（如使用 `net.Pipe` 不适合 UDP，可考虑在本地开两个互相 DialUDP 的端口），模拟小规模发送/接收场景：
    - 单包、分片包；
    - 包丢失（通过不读某些 datagram），触发重传；
    - ack 处理是否正确更新窗口和 inFlight。

### 10.2 集成测试

- 在本机开：`UDPConn` client/server（不同端口）。
- 使用 `Transport` 在 client 端写数据，在 server 端读数据，验证：
  - 顺序一致；
  - 没有重复；
  - 在人工丢弃少数 datagram 时仍能恢复。

---

## 11. 对 Cursor 的使用建议

1. **先从头两步开始：**
   - 创建 `internal/protocol/udp/` 目录；
   - 实现 `config.go`、`header.go`、`fragment_group.go` + 对应测试。

2. **再实现 session + sender + receiver：**
   - 按上述结构先写类型和函数签名；
   - 再补内部逻辑和简单测试。

3. **最后实现 transport + udp_adapter：**
   - 对齐现有 adapter 接口；
   - 写一个小 main 或测试，验证能通过 UDP 来跑 `StreamProcessor` 的简单读写。

4. **每完成一个阶段，跑：**
   - `go test ./...`  
   - 以及你已有的 `start_test.sh` / 集成测试脚本，逐步接入。
