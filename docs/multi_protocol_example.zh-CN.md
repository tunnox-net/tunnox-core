# 多协议适配器示例

本文档展示了如何在 Tunnox Core 中使用 TCP、WebSocket、UDP 和 QUIC 四种协议适配器，结合重构后的管理器架构。

## 概述

Tunnox Core 支持多种协议适配器，所有适配器都实现了统一的 `Adapter` 接口，并且都可以与 `ConnectionSession` 集成来处理业务逻辑。经过重构后，系统采用了分层管理器架构，提供了更好的可维护性和可扩展性。

## 支持的协议

1. **TCP Adapter** - 基于 TCP 协议的可靠连接
2. **WebSocket Adapter** - 基于 WebSocket 协议的 HTTP 升级连接
3. **UDP Adapter** - 基于 UDP 协议的数据包传输
4. **QUIC Adapter** - 基于 QUIC 协议的现代传输协议

## 基本用法

### 1. 创建多协议服务器

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/cloud/storages"
    "tunnox-core/internal/protocol"
)

func main() {
    ctx := context.Background()

    // 创建配置
    config := managers.DefaultConfig()
    
    // 创建存储后端
    storage := storages.NewMemoryStorage(ctx)
    
    // 创建云控实例
    cloudControl := managers.NewCloudControl(config, storage)
    cloudControl.Start()
    defer cloudControl.Close()

    // 创建 ConnectionSession
    session := &protocol.ConnectionSession{
        CloudApi: cloudControl,
    }
    session.SetCtx(ctx, session.onClose)

    // 创建协议管理器
    pm := protocol.NewManager(ctx)

    // 创建并注册所有适配器
    tcpAdapter := protocol.NewTcpAdapter(ctx, session)
    wsAdapter := protocol.NewWebSocketAdapter(ctx, session)
    udpAdapter := protocol.NewUdpAdapter(ctx, session)
    quicAdapter := protocol.NewQuicAdapter(ctx, session)

    pm.Register(tcpAdapter)
    pm.Register(wsAdapter)
    pm.Register(udpAdapter)
    pm.Register(quicAdapter)

    // 设置监听地址
    tcpAdapter.ListenFrom(":8080")
    wsAdapter.ListenFrom(":8081")
    udpAdapter.ListenFrom(":8082")
    quicAdapter.ListenFrom(":8083")

    // 启动所有适配器
    if err := pm.StartAll(ctx); err != nil {
        log.Fatal("Failed to start adapters:", err)
    }

    log.Println("多协议服务器已启动:")
    log.Println("  TCP:       :8080")
    log.Println("  WebSocket: :8081")
    log.Println("  UDP:       :8082")
    log.Println("  QUIC:      :8083")

    // 等待信号
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    log.Println("正在关闭...")
    pm.CloseAll()
    log.Println("服务器已停止")
}
```

### 2. 使用内置云控

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud/managers"
    "tunnox-core/internal/protocol"
)

func main() {
    ctx := context.Background()

    // 创建内置云控（使用内存存储）
    config := managers.DefaultConfig()
    cloudControl := managers.NewBuiltinCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Close()

    // 创建 ConnectionSession
    session := &protocol.ConnectionSession{
        CloudApi: cloudControl,
    }
    session.SetCtx(ctx, session.onClose)

    // 创建协议管理器
    pm := protocol.NewManager(ctx)

    // 创建 TCP 适配器
    tcpAdapter := protocol.NewTcpAdapter(ctx, session)
    pm.Register(tcpAdapter)
    tcpAdapter.ListenFrom(":8080")

    // 启动适配器
    if err := pm.StartAll(ctx); err != nil {
        log.Fatal("启动适配器失败:", err)
    }

    log.Println("TCP 服务器已启动: :8080")
    
    // 保持运行
    select {}
}
```

### 3. 客户端连接示例

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/protocol"
)

