# 映射通道无感知迁移设计方案

## 问题分析

### 当前架构
```
TunnelConnection（映射通道）架构：
1. TunnelOpen   - 打开隧道（携带MappingID, TunnelID）
2. TunnelData   - 纯数据透传（无序列号，无确认）
3. TunnelClose  - 关闭隧道

问题：
❌ 无序列号机制 → 无法保证数据顺序
❌ 无确认机制 → 无法知道数据是否送达
❌ 无重传机制 → 断线后数据丢失
❌ 无状态保存 → 重连后无法恢复
```

### 滚动更新场景
```
场景: HTTP请求正在通过隧道传输

ClientA → Server-1 (TunnelData传输中) → TargetService
         ↓
    Server-1收到SIGTERM
         ↓
    Server-1关闭连接
         ↓
    ❌ 数据丢失，HTTP请求失败
    ❌ 用户体验：502 Bad Gateway
```

## 行业最佳实践对比

### 方案对比

| 技术方案 | 实现难度 | 用户体验 | 适用场景 | Tunnox适配性 |
|---------|---------|---------|---------|-------------|
| **QUIC连接迁移** | 高 | 完美 | 长连接 | 需重构协议层 ⚠️ |
| **MPTCP子流切换** | 高 | 完美 | 长连接 | 需内核支持 ⚠️ |
| **应用层序列号+重传** | 中 | 良好 | 中短连接 | ✅ 可实现 |
| **优雅排空+快速重连** | 低 | 可接受 | 短连接 | ✅ 立即可用 |
| **客户端缓冲+重放** | 中 | 良好 | HTTP/短连接 | ✅ 可实现 |

### 各方案详解

#### 1. QUIC连接迁移（Cloudflare Tunnel方案）
```
特性：
- 连接ID替代4元组（IP+Port）
- 服务器切换，连接ID不变
- 0-RTT连接恢复

优点：✅ 完美的用户体验
缺点：❌ 需要QUIC协议支持（Tunnox已有QUIC adapter但未深度集成）
```

#### 2. MPTCP子流切换（Tailscale方案）
```
特性：
- 多路径TCP，多条子流
- 一条子流断开，自动切换到另一条
- 内核级实现

优点：✅ 透明无感知
缺点：❌ 需要内核支持，部署复杂
```

#### 3. 应用层序列号+重传（推荐 - 类似Ngrok）
```
特性：
- TunnelData增加序列号
- 接收方确认（ACK）
- 发送方缓冲未确认数据
- 断线重连后继续传输

优点：✅ 无需修改底层协议，✅ 灵活可控
缺点：⚠️ 需要改造TunnelData结构
```

#### 4. 优雅排空+快速重连（推荐 - 短期方案）
```
特性：
- 服务器关闭前等待活跃隧道完成
- 客户端快速重连建立新隧道
- 应用层重试（HTTP 502 → 客户端重试）

优点：✅ 实现简单，✅ 立即可用
缺点：⚠️ 短暂的服务中断（但可接受）
```

## 推荐方案：分阶段实施

### Phase 1: 优雅排空（立即实施）

#### 1.1 服务端优雅关闭增强

```go
// SessionManager新增方法
func (s *SessionManager) GetActiveTunnelCount() int {
    s.tunnelConnLock.RLock()
    defer s.tunnelConnLock.RUnlock()
    return len(s.tunnelConnMap)
}

func (s *SessionManager) WaitForTunnelsToComplete(timeout time.Duration) {
    deadline := time.Now().Add(timeout)
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        if time.Now().After(deadline) {
            remaining := s.GetActiveTunnelCount()
            utils.Warnf("Graceful shutdown timeout, %d tunnels still active", remaining)
            break
        }
        
        if s.GetActiveTunnelCount() == 0 {
            utils.Infof("All tunnels completed, proceeding with shutdown")
            break
        }
        
        <-ticker.C
    }
}
```

#### 1.2 优雅关闭流程
```
收到SIGTERM →
  1. 停止接受新连接（健康检查失败）
  2. 发送 ServerShutdown 给所有控制连接
  3. 等待活跃隧道完成（最多10秒）
     ├─ 每500ms检查一次活跃隧道数
     ├─ 如果为0，立即继续
     └─ 超时后强制继续
  4. 关闭剩余连接
  5. 清理资源
```

