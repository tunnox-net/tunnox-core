package server

import (
	"context"
	"fmt"

	"tunnox-core/internal/broker"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/node"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/health"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/security"
)

// ============================================================================
// 组件接口定义
// ============================================================================

// Component 服务器组件接口
// 每个组件负责自己的初始化、启动和停止逻辑
type Component interface {
	// Name 返回组件名称（用于日志和错误信息）
	Name() string

	// Initialize 初始化组件，注入依赖
	// 返回 error 表示初始化失败，服务器应该停止启动
	Initialize(ctx context.Context, deps *Dependencies) error

	// Start 启动组件（可选，大部分组件不需要）
	Start() error

	// Stop 停止组件（可选，大部分组件不需要）
	Stop() error
}

// Dependencies 依赖容器
// 组件初始化时从这里获取依赖，初始化完成后将自己的产出注入回来
type Dependencies struct {
	// 配置
	Config *Config

	// 基础设施层
	Storage   storage.Storage
	IDManager *idgen.IDManager

	// 云控制层
	CloudControl managers.CloudControlAPI
	CloudBuiltin *managers.BuiltinCloudControl

	// 节点信息
	NodeID        string
	NodeAllocator *node.NodeIDAllocator

	// 会话管理
	SessionMgr *session.SessionManager

	// 安全组件
	BruteForceProtector   *security.BruteForceProtector
	IPManager             *security.IPManager
	RateLimiter           *security.RateLimiter
	ReconnectTokenManager *security.ReconnectTokenManager
	SessionTokenManager   *security.SessionTokenManager

	// 服务层
	ConnCodeService *services.ConnectionCodeService
	HealthManager   *health.HealthManager

	// 协议层
	ProtocolMgr     *protocol.ProtocolManager
	ProtocolFactory *ProtocolFactory

	// 消息和桥接
	MessageBroker broker.MessageBroker
	BridgeAdapter *BridgeAdapter

	// HTTP 服务
	HTTPService *httpservice.HTTPService

	// 处理器
	AuthHandler   *ServerAuthHandler
	TunnelHandler *ServerTunnelHandler

	// 仓库（供组件间共享）
	Repository     *repos.Repository
	HTTPDomainRepo repos.IHTTPDomainMappingRepository

	// Webhook 组件
	WebhookRepo    repos.IWebhookRepository
	WebhookManager managers.WebhookManagerAPI
}

// ============================================================================
// 基础组件实现
// ============================================================================

// BaseComponent 组件基类，提供默认的 Start/Stop 实现
type BaseComponent struct {
	name string
}

// NewBaseComponent 创建基础组件
func NewBaseComponent(name string) *BaseComponent {
	return &BaseComponent{name: name}
}

func (c *BaseComponent) Name() string {
	return c.name
}

func (c *BaseComponent) Start() error {
	return nil
}

func (c *BaseComponent) Stop() error {
	return nil
}

// ============================================================================
// 组件初始化错误
// ============================================================================

// ComponentError 组件初始化错误
type ComponentError struct {
	ComponentName string
	Err           error
}

func (e *ComponentError) Error() string {
	return fmt.Sprintf("component %s initialization failed: %v", e.ComponentName, e.Err)
}

func (e *ComponentError) Unwrap() error {
	return e.Err
}

// NewComponentError 创建组件错误
func NewComponentError(name string, err error) *ComponentError {
	return &ComponentError{
		ComponentName: name,
		Err:           err,
	}
}
