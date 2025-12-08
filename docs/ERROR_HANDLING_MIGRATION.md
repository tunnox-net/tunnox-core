# 错误处理统一迁移指南

## 目标

统一使用 `TypedError`（`internal/core/errors/typed_error.go`）作为标准错误类型，替换所有 `fmt.Errorf`、`StandardError` 和其他错误处理方式。

## 迁移策略

### 1. 错误类型映射

- **网络错误** → `ErrorTypeNetwork`（可重试）
- **存储错误** → `ErrorTypeStorage`（可重试，需告警）
- **协议错误** → `ErrorTypeProtocol`（需告警）
- **认证错误** → `ErrorTypeAuth`（不可重试，需告警）
- **永久错误** → `ErrorTypePermanent`（不可重试）
- **临时错误** → `ErrorTypeTemporary`（可重试）
- **致命错误** → `ErrorTypeFatal`（不可重试，需告警）

### 2. 迁移规则

#### 替换 `fmt.Errorf`
```go
// 旧代码
return fmt.Errorf("failed to connect: %w", err)

// 新代码
return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to connect")
```

#### 替换 `fmt.Errorf`（无包装错误）
```go
// 旧代码
return fmt.Errorf("config is required")

// 新代码
return coreErrors.New(coreErrors.ErrorTypePermanent, "config is required")
```

#### 替换 `fmt.Errorf`（格式化）
```go
// 旧代码
return fmt.Errorf("failed to set key %s: %w", key, err)

// 新代码
return coreErrors.Wrapf(err, coreErrors.ErrorTypeStorage, "failed to set key %s", key)
```

### 3. 迁移优先级

1. **P0 - 关键路径**（已完成）：
   - `internal/core/storage/redis_storage.go` ✅
   - `internal/core/storage/remote_storage.go` ✅
   - `internal/core/storage/typed_storage.go` ✅
   - `internal/app/server/` ✅
   - `internal/api/` ✅

2. **P1 - 协议层**（已完成）：
   - `internal/protocol/adapter/` ✅（所有适配器：tcp, udp, websocket, quic, socks）
   - `internal/protocol/httppoll/` ✅（所有文件）
   - `internal/protocol/udp/` ✅（fragment_group.go, header.go）
   - `internal/protocol/session/` ✅（所有文件）
   - `internal/protocol/service.go` ✅（已完成）

3. **P2 - 客户端层**（已完成）：
   - `internal/client/` ✅（所有文件）

4. **P3 - 其他层**（已完成）：
   - `internal/cloud/services/` ✅（所有文件）
   - `internal/cloud/managers/` ✅（所有文件）
   - `internal/cloud/repos/` ✅（所有文件）
   - `internal/command/` ✅（所有文件）
   - `internal/stream/` ✅（所有文件）
   - `internal/bridge/` ✅（所有文件）
   - `internal/broker/` ✅（所有文件）
   - `internal/security/` ✅（所有文件）
   - `internal/utils/` ✅（所有文件）
   - `internal/health/` ✅（所有文件）

### 4. 导入规范

```go
import (
    coreErrors "tunnox-core/internal/core/errors"
)
```

### 5. 注意事项

- 保留 `ErrKeyNotFound` 等预定义错误（sentinel errors）
- 对于类型错误，使用 `ErrorTypePermanent`
- 对于网络/存储错误，使用 `ErrorTypeNetwork`/`ErrorTypeStorage`（可重试）
- 对于认证错误，使用 `ErrorTypeAuth`（需告警）

## 迁移进度

### 已完成 ✅
- **Core 层**：
  - `storage/`：所有文件（typed_storage.go, json_storage.go, hybrid_storage.go, factory.go, redis_storage.go, remote_storage.go）
  - `dispose/`：所有文件（resource_base.go, manager.go）
  - `node/`：所有文件（node_id_allocator.go）
  - `errors/`：所有文件（standard_errors.go 中的 HandleErrorWithCleanup）
- **App 层**：所有文件（server.go, handlers.go, config.go, storage.go, services.go, wiring.go, bridge_adapter.go, config_command_handlers.go, mapping_command_handlers.go, connection_code_commands_setup.go, connection_code_command_handlers.go）
- **API 层**：所有文件（server.go, push_config.go, connection_helpers.go, transaction.go, pprof_capture.go）
- **Protocol 层**：
  - `adapter/`：所有适配器（tcp, udp, websocket, quic, socks, adapter.go）
  - `httppoll/`：所有文件（stream_processor.go, server_stream_processor_http.go, packet_converter.go, fragment_reassembler.go, stream_processor_fragment.go）
  - `udp/`：所有文件（header.go）
  - `session/`：所有文件（tunnel_state.go 等）
- **Bridge 层**：所有文件（bridge_manager.go, connection_pool.go, multiplexed_conn.go, forward_session.go, node_pool.go）
- **Broker 层**：所有文件（memory_broker.go, factory.go, redis_broker.go）
- **Security 层**：所有文件（reconnect_token.go, session_token.go, ip_manager.go）
- **Utils 层**：所有文件（copy.go, path_expand.go, log_path.go, logger.go, buffer_pool.go, server.go, monitor.go）
- **Health 层**：所有文件（adapters.go）
- **错误工具函数**：`internal/errors/errors.go`（WrapError, WrapErrorf）

### 已完成 ✅（新增）
- **Core 层**：`storage/`（typed_storage.go, json_storage.go, hybrid_storage.go, factory.go）、`dispose/`（resource_base.go, manager.go）、`node/`（node_id_allocator.go）
- **Bridge 层**：所有文件（bridge_manager.go, connection_pool.go, multiplexed_conn.go, forward_session.go, node_pool.go）
- **Broker 层**：所有文件（memory_broker.go, factory.go, redis_broker.go）
- **Protocol 层**：所有文件（adapter/, httppoll/, udp/, session/）
- **Security 层**：所有文件（reconnect_token.go, session_token.go, ip_manager.go）
- **Utils 层**：所有文件（copy.go, path_expand.go, log_path.go, logger.go, buffer_pool.go, server.go, monitor.go）
- **Health 层**：所有文件（adapters.go）
- **错误工具函数**：`internal/core/errors/standard_errors.go`（HandleErrorWithCleanup）、`internal/errors/errors.go`（WrapError, WrapErrorf）

### 待处理 ⏳（仅测试文件和备份文件）
- **测试文件**：`internal/command/base_handler_test.go`、`internal/cloud/services/service_manager_test.go`、`internal/bridge/integration_test.go`（测试文件可忽略）
- **备份文件**：`internal/cloud/services/client_service_old.go.bak`（备份文件可忽略）

## 统计信息

- **所有非测试文件**：✅ 已完成
- **测试文件**：可忽略（测试文件中的 `fmt.Errorf` 不影响生产代码）
- **备份文件**：可忽略

**总计**：所有生产代码的错误处理迁移已完成 ✅

