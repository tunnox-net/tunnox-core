# Session 接口使用示例

本文档展示了如何使用重构后的 `Session` 接口，该接口提供了统一的连接管理和数据包处理能力。

## 新的架构设计

### Session 接口

```go
type Session interface {
    // 初始化连接
    InitConnection(reader io.Reader, writer io.Writer) (*StreamConnectionInfo, error)
    
    // 处理带连接信息的数据包
    HandlePacket(packet *StreamPacket) error
    
    // 关闭连接
    CloseConnection(connectionId string) error
}
```

### 数据结构

```go
// 流连接信息
type StreamConnectionInfo struct {
    ID       string
    Stream   *stream.StreamProcessor
    Metadata map[string]interface{}
}

// 流数据包
type StreamPacket struct {
    ConnectionID string
    Packet       *packet.TransferPacket
    Timestamp    time.Time
}
```

## 使用示例

### 1. 创建 Session

```go
package main

import (
    "context"
    "log"
    "tunnox-core/internal/protocol"
)

func main() {
    // 创建连接会话
    session := protocol.NewConnectionSession(context.Background())
    defer session.Close()
    
    // 你的应用逻辑...
}
```

### 2. 在协议适配器中使用

#### TCP 适配器

```go
func (t *TcpAdapter) handleConn(conn net.Conn) {
    defer conn.Close()
    utils.Infof("TCP adapter handling connection from %s", conn.RemoteAddr())
    
    // 初始化连接
    connInfo, err := t.session.InitConnection(conn, conn)
    if err != nil {
        utils.Errorf("Failed to initialize connection: %v", err)
        return
    }
    defer t.session.CloseConnection(connInfo.ID)
    
    // 处理数据流
    for {
        packet, err := connInfo.Stream.ReadPacket()
        if err != nil {
            if err == io.EOF {
                utils.Infof("Connection closed by peer: %s", connInfo.ID)
            } else {
                utils.Errorf("Failed to read packet: %v", err)
            }
            break
        }
        
        // 包装成 StreamPacket
        connPacket := &protocol.StreamPacket{
            ConnectionID: connInfo.ID,
            Packet:       packet,
            Timestamp:    time.Now(),
        }
        
        // 处理数据包
        if err := t.session.HandlePacket(connPacket); err != nil {
            utils.Errorf("Failed to handle packet: %v", err)
            break
        }
    }
}
```

#### WebSocket 适配器

```go
func (w *WebSocketAdapter) handleWebSocket(writer http.ResponseWriter, request *http.Request) {
    conn, err := w.upgrader.Upgrade(writer, request, nil)
    if err != nil {
        utils.Errorf("Failed to upgrade connection: %v", err)
        return
    }
    
    utils.Infof("WebSocket connection established from %s", conn.RemoteAddr())
    
    // 创建连接包装器
    wrapper := &WebSocketConnWrapper{conn: conn}
    
    // 初始化连接
    connInfo, err := w.session.InitConnection(wrapper, wrapper)
    if err != nil {
        utils.Errorf("Failed to initialize WebSocket connection: %v", err)
        return
    }
    defer w.session.CloseConnection(connInfo.ID)
    
    // 处理数据流
    for {
        packet, err := connInfo.Stream.ReadPacket()
        if err != nil {
            break
        }
        
        // 包装成 StreamPacket
        connPacket := &protocol.StreamPacket{
            ConnectionID: connInfo.ID,
            Packet:       packet,
            Timestamp:    time.Now(),
        }
        
        // 处理数据包
        if err := w.session.HandlePacket(connPacket); err != nil {
            utils.Errorf("Failed to handle WebSocket packet: %v", err)
            break
        }
    }
}
```

### 3. 创建不同类型的 Session

#### 服务端 Session