func main() {
    ctx := context.Background()

    // TCP 客户端
    tcpClient := protocol.NewTcpAdapter(ctx, nil)
    if err := tcpClient.ConnectTo("localhost:8080"); err != nil {
        log.Printf("TCP 连接失败: %v", err)
    } else {
        log.Println("TCP 连接成功")
    }

    // WebSocket 客户端
    wsClient := protocol.NewWebSocketAdapter(ctx, nil)
    if err := wsClient.ConnectTo("ws://localhost:8081"); err != nil {
        log.Printf("WebSocket 连接失败: %v", err)
    } else {
        log.Println("WebSocket 连接成功")
    }

    // UDP 客户端
    udpClient := protocol.NewUdpAdapter(ctx, nil)
    if err := udpClient.ConnectTo("localhost:8082"); err != nil {
        log.Printf("UDP 连接失败: %v", err)
    } else {
        log.Println("UDP 连接成功")
    }

    // QUIC 客户端
    quicClient := protocol.NewQuicAdapter(ctx, nil)
    if err := quicClient.ConnectTo("localhost:8083"); err != nil {
        log.Printf("QUIC 连接失败: %v", err)
    } else {
        log.Println("QUIC 连接成功")
    }
}
```

## 协议特性对比

| 协议 | 可靠性 | 性能 | 防火墙友好 | 延迟 | 适用场景 |
|------|--------|------|------------|------|----------|
| TCP | 高 | 中等 | 好 | 中等 | 文件传输、数据库连接 |
| WebSocket | 高 | 中等 | 很好 | 中等 | Web应用、实时通信 |
| UDP | 低 | 高 | 好 | 低 | 游戏、流媒体、DNS |
| QUIC | 高 | 高 | 中等 | 低 | 现代Web、移动应用 |

## 连接处理流程

所有适配器都遵循相同的连接处理流程：

1. **接受连接**: 适配器接受新的连接
2. **调用 AcceptConnection**: 调用 `session.AcceptConnection(reader, writer)`
3. **业务处理**: ConnectionSession 处理具体的业务逻辑
4. **资源清理**: 连接关闭时自动清理资源

```go
// 所有适配器都调用相同的接口
func (t *TcpAdapter) handleConn(conn net.Conn) {
    t.session.AcceptConnection(conn, conn)
}

func (w *WebSocketAdapter) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
    wrapper := &WebSocketConnWrapper{conn: conn}
    w.session.AcceptConnection(wrapper, wrapper)
}

func (u *UdpAdapter) handlePacket(data []byte, addr net.Addr) {
    virtualConn := &UdpVirtualConn{data: data, addr: addr, conn: u.conn}
    u.session.AcceptConnection(virtualConn, virtualConn)
}

func (q *QuicAdapter) handleStream(stream *quic.Stream) {
    wrapper := &QuicStreamWrapper{stream: *stream}
    q.session.AcceptConnection(wrapper, wrapper)
}
```

## 与云控集成

### 用户和客户端管理

```go
func setupCloudControl() managers.CloudControlAPI {
    // 创建配置
    config := managers.DefaultConfig()
    
    // 创建存储后端
    storage := storages.NewMemoryStorage(context.Background())
    
    // 创建云控实例
    cloudControl := managers.NewCloudControl(config, storage)
    cloudControl.Start()
    
    return cloudControl
}

func createUserAndClient(cloudControl managers.CloudControlAPI) error {
    // 创建用户
    user, err := cloudControl.CreateUser("testuser", "test@example.com")
    if err != nil {
        return fmt.Errorf("创建用户失败: %w", err)
    }
    
    // 创建客户端
    client, err := cloudControl.CreateClient(user.ID, "test-client")
    if err != nil {
        return fmt.Errorf("创建客户端失败: %w", err)
    }
    
    log.Printf("创建用户: %s, 客户端: %d", user.ID, client.ID)
    return nil
}
```

### JWT 令牌管理

```go
func generateToken(cloudControl managers.CloudControlAPI, clientID int64) error {
    // 生成 JWT 令牌
    tokenInfo, err := cloudControl.GenerateJWTToken(clientID)
    if err != nil {
        return fmt.Errorf("生成令牌失败: %w", err)
    }
    
    log.Printf("为客户端 %d 生成令牌", clientID)
    log.Printf("令牌过期时间: %v", tokenInfo.ExpiresAt)
    
    return nil
}
```

### 端口映射管理

```go
func createPortMapping(cloudControl managers.CloudControlAPI, userID string, clientID int64) error {
    // 创建端口映射
    mapping := &models.PortMapping{
        UserID:         userID,
        SourceClientID: clientID,
        TargetClientID: clientID,
        Protocol:       models.ProtocolTCP,
        SourcePort:     8080,
        TargetPort:     80,
        Status:         models.MappingStatusActive,
        Type:           models.MappingTypeStandard,
    }
    
    createdMapping, err := cloudControl.CreatePortMapping(mapping)
    if err != nil {
        return fmt.Errorf("创建端口映射失败: %w", err)
    }
    
    log.Printf("创建端口映射: %s", createdMapping.ID)
    return nil
}
```

## 配置选项

### TCP 适配器
- 支持标准的 TCP 地址格式: `host:port`
- 自动处理连接的生命周期
- 支持长连接和短连接

### WebSocket 适配器
- 支持 `ws://` 和 `wss://` 协议
- 自动处理 HTTP 升级
- 内置心跳机制
- 支持二进制和文本消息