**预期效果**：
- 短HTTP请求（< 10秒）：✅ 完全无感知
- 长连接传输（> 10秒）：⚠️ 可能中断，但客户端可重试

#### 1.3 客户端HTTP代理增强
```go
// HTTP Tunnel代理（客户端）
type HTTPTunnelProxy struct {
    maxRetries int // 3次
    retryDelay time.Duration // 100ms
}

func (p *HTTPTunnelProxy) ProxyHTTP(req *http.Request) error {
    var lastErr error
    for i := 0; i < p.maxRetries; i++ {
        err := p.sendOverTunnel(req)
        if err == nil {
            return nil // 成功
        }
        
        if isRetriable(err) { // 连接断开、502等
            utils.Warnf("Tunnel request failed (attempt %d/%d): %v", 
                i+1, p.maxRetries, err)
            time.Sleep(p.retryDelay * time.Duration(i+1))
            continue
        }
        
        return err // 不可重试的错误
    }
    return lastErr
}
```

### Phase 2: 序列号+缓冲机制（中期优化）

#### 2.1 扩展TunnelData结构
```go
// 当前结构（简单）
type TransferPacket struct {
    PacketType Type    // TunnelData
    TunnelID   string
    Payload    []byte  // 纯数据
}

// 新结构（增强）
type TunnelDataPacket struct {
    TunnelID   string
    SeqNum     uint64  // 序列号（递增）
    AckNum     uint64  // 确认号（接收方已收到的序号）
    Flags      uint8   // SYN, FIN, ACK等标志
    Payload    []byte
}

// Flags定义
const (
    FLAG_SYN = 1 << 0  // 开始传输
    FLAG_FIN = 1 << 1  // 结束传输
    FLAG_ACK = 1 << 2  // 确认
    FLAG_RST = 1 << 3  // 重置
)
```

#### 2.2 发送端缓冲机制
```go
type TunnelSendBuffer struct {
    tunnelID      string
    buffer        map[uint64][]byte // seqNum -> data
    nextSeq       uint64            // 下一个发送序号
    confirmedSeq  uint64            // 已确认的序号
    maxBufferSize int               // 最大缓冲（如10MB）
    mu            sync.RWMutex
}

func (b *TunnelSendBuffer) Send(data []byte) (uint64, error) {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    // 检查缓冲区大小
    if len(b.buffer) * 65536 > b.maxBufferSize {
        return 0, ErrBufferFull
    }
    
    seqNum := b.nextSeq
    b.buffer[seqNum] = data
    b.nextSeq++
    
    return seqNum, nil
}

func (b *TunnelSendBuffer) ConfirmUpTo(ackNum uint64) {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    // 清理已确认的数据
    for seq := b.confirmedSeq; seq < ackNum; seq++ {
        delete(b.buffer, seq)
    }
    b.confirmedSeq = ackNum
}

func (b *TunnelSendBuffer) ResendUnconfirmed() []TunnelDataPacket {
    b.mu.RLock()
    defer b.mu.RUnlock()
    
    packets := []TunnelDataPacket{}
    for seq := b.confirmedSeq; seq < b.nextSeq; seq++ {
        if data, ok := b.buffer[seq]; ok {
            packets = append(packets, TunnelDataPacket{
                TunnelID: b.tunnelID,
                SeqNum:   seq,
                Flags:    FLAG_ACK, // 需要确认
                Payload:  data,
            })
        }
    }
    return packets
}
```

