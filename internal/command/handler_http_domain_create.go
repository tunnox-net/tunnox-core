package command

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射创建相关 Handler
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// HTTPDomainGenSubdomainHandler 生成随机子域名处理器
type HTTPDomainGenSubdomainHandler struct {
	*BaseHandler
	checker SubdomainChecker
}

// NewHTTPDomainGenSubdomainHandler 创建处理器
func NewHTTPDomainGenSubdomainHandler(checker SubdomainChecker) *HTTPDomainGenSubdomainHandler {
	return &HTTPDomainGenSubdomainHandler{
		BaseHandler: NewBaseHandler(
			packet.HTTPDomainGenSubdomain,
			CategoryMapping,
			DirectionDuplex,
			"http_domain_gen_subdomain",
			"生成随机子域名",
		),
		checker: checker,
	}
}

func (h *HTTPDomainGenSubdomainHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	corelog.Debugf("HTTPDomainGenSubdomainHandler: handling request from connection %s", ctx.ConnectionID)

	var req packet.HTTPDomainGenSubdomainRequest
	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		resp := packet.HTTPDomainGenSubdomainResponse{
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

	// 验证基础域名
	if req.BaseDomain == "" {
		resp := packet.HTTPDomainGenSubdomainResponse{
			Success: false,
			Error:   "base_domain is required",
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 生成可用的子域名（最多尝试10次）
	var subdomain string
	for i := 0; i < 10; i++ {
		subdomain = generateRandomSubdomain()
		if h.checker == nil || h.checker.IsSubdomainAvailable(subdomain, req.BaseDomain) {
			break
		}
	}

	fullDomain := subdomain + "." + req.BaseDomain
	resp := packet.HTTPDomainGenSubdomainResponse{
		Success:    true,
		Subdomain:  subdomain,
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
func (h *HTTPDomainGenSubdomainHandler) SetChecker(checker SubdomainChecker) {
	h.checker = checker
}

// HTTPDomainCreateHandler 创建 HTTP 域名映射处理器
type HTTPDomainCreateHandler struct {
	*BaseHandler
	checker SubdomainChecker
	creator HTTPDomainCreator
}

// NewHTTPDomainCreateHandler 创建处理器
func NewHTTPDomainCreateHandler(checker SubdomainChecker, creator HTTPDomainCreator) *HTTPDomainCreateHandler {
	return &HTTPDomainCreateHandler{
		BaseHandler: NewBaseHandler(
			packet.HTTPDomainCreate,
			CategoryMapping,
			DirectionDuplex,
			"http_domain_create",
			"创建 HTTP 域名映射",
		),
		checker: checker,
		creator: creator,
	}
}

func (h *HTTPDomainCreateHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	corelog.Infof("HTTPDomainCreateHandler: handling request from connection %s, clientID=%d", ctx.ConnectionID, ctx.ClientID)

	var req packet.HTTPDomainCreateRequest
	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		resp := packet.HTTPDomainCreateResponse{
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
	if req.TargetURL == "" || req.Subdomain == "" || req.BaseDomain == "" {
		resp := packet.HTTPDomainCreateResponse{
			Success: false,
			Error:   "target_url, subdomain and base_domain are required",
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 解析目标 URL
	parsedURL, err := url.Parse(req.TargetURL)
	if err != nil {
		resp := packet.HTTPDomainCreateResponse{
			Success: false,
			Error:   fmt.Sprintf("invalid target_url: %v", err),
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	targetHost := parsedURL.Hostname()
	targetPort := 80
	if parsedURL.Port() != "" {
		if port, err := strconv.Atoi(parsedURL.Port()); err == nil {
			targetPort = port
		}
	} else if parsedURL.Scheme == "https" {
		targetPort = 443
	}

	// 检查基础域名是否允许
	if h.checker != nil && !h.checker.IsBaseDomainAllowed(req.BaseDomain) {
		resp := packet.HTTPDomainCreateResponse{
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
	if h.checker != nil && !h.checker.IsSubdomainAvailable(req.Subdomain, req.BaseDomain) {
		resp := packet.HTTPDomainCreateResponse{
			Success: false,
			Error:   fmt.Sprintf("subdomain already in use: %s.%s", req.Subdomain, req.BaseDomain),
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	// 设置默认 TTL（7天）
	ttl := req.MappingTTL
	if ttl <= 0 {
		ttl = 7 * 24 * 3600 // 7 days
	}

	// 创建映射
	if h.creator == nil {
		resp := packet.HTTPDomainCreateResponse{
			Success: false,
			Error:   "domain creator not configured",
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	mappingID, fullDomain, expiresAt, err := h.creator.CreateHTTPDomainMapping(
		ctx.ClientID,
		targetHost,
		targetPort,
		req.Subdomain,
		req.BaseDomain,
		req.Description,
		ttl,
	)
	if err != nil {
		resp := packet.HTTPDomainCreateResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to create mapping: %v", err),
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	resp := packet.HTTPDomainCreateResponse{
		Success:    true,
		MappingID:  mappingID,
		FullDomain: fullDomain,
		TargetURL:  req.TargetURL,
		ExpiresAt:  expiresAt,
	}

	data, _ := json.Marshal(resp)
	corelog.Infof("HTTPDomainCreateHandler: created mapping %s for domain %s -> %s", mappingID, fullDomain, req.TargetURL)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// SetChecker 设置检查器
func (h *HTTPDomainCreateHandler) SetChecker(checker SubdomainChecker) {
	h.checker = checker
}

// SetCreator 设置创建器
func (h *HTTPDomainCreateHandler) SetCreator(creator HTTPDomainCreator) {
	h.creator = creator
}
