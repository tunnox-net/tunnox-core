# UDP 可靠传输协议设计文档

## 1. 概述

本文档设计一个基于 UDP 的可靠传输协议（TUTP - Tunnox UDP Transport Protocol），用于 Tunnox 的 UDP 传输层。

### 1.1 设计目标

1. **可靠性**：保证数据按序无损传输
2. **效率**：充分利用网络带宽，低延迟
3. **公平性**：与 TCP 友好，避免过度占用带宽
4. **简洁性**：实现复杂度可控，易于维护

### 1.2 参考协议

- **QUIC**：现代化的 UDP 可靠传输协议
- **KCP**：游戏行业广泛使用的 ARQ 协议
- **UDT**：高性能数据传输协议
- **HTTPPoll**：我们刚实现的长轮询协议（架构参考）

---

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────┐
│         Application Layer               │
│    (io.ReadWriteCloser Interface)      │
└────────────────┬────────────────────────┘
                 │
┌────────────────▼────────────────────────┐
│         Transport Layer                 │
│  ┌──────────────────────────────────┐  │
│  │  Connection Manager              │  │
│  │  - Session routing               │  │
│  │  - Lifecycle management          │  │
│  └──────────────────────────────────┘  │
│                                         │
│  ┌──────────────────────────────────┐  │
│  │  Stream Interface                │  │
│  │  - Read/Write buffers            │  │
│  │  - Flow control                  │  │
│  └──────────────────────────────────┘  │
└────────────────┬────────────────────────┘
                 │
┌────────────────▼────────────────────────┐
│      Reliable Protocol Layer            │
│  ┌──────────────┐  ┌─────────────────┐ │
│  │   Sender     │  │    Receiver     │ │
│  │  - Send buf  │  │  - Recv buf     │ │
│  │  - Retrans   │  │  - Reorder      │ │
│  │  - Congestion│  │  - ACK gen      │ │
│  └──────────────┘  └─────────────────┘ │
│                                         │
│  ┌──────────────────────────────────┐  │
│  │  Packet Manager                  │  │
│  │  - Fragmentation                 │  │
│  │  - Reassembly                    │  │
│  └──────────────────────────────────┘  │
└────────────────┬────────────────────────┘
                 │
┌────────────────▼────────────────────────┐
│       UDP Socket Layer                  │
│  ┌──────────────────────────────────┐  │
│  │  Packet Dispatcher (中心化)      │  │
│  │  - Single UDP socket reader      │  │
│  │  - Route by (addr, sessionID)    │  │
│  └──────────────────────────────────┘  │
│                                         │
│  ┌──────────────────────────────────┐  │
│  │  UDP Socket (net.UDPConn)        │  │
│  └──────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

### 2.2 关键变更

#### 2.2.1 中心化 Packet Dispatcher（核心改进）

**问题**：当前实现中，多个 Transport 共享一个 UDP socket，每个都调用 ReadFromUDP()，导致数据包被随机分配。

**解决方案**：
```go
type PacketDispatcher struct {
    conn       *net.UDPConn
    sessions   map[SessionKey]*Session  // 路由表
    mu         sync.RWMutex
    closeCh    chan struct{}
}

// 唯一的读取循环
func (d *PacketDispatcher) readLoop() {
    for {
        buf := make([]byte, MaxUDPPacketSize)
        n, remoteAddr, err := d.conn.ReadFromUDP(buf)
        if err != nil {
            continue
        }

        // 解析头部获取 SessionID
        header := parseHeader(buf[:n])
        key := SessionKey{
            RemoteAddr: remoteAddr.String(),
            SessionID:  header.SessionID,
        }

        // 路由到对应 Session
        d.mu.RLock()
        session := d.sessions[key]
        d.mu.RUnlock()

        if session != nil {
            session.HandlePacket(buf[:n], remoteAddr)
        } else {
            // 新连接，触发 Accept
            d.handleNewConnection(buf[:n], remoteAddr, header.SessionID)
        }
    }
}
```

#### 2.2.2 Session 管理

```go
type Session struct {
    key        SessionKey
    state      SessionState  // CONNECTING, ESTABLISHED, CLOSING, CLOSED
    sender     *Sender
    receiver   *Receiver
    dispatcher *PacketDispatcher

    // 流控与拥塞控制
    sendWindow    *SendWindow
    recvWindow    *RecvWindow
    congestion    *CongestionControl
}
```

---

## 3. 协议细节

### 3.1 数据包格式

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|Version|  Type |     Flags     |          SessionID            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          StreamID             |          PacketSeq            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|          AckSeq               |          WindowSize           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                          Timestamp                            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|   FragSeq     |  FragCount    |           Checksum            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                          Payload ...                          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

**字段说明**：
- **Version** (4 bits): 协议版本
- **Type** (4 bits): 包类型（DATA, ACK, SYN, FIN, RESET）
- **Flags** (8 bits): 控制标志（ACK, SYN, FIN, PSH, etc）
- **SessionID** (32 bits): 会话标识
- **StreamID** (16 bits): 流标识（支持多路复用）
- **PacketSeq** (32 bits): 数据包序号
- **AckSeq** (32 bits): 确认序号
- **WindowSize** (16 bits): 接收窗口大小（以数据包为单位）
- **Timestamp** (32 bits): 时间戳（用于 RTT 计算）
- **FragSeq** (8 bits): 分片序号
- **FragCount** (8 bits): 总分片数
- **Checksum** (16 bits): 校验和
- **Payload**: 数据负载