```go
// 服务端 Session - 处理客户端连接
type ServerSession struct {
    *protocol.ConnectionSession
    cloudApi managers.CloudControlAPI
}

func NewServerSession(cloudApi managers.CloudControlAPI) *ServerSession {
    return &ServerSession{
        ConnectionSession: protocol.NewConnectionSession(context.Background()),
        cloudApi:          cloudApi,
    }
}

// 重写 HandlePacket 方法，添加服务端特有的业务逻辑
func (s *ServerSession) HandlePacket(connPacket *protocol.StreamPacket) error {
    // 服务端特有的处理逻辑
    // 例如：认证、权限检查、业务处理等
    
    // 调用基类方法
    return s.ConnectionSession.HandlePacket(connPacket)
}
```

#### 客户端 Session

```go
// 客户端 Session - 处理服务端连接
type ClientSession struct {
    *protocol.ConnectionSession
    config *ClientConfig
}

func NewClientSession(config *ClientConfig) *ClientSession {
    return &ClientSession{
        ConnectionSession: protocol.NewConnectionSession(context.Background()),
        config:            config,
    }
}

// 重写 HandlePacket 方法，添加客户端特有的业务逻辑
func (s *ClientSession) HandlePacket(connPacket *protocol.StreamPacket) error {
    // 客户端特有的处理逻辑
    // 例如：响应服务端命令、本地处理等
    
    // 调用基类方法
    return s.ConnectionSession.HandlePacket(connPacket)
}
```

#### 转发 Session

```go
// 转发 Session - 处理服务端间转发
type ForwardSession struct {
    *protocol.ConnectionSession
    nodeManager NodeManager
}

func NewForwardSession(nodeManager NodeManager) *ForwardSession {
    return &ForwardSession{
        ConnectionSession: protocol.NewConnectionSession(context.Background()),
        nodeManager:       nodeManager,
    }
}

// 重写 HandlePacket 方法，添加转发特有的业务逻辑
func (s *ForwardSession) HandlePacket(connPacket *protocol.StreamPacket) error {
    // 转发特有的处理逻辑
    // 例如：跨节点转发、负载均衡等
    
    // 调用基类方法
    return s.ConnectionSession.HandlePacket(connPacket)
}
```

### 4. 监控和调试

```go
// 获取连接信息
connInfo, exists := session.GetStreamConnectionInfo("conn_1234567890_1")
if exists {
    log.Printf("Connection %s has %d metadata items", 
        connInfo.ID, len(connInfo.Metadata))
}

// 获取活跃连接数量
activeCount := session.GetActiveConnections()
log.Printf("Active connections: %d", activeCount)
```

## 架构优势

### 1. 统一接口
- 所有协议适配器使用相同的 Session 接口
- 连接管理逻辑统一
- 数据包处理流程一致

### 2. 职责分离
- Session 负责连接管理和数据包分发
- 具体业务逻辑由不同的 Session 实现处理
- 协议适配器只负责协议层面的处理

### 3. 扩展性好
- 可以轻松创建新的 Session 类型
- 支持不同的业务场景
- 便于添加新的功能

### 4. 类型安全
- 通过 StreamPacket 确保连接信息的存在
- 编译时检查接口实现
- 减少运行时错误

## 迁移指南

### 从旧版本迁移

1. **替换 AcceptConnection 调用**
   ```go
   // 旧版本
   session.AcceptConnection(conn, conn)
   
   // 新版本
   connInfo, err := session.InitConnection(conn, conn)
   if err != nil {
       return
   }
   defer session.CloseConnection(connInfo.ID)
   
   // 处理数据流...
   ```

2. **更新数据包处理**
   ```go
   // 旧版本
   session.processPacket(packet, stream, connID)
   
   // 新版本
   connPacket := &protocol.StreamPacket{
       ConnectionID: connInfo.ID,
       Packet:       packet,
       Timestamp:    time.Now(),
   }
   session.HandlePacket(connPacket)
   ```

3. **更新连接管理**
   ```go
   // 旧版本：自动管理
   // 新版本：显式管理
   connInfo, err := session.InitConnection(reader, writer)
   defer session.CloseConnection(connInfo.ID)
   ```

这个新的架构设计提供了更好的灵活性、可维护性和扩展性，同时保持了接口的简洁性。 