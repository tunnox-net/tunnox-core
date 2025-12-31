package session

import (
	"tunnox-core/internal/protocol/session/connection"
)

// ============================================================================
// 类型别名 - 向后兼容
// 实际类型定义已迁移至 connection 子包
// ============================================================================

// TunnelConnectionInterface 隧道连接接口（所有协议通用）
// Deprecated: 请使用 connection.TunnelConnectionInterface
type TunnelConnectionInterface = connection.TunnelConnectionInterface

// ControlConnectionInterface 控制连接接口
// Deprecated: 请使用 connection.ControlConnectionInterface
type ControlConnectionInterface = connection.ControlConnectionInterface

// ConnectionStateManager 连接状态管理器接口
// Deprecated: 请使用 connection.ConnectionStateManager
type ConnectionStateManager = connection.ConnectionStateManager

// ConnectionStateType 连接状态类型
// Deprecated: 请使用 connection.ConnectionStateType
type ConnectionStateType = connection.ConnectionStateType

// 状态常量别名
const (
	StateConnecting = connection.StateConnecting
	StateConnected  = connection.StateConnected
	StateStreaming  = connection.StateStreaming
	StateClosing    = connection.StateClosing
	StateClosed     = connection.StateClosed
)

// ConnectionTimeoutManager 连接超时管理器接口
// Deprecated: 请使用 connection.ConnectionTimeoutManager
type ConnectionTimeoutManager = connection.ConnectionTimeoutManager

// ConnectionErrorHandler 连接错误处理器接口
// Deprecated: 请使用 connection.ConnectionErrorHandler
type ConnectionErrorHandler = connection.ConnectionErrorHandler

// ErrorType 错误类型
// Deprecated: 请使用 connection.ErrorType
type ErrorType = connection.ErrorType

// 错误类型常量别名
const (
	ErrorNone     = connection.ErrorNone
	ErrorNetwork  = connection.ErrorNetwork
	ErrorTimeout  = connection.ErrorTimeout
	ErrorProtocol = connection.ErrorProtocol
	ErrorAuth     = connection.ErrorAuth
	ErrorClosed   = connection.ErrorClosed
	ErrorUnknown  = connection.ErrorUnknown
)

// ConnectionReuseStrategy 连接复用策略接口
// Deprecated: 请使用 connection.ConnectionReuseStrategy
type ConnectionReuseStrategy = connection.ConnectionReuseStrategy

// ControlConnection 指令连接
// Deprecated: 请使用 connection.ControlConnection
type ControlConnection = connection.ControlConnection

// NewControlConnection 创建指令连接
// Deprecated: 请使用 connection.NewControlConnection
var NewControlConnection = connection.NewControlConnection

// TunnelConnection 映射连接
// Deprecated: 请使用 connection.TunnelConnection
type TunnelConnection = connection.TunnelConnection

// NewTunnelConnection 创建映射连接
// Deprecated: 请使用 connection.NewTunnelConnection
var NewTunnelConnection = connection.NewTunnelConnection

// ClientConnection 通用客户端连接别名
// Deprecated: 请使用 connection.ClientConnection
type ClientConnection = connection.ClientConnection

// NewClientConnection 创建客户端连接的别名
// Deprecated: 请使用 connection.NewClientConnection
var NewClientConnection = connection.NewClientConnection

