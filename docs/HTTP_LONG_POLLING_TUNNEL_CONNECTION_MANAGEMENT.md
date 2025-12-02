# HTTP 长轮询隧道连接管理与生命周期设计

## 1. 概述

HTTP 长轮询透传时，服务端需要管理多种类型的连接和它们的生命周期。本文档详细说明连接类型、识别方式、关联关系以及资源释放策略。

## 2. 连接类型与标识

### 2.1 连接标识符

#### ConnectionID
- **作用**：唯一标识一个 HTTP 长轮询连接（物理连接）
- **格式**：`conn_` + UUID 前8位（如 `conn_3ec5c8ab`）
- **生命周期**：从连接创建到连接关闭
- **作用域**：单个 Server 节点内唯一

#### TunnelID
- **作用**：唯一标识一个逻辑隧道（透传会话）
- **格式**：`{protocol}-tunnel-{timestamp}-{port}`（如 `tcp-tunnel-1764658937066215000-7788`）
- **生命周期**：从隧道创建到隧道关闭
- **作用域**：全局唯一（跨 Server 节点）

#### MappingID
- **作用**：标识端口映射配置
- **格式**：`pmap_` + 随机字符串（如 `pmap_wMzyflXe`）
- **生命周期**：配置存在期间
- **作用域**：全局唯一

### 2.2 TunnelType（连接类型）

在 `TunnelPackage` 中通过 `tunnel_type` 字段标识：

```go
type TunnelPackage struct {
    ConnectionID string `json:"connection_id"`  // 物理连接标识
    TunnelType   string `json:"tunnel_type"`    // "control" | "data" | "keepalive"
    MappingID    string `json:"mapping_id"`     // 映射ID（data 连接才有）
    // ...
}
```

#### "control" - 控制连接
- **用途**：握手、命令、控制包传输
- **特点**：
  - 每个客户端只有一个控制连接
  - 长期保持，用于接收 Server 推送的命令
  - 不包含 `mappingID`
- **生命周期**：客户端连接期间一直存在

#### "data" - 数据连接
- **用途**：隧道数据传输
- **特点**：
  - 每个隧道会话创建一个新的数据连接
  - 包含 `mappingID` 和 `TunnelID`
  - 透传建立后切换到流模式
- **生命周期**：隧道会话期间存在

#### "keepalive" - 保持连接
- **用途**：维持连接并接收服务端响应
- **特点**：
  - 仅用于维持连接，不传输实际数据
  - 可以复用控制连接或数据连接
- **生命周期**：连接期间定期发送

## 3. 数据通道类型与识别

### 3.1 数据通道分类

#### 类型 A：用户 → Listen Client（本地连接）
- **描述**：用户应用程序连接到 Listen Client 的本地端口
- **管理位置**：Listen Client 本地管理
- **标识**：本地 `net.Conn`，不需要 Server 管理
- **生命周期**：用户连接期间

#### 类型 B：Listen Client → Server（源端数据连接）
- **描述**：Listen Client 创建的数据连接，用于发送用户数据到 Server
- **管理位置**：Server 端管理
- **标识**：
  - `ConnectionID`：唯一标识该连接
  - `TunnelID`：标识所属隧道
  - `MappingID`：标识端口映射
  - `ClientID`：Listen Client 的 ID
  - `TunnelType`：`"data"`
- **识别方式**：
  ```go
  // Server 端识别逻辑
  mapping, _ := cloudControl.GetPortMapping(mappingID)
  isSourceClient := (connClientID == mapping.ListenClientID)
  if isSourceClient {
      // 这是源端数据连接
      bridge.SetSourceConnection(nil, conn.Stream)
  }
  ```
- **生命周期**：隧道会话期间

#### 类型 C：Target Client → Target（目标服务连接）
- **描述**：Target Client 连接到目标服务（如 MySQL）
- **管理位置**：Target Client 本地管理
- **标识**：本地 `net.Conn`，不需要 Server 管理
- **生命周期**：目标服务连接期间

#### 类型 D：Target Client → Server（目标端数据连接）
- **描述**：Target Client 创建的数据连接，用于接收 Server 数据并转发到目标服务
- **管理位置**：Server 端管理
- **标识**：
  - `ConnectionID`：唯一标识该连接
  - `TunnelID`：标识所属隧道
  - `MappingID`：标识端口映射
  - `ClientID`：Target Client 的 ID
  - `TunnelType`：`"data"`