#### 2.3 接收端重组机制
```go
type TunnelReceiveBuffer struct {
    tunnelID      string
    buffer        map[uint64][]byte // seqNum -> data
    nextExpected  uint64            // 期望的下一个序号
    maxOutOfOrder int               // 最大乱序包数（如100）
    mu            sync.RWMutex
}

func (b *TunnelReceiveBuffer) Receive(pkt TunnelDataPacket) ([]byte, error) {
    b.mu.Lock()
    defer b.mu.Unlock()
    
    // 已经收到的包，忽略
    if pkt.SeqNum < b.nextExpected {
        return nil, nil
    }
    
    // 期望的包，直接返回
    if pkt.SeqNum == b.nextExpected {
        b.nextExpected++
        
        // 检查是否有后续连续的包
        result := pkt.Payload
        for {
            if data, ok := b.buffer[b.nextExpected]; ok {
                result = append(result, data...)
                delete(b.buffer, b.nextExpected)
                b.nextExpected++
            } else {
                break
            }
        }
        return result, nil
    }
    
    // 乱序包，缓冲
    if pkt.SeqNum < b.nextExpected + uint64(b.maxOutOfOrder) {
        b.buffer[pkt.SeqNum] = pkt.Payload
        return nil, nil // 等待前面的包
    }
    
    // 序号太大，可能是攻击或错误
    return nil, ErrInvalidSeqNum
}
```

#### 2.4 重连恢复流程
```
隧道中断 (Server-1 关闭) →
  1. 客户端检测到连接断开
  2. 保留发送缓冲区（未确认的数据）
  3. 快速重连到新节点（Server-2）
  4. 发送 TunnelReconnect 命令：
     {
       "tunnel_id": "tunnel_xxx",
       "last_sent_seq": 12345,
       "last_ack_seq": 12340
     }
  5. Server-2 查询 Redis（TunnelState）：
     {
       "tunnel_id": "tunnel_xxx",
       "last_received_seq": 12340,
       "next_expected_seq": 12341
     }
  6. 客户端重传未确认数据（seq 12341-12345）
  7. 传输继续，用户无感知 ✅
```

### Phase 3: 高级优化（长期）

#### 3.1 智能缓冲策略
```go
// 根据网络状况动态调整缓冲区
type AdaptiveBuffer struct {
    currentSize int
    minSize     int  // 1MB
    maxSize     int  // 100MB
    rtt         time.Duration
}

func (b *AdaptiveBuffer) AdjustSize(latency time.Duration, lossRate float64) {
    // BDP (Bandwidth-Delay Product) 估算
    bandwidthMbps := 100.0 // 假设100Mbps
    bdp := int(bandwidthMbps * 1000000 * latency.Seconds() / 8)
    
    // 根据丢包率调整
    if lossRate > 0.01 { // 丢包率 > 1%
        bdp *= 2 // 增加缓冲
    }
    
    b.currentSize = clamp(bdp, b.minSize, b.maxSize)
}
```

#### 3.2 拥塞控制
```go
// 类似TCP的慢启动和拥塞避免
type CongestionControl struct {
    cwnd        int     // 拥塞窗口
    ssthresh    int     // 慢启动阈值
    inFlight    int     // 在途字节数
    mode        string  // "slow_start" | "congestion_avoidance"
}
```

#### 3.3 QUIC Integration（可选）
```
如果未来全面升级到QUIC：
- 使用QUIC的原生连接迁移
- 0-RTT重连
- 内置的序列号和重传
```

## 实施优先级

### 立即实施（Phase 1）- 优雅排空

| 工作项 | 难度 | 预期效果 | 实施时间 |
|--------|------|----------|----------|
| GetActiveTunnelCount() | 低 | 统计活跃隧道 | 1小时 |
| WaitForTunnelsToComplete() | 低 | 等待隧道完成 | 2小时 |
| 集成到优雅关闭流程 | 低 | 短请求无感知 | 2小时 |
| 客户端HTTP重试机制 | 中 | 长请求自动重试 | 4小时 |
| **总计** | - | **95%场景无感知** | **1天** |

### 中期优化（Phase 2）- 序列号机制

| 工作项 | 难度 | 预期效果 | 实施时间 |
|--------|------|----------|----------|
| 扩展TunnelData结构 | 中 | 支持序列号 | 1天 |
| SendBuffer实现 | 中 | 发送端缓冲 | 2天 |
| ReceiveBuffer实现 | 中 | 接收端重组 | 2天 |
| TunnelReconnect命令 | 中 | 重连恢复 | 2天 |
| Redis TunnelState存储 | 低 | 跨节点状态 | 1天 |
| **总计** | - | **99.9%无感知** | **1-2周** |

