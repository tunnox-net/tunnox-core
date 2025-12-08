package protocols

import (
	"context"
	"io"

	"tunnox-core/internal/api"
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/protocol/adapter"
	"tunnox-core/internal/protocol/registry"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

// HTTPPollProtocol HTTP 长轮询协议实现
type HTTPPollProtocol struct{}

// NewHTTPPollProtocol 创建 HTTP 长轮询协议
func NewHTTPPollProtocol() *HTTPPollProtocol {
	return &HTTPPollProtocol{}
}

// Name 返回协议名称
func (p *HTTPPollProtocol) Name() string {
	return "httppoll"
}

// Dependencies 返回依赖服务
// HTTP Poll 协议依赖 SessionManager 和 HTTP 服务器（Management API）
func (p *HTTPPollProtocol) Dependencies() []string {
	return []string{"session_manager", "http_server"}
}

// ValidateConfig 验证配置（使用统一的验证接口）
// HTTP Poll 协议不需要端口配置，因为它使用 HTTP 服务器的路由
func (p *HTTPPollProtocol) ValidateConfig(config *registry.Config) error {
	// 先调用基础验证（HTTP Poll 不需要端口，所以只验证名称和主机）
	if err := config.Validate(); err != nil {
		return err
	}
	// HTTP Poll 协议不需要端口验证，因为它使用 HTTP 服务器的路由
	return nil
}

// Initialize 初始化协议
func (p *HTTPPollProtocol) Initialize(ctx context.Context, container registry.Container, config *registry.Config) (adapter.Adapter, error) {
	// 1. 解析 SessionManager
	var sessionMgr *session.SessionManager
	if err := container.ResolveTyped("session_manager", &sessionMgr); err != nil {
		return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to resolve session_manager")
	}

	// 2. 检查 HTTP 路由接口是否存在
	if !container.HasService("http_router") {
		utils.Warn("HTTP Poll protocol requires HTTP router, but it's not available. HTTP Poll routes will not be registered.")
		return &httppollAdapter{
			sessionMgr: sessionMgr,
			available:  false,
		}, nil
	}

	// 3. 解析 HTTP 路由接口（依赖倒置原则：依赖抽象接口而非具体实现）
	var httpRouter registry.HTTPRouter
	if err := container.ResolveTyped("http_router", &httpRouter); err != nil {
		utils.Warnf("HTTP Poll protocol: failed to resolve http_router: %v. HTTP Poll routes may not be available.", err)
		return &httppollAdapter{
			sessionMgr: sessionMgr,
			available:  false,
		}, nil
	}

	// 4. 解析 ManagementAPIServer（用于获取 handler 方法）
	var apiServer *api.ManagementAPIServer
	if err := container.ResolveTyped("http_server", &apiServer); err != nil {
		utils.Warnf("HTTP Poll protocol: failed to resolve http_server for handlers: %v", err)
		return &httppollAdapter{
			sessionMgr: sessionMgr,
			available:  false,
		}, nil
	}

	// 5. HTTP Poll 协议自己注册路由（符合依赖倒置原则）
	// 注册 Push 端点
	if err := httpRouter.RegisterRoute("POST", "/push", apiServer.HandleHTTPPush); err != nil {
		return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to register HTTP Poll push route")
	}

	// 注册 Poll 端点
	if err := httpRouter.RegisterRoute("GET", "/poll", apiServer.HandleHTTPPoll); err != nil {
		return nil, coreErrors.Wrapf(err, coreErrors.ErrorTypePermanent, "failed to register HTTP Poll poll route")
	}

	utils.Infof("HTTP Poll protocol: registered routes /tunnox/v1/push and /tunnox/v1/poll")

	// 6. 创建适配器
	return &httppollAdapter{
		sessionMgr: sessionMgr,
		apiServer:  apiServer,
		httpRouter: httpRouter,
		available:  true,
	}, nil
}

// httppollAdapter HTTP 长轮询适配器（特殊适配器）
// 注意：HTTP 长轮询的连接是通过 HTTP 请求建立的，不是通过 Listen/Accept
type httppollAdapter struct {
	sessionMgr *session.SessionManager
	apiServer  *api.ManagementAPIServer
	httpRouter registry.HTTPRouter
	available  bool
	addr       string
}

func (a *httppollAdapter) Name() string {
	return "httppoll"
}

func (a *httppollAdapter) GetAddr() string {
	if a.apiServer != nil {
		// 返回 HTTP 服务器的地址标识
		// HTTP Poll 使用 HTTP 路由，不需要传统意义上的地址
		return "httppoll://management-api"
	}
	return ""
}

func (a *httppollAdapter) SetAddr(addr string) {
	a.addr = addr
}

// ConnectTo 连接逻辑（客户端使用，服务端不需要）
func (a *httppollAdapter) ConnectTo(serverAddr string) error {
	return coreErrors.New(coreErrors.ErrorTypePermanent, "HTTP Poll adapter does not support ConnectTo (use HTTP requests instead)")
}

// ListenFrom 监听逻辑（HTTP Poll 不需要，因为它使用 HTTP 路由）
func (a *httppollAdapter) ListenFrom(serverAddr string) error {
	// HTTP Poll 协议不需要监听，因为它使用 HTTP 服务器的路由
	// 路由已经在 ManagementAPIServer 中注册了
	if !a.available {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "HTTP Poll protocol is not available (HTTP server not found)")
	}
	utils.Infof("HTTP Poll protocol: routes are ready at /tunnox/v1/push and /tunnox/v1/poll")
	return nil
}

// GetReader 获取读取器（HTTP Poll 不需要）
func (a *httppollAdapter) GetReader() io.Reader {
	return nil
}

// GetWriter 获取写入器（HTTP Poll 不需要）
func (a *httppollAdapter) GetWriter() io.Writer {
	return nil
}

// Close 关闭适配器
func (a *httppollAdapter) Close() error {
	// HTTP Poll 适配器不需要特殊清理
	return nil
}
