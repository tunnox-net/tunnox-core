# 多协议适配器示例

本文档展示了如何在 tunnox-core 中使用 TCP、WebSocket、UDP 和 QUIC 四种协议适配器。

## 概述

tunnox-core 支持多种协议适配器，所有适配器都实现了统一的 `Adapter` 接口，并且都可以与 `ConnectionSession` 集成来处理业务逻辑。

## 支持的协议

1. **TCP Adapter** - 基于 TCP 协议的可靠连接
2. **WebSocket Adapter** - 基于 WebSocket 协议的 HTTP 升级连接
3. **UDP Adapter** - 基于 UDP 协议的数据包传输
4. **QUIC Adapter** - 基于 QUIC 协议的现代传输协议

## 基本用法

### 1. 创建所有适配器

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "tunnox-core/internal/cloud"
    "tunnox-core/internal/protocol"
)

func main() {
    ctx := context.Background()
    
    // 创建 CloudControlAPI
    config := cloud.DefaultConfig()
    cloudControl := cloud.NewBuiltInCloudControl(config)
    cloudControl.Start()
    defer cloudControl.Stop()
    
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
    
    log.Println("Multi-protocol server started:")
    log.Println("  TCP:     :8080")
    log.Println("  WebSocket: :8081")
    log.Println("  UDP:     :8082")
    log.Println("  QUIC:    :8083")
    
    // 等待信号
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    
    log.Println("Shutting down...")
    pm.CloseAll()
    log.Println("Server stopped")
}
```

### 2. 客户端连接示例

```go
// TCP 客户端
tcpClient := protocol.NewTcpAdapter(ctx, nil)
if err := tcpClient.ConnectTo("localhost:8080"); err != nil {
    log.Printf("TCP connection failed: %v", err)
}

// WebSocket 客户端
wsClient := protocol.NewWebSocketAdapter(ctx, nil)
if err := wsClient.ConnectTo("ws://localhost:8081"); err != nil {
    log.Printf("WebSocket connection failed: %v", err)
}

// UDP 客户端
udpClient := protocol.NewUdpAdapter(ctx, nil)
if err := udpClient.ConnectTo("localhost:8082"); err != nil {
    log.Printf("UDP connection failed: %v", err)
}

// QUIC 客户端
quicClient := protocol.NewQuicAdapter(ctx, nil)
if err := quicClient.ConnectTo("localhost:8083"); err != nil {
    log.Printf("QUIC connection failed: %v", err)
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
    log.Printf("Failed to start %s adapter: %v", adapter.Name(), err)
    // 处理错误
}

// 连接错误处理
if err := adapter.ConnectTo(serverAddr); err != nil {
    log.Printf("Failed to connect to %s server: %v", adapter.Name(), err)
    // 重试或使用备用服务器
}
```

## 性能考虑

1. **TCP**: 适合需要可靠传输的场景
2. **WebSocket**: 适合需要双向通信的 Web 应用
3. **UDP**: 适合对延迟敏感的场景
4. **QUIC**: 适合现代网络环境，特别是移动网络

## 安全考虑

1. **TCP**: 可以配合 TLS 使用
2. **WebSocket**: 支持 WSS (WebSocket Secure)
3. **UDP**: 需要应用层加密
4. **QUIC**: 内置 TLS 1.3 支持

## 测试

运行所有适配器的测试：

```bash
# 测试所有适配器
go test ./tests -v -run "Test.*Adapter"

# 测试特定适配器
go test ./tests -v -run "TestTcpAdapter"
go test ./tests -v -run "TestWebSocketAdapter"
go test ./tests -v -run "TestUdpAdapter"
go test ./tests -v -run "TestQuicAdapter"
```

## 总结

通过统一的 `Adapter` 接口和 `ConnectionSession` 集成，tunnox-core 提供了灵活的多协议支持。开发者可以根据具体需求选择合适的协议，而业务逻辑可以完全复用。

这种设计实现了：
- **协议无关性**: 业务逻辑不依赖具体协议
- **可扩展性**: 容易添加新的协议支持
- **统一性**: 所有协议使用相同的接口和模式
- **灵活性**: 可以根据场景选择最适合的协议
</rewritten_file> 