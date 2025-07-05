# ConnectionSession 集成示例

本文档展示了如何在 TCP 和 WebSocket adapter 中集成 `ConnectionSession.AcceptConnection` 来处理连接。

## 概述

`ConnectionSession.AcceptConnection` 是处理新连接的核心方法，它负责：
- 创建 PackageStream 来处理数据流
- 管理连接的生命周期
- 处理连接的业务逻辑

## 基本用法

### 1. 创建 ConnectionSession

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/cloud"
    "tunnox-core/internal/protocol"
)

func main() {
    ctx := context.Background()
    
    // 创建 CloudControlAPI (这里使用内置实现)
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
    
    // 创建并注册 TCP adapter，传入 session
    tcpAdapter := protocol.NewTcpAdapter(ctx, session)
    pm.Register(tcpAdapter)
    
    // 创建并注册 WebSocket adapter，传入 session
    wsAdapter := protocol.NewWebSocketAdapter(ctx, session)
    pm.Register(wsAdapter)
    
    // 启动所有适配器
    if err := pm.StartAll(ctx); err != nil {
        log.Fatal("Failed to start adapters:", err)
    }
    
    log.Println("Server started with ConnectionSession integration")
    
    // 等待信号
    select {}
}
```

### 2. 连接处理流程

当新的连接到达时，处理流程如下：

1. **TCP Adapter**: 在 `handleConn` 方法中调用 `session.AcceptConnection(conn, conn)`
2. **WebSocket Adapter**: 在 `handleWebSocket` 方法中调用 `session.AcceptConnection(wrapper, wrapper)`
3. **ConnectionSession**: 在 `AcceptConnection` 方法中创建 PackageStream 并处理业务逻辑

```go
// TCP Adapter 中的处理
func (t *TcpAdapter) handleConn(conn net.Conn) {
    defer conn.Close()
    utils.Infof("TCP adapter handling connection from %s", conn.RemoteAddr())
    
    // 调用 ConnectionSession.AcceptConnection 处理连接
    if t.session != nil {
        t.session.AcceptConnection(conn, conn)
    } else {
        // 如果没有 session，使用默认的 echo 处理
        // ... 默认处理逻辑
    }
}

// WebSocket Adapter 中的处理
func (w *WebSocketAdapter) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
    conn, err := w.upgrader.Upgrade(writer, request, nil)
    if err != nil {
        utils.Errorf("Failed to upgrade connection: %v", err)
        return
    }
    
    utils.Infof("WebSocket connection established from %s", conn.RemoteAddr())
    
    // 调用 ConnectionSession.AcceptConnection 处理连接
    if w.session != nil {
        wrapper := &WebSocketConnWrapper{conn: conn}
        w.session.AcceptConnection(wrapper, wrapper)
    } else {
        // 如果没有 session，使用默认处理
        // ... 默认处理逻辑
    }
}
```

### 3. ConnectionSession 实现

```go
// ConnectionSession 的 AcceptConnection 方法
func (s *ConnectionSession) AcceptConnection(reader io.Reader, writer io.Writer) {
    // 创建 PackageStream
    ps := stream.NewPackageStream(reader, writer, s.Ctx())
    
    // 添加关闭回调
    ps.AddCloseFunc(func() {
        s.connMapLock.Lock()
        defer s.connMapLock.Unlock()
        // 清理连接映射
        // delete(s.connMap, conn)
    })
    
    // 开始读取数据包
    // ps.ReadPacket()
    
    // 这里可以添加具体的业务逻辑
    // 例如：身份验证、数据转发、连接管理等
}
```

## 高级用法

### 1. 自定义连接处理

```go
// 扩展 ConnectionSession 以支持自定义处理
type CustomConnectionSession struct {
    protocol.ConnectionSession
    customHandler func(stream.PackageStreamer)
}

func (s *CustomConnectionSession) AcceptConnection(reader io.Reader, writer io.Writer) {
    ps := stream.NewPackageStream(reader, writer, s.Ctx())
    
    // 调用自定义处理器
    if s.customHandler != nil {
        go s.customHandler(ps)
    } else {
        // 默认处理
        s.ConnectionSession.AcceptConnection(reader, writer)
    }
}
```

### 2. 连接统计和管理

```go
func (s *ConnectionSession) AcceptConnection(reader io.Reader, writer io.Writer) {
    ps := stream.NewPackageStream(reader, writer, s.Ctx())
    
    // 生成连接ID
    connID := generateConnectionID()
    
    // 记录连接
    s.connMapLock.Lock()
    s.connMap[reader] = connID
    s.streamer[connID] = ps
    s.connMapLock.Unlock()
    
    // 添加关闭回调
    ps.AddCloseFunc(func() {
        s.connMapLock.Lock()
        defer s.connMapLock.Unlock()
        delete(s.connMap, reader)
        delete(s.streamer, connID)
    })
    
    // 开始处理数据
    go s.handleConnection(ps, connID)
}

func (s *ConnectionSession) handleConnection(ps stream.PackageStreamer, connID string) {
    // 实现具体的连接处理逻辑
    // 例如：数据包解析、业务处理等
}
```

## 注意事项

1. **线程安全**: ConnectionSession 使用读写锁保护共享数据
2. **资源管理**: 确保在连接关闭时正确清理资源
3. **错误处理**: 在 AcceptConnection 中处理可能的错误
4. **性能考虑**: 对于高并发场景，考虑使用连接池

## 测试

```go
func TestConnectionSessionIntegration(t *testing.T) {
    ctx := context.Background()
    
    // 创建 session
    session := &protocol.ConnectionSession{}
    session.SetCtx(ctx, session.onClose)
    
    // 创建 adapter
    adapter := protocol.NewTcpAdapter(ctx, session)
    
    // 测试连接处理
    // ... 测试逻辑
}
```

## 总结

通过集成 `ConnectionSession.AcceptConnection`，TCP 和 WebSocket adapter 可以：

- 统一处理连接逻辑
- 支持复杂的业务处理
- 提供连接管理和统计
- 实现可扩展的架构

这种设计使得 adapter 专注于协议处理，而将业务逻辑委托给 ConnectionSession，实现了良好的关注点分离。 