### 3.2 连接建立（三次握手）

借鉴 QUIC 的简化握手：

```
Client                                  Server
  |                                       |
  |  SYN (SessionID, InitSeq)            |
  |-------------------------------------->|
  |                                       |
  |  SYN-ACK (SessionID, InitSeq)        |
  |<--------------------------------------|
  |                                       |
  |  ACK + DATA                          |
  |-------------------------------------->|
  |                                       |
```

**优化**：第三个包可以携带数据（0-RTT）

### 3.3 可靠传输机制

#### 3.3.1 ARQ（自动重传请求）

**选择重传 ARQ (Selective Repeat)**：
- 只重传丢失的数据包
- 接收端乱序接收，缓存后按序交付
- 使用 SACK（Selective ACK）告知发送端哪些包已收到

```go
type Sender struct {
    sendBuf       *SendBuffer      // 发送缓冲区
    unackedPkts   map[uint32]*Packet  // 未确认的包
    rto           time.Duration    // 重传超时
    rtt           *RTTEstimator    // RTT 估算器
    congestion    *CongestionControl
}

func (s *Sender) handleTimeout(seq uint32) {
    pkt := s.unackedPkts[seq]
    if pkt.retryCount >= MaxRetries {
        // 连接失败
        s.notifyError(ErrMaxRetriesExceeded)
        return
    }

    // 拥塞控制：超时 → 慢启动
    s.congestion.OnTimeout()

    // 重传
    pkt.retryCount++
    pkt.nextRetryTime = time.Now().Add(s.rto)
    s.sendPacket(pkt)
}
```

#### 3.3.2 RTT 测量与 RTO 计算

使用 **Karn's Algorithm** + **Jacobson's Algorithm**：

```go
type RTTEstimator struct {
    srtt    time.Duration  // 平滑 RTT
    rttvar  time.Duration  // RTT 方差
}

func (r *RTTEstimator) Update(measured time.Duration) {
    if r.srtt == 0 {
        // 首次测量
        r.srtt = measured
        r.rttvar = measured / 2
    } else {
        // RFC 6298
        rttvar := (3*r.rttvar + abs(r.srtt-measured)) / 4
        r.srtt = (7*r.srtt + measured) / 8
        r.rttvar = rttvar
    }
}

func (r *RTTEstimator) RTO() time.Duration {
    // RTO = SRTT + 4 * RTTVAR
    rto := r.srtt + 4*r.rttvar
    if rto < MinRTO {
        return MinRTO
    }
    if rto > MaxRTO {
        return MaxRTO
    }
    return rto
}
```

#### 3.3.3 滑动窗口协议

**发送窗口**：
```go
type SendWindow struct {
    size        uint32         // 窗口大小（动态调整）
    nextSeq     uint32         // 下一个要发送的序号
    sendBase    uint32         // 最旧的未确认序号
    buffer      *CircularBuffer
}

func (w *SendWindow) CanSend() bool {
    return (w.nextSeq - w.sendBase) < w.size
}
```

**接收窗口**：
```go
type RecvWindow struct {
    size        uint32
    recvBase    uint32         // 期望接收的序号
    buffer      map[uint32][]byte  // 乱序缓存
    maxSize     uint32         // 最大缓存大小
}

func (w *RecvWindow) Receive(seq uint32, data []byte) error {
    if seq < w.recvBase {
        // 重复包，丢弃
        return nil
    }

    if seq > w.recvBase + w.size {
        // 超出窗口，丢弃
        return ErrWindowOverflow
    }

    // 缓存乱序包
    w.buffer[seq] = data

    // 尝试交付连续的包
    w.deliverOrdered()
    return nil
}
```

### 3.4 拥塞控制

借鉴 TCP Cubic 算法（简化版）：

```go
type CongestionControl struct {
    cwnd        float64    // 拥塞窗口（单位：包）
    ssthresh    float64    // 慢启动阈值
    state       CCState    // SlowStart, CongestionAvoidance, FastRecovery
}

func (cc *CongestionControl) OnAck(ackedBytes uint32) {
    switch cc.state {
    case SlowStart:
        // 指数增长
        cc.cwnd += float64(ackedBytes) / MSS
        if cc.cwnd >= cc.ssthresh {
            cc.state = CongestionAvoidance
        }

    case CongestionAvoidance:
        // 线性增长（Cubic 简化）
        cc.cwnd += MSS * MSS / cc.cwnd
    }
}

func (cc *CongestionControl) OnTimeout() {
    // 超时 → 慢启动
    cc.ssthresh = cc.cwnd / 2
    cc.cwnd = MSS
    cc.state = SlowStart
}

func (cc *CongestionControl) OnDupAck() {
    // 快速重传 → 快速恢复
    cc.ssthresh = cc.cwnd / 2
    cc.cwnd = cc.ssthresh + 3*MSS
    cc.state = FastRecovery
}
```

