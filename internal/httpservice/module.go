// Package httpservice 提供统一的 HTTP 服务框架
// 支持模块化设计，各模块自注册路由，独立配置启用/禁用
package httpservice

import (
	"net"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/health"
	"tunnox-core/internal/protocol/httptypes"
	"tunnox-core/internal/stream"

	"github.com/gorilla/mux"
)

// HTTPModule HTTP 服务模块接口
// 所有 HTTP 子服务（Management API、WebSocket、Domain Proxy 等）都需要实现此接口
type HTTPModule interface {
	// Name 模块名称（用于日志和配置）
	Name() string

	// RegisterRoutes 注册路由到 router
	// 模块自行决定注册哪些路径
	RegisterRoutes(router *mux.Router)

	// SetDependencies 注入依赖
	SetDependencies(deps *ModuleDependencies)

	// Start 启动模块（可选的后台任务）
	Start() error

	// Stop 停止模块
	Stop() error
}

// ModuleDependencies 模块依赖
// 包含所有模块可能需要的公共依赖
type ModuleDependencies struct {
	// SessionMgr 会话管理器接口
	SessionMgr SessionManagerInterface

	// CloudControl 云控 API
	CloudControl managers.CloudControlAPI

	// Storage 存储接口
	Storage storage.Storage

	// HealthManager 健康检查管理器
	HealthManager *health.HealthManager

	// DomainRegistry 域名映射注册表（仅域名代理模块使用）
	DomainRegistry *DomainRegistry
}

// SessionManagerInterface 会话管理器接口
// 用于解耦 HTTP 服务与 session 包的依赖
type SessionManagerInterface interface {
	// GetControlConnectionInterface 获取控制连接
	GetControlConnectionInterface(clientID int64) ControlConnectionAccessor

	// BroadcastConfigPush 广播配置推送
	BroadcastConfigPush(clientID int64, configBody string) error

	// GetNodeID 获取当前节点ID
	GetNodeID() string

	// SendHTTPProxyRequest 发送 HTTP 代理请求（命令模式）
	SendHTTPProxyRequest(clientID int64, request *httptypes.HTTPProxyRequest) (*httptypes.HTTPProxyResponse, error)

	// RequestTunnelForHTTP 请求为 HTTP 代理创建隧道连接（隧道模式）
	RequestTunnelForHTTP(clientID int64, mappingID string, targetURL string, method string) (TunnelConnectionInterface, error)

	// NotifyClientUpdate 通知客户端更新配置
	NotifyClientUpdate(clientID int64)
}

// TunnelConnectionInterface 隧道连接接口
// 用于解耦 HTTP 服务与 session 包的隧道连接依赖
type TunnelConnectionInterface interface {
	// GetNetConn 获取底层网络连接
	GetNetConn() net.Conn

	// GetStream 获取数据流
	GetStream() stream.PackageStreamer

	// Read 读取数据
	Read(p []byte) (n int, err error)

	// Write 写入数据
	Write(p []byte) (n int, err error)

	// Close 关闭连接
	Close() error
}

// ControlConnectionAccessor 控制连接访问器接口
type ControlConnectionAccessor interface {
	GetConnID() string
	GetRemoteAddr() string
}
