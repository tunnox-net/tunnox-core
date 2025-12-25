package server

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/command"
	"tunnox-core/internal/protocol/session"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名映射命令处理器
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// HTTPDomainCommandHandlers HTTP 域名命令处理器集合
type HTTPDomainCommandHandlers struct {
	sessionMgr     *session.SessionManager
	baseDomains    []string
	domainRegistry *InMemoryDomainRegistry
}

// NewHTTPDomainCommandHandlers 创建 HTTP 域名命令处理器
func NewHTTPDomainCommandHandlers(
	sessionMgr *session.SessionManager,
	baseDomains []string,
) *HTTPDomainCommandHandlers {
	if len(baseDomains) == 0 {
		baseDomains = []string{"tunnox.net"} // 默认域名
	}
	return &HTTPDomainCommandHandlers{
		sessionMgr:     sessionMgr,
		baseDomains:    baseDomains,
		domainRegistry: NewInMemoryDomainRegistry(),
	}
}

// RegisterHandlers 注册所有 HTTP 域名命令处理器
func (h *HTTPDomainCommandHandlers) RegisterHandlers(registry *command.CommandRegistry) error {
	if registry == nil {
		return fmt.Errorf("command registry is nil")
	}

	// 注册获取基础域名列表命令
	getBaseDomainsHandler := command.NewHTTPDomainGetBaseDomainsHandler(h.baseDomains)
	if err := registry.Register(getBaseDomainsHandler); err != nil {
		return fmt.Errorf("failed to register get base domains handler: %w", err)
	}

	// 注册检查子域名可用性命令
	checkSubdomainHandler := command.NewHTTPDomainCheckSubdomainHandler(h.domainRegistry)
	if err := registry.Register(checkSubdomainHandler); err != nil {
		return fmt.Errorf("failed to register check subdomain handler: %w", err)
	}

	// 注册生成随机子域名命令
	genSubdomainHandler := command.NewHTTPDomainGenSubdomainHandler(h.domainRegistry)
	if err := registry.Register(genSubdomainHandler); err != nil {
		return fmt.Errorf("failed to register gen subdomain handler: %w", err)
	}

	// 注册创建 HTTP 域名映射命令
	createHandler := command.NewHTTPDomainCreateHandler(h.domainRegistry, h.domainRegistry)
	if err := registry.Register(createHandler); err != nil {
		return fmt.Errorf("failed to register create HTTP domain handler: %w", err)
	}

	// 注册列出 HTTP 域名映射命令
	listHandler := command.NewHTTPDomainListHandler(h.domainRegistry)
	if err := registry.Register(listHandler); err != nil {
		return fmt.Errorf("failed to register list HTTP domain handler: %w", err)
	}

	// 注册删除 HTTP 域名映射命令
	deleteHandler := command.NewHTTPDomainDeleteHandler(h.domainRegistry)
	if err := registry.Register(deleteHandler); err != nil {
		return fmt.Errorf("failed to register delete HTTP domain handler: %w", err)
	}

	corelog.Infof("HTTPDomainCommandHandlers: registered %d handlers, baseDomains=%v", 6, h.baseDomains)
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 内存域名注册表
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// HTTPDomainMapping HTTP 域名映射信息
type HTTPDomainMapping struct {
	MappingID   string
	ClientID    int64
	Subdomain   string
	BaseDomain  string
	FullDomain  string
	TargetHost  string
	TargetPort  int
	Description string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
	Status      string
}

// InMemoryDomainRegistry 内存中的域名注册表
type InMemoryDomainRegistry struct {
	mu           sync.RWMutex
	mappings     map[string]*HTTPDomainMapping // mappingID -> mapping
	domainIndex  map[string]string             // fullDomain -> mappingID
	baseDomains  []string                      // 允许的基础域名
	nextID       int64
}

// NewInMemoryDomainRegistry 创建内存域名注册表
func NewInMemoryDomainRegistry() *InMemoryDomainRegistry {
	return &InMemoryDomainRegistry{
		mappings:    make(map[string]*HTTPDomainMapping),
		domainIndex: make(map[string]string),
		baseDomains: []string{"tunnox.net", "tunnel.test.local"},
		nextID:      1,
	}
}

// IsBaseDomainAllowed 检查基础域名是否允许
func (r *InMemoryDomainRegistry) IsBaseDomainAllowed(baseDomain string) bool {
	for _, d := range r.baseDomains {
		if d == baseDomain {
			return true
		}
	}
	// 如果列表为空，允许所有域名（测试模式）
	return len(r.baseDomains) == 0
}

// IsSubdomainAvailable 检查子域名是否可用
func (r *InMemoryDomainRegistry) IsSubdomainAvailable(subdomain, baseDomain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fullDomain := subdomain + "." + baseDomain
	_, exists := r.domainIndex[fullDomain]
	return !exists
}

// CreateHTTPDomainMapping 创建 HTTP 域名映射
func (r *InMemoryDomainRegistry) CreateHTTPDomainMapping(
	clientID int64,
	targetHost string,
	targetPort int,
	subdomain, baseDomain, description string,
	ttlSeconds int,
) (mappingID, fullDomain, expiresAt string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	fullDomain = subdomain + "." + baseDomain

	// 检查是否已存在
	if _, exists := r.domainIndex[fullDomain]; exists {
		return "", "", "", fmt.Errorf("domain already in use: %s", fullDomain)
	}

	// 生成映射 ID
	mappingID = fmt.Sprintf("hdm_%d", r.nextID)
	r.nextID++

	// 计算过期时间
	var expiresAtTime *time.Time
	if ttlSeconds > 0 {
		t := time.Now().Add(time.Duration(ttlSeconds) * time.Second)
		expiresAtTime = &t
		expiresAt = t.Format(time.RFC3339)
	}

	// 创建映射
	mapping := &HTTPDomainMapping{
		MappingID:   mappingID,
		ClientID:    clientID,
		Subdomain:   subdomain,
		BaseDomain:  baseDomain,
		FullDomain:  fullDomain,
		TargetHost:  targetHost,
		TargetPort:  targetPort,
		Description: description,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAtTime,
		Status:      "active",
	}

	r.mappings[mappingID] = mapping
	r.domainIndex[fullDomain] = mappingID

	corelog.Infof("InMemoryDomainRegistry: created mapping %s: %s -> %s:%d", mappingID, fullDomain, targetHost, targetPort)
	return mappingID, fullDomain, expiresAt, nil
}

// ListHTTPDomainMappings 列出客户端的 HTTP 域名映射
func (r *InMemoryDomainRegistry) ListHTTPDomainMappings(clientID int64) ([]command.HTTPDomainMappingInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []command.HTTPDomainMappingInfo
	for _, m := range r.mappings {
		if m.ClientID == clientID {
			expiresAt := ""
			if m.ExpiresAt != nil {
				expiresAt = m.ExpiresAt.Format(time.RFC3339)
			}
			result = append(result, command.HTTPDomainMappingInfo{
				MappingID:  m.MappingID,
				FullDomain: m.FullDomain,
				TargetURL:  fmt.Sprintf("http://%s:%d", m.TargetHost, m.TargetPort),
				Status:     m.Status,
				CreatedAt:  m.CreatedAt.Format(time.RFC3339),
				ExpiresAt:  expiresAt,
			})
		}
	}
	return result, nil
}

// DeleteHTTPDomainMapping 删除 HTTP 域名映射
func (r *InMemoryDomainRegistry) DeleteHTTPDomainMapping(clientID int64, mappingID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	mapping, exists := r.mappings[mappingID]
	if !exists {
		return fmt.Errorf("mapping not found: %s", mappingID)
	}

	// 验证所有权
	if mapping.ClientID != clientID {
		return fmt.Errorf("permission denied: mapping belongs to another client")
	}

	// 删除索引
	delete(r.domainIndex, mapping.FullDomain)
	delete(r.mappings, mappingID)

	corelog.Infof("InMemoryDomainRegistry: deleted mapping %s", mappingID)
	return nil
}

// GetMapping 获取映射（用于 HTTP 代理）
func (r *InMemoryDomainRegistry) GetMapping(fullDomain string) *HTTPDomainMapping {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mappingID, exists := r.domainIndex[fullDomain]
	if !exists {
		return nil
	}
	return r.mappings[mappingID]
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 响应类型
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// HTTPDomainResponse HTTP 域名响应
type HTTPDomainResponse struct {
	Success    bool   `json:"success"`
	MappingID  string `json:"mapping_id,omitempty"`
	FullDomain string `json:"full_domain,omitempty"`
	TargetURL  string `json:"target_url,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// marshalResponse 序列化响应
func marshalResponse(resp interface{}) string {
	data, _ := json.Marshal(resp)
	return string(data)
}