### 3.5 流量控制（背压）

**接收端背压**：
```go
type Receiver struct {
    recvBuf     *CircularBuffer
    maxBufSize  uint32
}

func (r *Receiver) GetWindowSize() uint16 {
    available := r.maxBufSize - r.recvBuf.Len()
    return uint16(available / MSS)
}

// 在 ACK 中携带窗口大小
func (r *Receiver) sendAck(seq uint32) {
    ack := &Packet{
        Type:       PacketTypeACK,
        AckSeq:     seq,
        WindowSize: r.GetWindowSize(),  // 告知发送端可用窗口
    }
    r.send(ack)
}
```

**发送端流控**：
```go
func (s *Sender) updateWindow(remoteWindow uint16) {
    // 发送窗口 = min(拥塞窗口, 接收窗口)
    effWin := min(s.congestion.cwnd, float64(remoteWindow))
    s.sendWindow.size = uint32(effWin)
}
```

---

## 4. HTTPPoll 的经验借鉴

### 4.1 中心化管理

HTTPPoll 的 PollManager 管理所有长轮询连接：
```go
type PollManager struct {
    sessions map[string]*PollSession
    mu       sync.RWMutex
}
```

**借鉴**：UDP 也需要中心化的 PacketDispatcher

### 4.2 队列管理

HTTPPoll 使用消息队列缓存待发送数据：
```go
type PollSession struct {
    outQueue chan []byte  // 发送队列
}
```

**借鉴**：UDP 的 SendBuffer 也需要队列管理，配合流控

### 4.3 超时处理

HTTPPoll 有完善的超时机制：
```go
func (s *PollSession) cleanup() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            if time.Since(s.lastActive) > IdleTimeout {
                s.Close()
                return
            }
        }
    }
}
```

**借鉴**：UDP 需要更精细的超时管理（RTO、Keep-alive）

---

## 5. 实现计划

### 5.1 Phase 1：基础架构（1-2天）

1. **PacketDispatcher**：中心化数据包分发器
2. **Session 管理**：连接建立、路由、生命周期
3. **基础 Sender/Receiver**：简单的发送和接收

### 5.2 Phase 2：可靠性（2-3天）

1. **ARQ 机制**：重传逻辑、超时处理
2. **RTT 测量**：动态 RTO 计算
3. **滑动窗口**：发送窗口、接收窗口
4. **顺序保证**：乱序缓存和按序交付

### 5.3 Phase 3：流控与拥塞控制（2-3天）

1. **拥塞控制**：慢启动、拥塞避免、快速重传
2. **流量控制**：接收窗口通告、背压处理
3. **速率限制**：平滑发送速率

### 5.4 Phase 4：优化与测试（1-2天）

1. **性能优化**：零拷贝、批量发送
2. **压力测试**：高并发、丢包场景
3. **与 TCP 对比**：吞吐量、延迟

---

## 6. 关键指标

### 6.1 性能目标

- **吞吐量**：在 1% 丢包率下达到 TCP 的 90% 以上
- **延迟**：低于 TCP 20%（减少握手开销）
- **并发连接数**：支持 10,000+ 并发会话

### 6.2 可靠性目标

- **丢包恢复**：在 5% 丢包率下正常工作
- **乱序处理**：正确处理 50% 乱序率
- **超时恢复**：网络波动时自动调整 RTO

---

## 7. 与现有代码的对比

### 7.1 当前实现的优点

- ✅ 分片重组逻辑基本正确
- ✅ 数据包头部格式合理
- ✅ 基础的重传机制

### 7.2 当前实现的不足

- ❌ **架构缺陷**：多 Transport 竞争读取
- ❌ **无拥塞控制**：可能导致网络拥塞
- ❌ **RTT 估算缺失**：RTO 固定不合理
- ❌ **流控缺失**：无法处理背压

### 7.3 重构策略

**建议**：**全部重写**

理由：
1. 架构问题需要大量改动，修补成本高
2. 可靠性机制缺失太多，不如重新设计
3. 参考成熟协议（QUIC/KCP），实现更稳定

---

## 8. 参考资料

1. **RFC 6298** - Computing TCP's Retransmission Timer
2. **RFC 5681** - TCP Congestion Control
3. **QUIC Protocol** - https://www.chromium.org/quic
4. **KCP** - https://github.com/skywind3000/kcp
5. **UDT** - http://udt.sourceforge.net/

---

## 9. 总结

UDP 可靠传输协议的实现远比想象中复杂，需要考虑：

1. **架构设计**：中心化分发、清晰的层次
2. **可靠性**：ARQ、滑动窗口、顺序保证
3. **效率**：RTT 估算、拥塞控制、流量控制
4. **工程性**：易测试、易维护、易扩展

**推荐方案**：移除现有实现，参考 HTTPPoll 的架构经验和 QUIC/KCP 的协议设计，从头实现一个高质量的 UDP 可靠传输协议。

---

**作者**: Claude Code
**日期**: 2025-12-09
**版本**: v1.0
