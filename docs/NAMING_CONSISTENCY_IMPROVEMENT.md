# 命名一致性改进设计

**创建时间**: 2025-01-XX  
**目标**: 统一代码库中的命名规范，提高代码可读性和一致性

---

## 1. 命名规范方案

### 1.1 方案选择：C# 风格命名

经过分析，采用 **C# 风格的命名方式**，原因：
- ✅ 接口和实现类清晰区分
- ✅ 默认实现和具体实现清晰区分
- ✅ 符合常见编程语言规范
- ✅ 易于理解和维护

### 1.2 命名规则

#### 接口命名
- **格式**: `I{Name}`
- **示例**: `IConnection`, `IStreamProcessor`, `IManager`
- **说明**: 所有接口使用 `I` 前缀

#### 默认实现命名
- **格式**: `Default{Name}`
- **示例**: `DefaultConnection`, `DefaultStreamProcessor`, `DefaultManager`
- **说明**: 默认实现使用 `Default` 前缀

#### 具体实现命名
- **格式**: `{Protocol/Type}{Name}`
- **示例**: `TCPConnection`, `HTTPPollStreamProcessor`, `RedisBroker`
- **说明**: 具体实现使用协议/类型前缀

#### 访问器接口命名
- **格式**: `I{Name}Accessor`
- **示例**: `IConnectionAccessor`, `IStreamProcessorAccessor`
- **说明**: 访问器接口使用 `Accessor` 后缀

---

## 2. 需要重命名的文件

### 2.1 接口文件命名规范

**决定**: 保持 `_interface.go` 后缀，符合 Go 语言常见命名习惯。

**说明**: 
- ✅ 接口文件使用 `_interface.go` 后缀是 Go 语言中的常见做法
- ✅ 清晰表达文件包含接口定义
- ✅ 与实现文件（如 `connection.go`）形成良好对比
- ❌ 不需要改为 `iconnection.go` 等格式

**当前接口文件**（保持不变）:
- `internal/protocol/session/connection_interface.go` ✅
- `internal/protocol/session/tunnel_bridge_interface.go` ✅
- `internal/client/mapping_interface.go` ✅
- `internal/stream/processor_accessor.go` ✅ (访问器接口，可考虑改为 `stream_processor_accessor_interface.go` 以保持一致性)

---

## 3. 需要重命名的接口

### 3.1 连接相关接口

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `ControlConnectionInterface` | `IControlConnection` | `internal/protocol/session/connection.go` | 统一接口命名规范 |
| `TunnelConnectionInterface` | `ITunnelConnection` | `internal/protocol/session/connection_interface.go` | 统一接口命名规范 |
| `ConnectionStateManager` | `IConnectionStateManager` | `internal/protocol/session/connection_interface.go` | 统一接口命名规范 |
| `ConnectionTimeoutManager` | `IConnectionTimeoutManager` | `internal/protocol/session/connection_interface.go` | 统一接口命名规范 |
| `ConnectionErrorHandler` | `IConnectionErrorHandler` | `internal/protocol/session/connection_interface.go` | 统一接口命名规范 |
| `ConnectionReuseStrategy` | `IConnectionReuseStrategy` | `internal/protocol/session/connection_interface.go` | 统一接口命名规范 |
| `ControlConnectionAccessor` | `IControlConnectionAccessor` | `internal/api/server.go` | 统一访问器接口命名 |
| `TunnelBridgeAccessor` | `ITunnelBridgeAccessor` | `internal/protocol/session/tunnel_bridge_interface.go` | 统一访问器接口命名 |

### 3.2 流处理相关接口

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `StreamProcessor` (接口) | `IStreamProcessor` | `internal/stream/processor/processor.go` | 统一接口命名规范 |
| `StreamProcessorAccessor` | `IStreamProcessorAccessor` | `internal/stream/processor_accessor.go` | 统一访问器接口命名 |
| `PackageStreamer` | `IPackageStreamer` | `internal/stream/interfaces.go` | 统一接口命名规范 |
| `StreamReader` | `IStreamReader` | `internal/stream/interfaces.go` | 统一接口命名规范 |
| `StreamWriter` | `IStreamWriter` | `internal/stream/interfaces.go` | 统一接口命名规范 |
| `Stream` | `IStream` | `internal/stream/interfaces.go` | 统一接口命名规范 |

### 3.3 管理器相关接口

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `SessionManager` (接口) | `ISessionManager` | `internal/api/server.go` | 统一接口命名规范 |
| `SessionManagerWithConnection` | `ISessionManagerWithConnection` | `internal/api/handlers_httppoll.go` | 统一接口命名规范 |
| `BridgeManager` | `IBridgeManager` | `internal/protocol/session/bridge_adapter.go` | 统一接口命名规范 |
| `StorageManager` | `IStorageManager` | `internal/cloud/infrastructure/storage.go` | 统一接口命名规范 |
| `NetworkManager` | `INetworkManager` | `internal/cloud/infrastructure/network.go` | 统一接口命名规范 |

