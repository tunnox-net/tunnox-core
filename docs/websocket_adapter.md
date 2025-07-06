# WebSocket Adapter

WebSocket Adapter 是 tunnox-core 项目中的一个协议适配器实现，提供了基于 WebSocket 协议的通信能力。

## 功能特性

- 支持 WebSocket 客户端和服务器模式
- 自动处理 WebSocket 握手和升级
- 内置心跳机制（ping/pong）
- 线程安全的连接管理
- 与现有的 StreamProcessor 系统完全兼容
- 支持二进制消息传输

## 基本用法

### 创建 WebSocket 适配器

```go
import (
    "context"
    "tunnox-core/internal/protocol"
)

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// 创建 WebSocket 适配器
adapter := protocol.NewWebSocketAdapter(ctx, nil)
```

### 服务器模式

```go
// 设置监听地址
adapter.ListenFrom("localhost:8080")

// 启动服务器
if err := adapter.Start(ctx); err != nil {
    log.Fatalf("Failed to start WebSocket server: %v", err)
}
defer adapter.Close()
```

### 客户端模式

```go
// 连接到 WebSocket 服务器
if err := adapter.ConnectTo("ws://localhost:8080"); err != nil {
    log.Fatalf("Failed to connect: %v", err)
}
defer adapter.Close()
```

### 数据传输

```go
import "tunnox-core/internal/stream"

// 获取读写器
reader := adapter.GetReader()
writer := adapter.GetWriter()

// 创建数据包流
ps := stream.NewStreamProcessor(reader, writer, ctx)
defer ps.Close()

// 发送数据
testData := []byte("Hello, WebSocket!")
if err := ps.WriteExact(testData); err != nil {
    log.Printf("Failed to write: %v", err)
}

// 接收数据
response, err := ps.ReadExact(len(testData))
if err != nil {
    log.Printf("Failed to read: %v", err)
}
```

## 配置选项

### 地址格式

WebSocket 适配器支持以下地址格式：

- `ws://host:port` - 明文 WebSocket
- `wss://host:port` - 加密 WebSocket (WSS)
- `host:port` - 自动添加 `ws://` 前缀

### 心跳配置

WebSocket 适配器内置了心跳机制：

- 自动发送 ping 消息（每30秒）
- 自动响应 pong 消息
- 支持自定义 ping/pong 处理器

## 错误处理

```go
// 连接错误
if err := adapter.ConnectTo("ws://invalid-address"); err != nil {
    log.Printf("Connection failed: %v", err)
}

// 数据传输错误
if err := ps.WriteExact(data); err != nil {
    log.Printf("Write failed: %v", err)
}
```

## 线程安全

WebSocket 适配器是线程安全的，支持并发访问：

- 使用读写锁保护连接状态
- 安全的连接关闭机制
- 并发安全的数据流访问

## 生命周期管理

```go
// 创建适配器
adapter := protocol.NewWebSocketAdapter(ctx, nil)

// 启动服务
adapter.Start(ctx)

// 停止服务
adapter.Stop()

// 关闭并清理资源
adapter.Close()
```

## 示例

完整的使用示例请参考 `examples/websocket_example.go`。

## 测试

运行 WebSocket 适配器的测试：

```bash
go test ./tests -v -run TestWebSocketAdapter
```

## 注意事项

1. **安全性**: 生产环境中应该配置适当的 `CheckOrigin` 函数
2. **性能**: WebSocket 连接适合长连接场景，短连接建议使用 HTTP
3. **错误处理**: 始终检查连接和数据传输的错误
4. **资源清理**: 确保在程序结束时调用 `Close()` 方法

## 依赖

- `github.com/gorilla/websocket` - WebSocket 实现
- `tunnox-core/internal/stream` - 数据流处理
- `tunnox-core/internal/utils` - 工具函数 