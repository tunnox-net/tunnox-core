# 代码质量改进计划

## 问题分类

### P0: 关键问题（立即修复）

#### 1. context.Background() 使用问题 ✅ 已完成

**问题描述：**
- `context.Background()` 在业务入口之外（不仅仅是 main/测试）直接使用
- 导致 goroutine 失去退出信号，无法优雅关闭

**修复方案：**
1. ✅ 扫描所有 `context.Background()` 的使用位置
2. ✅ 检查是否可以合并到 dispose 体系的子树节点
3. ✅ 确保所有 goroutine 都能接收到退出信号

**修复内容：**
- ✅ `internal/cloud/repos/generic_repository.go`: 移除 Repository 的 context.Background()，Repository 不管理自己的 context
- ✅ `internal/protocol/httppoll/stream_processor.go`: 使用 StreamProcessor 的 context
- ✅ `internal/app/server/wiring.go`: 使用 serviceManager 的 context
- ✅ `internal/api/server.go`: 使用 ManagementAPIServer 的 context
- ✅ `internal/command/executor.go`: 使用 CommandExecutor 的 context
- ✅ `internal/client/mapping/base.go`: controlledConn 添加 context 字段，使用 handler 的 context
- ✅ `internal/protocol/adapter/websocket_adapter.go`: 使用 WebSocketAdapter 的 context
- ✅ `internal/app/server/bridge_adapter.go`: BridgeAdapter 添加 context 字段，在创建时传入
- ✅ `internal/protocol/session/config_push_broadcast.go`: 使用 SessionManager 的 context

**影响范围：**
- ✅ 所有使用 `context.Background()` 的业务代码已修复
- ✅ 所有启动 goroutine 的地方已确保能接收退出信号

---

#### 2. Mutex/RWMutex 并发安全问题 ✅ 已完成

**问题描述：**
- 多处使用 Mutex/RWMutex 管理 map/状态
- 需要核对是否正确，保证在 `-race` 下没有问题

**修复方案：**
1. ✅ 扫描所有 Mutex/RWMutex 的使用
2. ✅ 使用 `go test -race` 验证并发安全
3. ✅ 修复所有 data race 问题

**已修复的问题：**
- ✅ `internal/core/storage/memory.go`: 修复了在 RLock 下执行 delete 操作的并发安全问题
  - `Get` 方法：在 RLock 下检查过期，需要删除时升级为 Lock
  - `Exists` 方法：同上
  - `GetHash` 方法：同上
  - `GetAllHash` 方法：同上
  - `GetExpiration` 方法：同上
- ✅ `internal/cloud/distributed/distributed_lock.go`: 为 MemoryLock 添加 RWMutex 保护 map 访问

**影响范围：**
- 所有使用 Mutex/RWMutex 的代码
- 所有共享状态的代码

**检查结果：**
- ✅ `internal/core/storage/json_storage.go`: 锁使用正确，save() 是只读操作
- ✅ `internal/security/brute_force_protector.go`: 使用独立的锁保护不同的 map，设计正确
- ✅ `internal/protocol/httppoll/fragment_reassembler.go`: 使用 RWMutex 保护 map，设计正确
- ✅ `internal/protocol/session/manager.go`: 使用独立的锁保护不同的 map，设计正确
- ✅ `internal/core/events/event_bus.go`: 锁使用正确
- ✅ `internal/command/registry.go`: 锁使用正确

**验证：**
- ✅ 已运行 race 检测，修复后的代码通过测试
- ✅ 关键文件的并发安全设计检查通过

---

### P1: 重要改进（1-2周内）

#### 3. 错误处理分层体系 ✅ 已完成

**问题描述：**
- 有些地方用 `fmt.Errorf("xxx: %w", err)` 自己处理
- 有些地方只 log 错误但不返回上层（或反之）
- 没有明显的"可重试/需告警/致命"分类体系

**修复方案：**

**已创建错误分层方案：**

✅ `internal/core/errors/typed_error.go`: 实现了 TypedError 和错误分层体系
- 定义了 7 种错误类型：Temporary, Permanent, Protocol, Network, Storage, Auth, Fatal
- 实现了 `Wrap`, `Wrapf`, `New`, `Newf` 函数
- 实现了 `IsRetryable`, `IsAlertable`, `GetErrorType` 判断函数
- 提供了 Sentinel errors（预定义错误实例）

✅ `internal/utils/logger.go`: 集成错误类型到日志系统
- `WithError` 方法自动提取错误类型、可重试、需告警信息
- `logErrorWithLevel` 函数根据错误类型自动选择日志级别：
  - Fatal -> Fatal 级别
  - Auth/Protocol/Storage -> Error 级别（需告警）
  - Network/Temporary -> Warn 级别（可重试）
  - Permanent -> Error 级别