### 3.4 处理器相关接口

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `CommandHandler` | `ICommandHandler` | `internal/core/types/interfaces.go` | 统一接口命名规范 |
| `MappingHandler` | `IMappingHandler` | `internal/client/mapping_interface.go` | 统一接口命名规范 |
| `MappingHandlerInterface` | `IMappingHandler` | `internal/client/mapping_interface.go` | 移除重复的 Interface 后缀 |
| `TunnelHandler` | `ITunnelHandler` | `internal/protocol/session/tunnel_handler.go` | 统一接口命名规范 |
| `AuthHandler` | `IAuthHandler` | `internal/protocol/session/tunnel_handler.go` | 统一接口命名规范 |

### 3.5 适配器相关接口

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `Adapter` | `IAdapter` | `internal/protocol/adapter/adapter.go` | 统一接口命名规范 |
| `ProtocolAdapter` | `IProtocolAdapter` | `internal/protocol/adapter/adapter.go` | 统一接口命名规范 |
| `ClientInterface` | `IClient` | `internal/client/mapping/types.go` | 统一接口命名规范 |

### 3.6 其他接口

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `MessageBroker` | `IMessageBroker` | `internal/broker/interface.go` | 统一接口命名规范 |
| `Disposable` | `IDisposable` | `internal/core/dispose/dispose.go` | 统一接口命名规范 |
| `DisposableResource` | `IDisposableResource` | `internal/core/dispose/resource_base.go` | 统一接口命名规范 |
| `ResourceInitializer` | `IResourceInitializer` | `internal/core/dispose/resource_base.go` | 统一接口命名规范 |
| `Closeable` | `ICloseable` | `internal/app/server/services.go` | 统一接口命名规范 |
| `CopyStrategy` | `ICopyStrategy` | `internal/utils/copy_strategy.go` | 统一接口命名规范 |
| `KeepAliveConn` | `IKeepAliveConn` | `internal/client/keepalive_conn.go` | 统一接口命名规范 |
| `EventBus` | `IEventBus` | `internal/core/events/events.go` | 统一接口命名规范 |
| `Storage` | `IStorage` | `internal/core/storage/interface.go` | 统一接口命名规范 |
| `DistributedLock` | `IDistributedLock` | `internal/cloud/distributed/distributed_lock.go` | 统一接口命名规范 |

---

## 4. 需要重命名的类（结构体）

### 4.1 流处理器相关类

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `StreamProcessor` (客户端) | `ClientStreamProcessor` | `internal/stream/stream_processor.go` | 明确区分客户端和服务端实现 |
| `StreamProcessor` (HTTP长轮询客户端) | `HTTPPollClientStreamProcessor` | `internal/protocol/httppoll/stream_processor.go` | 明确协议和角色 |
| `ServerStreamProcessor` | `HTTPPollServerStreamProcessor` | `internal/protocol/httppoll/server_stream_processor.go` | 明确协议和角色 |
| `DefaultStreamProcessor` | `DefaultStreamProcessor` | `internal/stream/processor/processor.go` | 保持不变（已符合规范） |

### 4.2 连接相关类

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `ControlConnection` | `ControlConnection` | `internal/protocol/session/connection.go` | 保持不变（实现类无需前缀） |
| `TunnelConnection` | `TunnelConnection` | `internal/protocol/session/connection.go` | 保持不变（实现类无需前缀） |
| `HTTPPollTunnelConnection` | `HTTPPollTunnelConnection` | `internal/protocol/session/httppoll_connection.go` | 保持不变（已符合规范） |
| `TCPTunnelConnection` | `TCPTunnelConnection` | `internal/protocol/session/tcp_connection.go` | 保持不变（已符合规范） |

### 4.3 管理器相关类

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `SessionManager` (实现类) | `SessionManager` | `internal/protocol/session/manager.go` | 保持不变（实现类无需前缀） |
| `StreamManager` | `StreamManager` | `internal/stream/manager.go` | 保持不变（实现类无需前缀） |
| `ResourceManager` | `ResourceManager` | `internal/core/dispose/manager.go` | 保持不变（实现类无需前缀） |
| `ProtocolManager` | `ProtocolManager` | `internal/protocol/manager.go` | 保持不变（实现类无需前缀） |
| `HealthManager` | `HealthManager` | `internal/health/manager.go` | 保持不变（实现类无需前缀） |
| `BridgeManager` (实现类) | `BridgeManager` | `internal/bridge/bridge_manager.go` | 保持不变（实现类无需前缀） |

