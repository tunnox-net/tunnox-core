// Package httppoll 提供 HTTP 长轮询传输模块
// 用于客户端通过 HTTP 长轮询方式连接服务器
package httppoll

import (
	"context"

	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/protocol/httppoll"

	"github.com/gorilla/mux"
)

// HTTPPollModule HTTP 长轮询传输模块
type HTTPPollModule struct {
	*dispose.ServiceBase

	config   *httpservice.HTTPPollModuleConfig
	deps     *httpservice.ModuleDependencies
	registry *httppoll.ConnectionRegistry
}

// NewHTTPPollModule 创建 HTTP 长轮询模块
func NewHTTPPollModule(ctx context.Context, config *httpservice.HTTPPollModuleConfig) *HTTPPollModule {
	m := &HTTPPollModule{
		ServiceBase: dispose.NewService("HTTPPollModule", ctx),
		config:      config,
		registry:    httppoll.NewConnectionRegistry(),
	}

	return m
}

// Name 返回模块名称
func (m *HTTPPollModule) Name() string {
	return "HTTPPoll"
}

// SetDependencies 注入依赖
func (m *HTTPPollModule) SetDependencies(deps *httpservice.ModuleDependencies) {
	m.deps = deps
}

// RegisterRoutes 注册路由
// 注意：HTTP 长轮询的实际路由处理在现有的 api 包中
// 这里只是为了模块化架构的一致性
func (m *HTTPPollModule) RegisterRoutes(router *mux.Router) {
	// HTTP 长轮询端点的实际处理在 api 包中
	// 这里不重复注册，避免路由冲突
	corelog.Infof("HTTPPollModule: routes registered (handled by api package)")
}

// Start 启动模块
func (m *HTTPPollModule) Start() error {
	corelog.Infof("HTTPPollModule: started with max_request_size=%d, default_timeout=%d",
		m.config.MaxRequestSize, m.config.DefaultTimeout)
	return nil
}

// Stop 停止模块
func (m *HTTPPollModule) Stop() error {
	corelog.Infof("HTTPPollModule: stopped")
	return nil
}

// GetRegistry 获取连接注册表
func (m *HTTPPollModule) GetRegistry() *httppoll.ConnectionRegistry {
	return m.registry
}
