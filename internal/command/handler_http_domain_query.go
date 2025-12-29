package command

import (
	"encoding/json"
	"fmt"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射查询相关 Handler
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// HTTPDomainGetBaseDomainsHandler 获取基础域名列表处理器
type HTTPDomainGetBaseDomainsHandler struct {
	*BaseHandler
	baseDomains []string
}

// NewHTTPDomainGetBaseDomainsHandler 创建处理器
func NewHTTPDomainGetBaseDomainsHandler(baseDomains []string) *HTTPDomainGetBaseDomainsHandler {
	return &HTTPDomainGetBaseDomainsHandler{
		BaseHandler: NewBaseHandler(
			packet.HTTPDomainGetBaseDomains,
			CategoryMapping,
			DirectionDuplex,
			"http_domain_get_base_domains",
			"获取可用的基础域名列表",
		),
		baseDomains: baseDomains,
	}
}

func (h *HTTPDomainGetBaseDomainsHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	corelog.Debugf("HTTPDomainGetBaseDomainsHandler: handling request from connection %s", ctx.ConnectionID)

	// 构建响应
	baseDomains := make([]packet.HTTPDomainBaseDomainInfo, len(h.baseDomains))
	for i, domain := range h.baseDomains {
		baseDomains[i] = packet.HTTPDomainBaseDomainInfo{
			Domain:      domain,
			Description: fmt.Sprintf("Base domain: %s", domain),
			IsDefault:   i == 0, // 第一个为默认
		}
	}

	resp := packet.HTTPDomainGetBaseDomainsResponse{
		Success:     true,
		BaseDomains: baseDomains,
	}

	data, _ := json.Marshal(resp)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// SetBaseDomains 更新基础域名列表
func (h *HTTPDomainGetBaseDomainsHandler) SetBaseDomains(domains []string) {
	h.baseDomains = domains
}

// HTTPDomainCheckSubdomainHandler 检查子域名可用性处理器
type HTTPDomainCheckSubdomainHandler struct {
	*BaseHandler
	// 子域名检查器（由外部注入）
	checker SubdomainChecker
}

// NewHTTPDomainCheckSubdomainHandler 创建处理器
func NewHTTPDomainCheckSubdomainHandler(checker SubdomainChecker) *HTTPDomainCheckSubdomainHandler {
	return &HTTPDomainCheckSubdomainHandler{
		BaseHandler: NewBaseHandler(
			packet.HTTPDomainCheckSubdomain,
			CategoryMapping,
			DirectionDuplex,
			"http_domain_check_subdomain",
			"检查子域名可用性",
		),
		checker: checker,
	}
}

func (h *HTTPDomainCheckSubdomainHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	corelog.Debugf("HTTPDomainCheckSubdomainHandler: handling request from connection %s", ctx.ConnectionID)

	var req packet.HTTPDomainCheckSubdomainRequest
	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		resp := packet.HTTPDomainCheckSubdomainResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid request: %v", err),
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 验证参数
	if req.Subdomain == "" || req.BaseDomain == "" {
		resp := packet.HTTPDomainCheckSubdomainResponse{
			Success: false,
			Error:   "subdomain and base_domain are required",
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 检查基础域名是否允许
	if h.checker != nil && !h.checker.IsBaseDomainAllowed(req.BaseDomain) {
		resp := packet.HTTPDomainCheckSubdomainResponse{
			Success: false,
			Error:   fmt.Sprintf("base domain not allowed: %s", req.BaseDomain),
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 检查子域名可用性
	available := true
	if h.checker != nil {
		available = h.checker.IsSubdomainAvailable(req.Subdomain, req.BaseDomain)
	}

	fullDomain := req.Subdomain + "." + req.BaseDomain
	resp := packet.HTTPDomainCheckSubdomainResponse{
		Success:    true,
		Available:  available,
		FullDomain: fullDomain,
	}

	data, _ := json.Marshal(resp)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// SetChecker 设置检查器
func (h *HTTPDomainCheckSubdomainHandler) SetChecker(checker SubdomainChecker) {
	h.checker = checker
}

// HTTPDomainListHandler 列出 HTTP 域名映射处理器
type HTTPDomainListHandler struct {
	*BaseHandler
	lister HTTPDomainLister
}

// NewHTTPDomainListHandler 创建处理器
func NewHTTPDomainListHandler(lister HTTPDomainLister) *HTTPDomainListHandler {
	return &HTTPDomainListHandler{
		BaseHandler: NewBaseHandler(
			packet.HTTPDomainList,
			CategoryMapping,
			DirectionDuplex,
			"http_domain_list",
			"列出 HTTP 域名映射",
		),
		lister: lister,
	}
}

func (h *HTTPDomainListHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	corelog.Debugf("HTTPDomainListHandler: handling request from connection %s, clientID=%d", ctx.ConnectionID, ctx.ClientID)

	var mappings []HTTPDomainMappingInfo
	if h.lister != nil {
		var err error
		mappings, err = h.lister.ListHTTPDomainMappings(ctx.ClientID)
		if err != nil {
			resp := packet.HTTPDomainListResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to list mappings: %v", err),
			}
			data, _ := json.Marshal(resp)
			return &CommandResponse{
				Success:   false,
				Data:      string(data),
				RequestID: ctx.RequestID,
				CommandId: ctx.CommandId,
			}, nil
		}
	}

	// 转换为 packet 类型
	packetMappings := make([]packet.HTTPDomainMappingInfo, len(mappings))
	for i, m := range mappings {
		packetMappings[i] = packet.HTTPDomainMappingInfo{
			MappingID:  m.MappingID,
			FullDomain: m.FullDomain,
			TargetURL:  m.TargetURL,
			Status:     m.Status,
			CreatedAt:  m.CreatedAt,
			ExpiresAt:  m.ExpiresAt,
		}
	}

	resp := packet.HTTPDomainListResponse{
		Success:  true,
		Mappings: packetMappings,
		Total:    len(packetMappings),
	}

	data, _ := json.Marshal(resp)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// SetLister 设置列表查询器
func (h *HTTPDomainListHandler) SetLister(lister HTTPDomainLister) {
	h.lister = lister
}
