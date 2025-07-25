# 命令体系设计文档

## 📋 概述

本文档描述了 tunnox-core 项目的命令体系设计，包括命令分类、命名规范、设计原则和实现细节。

## 🎯 设计原则

### 1. **命名一致性**
- 使用 `snake_case` 命名风格
- 动词 + 名词的命名模式
- 语义清晰，避免歧义

### 2. **分类明确**
- 按功能域进行分类
- 每个分类有明确的职责边界
- 便于扩展和维护

### 3. **方向设计**
- `Oneway`: 单向命令，不需要响应
- `Duplex`: 双向命令，需要响应结果

### 4. **向后兼容**
- 保留原有命令ID
- 提供兼容性映射
- 平滑升级路径

## 📊 命令分类体系

### 🔗 连接管理类命令 (CategoryConnection)

| 命令ID | 命令名称 | 方向 | 描述 |
|--------|----------|------|------|
| 1 | `connect` | Duplex | 建立连接 |
| 8 | `disconnect` | Oneway | 断开连接 |
| 10 | `reconnect` | Duplex | 重新连接 |
| 11 | `heartbeat` | Oneway | 心跳保活 |

**设计说明：**
- 连接生命周期管理
- 心跳机制保证连接活跃
- 支持连接重试和恢复

### 🗺️ 端口映射类命令 (CategoryMapping)

#### TCP 映射系列
| 命令ID | 命令名称 | 方向 | 描述 |
|--------|----------|------|------|
| 2 | `tcp_map_create` | Duplex | 创建TCP端口映射 |
| 12 | `tcp_map_delete` | Duplex | 删除TCP端口映射 |
| 13 | `tcp_map_update` | Duplex | 更新TCP端口映射 |
| 14 | `tcp_map_list` | Duplex | 列出TCP端口映射 |
| 15 | `tcp_map_status` | Duplex | 获取TCP端口映射状态 |

#### HTTP 映射系列
| 命令ID | 命令名称 | 方向 | 描述 |
|--------|----------|------|------|
| 3 | `http_map_create` | Duplex | 创建HTTP端口映射 |
| 16 | `http_map_delete` | Duplex | 删除HTTP端口映射 |
| 17 | `http_map_update` | Duplex | 更新HTTP端口映射 |
| 18 | `http_map_list` | Duplex | 列出HTTP端口映射 |
| 19 | `http_map_status` | Duplex | 获取HTTP端口映射状态 |

#### SOCKS 映射系列
| 命令ID | 命令名称 | 方向 | 描述 |
|--------|----------|------|------|
| 4 | `socks_map_create` | Duplex | 创建SOCKS代理映射 |
| 20 | `socks_map_delete` | Duplex | 删除SOCKS代理映射 |
| 21 | `socks_map_update` | Duplex | 更新SOCKS代理映射 |
| 22 | `socks_map_list` | Duplex | 列出SOCKS代理映射 |
| 23 | `socks_map_status` | Duplex | 获取SOCKS代理映射状态 |

**设计说明：**
- 完整的 CRUD 操作支持
- 状态查询和监控
- 统一的映射管理接口

### 📡 数据传输类命令 (CategoryTransport)

| 命令ID | 命令名称 | 方向 | 描述 |
|--------|----------|------|------|
| 5 | `data_transfer_start` | Duplex | 开始数据传输 |
| 24 | `data_transfer_stop` | Oneway | 停止数据传输 |
| 25 | `data_transfer_status` | Duplex | 获取数据传输状态 |
| 6 | `proxy_forward` | Oneway | 代理转发数据 |

**设计说明：**
- 数据传输生命周期管理
- 实时状态监控
- 高效的数据转发

### ⚙️ 系统管理类命令 (CategoryManagement)