### UDP 适配器
- 支持标准的 UDP 地址格式: `host:port`
- 数据包级别的处理
- 适合无连接通信

### QUIC 适配器
- 支持标准的 QUIC 地址格式: `host:port`
- 自动生成 TLS 证书（开发环境）
- 支持多路复用
- 内置拥塞控制

## 错误处理

```go
// 统一的错误处理模式
if err := adapter.Start(ctx); err != nil {
    log.Printf("启动 %s 适配器失败: %v", adapter.Name(), err)
    // 处理错误
}

// 连接错误处理
if err := adapter.ConnectTo(serverAddr); err != nil {
    log.Printf("连接到 %s 服务器失败: %v", adapter.Name(), err)
    // 重试或使用备用服务器
}

// 云控错误处理
if err := cloudControl.CreateUser(username, email); err != nil {
    log.Printf("创建用户失败: %v", err)
    // 处理业务错误
}
```

## 性能考虑

1. **TCP**: 适合需要可靠传输的场景
   - 使用连接池复用连接
   - 设置合适的超时时间
   - 监控连接状态

2. **WebSocket**: 适合实时通信场景
   - 实现心跳机制
   - 处理连接断开重连
   - 优化消息大小

3. **UDP**: 适合低延迟场景
   - 实现重传机制
   - 处理丢包情况
   - 优化数据包大小

4. **QUIC**: 适合现代网络场景
   - 利用多路复用
   - 处理网络切换
   - 优化拥塞控制

## 资源管理

### Dispose 树结构

```go
// 所有组件都集成到 Dispose 树中
Server (根节点)
├── CloudControl
│   ├── JWTManager
│   ├── StatsManager
│   ├── NodeManager
│   └── ... (其他管理器)
├── ProtocolManager
│   ├── TcpAdapter
│   ├── WebSocketAdapter
│   ├── UdpAdapter
│   └── QuicAdapter
└── Storage Backends
    ├── MemoryStorage
    └── ... (其他存储后端)
```

### 优雅关闭

```go
func gracefulShutdown(cloudControl managers.CloudControlAPI, pm *protocol.Manager) {
    log.Println("开始优雅关闭...")
    
    // 关闭协议管理器
    pm.CloseAll()
    
    // 关闭云控
    cloudControl.Close()
    
    log.Println("优雅关闭完成")
}
```

## 扩展新协议

1. **实现 Adapter 接口**
```go
type CustomAdapter struct {
    // 实现 Adapter 接口的所有方法
}

func (c *CustomAdapter) ConnectTo(serverAddr string) error {
    // 实现连接逻辑
}

func (c *CustomAdapter) ListenFrom(serverAddr string) error {
    // 实现监听逻辑
}

// ... 其他方法
```

2. **注册到管理器**
```go
customAdapter := protocol.NewCustomAdapter(ctx, session)
pm.Register(customAdapter)
```

3. **业务逻辑无需修改**
- 所有协议都使用相同的 ConnectionSession
- 业务逻辑与协议完全解耦

## 最佳实践

1. **错误处理**: 始终检查错误并适当处理
2. **资源清理**: 使用 defer 确保资源正确释放
3. **日志记录**: 记录关键操作和错误信息
4. **配置管理**: 使用配置文件管理服务器设置
5. **监控指标**: 收集性能指标和错误统计

## 故障排除

### 常见问题

1. **端口被占用**
   ```bash
   # 检查端口使用情况
   netstat -an | grep :8080
   ```

2. **连接超时**
   - 检查网络连接
   - 验证防火墙设置
   - 调整超时配置

3. **内存泄漏**
   - 确保正确实现 Dispose 接口
   - 检查资源清理逻辑
   - 监控内存使用情况

### 调试技巧

```go
// 启用详细日志
log.SetLevel(log.DebugLevel)

// 添加性能监控
start := time.Now()
// ... 操作
log.Printf("操作耗时: %v", time.Since(start))
```

---

通过这个多协议示例，你可以看到 Tunnox Core 如何优雅地处理多种网络协议，同时保持业务逻辑的统一性和可维护性。重构后的架构使得系统更加模块化，每个组件都有明确的职责，便于扩展和维护。 