# CommandUtils 改进文档

## 概述

为了支持新的命令系统，我们对 `CommandUtils` 进行了重大改进，添加了所有新命令类型的便捷创建方法。

## 新增的命令辅助方法

### 1. 连接管理类命令

```go
// 建立连接
utils.Connect()

// 重新连接
utils.Reconnect()

// 断开连接
utils.Disconnect()

// 心跳保活
utils.Heartbeat()
```

### 2. 端口映射类命令

#### TCP映射相关命令
```go
// 创建TCP端口映射
utils.TcpMapCreate()

// 删除TCP端口映射
utils.TcpMapDelete()

// 更新TCP端口映射
utils.TcpMapUpdate()

// 列出TCP端口映射
utils.TcpMapList()

// 获取TCP端口映射状态
utils.TcpMapStatus()
```

#### HTTP映射相关命令
```go
// 创建HTTP端口映射
utils.HttpMapCreate()

// 删除HTTP端口映射
utils.HttpMapDelete()

// 更新HTTP端口映射
utils.HttpMapUpdate()

// 列出HTTP端口映射
utils.HttpMapList()

// 获取HTTP端口映射状态
utils.HttpMapStatus()
```

#### SOCKS映射相关命令
```go
// 创建SOCKS代理映射
utils.SocksMapCreate()

// 删除SOCKS代理映射
utils.SocksMapDelete()

// 更新SOCKS代理映射
utils.SocksMapUpdate()

// 列出SOCKS代理映射
utils.SocksMapList()

// 获取SOCKS代理映射状态
utils.SocksMapStatus()
```

### 3. 数据传输类命令

```go
// 开始数据传输
utils.DataTransferStart()

// 停止数据传输
utils.DataTransferStop()

// 获取数据传输状态
utils.DataTransferStatus()

// 代理转发数据
utils.ProxyForward()
```

### 4. 系统管理类命令

```go
// 获取配置信息
utils.ConfigGet()

// 设置配置信息
utils.ConfigSet()

// 获取统计信息
utils.StatsGet()

// 获取日志信息
utils.LogGet()

// 健康检查
utils.HealthCheck()
```

### 5. RPC类命令

```go
// RPC调用
utils.RpcInvoke()

// 注册RPC服务
utils.RpcRegister()

// 注销RPC服务
utils.RpcUnregister()

// 列出RPC服务
utils.RpcList()
```

### 6. 兼容性命令（保留原有方法）

```go
// 兼容性：TCP端口映射
utils.TcpMap()

// 兼容性：HTTP端口映射
utils.HttpMap()

// 兼容性：SOCKS代理映射
utils.SocksMap()

// 兼容性：数据输入通知
utils.DataIn()

// 兼容性：服务端间转发
utils.Forward()

// 兼容性：数据输出通知
utils.DataOut()
```

## 使用示例

### 基本用法

```go
session := &YourSession{}
utils := command.NewCommandUtils(session)

// 创建TCP映射命令
tcpCmd := utils.TcpMapCreate().
    PutRequest(map[string]interface{}{
        "local_port":  8080,
        "remote_port": 80,
        "protocol":    "tcp",
    }).
    Timeout(30 * time.Second).
    WithAuthentication(true).
    WithUserID("user123")

// 执行命令
response, err := tcpCmd.Execute()
```

### 链式调用示例

```go
// 复杂的链式调用
complexCmd := utils.
    TcpMapCreate().
    PutRequest(map[string]interface{}{
        "local_port":  9090,
        "remote_host": "localhost",
        "remote_port": 3000,
        "description": "Web服务映射",
    }).
    Timeout(30 * time.Second).
    WithAuthentication(true).
    WithUserID("admin").
    WithStartTime(time.Now()).
    WithEndTime(time.Now().Add(24 * time.Hour))

response, err := complexCmd.Execute()
```

### 连接管理示例

```go
// 建立连接
connectCmd := utils.Connect().
    PutRequest(map[string]interface{}{
        "client_id": "client_123",
        "auth_code": "auth_456",
    }).
    Timeout(10 * time.Second).
    WithAuthentication(true).
    WithUserID("user_789")

// 心跳保活
heartbeatCmd := utils.Heartbeat().
    PutRequest(map[string]interface{}{
        "timestamp": time.Now().Unix(),
    }).
    Timeout(5 * time.Second)
```

### 系统管理示例

```go
// 获取配置
configCmd := utils.ConfigGet().
    PutRequest(map[string]interface{}{
        "key": "server.port",
    }).
    Timeout(10 * time.Second)

// 健康检查
healthCmd := utils.HealthCheck().
    PutRequest(map[string]interface{}{
        "check_connections": true,
        "check_memory":      true,
    }).
    Timeout(5 * time.Second)
```

### RPC示例

```go
// 注册RPC服务
rpcCmd := utils.RpcRegister().
    PutRequest(map[string]interface{}{
        "service_name": "calculator",
        "methods": []string{
            "add",
            "subtract",
            "multiply",
            "divide",
        },
    }).
    Timeout(30 * time.Second)
```

## 改进特点

### 1. 完整性
- 覆盖了所有新定义的命令类型
- 按功能分类组织，便于查找和使用

### 2. 一致性
- 所有方法都遵循相同的命名规范
- 支持链式调用，提供流畅的API

### 3. 向后兼容
- 保留了原有的命令方法
- 确保现有代码不会受到影响

### 4. 类型安全
- 所有方法都返回 `*CommandUtils`，支持链式调用
- 与现有的 `PutRequest`、`Timeout` 等方法完全兼容

### 5. 易于扩展
- 新增命令类型时，只需添加对应的方法
- 遵循统一的模式，便于维护

## 测试覆盖

我们为所有新的命令辅助方法添加了完整的测试：

- `TestCommandUtils_NewCommands`: 测试所有新命令方法
- `TestCommandUtils_Chaining`: 测试链式调用功能

所有测试都通过，确保功能的正确性。

## 总结

这次改进大大增强了 `CommandUtils` 的功能性和易用性：

1. **完整性**: 支持所有新定义的命令类型
2. **易用性**: 提供直观的API，支持链式调用
3. **兼容性**: 保持向后兼容，不影响现有代码
4. **可维护性**: 统一的模式，便于扩展和维护

这些改进使得开发者能够更轻松地使用新的命令系统，提高了开发效率和代码质量。 