- **识别方式**：
  ```go
  // Server 端识别逻辑
  mapping, _ := cloudControl.GetPortMapping(mappingID)
  isSourceClient := (connClientID == mapping.ListenClientID)
  if !isSourceClient {
      // 这是目标端数据连接
      bridge.SetTargetConnection(nil, conn.Stream)
  }
  ```
- **生命周期**：隧道会话期间

### 3.2 连接匹配逻辑

#### 创建新隧道（源端连接先到）
1. Listen Client 创建数据连接，发送 `TunnelOpen` 请求
2. Server 识别为源端连接（通过 `clientID == mapping.ListenClientID`）
3. Server 创建 `TunnelBridge`，设置 `sourceConn`
4. Server 通知 Target Client 打开隧道（通过控制连接）

#### 连接已有隧道（目标端连接后到）
1. Target Client 创建数据连接，发送 `TunnelOpen` 请求
2. Server 识别为目标端连接（通过 `clientID == mapping.TargetClientID`）
3. Server 找到已存在的 `TunnelBridge`，设置 `targetConn`
4. 隧道桥接开始工作

#### 连接识别代码
```go
// internal/protocol/session/packet_handler.go
func (s *SessionManager) handleTunnelOpen(connPacket *types.StreamPacket) error {
    // ... 解析请求 ...
    
    // 检查是否已有 bridge
    bridge, exists := s.tunnelBridges[req.TunnelID]
    
    if exists {
        // 已有 bridge，判断是源端还是目标端连接
        var isSourceClient bool
        if s.cloudControl != nil && req.MappingID != "" {
            mapping, _ := s.cloudControl.GetPortMapping(req.MappingID)
            listenClientID := mapping.ListenClientID
            if listenClientID == 0 {
                listenClientID = mapping.SourceClientID
            }
            
            // 从连接中获取 clientID
            var connClientID int64
            if conn.Stream != nil {
                reader := conn.Stream.GetReader()
                if clientIDConn, ok := reader.(interface{ GetClientID() int64 }); ok {
                    connClientID = clientIDConn.GetClientID()
                }
            }
            
            isSourceClient = (connClientID == listenClientID)
        }
        
        if isSourceClient {
            // 源端连接（Listen Client → Server）
            bridge.SetSourceConnection(netConn, conn.Stream)
        } else {
            // 目标端连接（Target Client → Server）
            bridge.SetTargetConnection(netConn, conn.Stream)
        }
    } else {
        // 新隧道，创建 bridge
        // 识别为源端连接
        s.startSourceBridge(req, sourceConn, sourceStream)
    }
}
```

## 4. Server 到 Server 桥接

### 4.1 跨节点隧道场景

当 Listen Client 和 Target Client 连接到不同的 Server 节点时，需要 Server 到 Server 桥接：

```
User → Listen Client (Node A) → Server A → Server B → Target Client (Node B) → MySQL
```

### 4.2 TunnelID 与桥接关联

#### 桥接会话标识
- **StreamID**：gRPC 桥接流标识（在 `ForwardSession` 中）
- **TunnelID**：逻辑隧道标识（透传会话标识）
- **关联关系**：一个 `TunnelID` 对应一个 `ForwardSession`

#### 桥接管理结构
```go
// internal/bridge/forward_session.go
type ForwardSession struct {
    streamID     string              // gRPC 流标识
    conn         MultiplexedConn    // 多路复用连接
    metadata     *SessionMetadata    // 会话元数据
    // ...
}

type SessionMetadata struct {
    SourceClientID int64  // 源客户端ID
    TargetClientID int64  // 目标客户端ID
    TargetHost     string // 目标主机
    TargetPort     int    // 目标端口
    SourceNodeID   string // 源节点ID
    TargetNodeID   string // 目标节点ID
    RequestID      string // 请求ID
}
```

#### 桥接建立流程
1. **Server A（源节点）**：
   - 收到 Listen Client 的 `TunnelOpen` 请求
   - 创建 `TunnelBridge`
   - 发现 Target Client 在 Server B
   - 通过 `BridgeManager` 创建到 Server B 的桥接会话
   - 将 `TunnelID` 与 `ForwardSession` 关联

2. **Server B（目标节点）**：
   - 收到 Target Client 的 `TunnelOpen` 请求
   - 通过 `BridgeManager` 找到对应的桥接会话
   - 创建 `TunnelBridge`，连接到桥接会话
   - 将 `TunnelID` 与 `ForwardSession` 关联

