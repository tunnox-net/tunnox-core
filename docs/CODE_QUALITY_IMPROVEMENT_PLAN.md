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

#### 3. 错误处理分层体系

**问题描述：**
- 有些地方用 `fmt.Errorf("xxx: %w", err)` 自己处理
- 有些地方只 log 错误但不返回上层（或反之）
- 没有明显的"可重试/需告警/致命"分类体系

**修复方案：**

**创建错误分层方案：**

```go
// internal/core/errors/errors.go
package errors

import "fmt"

// 错误类型
type ErrorType string

const (
	ErrorTypeTemporary ErrorType = "temporary" // 可重试
	ErrorTypePermanent ErrorType = "permanent" // 永久错误
	ErrorTypeProtocol ErrorType = "protocol"   // 协议错误
	ErrorTypeNetwork  ErrorType = "network"   // 网络错误
	ErrorTypeStorage  ErrorType = "storage"   // 存储错误
	ErrorTypeAuth     ErrorType = "auth"       // 认证错误
	ErrorTypeFatal    ErrorType = "fatal"      // 致命错误
)

// TypedError 带类型的错误
type TypedError struct {
	Type    ErrorType
	Message string
	Err     error
	Retryable bool
	Alertable bool
}

func (e *TypedError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

func (e *TypedError) Unwrap() error {
	return e.Err
}

// Sentinel errors
var (
	ErrTemporary = &TypedError{Type: ErrorTypeTemporary, Retryable: true}
	ErrPermanent = &TypedError{Type: ErrorTypePermanent, Retryable: false}
	ErrProtocol  = &TypedError{Type: ErrorTypeProtocol, Retryable: false, Alertable: true}
	ErrNetwork   = &TypedError{Type: ErrorTypeNetwork, Retryable: true}
	ErrStorage   = &TypedError{Type: ErrorTypeStorage, Retryable: true, Alertable: true}
	ErrAuth      = &TypedError{Type: ErrorTypeAuth, Retryable: false, Alertable: true}
	ErrFatal     = &TypedError{Type: ErrorTypeFatal, Retryable: false, Alertable: true}
)

// Wrap 包装错误
func Wrap(err error, errType ErrorType, message string) error {
	if err == nil {
		return nil
	}
	return &TypedError{
		Type:      errType,
		Message:   message,
		Err:       err,
		Retryable: isRetryable(errType),
		Alertable: isAlertable(errType),
	}
}

// IsRetryable 判断是否可重试
func IsRetryable(err error) bool {
	if typedErr, ok := err.(*TypedError); ok {
		return typedErr.Retryable
	}
	return false
}

// IsAlertable 判断是否需要告警
func IsAlertable(err error) bool {
	if typedErr, ok := err.(*TypedError); ok {
		return typedErr.Alertable
	}
	return false
}

// GetErrorType 获取错误类型
func GetErrorType(err error) ErrorType {
	if typedErr, ok := err.(*TypedError); ok {
		return typedErr.Type
	}
	return ErrorTypePermanent
}
```

**日志集成：**
- 根据错误类型打不同级别
- 添加错误类型字段，方便统计分析

**影响范围：**
- 所有错误处理代码
- 日志系统

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

