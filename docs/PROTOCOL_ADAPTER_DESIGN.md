# 服务端协议适配器设计文档

> 版本: v1.0  
> 日期: 2025-11-26  
> 状态: ✅ 已实现  
> 位置: `internal/protocol/adapter/`

---

## 📋 目录

1. [设计目标](#设计目标)
2. [架构设计](#架构设计)
3. [核心接口](#核心接口)
4. [实现细节](#实现细节)
5. [使用示例](#使用示例)
6. [扩展协议](#扩展协议)

---

## 🎯 设计目标

### 问题背景
- 需要支持多种传输协议：TCP、UDP、QUIC、WebSocket、SOCKS5
- 每个协议有自己的特点，但核心流程相似
- 避免代码重复，提高可维护性

### 设计原则
1. **抽象公共逻辑** - 将相同的代码提取到基类
2. **协议隔离** - 协议特定代码独立实现
3. **统一接口** - 对外提供一致的API
4. **易于扩展** - 新增协议只需实现少量方法
5. **资源管理** - 统一的生命周期管理

---

## 🏗️ 架构设计

### 分层结构

```
┌─────────────────────────────────────────┐
│         ProtocolManager                 │  ← 协议管理器
│  (统一管理所有协议适配器)                │
└─────────────────────────────────────────┘
                    │
                    ├─────────────────────┐
                    ▼                     ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│    ProtocolAdapter       │  │    ProtocolAdapter       │
│  (协议特定接口)           │  │  (协议特定接口)           │
└──────────────────────────┘  └──────────────────────────┘
            │                             │
            ▼                             ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│     BaseAdapter          │  │     BaseAdapter          │
│  (公共逻辑基类)           │  │  (公共逻辑基类)           │
│  • ConnectTo()           │  │  • ConnectTo()           │
│  • ListenFrom()          │  │  • ListenFrom()          │
│  • acceptLoop()          │  │  • acceptLoop()          │
│  • handleConnection()    │  │  • handleConnection()    │
└──────────────────────────┘  └──────────────────────────┘
            │                             │
            ▼                             ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│     TcpAdapter           │  │    WebSocketAdapter      │
│  • Dial()                │  │  • Dial()                │
│  • Listen()              │  │  • Listen()              │
│  • Accept()              │  │  • Accept()              │
│  • getConnectionType()   │  │  • getConnectionType()   │
└──────────────────────────┘  └──────────────────────────┘
```

### 设计模式
- **模板方法模式**: BaseAdapter定义算法骨架
- **策略模式**: 不同协议实现不同策略
- **工厂模式**: ProtocolManager创建适配器实例

---

## 📐 核心接口

### 1. Adapter 接口（顶层接口）

```go
type Adapter interface {
    // 连接到服务器
    ConnectTo(serverAddr string) error
    
    // 启动监听
    ListenFrom(serverAddr string) error
    
    // 获取协议名称
    Name() string
    
    // 获取读写器
    GetReader() io.Reader
    GetWriter() io.Writer
    
    // 关闭资源
    Close() error
    
    // 地址管理
    SetAddr(addr string)
    GetAddr() string
}
```

### 2. ProtocolAdapter 接口（协议特定）

```go
type ProtocolAdapter interface {
    Adapter  // 继承顶层接口
    
    // 协议特定方法（子类必须实现）
    Dial(addr string) (io.ReadWriteCloser, error)
    Listen(addr string) error
    Accept() (io.ReadWriteCloser, error)
    getConnectionType() string
}
```

### 3. BaseAdapter 基类（公共逻辑）

```go
type BaseAdapter struct {
    dispose.Dispose
    
    name        string
    addr        string
    session     session.Session
    active      bool
    connMutex   sync.RWMutex
    stream      stream.PackageStreamer
    streamMutex sync.RWMutex
    protocol    ProtocolAdapter  // 具体协议适配器引用
}
```

---

## 🔧 实现细节

### 公共逻辑（BaseAdapter）

#### 1. ConnectTo - 客户端连接

```go
func (b *BaseAdapter) ConnectTo(serverAddr string) error {
    // 1. 加锁保护
    b.connMutex.Lock()
    defer b.connMutex.Unlock()
    
    // 2. 检查状态
    if b.stream != nil {
        return fmt.Errorf("already connected")
    }
    
    // 3. 调用协议特定的Dial（多态）
    conn, err := b.protocol.Dial(serverAddr)
    if err != nil {
        return err
    }
    
    // 4. 创建StreamProcessor（公共）
    b.stream = stream.NewStreamProcessor(conn, conn, b.Ctx())
    
    return nil
}
```

#### 2. ListenFrom - 服务端监听

```go
func (b *BaseAdapter) ListenFrom(listenAddr string) error {
    // 1. 设置地址
    b.SetAddr(listenAddr)
    
    // 2. 调用协议特定的Listen（多态）
    if err := b.protocol.Listen(b.Addr()); err != nil {
        return err
    }
    
    // 3. 启动接受循环（公共）
    b.active = true
    go b.acceptLoop(b.protocol)
    
    return nil
}
```

#### 3. acceptLoop - 接受连接循环

```go
func (b *BaseAdapter) acceptLoop(adapter ProtocolAdapter) {
    for b.active {
        // 1. 调用协议特定的Accept（多态）
        conn, err := adapter.Accept()
        if err != nil {
            if isIgnorableError(err) {
                continue  // 忽略超时等错误
            }
            return
        }
        
        // 2. 处理连接（公共）
        go b.handleConnection(adapter, conn)
    }
}
```

#### 4. handleConnection - 处理单个连接

```go
func (b *BaseAdapter) handleConnection(adapter ProtocolAdapter, conn io.ReadWriteCloser) {
    defer conn.Close()
    
    // 1. Session初始化（公共）
    streamConn, err := b.session.AcceptConnection(conn, conn)
    if err != nil {
        return
    }
    
    // 2. 数据包处理循环（公共）
    for {
        pkt, _, err := streamConn.Stream.ReadPacket()
        if err != nil {
            return
        }
        
        // 3. 包装并分发（公共）
        streamPacket := &types.StreamPacket{
            ConnectionID: streamConn.ID,
            Packet:       pkt,
        }
        
        b.session.HandlePacket(streamPacket)
    }
}
```

---

## 💡 协议实现示例

### TCP Adapter（最简单）

```go
type TcpAdapter struct {
    BaseAdapter
    listener net.Listener
}

func NewTcpAdapter(ctx context.Context, session session.Session) *TcpAdapter {
    t := &TcpAdapter{}
    t.BaseAdapter = BaseAdapter{}
    t.SetName("tcp")
    t.SetSession(session)
    t.SetProtocolAdapter(t)  // 设置自己为协议适配器
    return t
}

// 实现协议特定方法（只需~40行）
func (t *TcpAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
    return net.Dial("tcp", addr)
}

func (t *TcpAdapter) Listen(addr string) error {
    listener, err := net.Listen("tcp", addr)
    t.listener = listener
    return err
}

func (t *TcpAdapter) Accept() (io.ReadWriteCloser, error) {
    return t.listener.Accept()
}

func (t *TcpAdapter) getConnectionType() string {
    return "TCP"
}
```

### UDP Adapter（需要会话管理）

```go
type UdpAdapter struct {
    BaseAdapter
    conn            net.PacketConn
    sessions        *udpSessionManager
    packetQueue     chan *udpPacket
}

// 实现协议特定方法（~140行，包含会话管理）
func (u *UdpAdapter) Listen(addr string) error {
    // UDP特定：创建PacketConn
    conn, err := net.ListenPacket("udp", addr)
    u.conn = conn
    
    // UDP特定：启动接收循环
    go u.receivePackets()
    go u.cleanupSessions()
    
    return err
}

func (u *UdpAdapter) Accept() (io.ReadWriteCloser, error) {
    // UDP特定：等待数据包并创建虚拟连接
    // ... 会话管理逻辑 ...
}
```

### WebSocket Adapter（需要协议升级）

```go
type WebSocketAdapter struct {
    BaseAdapter
    upgrader websocket.Upgrader
    server   *http.Server
}

func (w *WebSocketAdapter) Listen(addr string) error {
    // WebSocket特定：启动HTTP服务器
    mux := http.NewServeMux()
    mux.HandleFunc("/", w.handleWebSocket)
    
    w.server = &http.Server{
        Addr:    addr,
        Handler: mux,
    }
    
    go w.server.ListenAndServe()
    return nil
}

func (w *WebSocketAdapter) handleWebSocket(rw http.ResponseWriter, r *http.Request) {
    // WebSocket特定：协议升级
    conn, err := w.upgrader.Upgrade(rw, r, nil)
    // ... WebSocket握手 ...
}
```

---

## 📊 代码复用统计

### 公共逻辑（BaseAdapter）
- ConnectTo: ~20行
- ListenFrom: ~15行
- acceptLoop: ~30行
- handleConnection: ~40行
- 资源管理: ~30行
- **总计**: ~135行公共代码

### 协议特定代码
| 协议 | 代码行数 | 复杂度 | 说明 |
|------|---------|--------|------|
| TCP | ~40行 | 低 | 最简单实现 |
| UDP | ~140行 | 高 | 需要会话管理 |
| QUIC | ~120行 | 中 | TLS配置 |
| WebSocket | ~100行 | 中 | HTTP升级 |
| SOCKS5 | ~500行 | 高 | 协议握手复杂 |

### 复用率分析
```
总代码行数: ~1035行
公共代码: ~135行 (13%)
协议特定代码: ~900行 (87%)

如果没有BaseAdapter，每个协议需要额外实现135行
节省代码: 135行 × 5个协议 = 675行 (约65%复用率)
```

---

## 🚀 使用示例

### 创建和启动TCP适配器

```go
// 1. 创建Session
sessionMgr := session.NewSessionManager(idManager, ctx)

// 2. 创建TCP适配器
tcpAdapter := adapter.NewTcpAdapter(ctx, sessionMgr)

// 3. 启动监听
err := tcpAdapter.ListenFrom(":7000")
if err != nil {
    log.Fatal(err)
}

// 4. 资源清理
defer tcpAdapter.Close()
```

### ProtocolManager统一管理

```go
type ProtocolManager struct {
    adapters map[string]adapter.ProtocolAdapter
}

func (pm *ProtocolManager) RegisterProtocol(name string, adapter adapter.ProtocolAdapter) {
    pm.adapters[name] = adapter
}

func (pm *ProtocolManager) StartAll() error {
    for name, adapter := range pm.adapters {
        if err := adapter.ListenFrom(config[name].Address); err != nil {
            return fmt.Errorf("failed to start %s: %w", name, err)
        }
    }
    return nil
}
```

---

## 🔌 扩展新协议

### 步骤

1. **定义协议适配器**
   ```go
   type NewProtocolAdapter struct {
       BaseAdapter
       // 协议特定字段
   }
   ```

2. **实现4个必需方法**
   ```go
   func (a *NewProtocolAdapter) Dial(addr string) (io.ReadWriteCloser, error) { }
   func (a *NewProtocolAdapter) Listen(addr string) error { }
   func (a *NewProtocolAdapter) Accept() (io.ReadWriteCloser, error) { }
   func (a *NewProtocolAdapter) getConnectionType() string { }
   ```

3. **实现构造函数**
   ```go
   func NewXXXAdapter(ctx context.Context, session session.Session) *NewProtocolAdapter {
       a := &NewProtocolAdapter{}
       a.BaseAdapter = BaseAdapter{}
       a.SetName("new-protocol")
       a.SetSession(session)
       a.SetProtocolAdapter(a)
       return a
   }
   ```

4. **注册到ProtocolManager**
   ```go
   protocolMgr.RegisterProtocol("new-protocol", adapter)
   ```

### 工作量估算
- 简单协议（类似TCP）: ~40-60行
- 中等复杂度（类似WebSocket）: ~100-150行
- 高复杂度（类似SOCKS5）: ~300-500行

---

## ✅ 优点总结

### 1. 代码复用
- ✅ 135行公共代码被5个协议复用
- ✅ 节省约65%重复代码
- ✅ 新协议只需40-500行

### 2. 架构清晰
- ✅ 职责分离：公共逻辑 vs 协议特定
- ✅ 统一接口：对外API一致
- ✅ 易于理解：模板方法模式

### 3. 易于扩展
- ✅ 新协议4个方法即可
- ✅ 不影响现有代码
- ✅ 可插拔设计

### 4. 资源管理
- ✅ 统一的dispose模式
- ✅ Context传播
- ✅ 优雅关闭

### 5. 可测试性
- ✅ 可以Mock ProtocolAdapter
- ✅ 公共逻辑单独测试
- ✅ 协议特定逻辑独立测试

---

## 📚 相关文档

- [架构设计文档](./ARCHITECTURE_DESIGN_V2.2.md)
- [开发指南](./DEVELOPMENT_GUIDE_V2.2.md)
- [SOCKS5实现说明](../internal/protocol/adapter/SOCKS5_README.md)

---

## 🎓 最佳实践

### DO ✅
1. 新协议继承BaseAdapter
2. 只实现必需的4个方法
3. 协议特定逻辑在子类
4. 使用dispose模式管理资源
5. 错误处理要完善

### DON'T ❌
1. 不要在BaseAdapter中添加协议特定代码
2. 不要绕过BaseAdapter直接实现Adapter接口
3. 不要在Accept中做复杂处理（移到handleConnection）
4. 不要忘记设置SetProtocolAdapter(self)
5. 不要忘记资源清理

---

**文档维护者**: Development Team  
**最后更新**: 2025-11-26  
**状态**: ✅ 生产使用中