#### 桥接关联代码
```go
// internal/protocol/session/server_bridge.go
func (s *SessionManager) startSourceBridge(req *packet.TunnelOpenRequest, 
    sourceConn net.Conn, sourceStream stream.PackageStreamer) error {
    
    // 创建 TunnelBridge
    bridge := NewTunnelBridge(s.Ctx(), &TunnelBridgeConfig{
        TunnelID:       req.TunnelID,
        MappingID:      req.MappingID,
        SourceConn:     sourceConn,
        SourceStream:   sourceStream,
        // ...
    })
    
    // 注册到路由表（用于跨服务器隧道）
    if s.tunnelRouting != nil {
        listenClientID := mapping.ListenClientID
        if listenClientID == 0 {
            listenClientID = mapping.SourceClientID
        }
        
        // 注册隧道路由：TunnelID -> (SourceNodeID, TargetNodeID)
        s.tunnelRouting.RegisterTunnel(req.TunnelID, &TunnelRoute{
            SourceNodeID: s.getNodeID(),
            TargetNodeID: "", // 待确定
            ListenClientID: listenClientID,
            TargetClientID: mapping.TargetClientID,
        })
    }
    
    // 如果 Target Client 在其他节点，创建桥接
    if targetNodeID != s.getNodeID() && s.bridgeManager != nil {
        // 创建桥接会话
        session, err := s.bridgeManager.CreateSession(targetNodeID, metadata)
        // 将 TunnelID 与桥接会话关联
        bridge.SetBridgeSession(session)
    }
}
```

## 5. 生命周期管理

### 5.1 连接生命周期

#### 控制连接（control）
- **创建**：客户端首次连接 Server
- **保持**：客户端运行期间一直保持
- **释放**：客户端断开或 Server 关闭

#### 数据连接（data）
- **创建**：客户端发送 `TunnelOpen` 请求时
- **保持**：隧道会话期间
- **释放**：
  - 隧道关闭时
  - 连接异常断开时
  - 超时无活动时

### 5.2 隧道生命周期

#### 隧道创建
1. Listen Client 创建数据连接，发送 `TunnelOpen`
2. Server 创建 `TunnelBridge`，设置 `sourceConn`
3. Server 通知 Target Client 打开隧道
4. Target Client 创建数据连接，发送 `TunnelOpen`
5. Server 找到 `TunnelBridge`，设置 `targetConn`
6. 隧道桥接开始工作

#### 隧道运行
- 数据双向转发：`sourceConn` ↔ `targetConn`
- 流量统计和限速
- 心跳检测（通过控制连接）

#### 隧道关闭
**触发条件**：
1. 用户连接关闭（Listen Client 检测到）
2. 目标服务连接关闭（Target Client 检测到）
3. 超时无活动
4. 错误导致连接断开

**关闭流程**：
```go
// internal/protocol/session/tunnel_bridge.go
func (b *TunnelBridge) Close() error {
    // 1. 停止数据转发
    b.cancel()
    
    // 2. 关闭源端连接
    if b.sourceForwarder != nil {
        b.sourceForwarder.Close()
    }
    
    // 3. 关闭目标端连接
    if b.targetForwarder != nil {
        b.targetForwarder.Close()
    }
    
    // 4. 关闭桥接会话（如果存在）
    if b.bridgeSession != nil {
        b.bridgeSession.Close()
    }
    
    // 5. 清理资源
    b.cleanup()
    
    return nil
}
```

### 5.3 资源释放时机

#### ConnectionID 释放
- **时机**：连接关闭时
- **位置**：`connMap` 中删除
- **代码**：
  ```go
  // internal/protocol/session/packet_handler.go
  if !shouldKeep && req.MappingID != "" {
      s.connLock.Lock()
      delete(s.connMap, connPacket.ConnectionID)
      s.connLock.Unlock()
  }
  ```

#### TunnelID 释放
- **时机**：隧道关闭时
- **位置**：`tunnelBridges` 中删除
- **代码**：
  ```go
  // internal/protocol/session/tunnel_bridge.go
  func (b *TunnelBridge) Close() error {
      // ... 关闭连接 ...
      
      // 从 tunnelBridges 中删除
      s.bridgeLock.Lock()
      delete(s.tunnelBridges, b.tunnelID)
      s.bridgeLock.Unlock()
      
      return nil
  }
  ```