| 命令ID | 命令名称 | 方向 | 描述 |
|--------|----------|------|------|
| 26 | `config_get` | Duplex | 获取配置信息 |
| 27 | `config_set` | Duplex | 设置配置信息 |
| 28 | `stats_get` | Duplex | 获取统计信息 |
| 29 | `log_get` | Duplex | 获取日志信息 |
| 30 | `health_check` | Duplex | 健康检查 |

**设计说明：**
- 系统配置管理
- 性能监控和统计
- 运维支持功能

### 🔄 RPC类命令 (CategoryRPC)

| 命令ID | 命令名称 | 方向 | 描述 |
|--------|----------|------|------|
| 9 | `rpc_invoke` | Duplex | RPC调用 |
| 31 | `rpc_register` | Duplex | 注册RPC服务 |
| 32 | `rpc_unregister` | Duplex | 注销RPC服务 |
| 33 | `rpc_list` | Duplex | 列出RPC服务 |

**设计说明：**
- 微服务架构支持
- 动态服务注册
- 服务发现机制

## 🔄 兼容性设计

### 原有命令映射
为了保持向后兼容，保留了原有的命令ID：

| 原命令ID | 原命令名称 | 新命令ID | 新命令名称 | 说明 |
|----------|------------|----------|------------|------|
| 2 | `TcpMap` | 2 | `tcp_map_create` | 兼容性映射 |
| 3 | `HttpMap` | 3 | `http_map_create` | 兼容性映射 |
| 4 | `SocksMap` | 4 | `socks_map_create` | 兼容性映射 |
| 5 | `DataIn` | 5 | `data_transfer_start` | 兼容性映射 |
| 6 | `Forward` | 6 | `proxy_forward` | 兼容性映射 |
| 7 | `DataOut` | 7 | `data_transfer_stop` | 兼容性映射 |
| 8 | `Disconnect` | 8 | `disconnect` | 兼容性映射 |
| 9 | `RpcInvoke` | 9 | `rpc_invoke` | 兼容性映射 |

## 🏗️ 实现架构

### 命令处理器结构
```go
type CommandHandler interface {
    Handle(ctx *CommandContext) (*CommandResponse, error)
    GetResponseType() ResponseType
    GetCommandType() packet.CommandType
    GetCategory() CommandCategory
    GetDirection() CommandDirection
}
```

### 中间件支持
- 认证中间件
- 日志中间件
- 性能监控中间件
- 错误处理中间件

### 响应格式
```json
{
    "success": true,
    "data": "JSON字符串",
    "error": "错误信息",
    "request_id": "请求ID",
    "command_id": "命令ID",
    "processing_time": "处理时间",
    "handler_name": "处理器名称"
}
```

## 🚀 扩展指南

### 添加新命令
1. 在 `packet.go` 中定义新的 `CommandType`
2. 在 `types.go` 中创建 `CommandType` 实例
3. 实现对应的 `CommandHandler`
4. 注册到 `CommandRegistry`
5. 添加测试用例

### 添加新分类
1. 在 `CommandCategory` 枚举中添加新分类
2. 更新分类的字符串表示
3. 实现分类相关的工具函数

## 📈 性能考虑

### 命令ID分配
- 使用 byte 类型，支持 0-255 个命令
- 按分类连续分配，便于管理
- 预留扩展空间

### 响应优化
- 使用 JSON 字符串避免数据丢失
- 支持压缩和加密
- 异步处理支持

## 🔒 安全考虑

### 认证机制
- 基于 Token 的认证
- 支持 JWT 令牌
- 权限控制

### 数据安全
- 传输加密
- 敏感信息脱敏
- 审计日志

## 📝 总结

新的命令体系设计具有以下优势：

1. **完整性** - 覆盖了内网穿透的所有功能需求
2. **一致性** - 统一的命名规范和设计模式
3. **可扩展性** - 清晰的分类和扩展机制
4. **兼容性** - 保持向后兼容，平滑升级
5. **可维护性** - 结构化的代码组织和文档

这个设计为 tunnox-core 项目提供了强大而灵活的命令处理能力，支持复杂的网络穿透场景和未来的功能扩展。 