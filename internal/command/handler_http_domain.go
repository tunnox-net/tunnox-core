package command

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"strconv"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射命令处理器
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

// SubdomainChecker 子域名检查接口
type SubdomainChecker interface {
	IsSubdomainAvailable(subdomain, baseDomain string) bool
	IsBaseDomainAllowed(baseDomain string) bool
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

// generateRandomSubdomain 生成随机子域名
func generateRandomSubdomain() string {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	const length = 4
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return "s" + string(result)
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

// HTTPDomainCreator HTTP 域名映射创建接口
type HTTPDomainCreator interface {
	CreateHTTPDomainMapping(clientID int64, targetHost string, targetPort int, subdomain, baseDomain, description string, ttlSeconds int) (mappingID, fullDomain, expiresAt string, err error)
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

// HTTPDomainLister HTTP 域名映射列表查询接口
type HTTPDomainLister interface {
	ListHTTPDomainMappings(clientID int64) ([]HTTPDomainMappingInfo, error)
}

// HTTPDomainMappingInfo HTTP 域名映射信息
type HTTPDomainMappingInfo struct {
	MappingID  string `json:"mapping_id"`
	FullDomain string `json:"full_domain"`
	TargetURL  string `json:"target_url"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at,omitempty"`
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

// HTTPDomainDeleter HTTP 域名映射删除接口
type HTTPDomainDeleter interface {
	DeleteHTTPDomainMapping(clientID int64, mappingID string) error
}

// HTTPDomainDeleteHandler 删除 HTTP 域名映射处理器
type HTTPDomainDeleteHandler struct {
	*BaseHandler
	deleter HTTPDomainDeleter
}

// NewHTTPDomainDeleteHandler 创建处理器
func NewHTTPDomainDeleteHandler(deleter HTTPDomainDeleter) *HTTPDomainDeleteHandler {
	return &HTTPDomainDeleteHandler{
		BaseHandler: NewBaseHandler(
			packet.HTTPDomainDelete,
			CategoryMapping,
			DirectionDuplex,
			"http_domain_delete",
			"删除 HTTP 域名映射",
		),
		deleter: deleter,
	}
}

func (h *HTTPDomainDeleteHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	corelog.Infof("HTTPDomainDeleteHandler: handling request from connection %s, clientID=%d", ctx.ConnectionID, ctx.ClientID)

	var req packet.HTTPDomainDeleteRequest
	if err := json.Unmarshal([]byte(ctx.RequestBody), &req); err != nil {
		resp := packet.HTTPDomainDeleteResponse{
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

	if req.MappingID == "" {
		resp := packet.HTTPDomainDeleteResponse{
			Success: false,
			Error:   "mapping_id is required",
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	if h.deleter == nil {
		resp := packet.HTTPDomainDeleteResponse{
			Success: false,
			Error:   "domain deleter not configured",
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	if err := h.deleter.DeleteHTTPDomainMapping(ctx.ClientID, req.MappingID); err != nil {
		resp := packet.HTTPDomainDeleteResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to delete mapping: %v", err),
		}
		data, _ := json.Marshal(resp)
		return &CommandResponse{
			Success:   false,
			Data:      string(data),
			RequestID: ctx.RequestID,
			CommandId: ctx.CommandId,
		}, nil
	}

	resp := packet.HTTPDomainDeleteResponse{
		Success: true,
	}

	data, _ := json.Marshal(resp)
	corelog.Infof("HTTPDomainDeleteHandler: deleted mapping %s", req.MappingID)
	return &CommandResponse{
		Success:   true,
		Data:      string(data),
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

// SetDeleter 设置删除器
func (h *HTTPDomainDeleteHandler) SetDeleter(deleter HTTPDomainDeleter) {
	h.deleter = deleter
}

func init() {
	// 初始化随机数种子
	rand.Seed(time.Now().UnixNano())
}