#### 桥接会话释放
- **时机**：隧道关闭时
- **位置**：`BridgeManager` 中删除
- **代码**：
  ```go
  // internal/bridge/forward_session.go
  func (s *ForwardSession) Close() error {
      // 关闭 gRPC 流
      s.stream.CloseSend()
      
      // 从 MultiplexedConn 中注销
      s.conn.UnregisterSession(s.streamID)
      
      return nil
  }
  ```

### 5.4 超时与清理

#### 连接超时
- **控制连接**：无活动超时（如 5 分钟）
- **数据连接**：无活动超时（如 30 秒）

#### 隧道超时
- **无活动超时**：隧道无数据传输超过阈值（如 60 秒）
- **清理策略**：自动关闭隧道并释放资源

#### 定期清理
```go
// 定期清理超时的隧道和连接
func (s *SessionManager) cleanupExpiredResources() {
    now := time.Now()
    
    // 清理超时的隧道
    s.bridgeLock.Lock()
    for tunnelID, bridge := range s.tunnelBridges {
        if now.Sub(bridge.lastActiveAt) > tunnelTimeout {
            bridge.Close()
            delete(s.tunnelBridges, tunnelID)
        }
    }
    s.bridgeLock.Unlock()
    
    // 清理超时的连接
    s.connLock.Lock()
    for connID, conn := range s.connMap {
        if now.Sub(conn.lastActiveAt) > connTimeout {
            conn.Close()
            delete(s.connMap, connID)
        }
    }
    s.connLock.Unlock()
}
```

## 6. 数据结构设计

### 6.1 Server 端连接管理

```go
// internal/protocol/session/manager.go
type SessionManager struct {
    // 连接管理
    connMap          map[string]*types.Connection  // ConnectionID -> Connection
    connLock         sync.RWMutex
    
    // 隧道管理
    tunnelBridges    map[string]*TunnelBridge      // TunnelID -> TunnelBridge
    bridgeLock       sync.RWMutex
    
    // 控制连接管理
    controlConnMap   map[string]ControlConnectionInterface  // ConnectionID -> ControlConnection
    clientIDIndexMap map[int64]ControlConnectionInfo      // ClientID -> ControlConnectionInfo
    controlConnLock  sync.RWMutex
    
    // 桥接管理
    bridgeManager    *bridge.BridgeManager
    
    // 路由管理
    tunnelRouting    TunnelRoutingInterface
}
```

### 6.2 TunnelBridge 结构

```go
// internal/protocol/session/tunnel_bridge.go
type TunnelBridge struct {
    tunnelID        string
    mappingID       string
    
    // 源端连接（Listen Client → Server）
    sourceConn      net.Conn
    sourceStream    stream.PackageStreamer
    sourceForwarder DataForwarder
    
    // 目标端连接（Target Client → Server）
    targetConn      net.Conn
    targetStream    stream.PackageStreamer
    targetForwarder DataForwarder
    
    // 桥接会话（跨节点时使用）
    bridgeSession   *bridge.ForwardSession
    
    // 统计和限速
    bytesSent       atomic.Int64
    bytesReceived   atomic.Int64
    rateLimiter     *rate.Limiter
    
    // 生命周期
    lastActiveAt    time.Time
    createdAt       time.Time
}
```

## 7. 总结

### 7.1 关键点

1. **ConnectionID**：标识物理连接，Server 端管理
2. **TunnelID**：标识逻辑隧道，全局唯一
3. **连接识别**：通过 `clientID` 和 `mappingID` 判断是源端还是目标端
4. **桥接关联**：跨节点时，`TunnelID` 与 `ForwardSession` 关联
5. **生命周期**：连接、隧道、桥接会话都有明确的创建和释放时机

### 7.2 资源管理原则

1. **谁创建谁释放**：连接创建方负责释放
2. **级联释放**：隧道关闭时，自动关闭相关连接和桥接会话
3. **超时清理**：定期清理超时的资源
4. **错误恢复**：连接异常时，自动清理相关资源

### 7.3 待实现功能

1. **连接超时检测**：实现连接无活动超时
2. **隧道超时检测**：实现隧道无活动超时
3. **定期清理任务**：实现定期清理超时资源
4. **桥接会话管理**：完善跨节点桥接的会话管理

