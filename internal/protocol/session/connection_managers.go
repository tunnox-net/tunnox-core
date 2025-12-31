package session

import (
	"tunnox-core/internal/protocol/session/connection"
)

// ============================================================================
// TCP 连接管理器类型别名 - 向后兼容
// 实际实现已迁移至 connection 子包
// ============================================================================

// TCPConnectionState TCP 连接状态管理器
// Deprecated: 请使用 connection.TCPConnectionState
type TCPConnectionState = connection.TCPConnectionState

// NewTCPConnectionState 创建 TCP 连接状态管理器
// Deprecated: 请使用 connection.NewTCPConnectionState
var NewTCPConnectionState = connection.NewTCPConnectionState

// TCPConnectionTimeout TCP 超时管理器
// Deprecated: 请使用 connection.TCPConnectionTimeout
type TCPConnectionTimeout = connection.TCPConnectionTimeout

// NewTCPConnectionTimeout 创建 TCP 超时管理器
// Deprecated: 请使用 connection.NewTCPConnectionTimeout
var NewTCPConnectionTimeout = connection.NewTCPConnectionTimeout

// TCPConnectionError TCP 错误处理器
// Deprecated: 请使用 connection.TCPConnectionError
type TCPConnectionError = connection.TCPConnectionError

// NewTCPConnectionError 创建 TCP 错误处理器
// Deprecated: 请使用 connection.NewTCPConnectionError
var NewTCPConnectionError = connection.NewTCPConnectionError

// TCPConnectionReuse TCP 连接复用策略
// Deprecated: 请使用 connection.TCPConnectionReuse
type TCPConnectionReuse = connection.TCPConnectionReuse

// NewTCPConnectionReuse 创建 TCP 连接复用策略
// Deprecated: 请使用 connection.NewTCPConnectionReuse
var NewTCPConnectionReuse = connection.NewTCPConnectionReuse
