package core

import (
	"context"
	"net"
	"time"

	"tunnox-core/internal/core/events"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream"
)

// ============================================================================
// 会话上下文接口 - 子管理器依赖此接口而非 SessionManager
// ============================================================================

// SessionContext 会话上下文接口
type SessionContext interface {
	Ctx() context.Context
	GetNodeID() string
	GetCloudControl() CloudControlAPI
	GetEventBus() events.EventBus
	GetLogger() corelog.Logger
}

// ============================================================================
// 连接提供者接口
// ============================================================================

// ControlConnectionInfo 控制连接信息接口
type ControlConnectionInfo interface {
	GetConnID() string
	GetClientID() int64
	GetUserID() string
	GetStream() stream.PackageStreamer
	GetRemoteAddr() net.Addr
	GetProtocol() string
	IsAuthenticated() bool
	IsStale(timeout time.Duration) bool
	UpdateActivity()
	Close() error
}

// TunnelConnectionInfo 隧道连接信息接口
type TunnelConnectionInfo interface {
	GetConnID() string
	GetTunnelID() string
	GetMappingID() string
	GetStream() stream.PackageStreamer
	GetProtocol() string
	IsAuthenticated() bool
	UpdateActivity()
	Close() error
}

// ControlConnectionProvider 控制连接提供者接口
type ControlConnectionProvider interface {
	GetControlConnectionByClientID(clientID int64) ControlConnectionInfo
	GetControlConnectionByConnID(connID string) ControlConnectionInfo
}

// TunnelConnectionProvider 隧道连接提供者接口
type TunnelConnectionProvider interface {
	GetTunnelConnectionByTunnelID(tunnelID string) TunnelConnectionInfo
	GetTunnelConnectionByConnID(connID string) TunnelConnectionInfo
}

// ============================================================================
// 连接管理器接口
// ============================================================================

// ConnectionManagerInterface 连接管理器接口
type ConnectionManagerInterface interface {
	// 连接生命周期
	CreateConnection(connID string, s stream.PackageStreamer, rawConn net.Conn) error
	CloseConnection(connID string) error
	GetConnectionCount() int

	// 清理
	StartCleanup(interval time.Duration, timeout time.Duration)
	StopCleanup()
}

// ControlConnectionManagerInterface 控制连接管理器接口
type ControlConnectionManagerInterface interface {
	ControlConnectionProvider

	// 注册与移除
	Register(conn ControlConnectionInfo) error
	Remove(connID string)
	RemoveByClientID(clientID int64)

	// 认证
	UpdateAuth(connID string, clientID int64, userID string) error

	// 查询
	Count() int
	ListAll() []ControlConnectionInfo
}

// TunnelConnectionManagerInterface 隧道连接管理器接口
type TunnelConnectionManagerInterface interface {
	TunnelConnectionProvider

	// 注册与移除
	Register(conn TunnelConnectionInfo) error
	Remove(connID string)
	RemoveByTunnelID(tunnelID string)

	// 认证
	UpdateAuth(connID string, tunnelID string, mappingID string) error

	// 查询
	Count() int
}
