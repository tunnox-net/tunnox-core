// Package domainproxy 提供 HTTP 域名代理功能
// 支持通过域名访问内网服务，无需端口映射
package domainproxy

import (
	"context"
	"net/http"
	"time"

	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"

	"github.com/gorilla/mux"
)

// DomainProxyModule 域名代理模块
type DomainProxyModule struct {
	*dispose.ServiceBase

	config   *httpservice.DomainProxyModuleConfig
	deps     *httpservice.ModuleDependencies
	registry *httpservice.DomainRegistry

	// HTTP 客户端（用于命令模式响应）
	httpClient *http.Client
}

// NewDomainProxyModule 创建域名代理模块
func NewDomainProxyModule(ctx context.Context, config *httpservice.DomainProxyModuleConfig) *DomainProxyModule {
	m := &DomainProxyModule{
		ServiceBase: dispose.NewService("DomainProxyModule", ctx),
		config:      config,
		httpClient: &http.Client{
			Timeout: config.RequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	return m
}

// Name 返回模块名称
func (m *DomainProxyModule) Name() string {
	return "DomainProxy"
}

// SetDependencies 注入依赖
func (m *DomainProxyModule) SetDependencies(deps *httpservice.ModuleDependencies) {
	m.deps = deps
	m.registry = deps.DomainRegistry
}

// RegisterRoutes 注册路由
// 域名代理使用默认路由（/*），根据 Host Header 进行路由
func (m *DomainProxyModule) RegisterRoutes(router *mux.Router) {
	// 域名代理作为默认路由，处理所有未匹配的请求
	// 注意：这个路由应该最后注册，优先级最低
	router.PathPrefix("/").Handler(m).Methods("GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD")

	corelog.Infof("DomainProxyModule: registered default route for domain proxy")
}

// Start 启动模块
func (m *DomainProxyModule) Start() error {
	corelog.Infof("DomainProxyModule: started with %d base domains", len(m.config.BaseDomains))
	return nil
}

// Stop 停止模块
func (m *DomainProxyModule) Stop() error {
	corelog.Infof("DomainProxyModule: stopped")
	return nil
}
