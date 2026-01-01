package server

import (
	"fmt"

	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/command"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/protocol/session"
)

// HTTPDomainCommandHandlers HTTP 域名命令处理器集合
type HTTPDomainCommandHandlers struct {
	sessionMgr *session.SessionManager
	adapter    *HTTPDomainRepositoryAdapter
}

// NewHTTPDomainCommandHandlers 创建 HTTP 域名命令处理器
//
// 参数：
//   - sessionMgr: 会话管理器
//   - repo: HTTP 域名映射仓库
func NewHTTPDomainCommandHandlers(
	sessionMgr *session.SessionManager,
	repo repos.IHTTPDomainMappingRepository,
) *HTTPDomainCommandHandlers {
	return &HTTPDomainCommandHandlers{
		sessionMgr: sessionMgr,
		adapter:    NewHTTPDomainRepositoryAdapter(repo),
	}
}

// RegisterHandlers 注册所有 HTTP 域名命令处理器
func (h *HTTPDomainCommandHandlers) RegisterHandlers(registry *command.CommandRegistry) error {
	if registry == nil {
		return fmt.Errorf("command registry is nil")
	}

	// 获取基础域名列表
	baseDomains := h.adapter.repo.GetBaseDomains()

	// 注册获取基础域名列表命令
	getBaseDomainsHandler := command.NewHTTPDomainGetBaseDomainsHandler(baseDomains)
	if err := registry.Register(getBaseDomainsHandler); err != nil {
		return fmt.Errorf("failed to register get base domains handler: %w", err)
	}

	// 注册检查子域名可用性命令
	checkSubdomainHandler := command.NewHTTPDomainCheckSubdomainHandler(h.adapter)
	if err := registry.Register(checkSubdomainHandler); err != nil {
		return fmt.Errorf("failed to register check subdomain handler: %w", err)
	}

	// 注册生成随机子域名命令
	genSubdomainHandler := command.NewHTTPDomainGenSubdomainHandler(h.adapter)
	if err := registry.Register(genSubdomainHandler); err != nil {
		return fmt.Errorf("failed to register gen subdomain handler: %w", err)
	}

	// 注册创建 HTTP 域名映射命令
	createHandler := command.NewHTTPDomainCreateHandler(h.adapter, h.adapter)
	if err := registry.Register(createHandler); err != nil {
		return fmt.Errorf("failed to register create HTTP domain handler: %w", err)
	}

	// 注册列出 HTTP 域名映射命令
	listHandler := command.NewHTTPDomainListHandler(h.adapter)
	if err := registry.Register(listHandler); err != nil {
		return fmt.Errorf("failed to register list HTTP domain handler: %w", err)
	}

	// 注册删除 HTTP 域名映射命令
	deleteHandler := command.NewHTTPDomainDeleteHandler(h.adapter)
	if err := registry.Register(deleteHandler); err != nil {
		return fmt.Errorf("failed to register delete HTTP domain handler: %w", err)
	}

	corelog.Infof("HTTPDomainCommandHandlers: registered %d handlers, baseDomains=%v", 6, baseDomains)
	return nil
}

// GetAdapter 获取适配器（用于 HTTP 代理查找映射）
func (h *HTTPDomainCommandHandlers) GetAdapter() *HTTPDomainRepositoryAdapter {
	return h.adapter
}
