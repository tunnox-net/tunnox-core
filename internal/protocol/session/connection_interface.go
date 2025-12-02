package session

import (
	"net"
	"time"

	"tunnox-core/internal/stream"
)

// ============================================================================
// 通用连接接口（协议无关）
// ============================================================================

// TunnelConnectionInterface 隧道连接接口（所有协议通用）
// 抽象了不同协议的连接管理差异
// 注意：与现有的 TunnelConnection 结构体不同，这是接口定义
type TunnelConnectionInterface interface {
	// 基础信息
	GetConnectionID() string              // 连接标识（协议特定实现）
	GetClientID() int64                   // 客户端ID（所有协议通用）
	GetMappingID() string                 // 映射ID（所有协议通用）
	GetTunnelID() string                  // 隧道ID（所有协议通用）
	GetProtocol() string                  // 协议类型（tcp/websocket/quic/httppoll）

	// 流接口
	GetStream() stream.PackageStreamer    // 获取流（所有协议通用）
	GetNetConn() net.Conn                 // 获取底层连接（TCP/WebSocket/QUIC 返回 net.Conn，HTTP 长轮询返回 nil）

	// 连接状态管理（统一接口）
	ConnectionState() ConnectionStateManager      // 获取连接状态管理器
	ConnectionTimeout() ConnectionTimeoutManager  // 获取超时管理器
	ConnectionError() ConnectionErrorHandler     // 获取错误处理器
	ConnectionReuse() ConnectionReuseStrategy   // 获取复用策略

	// 生命周期
	Close() error                         // 关闭连接（所有协议通用）
	IsClosed() bool                       // 检查是否已关闭
}

// ============================================================================
// 连接状态管理接口（统一抽象）
// ============================================================================

// ConnectionStateManager 连接状态管理器接口
// 统一不同协议的状态管理方式
type ConnectionStateManager interface {
	// 状态查询
	IsConnected() bool                    // 连接是否活跃
	IsClosed() bool                       // 连接是否已关闭
	GetState() ConnectionStateType        // 获取当前状态

	// 状态更新
	SetState(state ConnectionStateType)   // 设置状态
	UpdateActivity()                      // 更新活跃时间
	GetLastActiveTime() time.Time         // 获取最后活跃时间
	GetCreatedTime() time.Time            // 获取创建时间

	// 状态检查
	IsStale(timeout time.Duration) bool   // 检查是否超时失效
}

// ConnectionStateType 连接状态类型
type ConnectionStateType int

const (
	StateConnecting ConnectionStateType = iota // 连接中
	StateConnected                              // 已连接
	StateStreaming                              // 流模式（隧道数据传输）
	StateClosing                                // 关闭中
	StateClosed                                 // 已关闭
)

// ============================================================================
// 超时管理接口（统一抽象）
// ============================================================================

// ConnectionTimeoutManager 连接超时管理器接口
// 统一不同协议的超时管理方式
type ConnectionTimeoutManager interface {
	// 设置超时（统一接口，协议特定实现）
	SetReadDeadline(t time.Time) error    // 设置读取超时
	SetWriteDeadline(t time.Time) error   // 设置写入超时
	SetDeadline(t time.Time) error        // 设置读写超时

	// 获取超时配置
	GetReadTimeout() time.Duration        // 获取读取超时配置
	GetWriteTimeout() time.Duration       // 获取写入超时配置
	GetIdleTimeout() time.Duration        // 获取空闲超时配置

	// 超时检查
	IsReadTimeout(err error) bool         // 检查是否是读取超时
	IsWriteTimeout(err error) bool        // 检查是否是写入超时
	IsIdleTimeout() bool                  // 检查是否空闲超时

	// 重置超时
	ResetReadDeadline() error             // 重置读取超时
	ResetWriteDeadline() error            // 重置写入超时
	ResetDeadline() error                 // 重置读写超时
}

// ============================================================================
// 错误处理接口（统一抽象）
// ============================================================================

// ConnectionErrorHandler 连接错误处理器接口
// 统一不同协议的错误处理方式
type ConnectionErrorHandler interface {
	// 错误处理
	HandleError(err error) error          // 处理错误，返回处理后的错误
	IsRetryable(err error) bool           // 检查错误是否可重试
	ShouldClose(err error) bool           // 检查错误是否应该关闭连接
	IsTemporary(err error) bool           // 检查是否是临时错误

	// 错误分类
	ClassifyError(err error) ErrorType    // 分类错误类型
	GetLastError() error                  // 获取最后一个错误
	ClearError()                          // 清除错误状态
}

// ErrorType 错误类型
type ErrorType int

const (
	ErrorNone ErrorType = iota            // 无错误
	ErrorNetwork                          // 网络错误（可重试）
	ErrorTimeout                          // 超时错误（可重试）
	ErrorProtocol                         // 协议错误（不可重试）
	ErrorAuth                             // 认证错误（不可重试）
	ErrorClosed                           // 连接已关闭（不可重试）
	ErrorUnknown                          // 未知错误
)

// ============================================================================
// 连接复用策略接口（统一抽象）
// ============================================================================

// ConnectionReuseStrategy 连接复用策略接口
// 统一不同协议的连接复用策略
type ConnectionReuseStrategy interface {
	// 复用判断
	CanReuse(conn TunnelConnectionInterface, tunnelID string) bool  // 检查连接是否可以复用
	ShouldCreateNew(tunnelID string) bool                            // 检查是否应该创建新连接

	// 复用管理
	MarkAsReusable(conn TunnelConnectionInterface)                    // 标记连接可复用
	MarkAsUsed(conn TunnelConnectionInterface, tunnelID string)       // 标记连接已使用
	Release(conn TunnelConnectionInterface)                           // 释放连接（可复用）

	// 复用统计
	GetReuseCount(conn TunnelConnectionInterface) int                 // 获取复用次数
	GetMaxReuseCount() int                                             // 获取最大复用次数
}

// 协议特定实现已移至独立文件：
// - connection_managers.go: TCP 和 HTTP 长轮询的连接管理器实现
// - tcp_connection.go: TCP 协议的隧道连接实现
// - httppoll_connection.go: HTTP 长轮询协议的隧道连接实现