### 4.4 适配器相关类

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `BaseAdapter` | `BaseAdapter` | `internal/protocol/adapter/adapter.go` | 保持不变（基类无需前缀） |
| `TCPAdapter` | `TCPAdapter` | `internal/protocol/adapter/tcp_adapter.go` | 保持不变（已符合规范） |
| `UDPAdapter` | `UDPAdapter` | `internal/protocol/adapter/udp_adapter.go` | 保持不变（已符合规范） |
| `WebSocketAdapter` | `WebSocketAdapter` | `internal/protocol/adapter/websocket_adapter.go` | 保持不变（已符合规范） |
| `QUICAdapter` | `QUICAdapter` | `internal/protocol/adapter/quic_adapter.go` | 保持不变（已符合规范） |

### 4.5 处理器相关类

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `BaseHandler` | `BaseHandler` | `internal/command/base_handler.go` | 保持不变（基类无需前缀） |
| `TcpMapHandler` | `TCPMapHandler` | `internal/command/handlers.go` | 统一大小写（TCP 全大写） |
| `HttpMapHandler` | `HTTPMapHandler` | `internal/command/handlers.go` | 统一大小写（HTTP 全大写） |
| `SocksMapHandler` | `SOCKSMapHandler` | `internal/command/handlers.go` | 统一大小写（SOCKS 全大写） |
| `DefaultHandler` | `DefaultHandler` | `internal/command/handlers.go` | 保持不变（已符合规范） |

### 4.6 存储相关类

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `MemoryStorage` | `MemoryStorage` | `internal/core/storage/memory.go` | 保持不变（已符合规范） |
| `RedisStorage` | `RedisStorage` | `internal/core/storage/redis_storage.go` | 保持不变（已符合规范） |
| `JSONStorage` | `JSONStorage` | `internal/core/storage/json_storage.go` | 保持不变（已符合规范） |
| `HybridStorage` | `HybridStorage` | `internal/core/storage/hybrid_storage.go` | 保持不变（已符合规范） |

### 4.7 代理相关类

| 当前名称 | 新名称 | 文件位置 | 原因 |
|---------|--------|---------|------|
| `MemoryBroker` | `MemoryBroker` | `internal/broker/memory_broker.go` | 保持不变（已符合规范） |
| `RedisBroker` | `RedisBroker` | `internal/broker/redis_broker.go` | 保持不变（已符合规范） |

---

## 5. 需要重命名的方法

### 5.1 接口方法命名

| 当前方法名 | 新方法名 | 接口 | 原因 |
|----------|---------|------|------|
| `GetStreamProcessor()` | `GetStreamProcessor()` | `IStreamProcessorAccessor` | 保持不变（方法名无需修改） |
| `GetControlConnectionInterface()` | `GetControlConnection()` | `ISessionManager` | 移除 Interface 后缀，返回接口类型 |
| `GetTunnelBridgeByConnectionID()` | `GetTunnelBridgeByConnectionID()` | `ISessionManager` | 保持不变（方法名无需修改） |
| `GetTunnelBridgeByMappingID()` | `GetTunnelBridgeByMappingID()` | `ISessionManager` | 保持不变（方法名无需修改） |

### 5.2 实现类方法命名

| 当前方法名 | 新方法名 | 类 | 原因 |
|----------|---------|------|------|
| `getControlConnectionByConnID()` | `getControlConnectionByConnID()` | `ManagementAPIServer` | 保持不变（私有方法无需修改） |

---

## 6. 重命名影响分析

### 6.1 高影响范围（需要大量修改）

1. **接口重命名** (约 50+ 个接口)
   - 影响所有实现类
   - 影响所有接口引用
   - 影响所有类型断言

2. **StreamProcessor 重命名** (3 个实现类)
   - `StreamProcessor` → `ClientStreamProcessor`
   - `StreamProcessor` (HTTP) → `HTTPPollClientStreamProcessor`
   - `ServerStreamProcessor` → `HTTPPollServerStreamProcessor`
   - 影响所有使用这些类的地方

### 6.2 中影响范围（需要中等修改）

1. **连接接口重命名** (8 个接口)
   - 影响连接管理相关代码
   - 影响适配器代码

2. **访问器接口重命名** (3 个接口)
   - 影响 API 层代码
   - 影响跨包访问代码

### 6.3 低影响范围（需要少量修改）

1. **文件重命名** (4 个文件)
   - 仅影响导入路径
   - 不影响功能

2. **方法重命名** (少量方法)
   - 仅影响调用方
   - 影响范围较小

---

## 7. 实施建议

### 7.1 分阶段实施

**阶段 1：接口重命名（高优先级）**
1. 重命名核心接口（连接、流处理）
2. 更新所有实现类
3. 更新所有引用