### 长期演进（Phase 3）- 高级特性

- 智能缓冲（根据网络状况调整）
- 拥塞控制（避免网络拥堵）
- QUIC深度集成（如果需要）

## 关键指标

| 场景 | 当前 | Phase 1 | Phase 2 | 目标 |
|------|------|---------|---------|------|
| **短HTTP请求（< 1s）** | ❌ 中断 | ✅ 无感知 | ✅ 无感知 | ✅ 无感知 |
| **中等请求（1-10s）** | ❌ 中断 | ⚠️ 可能中断 | ✅ 无感知 | ✅ 无感知 |
| **长连接传输（> 10s）** | ❌ 中断 | ❌ 中断 | ✅ 继续传输 | ✅ 继续传输 |
| **文件下载（分钟级）** | ❌ 失败 | ❌ 失败 | ✅ 断点续传 | ✅ 断点续传 |
| **WebSocket长连接** | ❌ 断开 | ❌ 断开 | ✅ 重连恢复 | ✅ 透明迁移 |

## 测试场景

### 场景1: HTTP请求中滚动更新
```bash
测试步骤：
1. 客户端发起100个并发HTTP请求（每个耗时5秒）
2. 在第3秒时，执行滚动更新（Server-1关闭）
3. 验证：
   Phase 1: ✅ 已完成的请求成功，✅ 新请求自动重试，⚠️ 进行中的可能失败
   Phase 2: ✅ 所有请求成功，✅ 透明重连，✅ 0失败
```

### 场景2: 大文件传输
```bash
测试步骤：
1. 通过隧道下载100MB文件
2. 在传输50%时，执行滚动更新
3. 验证：
   Phase 1: ❌ 下载失败，需要重新开始
   Phase 2: ✅ 断点续传，✅ 从50%继续，✅ 用户无感知
```

### 场景3: WebSocket长连接
```bash
测试步骤：
1. 建立WebSocket连接，持续传输消息
2. 执行滚动更新
3. 验证：
   Phase 1: ❌ 连接断开，应用层需重连
   Phase 2: ✅ 自动重连，✅ 消息队列缓冲，✅ 无消息丢失
```

## 架构演进路径

```
当前架构
  └─ 纯透传，无状态
     ↓
Phase 1: 优雅排空（1天实施）
  └─ 等待传输完成 + 快速重连
     ↓ 85% → 95% 无感知
Phase 2: 序列号机制（1-2周实施）
  └─ 缓冲 + 重传 + 状态恢复
     ↓ 95% → 99.9% 无感知
Phase 3: 高级优化（按需）
  └─ 拥塞控制 + QUIC集成
     ↓ 99.9% → 99.99% 无感知
```

## 推荐决策

**立即实施 Phase 1（优雅排空）**

理由：
1. ✅ 实现简单，1天完成
2. ✅ 对现有架构改动最小
3. ✅ 覆盖95%的使用场景（短HTTP请求）
4. ✅ 为Phase 2打基础

**规划 Phase 2（序列号机制）**

理由：
1. ✅ 覆盖剩余5%的场景（长连接、大文件）
2. ✅ 提供生产级别的可靠性
3. ⚠️ 需要1-2周开发时间
4. ⚠️ 需要充分测试

**暂缓 Phase 3（高级优化）**

理由：
1. ⚠️ 复杂度高，收益有限
2. ⚠️ 可等到用户量增长后再考虑
3. ✅ Phase 2已经足够好

## 与其他项目对比

| 项目 | 方案 | 优缺点 |
|------|------|--------|
| **Ngrok** | 序列号+重传 | ✅ 可靠，⚠️ 复杂 |
| **Cloudflare Tunnel** | QUIC连接迁移 | ✅ 完美，❌ 需QUIC |
| **Frp** | 优雅排空 | ✅ 简单，⚠️ 长连接中断 |
| **Tailscale** | WireGuard漫游 | ✅ 透明，❌ VPN场景 |
| **Tunnox（推荐）** | Phase1+Phase2组合 | ✅ 平衡，✅ 渐进式 |

---

**总结**：先实施Phase 1（优雅排空），解决95%问题，再根据实际需求考虑Phase 2（序列号机制）。