- 新增 `LogError` 和 `LogErrorf` 函数，自动根据错误类型选择日志级别
- 更新了 `LogOperation`, `LogAuthentication`, `LogStorageOperation` 使用新的错误日志函数

✅ `internal/constants/log.go`: 添加错误类型相关日志字段
- `LogFieldErrorType`: 错误类型字段
- `LogFieldRetryable`: 是否可重试字段
- `LogFieldAlertable`: 是否需要告警字段

✅ `internal/core/errors/typed_error_test.go`: 完整的单元测试覆盖

**日志集成：**
- ✅ 根据错误类型自动选择日志级别（Fatal/Error/Warn）
- ✅ 自动添加错误类型、可重试、需告警字段到日志
- ✅ 方便后续统计分析和告警系统集成

**影响范围：**
- ✅ 错误处理体系已创建，可供业务代码使用
- ✅ 日志系统已集成错误类型，自动提取和记录错误属性

**业务代码迁移：**
- ✅ `internal/cloud/services/base_service.go`: 已迁移所有错误处理函数到 TypedError 系统
  - `HandleErrorWithIDRelease` / `HandleErrorWithIDReleaseInt64` / `HandleErrorWithIDReleaseString`
  - `WrapError` / `WrapErrorWithID` / `WrapErrorWithInt64ID`
  - `LogWarning` 使用新的 `utils.LogErrorf` 函数
  - 所有使用 `baseService.WrapError` 的 service 实现文件已自动使用 TypedError 系统

**迁移策略：**
- ✅ 通过迁移 `base_service.go`，所有 service 层的错误处理已自动使用 TypedError 系统
- ✅ 通过迁移 `generic_repository.go`，所有 repository 层的错误处理已统一使用 TypedError 系统
- ✅ 其他业务代码中的 `fmt.Errorf` 可以逐步迁移，或在使用 service/repository 层时自动获得 TypedError 支持
- ✅ 关键路径（service 层和 repository 层）的错误处理已统一使用 TypedError 系统

**已迁移的关键文件：**
- ✅ `internal/cloud/services/base_service.go`: Service 层基础错误处理
- ✅ `internal/cloud/repos/generic_repository.go`: Repository 层基础错误处理
  - 所有 marshal/unmarshal 错误使用 `ErrorTypeStorage`
  - 实体不存在/已存在错误使用 `ErrorTypePermanent`
  - 存储不支持操作错误使用 `ErrorTypeStorage`
  - getIDFunc 未设置错误使用 `ErrorTypeFatal`

---

### P2: 文档和可观测性（1-2个月内）

#### 4. 协议处理模块文档

**问题描述：**
- 文件分散但宏观注释不足
- 需要文档描述状态机、报文格式、分片→重组→转发→应答链路

**修复方案：**
- 创建 `internal/protocol/httppoll/README.md` 或 `design.md`
- 用文字 + 简图描述：
  - 状态机
  - 报文格式
  - 分片→重组→转发→应答的链路

**影响范围：**
- `internal/protocol/httppoll/` 目录

---

#### 5. Metrics 扩展

**问题描述：**
- 需要更细粒度的 metrics
- 每种协议：当前连接数/错误数/RTT/重传率/分片命中率
- session：活跃 session 数/恢复的 tunnel 数

**修复方案：**
- 扩展现有的 metrics 系统
- 添加协议级别的 metrics
- 添加 session 级别的 metrics

**影响范围：**
- `internal/core/metrics/`
- 各协议适配器
- session 管理模块

---

#### 6. pprof 标准化

**问题描述：**
- 已经有运行时数据抓取，但对外暴露的 profile/调试接口需要标准化
- 需要权限保护

**修复方案：**
- 标准化 pprof 接口
- 添加权限保护
- 统一调试接口

**影响范围：**
- API 服务器
- 调试接口

---

#### 7. Healthcheck 接口

**问题描述：**
- 需要对外暴露 `/healthz` 或类似接口
- 检查 broker/storage/协议子系统的状态

**修复方案：**
- 创建 healthcheck 服务
- 检查各子系统状态
- 暴露 HTTP 接口

**影响范围：**
- API 服务器
- 各子系统

---

## 实施计划

### 第一阶段（立即执行）
1. 修复 context.Background() 使用问题
2. 修复 Mutex/RWMutex 并发安全问题

### 第二阶段（1-2周内）
3. 实现错误处理分层体系
4. 更新日志系统集成错误类型

### 第三阶段（1-2个月内）
5. 创建协议处理模块文档
6. 扩展 Metrics 系统
7. 标准化 pprof 接口
8. 实现 Healthcheck 接口

---

## 参考

- 原始代码审查：`docs/chatgpt5_review.md`
- 架构设计文档：`docs/ARCHITECTURE_DESIGN_V2.2.md`
- 术语文档：`docs/architecture/terminology.md`

