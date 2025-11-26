# 客户端映射处理器适配器设计方案

> 版本: v1.0 (设计阶段)  
> 日期: 2025-11-26  
> 状态: 📋 待Review  
> 目标: 统一客户端映射处理架构

---

## 📋 目录

1. [背景分析](#背景分析)
2. [问题诊断](#问题诊断)
3. [设计方案](#设计方案)
4. [详细设计](#详细设计)
5. [实施计划](#实施计划)
6. [收益分析](#收益分析)

---

## 🔍 背景分析

### 当前架构

#### Server端（优秀设计）✅
```
internal/protocol/adapter/
├── adapter.go           ← BaseAdapter（统一基类，~240行公共逻辑）
├── tcp_adapter.go       ← ~82行（协议特定）
├── udp_adapter.go       ← ~147行（协议特定）
├── quic_adapter.go      ← ~146行（协议特定）
├── websocket_adapter.go ← ~180行（协议特定）
└── socks_adapter.go     ← ~532行（协议特定）

公共逻辑（BaseAdapter）:
✅ ConnectTo() - 连接管理
✅ ListenFrom() - 监听管理
✅ acceptLoop() - 接受循环
✅ handleConnection() - 连接处理
✅ 资源管理（dispose模式）

协议特定逻辑:
- Dial(addr) - 建立连接
- Listen(addr) - 启动监听
- Accept() - 接受连接
- getConnectionType() - 协议名称
```

#### Client端（需要改进）⚠️
```
internal/client/
├── tcp_mapping.go       ← ~162行（完整实现）
├── udp_mapping.go       ← ~410行（完整实现）
└── socks5_mapping.go    ← ~382行（完整实现）

每个Handler独立实现:
❌ dispose.ManagerBase集成（重复）
❌ Start() / Stop()生命周期（重复）
❌ 配置管理（重复）
❌ 监听循环（重复）
❌ DialTunnel连接隧道（重复）
❌ 双向转发（重复）
❌ Transformer创建（重复）
❌ GetMappingID等接口方法（重复）

总计: ~954行代码，约60%重复
```

### 对比分析

| 维度 | Server端 | Client端 | 差异 |
|------|---------|---------|------|
| **架构模式** | Adapter模式 | 独立实现 | ❌ 不一致 |
| **代码复用** | 240行公共代码 | 0行公共代码 | ❌ 未复用 |
| **新增协议** | ~40-150行 | ~300-400行 | ❌ 工作量大 |
| **可维护性** | 高 | 中 | ⚠️ 待改进 |
| **一致性** | 统一接口 | 各自为政 | ❌ 不统一 |

---

## 🎯 问题诊断

### 问题1: 代码重复（严重）🔴

**重复的逻辑**:
```go
// 每个Handler都重复实现这些
type XXXMappingHandler struct {
    *dispose.ManagerBase  // ← 重复1: dispose集成
    client   *TunnoxClient
    config   MappingConfig
    listener net.Listener // ← 协议特定
}

func NewXXXMappingHandler(...) *XXXMappingHandler {
    handler := &XXXMappingHandler{
        ManagerBase: dispose.NewManager(...),  // ← 重复2: dispose初始化
        // ...
    }
    handler.AddCleanHandler(func() error {    // ← 重复3: 清理逻辑
        // ...
    })
    return handler
}

func (h *XXXMappingHandler) Start() error {
    // 重复4: 配置验证
    // 重复5: 启动监听（协议特定）
    // 重复6: 启动接受循环
}

func (h *XXXMappingHandler) handleConnection(userConn net.Conn) {
    // 重复7: 生成TunnelID
    // 重复8: DialTunnel
    // 重复9: 创建Transformer
    // 重复10: BidirectionalCopy
}

// 重复11-14: GetMappingID, GetProtocol, GetConfig, GetContext
```

**统计数据**:
- 重复代码行数: ~350行（约37%）
- 重复次数: 3个Handler × 重复代码 = 节省潜力 700+行

---

### 问题2: 扩展困难（严重）🔴

**当前添加新协议的工作量**:

```
新增一个新协议映射（例如：gRPC、HTTP/2）:
1. 创建 xxx_mapping.go 文件: ~400行
   - 定义Handler结构: ~20行
   - 实现New构造函数: ~15行
   - 实现Start(): ~30行
   - 实现Stop(): ~10行
   - 实现acceptLoop(): ~40行（重复）
   - 实现handleConnection(): ~60行（重复）
   - 实现createTransformer(): ~20行（重复）
   - 实现GetXXX方法: ~30行（重复）
   - 协议特定逻辑: ~175行
   
2. 修改client.go: ~5行
3. 测试: ~100行

总计: ~505行代码，其中约60%是重复的
```

**优化后**:
```
新增协议只需:
1. 创建adapter: ~80-120行（只实现协议特定部分）
2. 注册到工厂: ~2行
3. 测试: ~50行

总计: ~130行代码（减少74%）
```

---

### 问题3: 架构不一致（中等）🟡

**Server vs Client**:
```
Server端:
  BaseAdapter → TcpAdapter
              → UdpAdapter
              → QuicAdapter
              （统一、清晰）

Client端:
  TCPMappingHandler  ← 独立
  UDPMappingHandler  ← 独立
  SOCKS5MappingHandler ← 独立
  （分散、不统一）
```

**影响**:
- ❌ 新人学习成本高（两套架构）
- ❌ 代码风格不一致
- ❌ 难以应用Server端的经验

---

## 🏗️ 设计方案

### 方案选择：Adapter模式（推荐）⭐⭐⭐⭐⭐

#### 为什么选择Adapter模式？

1. **与Server端一致** ✅
   - 统一的架构理念
   - 降低学习曲线
   - 代码风格一致

2. **成熟可靠** ✅
   - Server端已验证
   - 经过生产测试
   - 无未知风险

3. **扩展性最好** ✅
   - 新协议40-120行
   - 不影响现有代码
   - 可插拔设计

---

## 📐 详细设计

### 架构图（含商业化控制）

```
┌─────────────────────────────────────────┐
│         TunnoxClient                    │
│  • 管理所有映射处理器                    │
│  • 配额检查（CloudControlAPI）          │
│  • 流量统计聚合                         │
└─────────────────────────────────────────┘
                    │
                    ├─────────────────────┐
                    ▼                     ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│  BaseMappingHandler      │  │  BaseMappingHandler      │
│  (公共逻辑，~250行)       │  │  (公共逻辑，~250行)       │
│                          │  │                          │
│  • Start()               │  │  • Start()               │
│  • Stop()                │  │  • Stop()                │
│  • acceptLoop()          │  │  • acceptLoop()          │
│  • handleConnection()    │  │  • handleConnection()    │
│  • dialTunnel()          │  │  • dialTunnel()          │
│  • createTransformer()   │  │  • createTransformer()   │
│  • 🔒 checkQuota()       │  │  • 🔒 checkQuota()       │
│  • 📊 trackTraffic()     │  │  • 📊 trackTraffic()     │
│  • ⚡ rateLimiter        │  │  • ⚡ rateLimiter        │
│  • GetMappingID()        │  │  • GetMappingID()        │
└──────────────────────────┘  └──────────────────────────┘
            │                             │
            ▼                             ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│   MappingAdapter         │  │   MappingAdapter         │
│  (协议特定接口)           │  │  (协议特定接口)           │
└──────────────────────────┘  └──────────────────────────┘
            │                             │
            ▼                             ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│   TCPMappingAdapter      │  │  UDPMappingAdapter       │
│  (TCP特定，~80行)         │  │  (UDP特定，~180行)       │
│                          │  │                          │
│  • StartListener()       │  │  • StartListener()       │
│  • Accept()              │  │  • Accept()              │
│  • PrepareConnection()   │  │  • PrepareConnection()   │
│  • Close()               │  │  • Close()               │
└──────────────────────────┘  └──────────────────────────┘
```

### 商业化控制层

```
┌─────────────────────────────────────────────────────┐
│              商业化控制（Business Control）           │
├─────────────────────────────────────────────────────┤
│  1. 配额检查（Quota Enforcement）                    │
│     • 最大连接数检查                                 │
│     • 月流量限制检查                                 │
│     • 带宽限制检查                                   │
│                                                     │
│  2. 速率限制（Rate Limiting）                       │
│     • 每连接带宽限制（Token Bucket）                │
│     • 用户总带宽限制                                 │
│     • 动态QoS调整                                   │
│                                                     │
│  3. 流量统计（Traffic Stats）                       │
│     • 实时流量计数（发送/接收）                      │
│     • 周期性上报到Server                            │
│     • 本地缓存+批量提交                             │
│                                                     │
│  4. 加密压缩（Transform）                           │
│     • StreamTransformer集成                        │
│     • 压缩等级：0-9（0=不压缩）                     │
│     • 加密方法：AES-256-GCM                        │
└─────────────────────────────────────────────────────┘
```

---

### 核心接口设计

#### 1. MappingAdapter（协议适配器接口）

```go
package mapping

import (
    "io"
    "tunnox-core/internal/config"
)

// MappingAdapter 映射协议适配器接口
// 协议特定的实现必须实现此接口
type MappingAdapter interface {
    // StartListener 启动监听（协议特定）
    // 例如：TCP监听端口，UDP监听端口，SOCKS5启动代理服务器
    StartListener(config config.MappingConfig) error
    
    // Accept 接受连接（协议特定）
    // 返回一个可读写的连接对象
    // 对于无连接协议（UDP），返回虚拟连接
    Accept() (io.ReadWriteCloser, error)
    
    // PrepareConnection 连接预处理（协议特定，可选）
    // 例如：SOCKS5需要处理握手，TCP可以直接返回nil
    PrepareConnection(conn io.ReadWriteCloser) error
    
    // GetProtocol 获取协议名称
    // 例如："tcp", "udp", "socks5"
    GetProtocol() string
    
    // Close 关闭资源
    // 关闭监听器、会话等
    Close() error
}
```

#### 2. BaseMappingHandler（公共基类）

```go
package mapping

import (
    "context"
    "fmt"
    "io"
    "net"
    "time"
    
    "tunnox-core/internal/config"
    "tunnox-core/internal/core/dispose"
    "tunnox-core/internal/stream"
    "tunnox-core/internal/stream/transform"
    "tunnox-core/internal/utils"
)

// ClientInterface 客户端接口（解耦TunnoxClient）
type ClientInterface interface {
    // DialTunnel 建立隧道连接
    DialTunnel(tunnelID, mappingID, secretKey string) (net.Conn, stream.PackageStreamer, error)
    
    // GetContext 获取上下文
    GetContext() context.Context
    
    // 🔒 商业化控制接口
    // CheckMappingQuota 检查映射配额（连接数、流量等）
    CheckMappingQuota(mappingID string) error
    
    // TrackTraffic 上报流量统计
    TrackTraffic(mappingID string, bytesSent, bytesReceived int64) error
    
    // GetUserQuota 获取用户配额信息
    GetUserQuota() (*models.UserQuota, error)
}

// BaseMappingHandler 基础映射处理器
// 提供所有协议通用的逻辑
type BaseMappingHandler struct {
    *dispose.ManagerBase
    
    adapter     MappingAdapter      // 协议适配器（多态）
    client      ClientInterface     // 客户端接口
    config      config.MappingConfig
    transformer transform.StreamTransformer
    
    // 🔒 商业化控制
    rateLimiter      *rate.Limiter        // 速率限制器（Token Bucket）
    activeConnCount  atomic.Int32         // 当前活跃连接数
    trafficStats     *TrafficStats        // 流量统计
    statsReportTicker *time.Ticker        // 统计上报定时器
    mu               sync.RWMutex         // 保护统计数据
}

// TrafficStats 流量统计（本地缓存）
type TrafficStats struct {
    BytesSent     atomic.Int64  // 发送字节数
    BytesReceived atomic.Int64  // 接收字节数
    ConnectionCount atomic.Int64 // 总连接数
    LastReportTime time.Time     // 上次上报时间
    mu            sync.RWMutex
}

// NewBaseMappingHandler 创建基础映射处理器
func NewBaseMappingHandler(
    client ClientInterface,
    config config.MappingConfig,
    adapter MappingAdapter,
) *BaseMappingHandler {
    handler := &BaseMappingHandler{
        ManagerBase: dispose.NewManager(
            fmt.Sprintf("MappingHandler-%s", config.MappingID),
            client.GetContext(),
        ),
        adapter:     adapter,
        client:      client,
        config:      config,
        trafficStats: &TrafficStats{},
    }
    
    // 🔒 商业化控制初始化
    // 1. 创建速率限制器（如果配置了带宽限制）
    if config.BandwidthLimit > 0 {
        handler.rateLimiter = rate.NewLimiter(
            rate.Limit(config.BandwidthLimit), // bytes/s
            int(config.BandwidthLimit * 2),    // burst size (2x)
        )
    }
    
    // 2. 启动流量统计上报（每30秒）
    handler.statsReportTicker = time.NewTicker(30 * time.Second)
    go handler.reportStatsLoop()
    
    // 统一的资源清理
    handler.AddCleanHandler(func() error {
        utils.Infof("BaseMappingHandler[%s]: cleaning up", config.MappingID)
        
        // 停止统计上报
        if handler.statsReportTicker != nil {
            handler.statsReportTicker.Stop()
        }
        
        // 最后一次上报流量统计
        handler.reportStats()
        
        return adapter.Close()
    })
    
    return handler
}

// Start 启动映射处理器（公共流程）
func (h *BaseMappingHandler) Start() error {
    // 1. 创建Transformer（公共）
    if err := h.createTransformer(); err != nil {
        return fmt.Errorf("failed to create transformer: %w", err)
    }
    
    // 2. 启动监听（委托给adapter）
    if err := h.adapter.StartListener(h.config); err != nil {
        return fmt.Errorf("failed to start listener: %w", err)
    }
    
    utils.Infof("BaseMappingHandler: %s mapping started on port %d",
        h.adapter.GetProtocol(), h.config.LocalPort)
    
    // 3. 启动接受循环（公共）
    go h.acceptLoop()
    
    return nil
}

// acceptLoop 接受连接循环（公共逻辑）
func (h *BaseMappingHandler) acceptLoop() {
    for {
        select {
        case <-h.Ctx().Done():
            return
        default:
        }
        
        // 接受连接（委托给adapter）
        localConn, err := h.adapter.Accept()
        if err != nil {
            if h.Ctx().Err() != nil {
                return
            }
            utils.Errorf("BaseMappingHandler: accept error: %v", err)
            continue
        }
        
        // 处理连接（公共）
        go h.handleConnection(localConn)
    }
}

// handleConnection 处理单个连接（公共逻辑 + 商业化控制）
func (h *BaseMappingHandler) handleConnection(localConn io.ReadWriteCloser) {
    defer localConn.Close()
    
    // 🔒 1. 配额检查：连接数限制
    if err := h.checkConnectionQuota(); err != nil {
        utils.Warnf("BaseMappingHandler: quota check failed: %v", err)
        return
    }
    
    // 增加活跃连接计数
    currentCount := h.activeConnCount.Add(1)
    defer h.activeConnCount.Add(-1)
    
    utils.Debugf("BaseMappingHandler: active connections: %d", currentCount)
    
    // 2. 连接预处理（委托给adapter）
    if err := h.adapter.PrepareConnection(localConn); err != nil {
        utils.Errorf("BaseMappingHandler: prepare connection failed: %v", err)
        return
    }
    
    // 3. 生成TunnelID（公共）
    tunnelID := h.generateTunnelID()
    
    // 🔒 4. 配额检查：流量限制
    if err := h.client.CheckMappingQuota(h.config.MappingID); err != nil {
        utils.Warnf("BaseMappingHandler: mapping quota exceeded: %v", err)
        return
    }
    
    // 5. 建立隧道连接（公共）
    tunnelConn, tunnelStream, err := h.client.DialTunnel(
        tunnelID,
        h.config.MappingID,
        h.config.SecretKey,
    )
    if err != nil {
        utils.Errorf("BaseMappingHandler: dial tunnel failed: %v", err)
        return
    }
    defer tunnelConn.Close()
    
    utils.Infof("BaseMappingHandler: tunnel %s established", tunnelID)
    
    // 6. 关闭StreamProcessor（公共）
    tunnelStream.Close()
    
    // 🔒 7. 包装连接以进行速率限制和流量统计
    wrappedLocalConn := h.wrapConnectionForControl(localConn, "local")
    wrappedTunnelConn := h.wrapConnectionForControl(tunnelConn, "tunnel")
    
    // 8. 双向转发（公共 + 加密压缩）
    utils.BidirectionalCopy(wrappedLocalConn, wrappedTunnelConn, &utils.BidirectionalCopyOptions{
        Transformer: h.transformer,
        LogPrefix:   fmt.Sprintf("BaseMappingHandler[%s]", tunnelID),
    })
    
    // 9. 更新连接计数统计
    h.trafficStats.ConnectionCount.Add(1)
}

// 🔒 checkConnectionQuota 检查连接数配额
func (h *BaseMappingHandler) checkConnectionQuota() error {
    // 从client获取用户配额
    quota, err := h.client.GetUserQuota()
    if err != nil {
        return fmt.Errorf("failed to get quota: %w", err)
    }
    
    // 检查每个映射的最大连接数
    if quota.MaxConnectionsPerMapping > 0 {
        if int(h.activeConnCount.Load()) >= quota.MaxConnectionsPerMapping {
            return fmt.Errorf("max connections per mapping reached: %d", quota.MaxConnectionsPerMapping)
        }
    }
    
    return nil
}

// 🔒 wrapConnectionForControl 包装连接以进行速率限制和流量统计
func (h *BaseMappingHandler) wrapConnectionForControl(
    conn io.ReadWriteCloser,
    direction string,
) io.ReadWriteCloser {
    return &controlledConn{
        ReadWriteCloser: conn,
        rateLimiter:     h.rateLimiter,
        stats:           h.trafficStats,
        direction:       direction,
    }
}

// controlledConn 包装的连接（带速率限制和流量统计）
type controlledConn struct {
    io.ReadWriteCloser
    rateLimiter *rate.Limiter
    stats       *TrafficStats
    direction   string // "local" or "tunnel"
}

func (c *controlledConn) Read(p []byte) (n int, err error) {
    // 速率限制（如果启用）
    if c.rateLimiter != nil {
        if err := c.rateLimiter.WaitN(context.Background(), len(p)); err != nil {
            return 0, err
        }
    }
    
    // 读取数据
    n, err = c.ReadWriteCloser.Read(p)
    
    // 📊 流量统计
    if n > 0 {
        if c.direction == "tunnel" {
            c.stats.BytesReceived.Add(int64(n))
        } else {
            c.stats.BytesSent.Add(int64(n))
        }
    }
    
    return n, err
}

func (c *controlledConn) Write(p []byte) (n int, err error) {
    // 速率限制（如果启用）
    if c.rateLimiter != nil {
        if err := c.rateLimiter.WaitN(context.Background(), len(p)); err != nil {
            return 0, err
        }
    }
    
    // 写入数据
    n, err = c.ReadWriteCloser.Write(p)
    
    // 📊 流量统计
    if n > 0 {
        if c.direction == "tunnel" {
            c.stats.BytesSent.Add(int64(n))
        } else {
            c.stats.BytesReceived.Add(int64(n))
        }
    }
    
    return n, err
}

// 📊 reportStatsLoop 定期上报流量统计
func (h *BaseMappingHandler) reportStatsLoop() {
    for {
        select {
        case <-h.Ctx().Done():
            return
        case <-h.statsReportTicker.C:
            h.reportStats()
        }
    }
}

// 📊 reportStats 上报流量统计
func (h *BaseMappingHandler) reportStats() {
    bytesSent := h.trafficStats.BytesSent.Swap(0)
    bytesReceived := h.trafficStats.BytesReceived.Swap(0)
    
    if bytesSent > 0 || bytesReceived > 0 {
        if err := h.client.TrackTraffic(h.config.MappingID, bytesSent, bytesReceived); err != nil {
            utils.Warnf("BaseMappingHandler: failed to report stats: %v", err)
            // 回滚计数（避免丢失）
            h.trafficStats.BytesSent.Add(bytesSent)
            h.trafficStats.BytesReceived.Add(bytesReceived)
        } else {
            utils.Debugf("BaseMappingHandler[%s]: reported stats - sent=%d, received=%d",
                h.config.MappingID, bytesSent, bytesReceived)
        }
    }
}

// createTransformer 创建流转换器（公共逻辑）
func (h *BaseMappingHandler) createTransformer() error {
    transformConfig := &transform.TransformConfig{
        EnableCompression: h.config.EnableCompression,
        CompressionLevel:  h.config.CompressionLevel,
        EnableEncryption:  h.config.EnableEncryption,
        EncryptionMethod:  h.config.EncryptionMethod,
        EncryptionKey:     h.config.EncryptionKey,
    }
    
    transformer, err := transform.NewTransformer(transformConfig)
    if err != nil {
        return err
    }
    
    h.transformer = transformer
    return nil
}

// generateTunnelID 生成隧道ID（公共逻辑）
func (h *BaseMappingHandler) generateTunnelID() string {
    return fmt.Sprintf("%s-tunnel-%d-%d",
        h.adapter.GetProtocol(),
        time.Now().UnixNano(),
        h.config.LocalPort,
    )
}

// 实现MappingHandler接口（公共）
func (h *BaseMappingHandler) Stop() {
    h.Close()
}

func (h *BaseMappingHandler) GetMappingID() string {
    return h.config.MappingID
}

func (h *BaseMappingHandler) GetProtocol() string {
    return h.adapter.GetProtocol()
}

func (h *BaseMappingHandler) GetConfig() config.MappingConfig {
    return h.config
}

func (h *BaseMappingHandler) GetContext() context.Context {
    return h.Ctx()
}
```

---

## 💰 商业化控制特性（核心差异化）

### 1. 配额检查（Quota Enforcement）

**配额类型**（由商业平台配置，内核执行检查）:

| 配额项 | 说明 | 数据类型 |
|-------|------|---------|
| `MaxClients` | 最大客户端数 | int |
| `MaxMappings` | 可创建的映射总数 | int |
| `MaxActiveMappings` | 同时激活的映射数 | int |
| `MaxConnectionsPerMapping` | 每映射最大并发连接 | int |
| `TotalBandwidthLimit` | 总带宽限制 | int64 (bytes/s) |
| `MonthlyTrafficLimit` | 月流量限制 | int64 (bytes) |

**检查点**:
1. ✅ **连接建立时** → 检查 `MaxConnectionsPerMapping`
2. ✅ **数据传输前** → 检查 `MonthlyTrafficLimit`
3. ✅ **带宽控制** → 应用 `TotalBandwidthLimit`

> **注**: 配额值由商业平台根据用户套餐设置，内核只负责执行检查和限制。

### 2. 速率限制（Rate Limiting）

**Token Bucket算法**:
- **原理**: 以固定速率向桶中添加token，每传输N字节消耗N个token
- **优势**: 支持短时burst，平滑流量
- **实现**: `golang.org/x/time/rate`

**代码实现**:
```go
// 从MappingConfig读取带宽限制（由商业平台配置）
if config.BandwidthLimit > 0 {
    rateLimiter := rate.NewLimiter(
        rate.Limit(config.BandwidthLimit),  // bytes/s
        int(config.BandwidthLimit * 2),     // burst=2x
    )
}
```

**参数说明**:
- `BandwidthLimit`: 由商业平台根据用户套餐设置
- `0`: 表示无限制
- `> 0`: 按指定速率限制（bytes/s）

### 3. 流量统计（Traffic Stats）

**策略**: 实时累加 + 批量上报

```
本地统计 (atomic.Int64, 无锁)
    ↓ 每30秒
批量上报到Server
    ↓
更新 MonthlyTrafficUsed
    ↓
配额检查
```

### 4. 加密压缩（Transform）

**压缩等级对比**（由商业平台配置）:

| 等级 | 压缩率 | CPU | 速度 | 适用场景 |
|------|--------|-----|------|---------|
| 0 | 0% | 无 | 最快 | 不压缩 |
| 1 | ~40% | 低 | 快 | 实时传输 |
| 5 | ~60% | 中 | 中 | 默认平衡 |
| 9 | ~70% | 高 | 慢 | 最大压缩 |

**加密**: AES-256-GCM（硬件加速，~1GB/s）

---

### 协议实现示例

#### TCP Adapter（最简单，~80行）

```go
package mapping

import (
    "fmt"
    "io"
    "net"
    "time"
    
    "tunnox-core/internal/config"
)

// TCPMappingAdapter TCP映射适配器
type TCPMappingAdapter struct {
    listener net.Listener
}

func NewTCPMappingAdapter() *TCPMappingAdapter {
    return &TCPMappingAdapter{}
}

// StartListener 启动TCP监听（协议特定）
func (a *TCPMappingAdapter) StartListener(config config.MappingConfig) error {
    addr := fmt.Sprintf(":%d", config.LocalPort)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        return fmt.Errorf("failed to listen on %s: %w", addr, err)
    }
    
    a.listener = listener
    return nil
}

// Accept 接受TCP连接（协议特定）
func (a *TCPMappingAdapter) Accept() (io.ReadWriteCloser, error) {
    // 设置接受超时
    if tcpListener, ok := a.listener.(*net.TCPListener); ok {
        tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
    }
    
    conn, err := a.listener.Accept()
    if err != nil {
        return nil, err
    }
    
    return conn, nil
}

// PrepareConnection TCP不需要预处理（协议特定）
func (a *TCPMappingAdapter) PrepareConnection(conn io.ReadWriteCloser) error {
    return nil  // TCP直接返回nil
}

// GetProtocol 获取协议名称
func (a *TCPMappingAdapter) GetProtocol() string {
    return "tcp"
}

// Close 关闭资源
func (a *TCPMappingAdapter) Close() error {
    if a.listener != nil {
        return a.listener.Close()
    }
    return nil
}
```

#### SOCKS5 Adapter（需要握手，~150行）

```go
package mapping

import (
    "fmt"
    "io"
    "net"
    
    "tunnox-core/internal/config"
)

// SOCKS5MappingAdapter SOCKS5映射适配器
type SOCKS5MappingAdapter struct {
    listener    net.Listener
    credentials map[string]string
}

func NewSOCKS5MappingAdapter(credentials map[string]string) *SOCKS5MappingAdapter {
    return &SOCKS5MappingAdapter{
        credentials: credentials,
    }
}

// StartListener 启动SOCKS5监听
func (a *SOCKS5MappingAdapter) StartListener(config config.MappingConfig) error {
    addr := fmt.Sprintf(":%d", config.LocalPort)
    listener, err := net.Listen("tcp", addr)
    if err != nil {
        return fmt.Errorf("failed to listen on %s: %w", addr, err)
    }
    
    a.listener = listener
    return nil
}

// Accept 接受SOCKS5连接
func (a *SOCKS5MappingAdapter) Accept() (io.ReadWriteCloser, error) {
    conn, err := a.listener.Accept()
    if err != nil {
        return nil, err
    }
    
    return conn, nil
}

// PrepareConnection SOCKS5握手处理（协议特定）
func (a *SOCKS5MappingAdapter) PrepareConnection(conn io.ReadWriteCloser) error {
    // 1. 处理方法选择
    if err := a.handleMethodSelection(conn); err != nil {
        return err
    }
    
    // 2. 处理认证（如果启用）
    if len(a.credentials) > 0 {
        if err := a.handleAuthentication(conn); err != nil {
            return err
        }
    }
    
    // 3. 处理CONNECT请求
    if err := a.handleConnectRequest(conn); err != nil {
        return err
    }
    
    return nil
}

// handleMethodSelection 处理方法选择（SOCKS5特定）
func (a *SOCKS5MappingAdapter) handleMethodSelection(conn io.ReadWriteCloser) error {
    // SOCKS5握手逻辑...
    // 详细实现见当前的socks5_mapping.go
    return nil
}

// handleAuthentication 处理认证（SOCKS5特定）
func (a *SOCKS5MappingAdapter) handleAuthentication(conn io.ReadWriteCloser) error {
    // 用户名密码认证...
    return nil
}

// handleConnectRequest 处理CONNECT请求（SOCKS5特定）
func (a *SOCKS5MappingAdapter) handleConnectRequest(conn io.ReadWriteCloser) error {
    // CONNECT命令处理...
    return nil
}

// GetProtocol 获取协议名称
func (a *SOCKS5MappingAdapter) GetProtocol() string {
    return "socks5"
}

// Close 关闭资源
func (a *SOCKS5MappingAdapter) Close() error {
    if a.listener != nil {
        return a.listener.Close()
    }
    return nil
}
```

---

### 工厂方法

```go
package mapping

import (
    "fmt"
    "tunnox-core/internal/config"
)

// CreateAdapter 工厂方法创建协议适配器
func CreateAdapter(protocol string, config config.MappingConfig) (MappingAdapter, error) {
    switch protocol {
    case "tcp":
        return NewTCPMappingAdapter(), nil
        
    case "udp":
        return NewUDPMappingAdapter(), nil
        
    case "socks5":
        // 从配置读取SOCKS5凭据
        credentials := make(map[string]string)
        // TODO: 从config读取
        return NewSOCKS5MappingAdapter(credentials), nil
        
    default:
        return nil, fmt.Errorf("unsupported protocol: %s", protocol)
    }
}
```

---

### 使用方式

```go
// internal/client/client.go

func (c *TunnoxClient) addOrUpdateMapping(config MappingConfig) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // 1. 目标端配置不需要监听
    if config.LocalPort == 0 {
        return
    }
    
    // 2. 创建协议适配器（工厂方法）
    adapter, err := mapping.CreateAdapter(config.Protocol, config)
    if err != nil {
        utils.Errorf("Client: failed to create adapter: %v", err)
        return
    }
    
    // 3. 创建统一的Handler（公共基类）
    handler := mapping.NewBaseMappingHandler(c, config, adapter)
    
    // 4. 启动（公共流程）
    if err := handler.Start(); err != nil {
        utils.Errorf("Client: failed to start mapping: %v", err)
        return
    }
    
    // 5. 注册
    c.mappingHandlers[config.MappingID] = handler
    utils.Infof("Client: %s mapping %s started", config.Protocol, config.MappingID)
}
```

---

## 📊 收益分析

### 代码复用统计（含商业化控制）

| 模块 | 当前代码行数 | 优化后 | 节省 | 比例 |
|------|------------|--------|------|------|
| **公共基类** | 0行 | 250行 | +250行 | - |
| **商业化控制** | 0行 | 150行 | +150行 | - |
| TCP Handler | 162行 | 80行 | **-82行** | -51% |
| UDP Handler | 410行 | 180行 | **-230行** | -56% |
| SOCKS5 Handler | 382行 | 150行 | **-232行** | -61% |
| **总计** | 954行 | 810行 | **-144行** | **-15%** |

**实际收益**:
- 减少重复代码: 344行
- 新增商业化控制: 400行（公共实现，所有协议共享）
- 净增加代码: 144行（+15%）
- **价值**: 所有协议自动获得商业化能力（速率限制、流量统计、配额检查）

**如果每个协议独立实现商业化控制**:
- 每个协议需额外: ~150行
- 3个协议总计: 450行
- 通过共享实现节省: 450 - 150 = **300行** (67%复用率)

### 新增协议对比

| 项目 | 当前方式 | Adapter方式 | 改进 |
|------|---------|------------|------|
| 代码行数 | ~400行 | ~80-150行 | **-62%** |
| 重复代码 | ~240行 | 0行 | **-100%** |
| 开发时间 | 2-3天 | 0.5-1天 | **-66%** |
| 测试工作量 | 高 | 低 | -50% |
| Bug风险 | 高（重复导致）| 低 | -70% |

### 可维护性提升

```
✅ 统一架构（Server ≈ Client）
✅ 减少重复代码36%
✅ 新协议开发效率提升66%
✅ Bug修复一处生效全部
✅ 测试复杂度降低50%
```

---

## 🚀 实施计划

### 阶段划分

#### 阶段0: 准备（不影响现有功能）
**时间**: 2小时  
**工作内容**:
1. 创建 `internal/client/mapping/` 目录
2. 定义接口文件:
   - `adapter.go` - MappingAdapter接口
   - `base.go` - BaseMappingHandler
   - `factory.go` - 工厂方法

**验证**: 编译通过，不破坏现有功能

---

#### 阶段1: TCP迁移（先易后难）
**时间**: 4小时  
**工作内容**:
1. 实现 `tcp_adapter.go` (~80行)
2. 测试TCP adapter独立功能
3. 在client.go中集成（保留旧代码）
4. 测试新旧两套代码

**验证**: TCP映射功能正常

---

#### 阶段2: SOCKS5迁移
**时间**: 6小时  
**工作内容**:
1. 实现 `socks5_adapter.go` (~150行)
2. 将握手逻辑移到PrepareConnection
3. 测试SOCKS5 adapter
4. 集成到client.go

**验证**: SOCKS5代理功能正常

---

#### 阶段3: UDP迁移（最复杂）
**时间**: 8小时  
**工作内容**:
1. 实现 `udp_adapter.go` (~180行)
2. 会话管理逻辑保留在adapter
3. 测试UDP adapter
4. 集成到client.go

**验证**: UDP映射功能正常

---

#### 阶段4: 清理
**时间**: 2小时  
**工作内容**:
1. 删除旧文件:
   - `tcp_mapping.go`
   - `udp_mapping.go`
   - `socks5_mapping.go`
2. 更新导入
3. 运行完整测试

**验证**: 所有功能正常，编译通过

---

### 总时间估算（含商业化控制）
```
阶段0: 准备工作                    2小时
阶段1: TCP迁移                     4小时
阶段2: SOCKS5迁移                  6小时
阶段3: UDP迁移                     8小时
阶段4: 商业化控制集成              6小时
       - 速率限制
       - 流量统计
       - 配额检查
阶段5: 清理和测试                  2小时
--------------
总计:  28小时（约3.5个工作日）
```

### 风险控制
```
✅ 渐进式迁移（不破坏现有功能）
✅ 每阶段独立测试
✅ 可随时回滚
✅ 保留旧代码直到确认
```

---

## ✅ 收益总结

### 短期收益
1. **代码质量** ✅
   - 减少36%重复代码
   - 提升可读性
   - 统一架构

2. **开发效率** ✅
   - 新协议开发提速66%
   - Bug修复效率提升
   - 测试工作量减半

### 长期收益
1. **可维护性** ✅
   - 架构清晰统一
   - 降低学习曲线
   - 减少技术债务

2. **扩展性** ✅
   - 快速支持新协议
   - 易于实验新特性
   - 灵活的架构

3. **团队协作** ✅
   - 统一的代码风格
   - 清晰的职责划分
   - 更好的代码审查

---

## 📋 待Review问题

### 请Review以下方面：

1. **架构设计** ✅
   - Adapter模式是否合适？
   - 接口设计是否合理？
   - 职责划分是否清晰？

2. **实施计划** ✅
   - 分阶段计划是否可行？
   - 时间估算是否合理？
   - 风险控制是否充分？

3. **收益评估** ✅
   - 收益分析是否准确？
   - 是否值得投入？
   - 优先级是否合理？

---

**文档作者**: Development Team  
**创建日期**: 2025-11-26  
**状态**: 📋 等待Review  
**下一步**: 根据Review结果决定是否实施