**阶段 2：实现类重命名（中优先级）**
1. 重命名 StreamProcessor 相关类
2. 更新所有使用这些类的地方

**阶段 3：文件重命名（低优先级，可选）**
1. 可选：统一访问器接口文件命名（如 `processor_accessor.go` → `stream_processor_accessor_interface.go`）
2. 其他接口文件保持 `_interface.go` 后缀不变

**阶段 4：方法重命名（低优先级）**
1. 重命名接口方法
2. 更新所有调用

### 7.2 注意事项

1. **向后兼容**
   - 考虑保留旧名称作为类型别名（带 deprecation 标记）
   - 逐步迁移，避免一次性大改

2. **测试覆盖**
   - 确保所有重命名后测试通过
   - 更新测试代码中的类型引用

3. **文档更新**
   - 更新所有相关文档
   - 更新 API 文档

4. **代码审查**
   - 每个阶段完成后进行代码审查
   - 确保命名一致性

---

## 8. 命名规范总结

### 8.1 接口命名
- ✅ 使用 `I` 前缀：`IConnection`, `IStreamProcessor`
- ✅ 访问器接口使用 `Accessor` 后缀：`IConnectionAccessor`
- ❌ 不使用 `Interface` 后缀：`ConnectionInterface` → `IConnection`
- ❌ 不使用直接命名：`Connection` (接口) → `IConnection`

### 8.2 实现类命名
- ✅ 默认实现使用 `Default` 前缀：`DefaultStreamProcessor`
- ✅ 具体实现使用协议/类型前缀：`TCPConnection`, `HTTPPollStreamProcessor`
- ✅ 客户端实现使用 `Client` 前缀：`ClientStreamProcessor`
- ✅ 服务端实现使用 `Server` 前缀：`ServerStreamProcessor`
- ❌ 不使用接口名称作为实现类名称：`StreamProcessor` (接口) vs `ClientStreamProcessor` (实现)

### 8.3 文件命名
- ✅ 接口文件使用 `_interface.go` 后缀：`connection_interface.go`, `stream_processor_interface.go`
- ✅ 实现文件使用正常命名：`connection.go`, `stream_processor.go`
- ✅ 访问器接口文件使用 `_accessor_interface.go` 后缀：`stream_processor_accessor_interface.go`
- ❌ 不使用 `i` 前缀：`iconnection.go` (不符合 Go 命名习惯)

### 8.4 方法命名
- ✅ 方法名清晰表达意图
- ✅ 避免在方法名中包含类型信息：`GetControlConnectionInterface()` → `GetControlConnection()`
- ✅ 返回接口类型时，方法名无需特殊后缀

---

## 9. 示例对比

### 9.1 接口定义示例

**当前（不一致）**:
```go
type ControlConnectionInterface interface {
    GetConnID() string
}

type TunnelConnectionInterface interface {
    GetConnectionID() string
}

type StreamProcessorAccessor interface {
    GetStreamProcessor() interface{}
}
```

**改进后（一致）**:
```go
type IControlConnection interface {
    GetConnID() string
}

type ITunnelConnection interface {
    GetConnectionID() string
}

type IStreamProcessorAccessor interface {
    GetStreamProcessor() IStreamProcessor
}
```

### 9.2 实现类示例

**当前（不一致）**:
```go
type StreamProcessor struct { // 客户端实现
    // ...
}

type StreamProcessor struct { // HTTP 长轮询客户端实现
    // ...
}

type ServerStreamProcessor struct { // 服务端实现
    // ...
}
```

**改进后（一致）**:
```go
type ClientStreamProcessor struct { // 客户端实现
    // ...
}

type HTTPPollClientStreamProcessor struct { // HTTP 长轮询客户端实现
    // ...
}

type HTTPPollServerStreamProcessor struct { // 服务端实现
    // ...
}
```

---

## 10. 总结

### 10.1 重命名统计

- **接口重命名**: 约 50+ 个
- **实现类重命名**: 约 10+ 个
- **文件重命名**: 0 个（保持 `_interface.go` 后缀）
- **方法重命名**: 少量

### 10.2 预期收益

1. **可读性提升**: 接口和实现类清晰区分
2. **一致性提升**: 统一的命名规范
3. **维护性提升**: 更容易理解和维护代码
4. **扩展性提升**: 更容易添加新实现

### 10.3 风险评估

1. **影响范围**: 大量代码需要修改
2. **测试成本**: 需要更新所有测试
3. **迁移成本**: 需要分阶段实施
4. **向后兼容**: 需要考虑兼容性

---

**文档状态**: 待实施  
**优先级**: 中（不影响功能，但提升代码质量）  
**建议实施时间**: 分阶段实施，每个阶段完成后进行测试和代码审查

