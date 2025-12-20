package httpservice

import (
	"sync"

	"tunnox-core/internal/cloud/models"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// 域名代理相关错误
var (
	ErrDomainNotFound     = coreerrors.New(coreerrors.CodeNotFound, "domain mapping not found")
	ErrClientOffline      = coreerrors.New(coreerrors.CodeUnavailable, "client is offline")
	ErrProxyTimeout       = coreerrors.New(coreerrors.CodeTimeout, "proxy request timeout")
	ErrDomainAlreadyExist = coreerrors.New(coreerrors.CodeAlreadyExists, "domain already registered")
	ErrInvalidDomain      = coreerrors.New(coreerrors.CodeInvalidParam, "invalid domain format")
	ErrBaseDomainNotAllow = coreerrors.New(coreerrors.CodeForbidden, "base domain not allowed")
)

// DomainRegistry 域名注册表（内存索引，数据来自 PortMapping）
// 线程安全，支持并发读写
type DomainRegistry struct {
	mu          sync.RWMutex
	mappings    map[string]*models.PortMapping // key: full_domain (subdomain.base_domain)
	baseDomains map[string]bool                // 允许的基础域名
}

// NewDomainRegistry 创建域名注册表
func NewDomainRegistry(baseDomains []string) *DomainRegistry {
	r := &DomainRegistry{
		mappings:    make(map[string]*models.PortMapping),
		baseDomains: make(map[string]bool),
	}

	for _, domain := range baseDomains {
		r.baseDomains[domain] = true
	}

	return r
}

// Rebuild 从存储重建索引（启动时调用）
func (r *DomainRegistry) Rebuild(mappings []*models.PortMapping) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 清空现有映射
	r.mappings = make(map[string]*models.PortMapping)

	// 重建索引
	for _, mapping := range mappings {
		if mapping.Protocol != models.ProtocolHTTP {
			continue
		}

		fullDomain := mapping.FullDomain()
		if fullDomain == "" {
			continue
		}

		r.mappings[fullDomain] = mapping
	}

	corelog.Infof("DomainRegistry: rebuilt with %d HTTP mappings", len(r.mappings))
}

// Register 注册域名映射
func (r *DomainRegistry) Register(mapping *models.PortMapping) error {
	if mapping == nil {
		return ErrInvalidDomain
	}

	if mapping.Protocol != models.ProtocolHTTP {
		return coreerrors.Newf(coreerrors.CodeInvalidParam, "invalid protocol: %s, expected http", mapping.Protocol)
	}

	fullDomain := mapping.FullDomain()
	if fullDomain == "" {
		return ErrInvalidDomain
	}

	// 验证基础域名是否允许
	if !r.IsBaseDomainAllowed(mapping.HTTPBaseDomain) {
		return ErrBaseDomainNotAllow
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查是否已存在
	if existing, exists := r.mappings[fullDomain]; exists {
		// 如果是同一个映射ID，允许更新
		if existing.ID != mapping.ID {
			return ErrDomainAlreadyExist
		}
	}

	r.mappings[fullDomain] = mapping
	corelog.Debugf("DomainRegistry: registered domain %s -> client=%d, target=%s:%d",
		fullDomain, mapping.TargetClientID, mapping.TargetHost, mapping.TargetPort)

	return nil
}

// Unregister 注销域名映射
func (r *DomainRegistry) Unregister(fullDomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.mappings[fullDomain]; exists {
		delete(r.mappings, fullDomain)
		corelog.Debugf("DomainRegistry: unregistered domain %s", fullDomain)
	}
}

// UnregisterByMappingID 通过映射ID注销
func (r *DomainRegistry) UnregisterByMappingID(mappingID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for domain, mapping := range r.mappings {
		if mapping.ID == mappingID {
			delete(r.mappings, domain)
			corelog.Debugf("DomainRegistry: unregistered domain %s by mapping ID %s", domain, mappingID)
			return
		}
	}
}

// Lookup 查找域名映射
func (r *DomainRegistry) Lookup(fullDomain string) (*models.PortMapping, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mapping, exists := r.mappings[fullDomain]
	return mapping, exists
}

// LookupByHost 通过 Host Header 查找域名映射
// Host 可能包含端口号，需要处理
func (r *DomainRegistry) LookupByHost(host string) (*models.PortMapping, bool) {
	// 移除端口号（如果有）
	domain := host
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			domain = host[:i]
			break
		}
	}

	return r.Lookup(domain)
}

// IsBaseDomainAllowed 检查基础域名是否允许
// 如果 baseDomains 为空，则允许所有域名（开发模式）
func (r *DomainRegistry) IsBaseDomainAllowed(baseDomain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 如果没有配置任何基础域名，允许所有域名
	if len(r.baseDomains) == 0 {
		return true
	}

	return r.baseDomains[baseDomain]
}

// AddBaseDomain 添加允许的基础域名
func (r *DomainRegistry) AddBaseDomain(baseDomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.baseDomains[baseDomain] = true
}

// RemoveBaseDomain 移除允许的基础域名
func (r *DomainRegistry) RemoveBaseDomain(baseDomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.baseDomains, baseDomain)
}

// GetBaseDomains 获取所有允许的基础域名
func (r *DomainRegistry) GetBaseDomains() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	domains := make([]string, 0, len(r.baseDomains))
	for domain := range r.baseDomains {
		domains = append(domains, domain)
	}
	return domains
}

// Count 返回注册的域名数量
func (r *DomainRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.mappings)
}

// GetAllMappings 获取所有映射（用于调试）
func (r *DomainRegistry) GetAllMappings() map[string]*models.PortMapping {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]*models.PortMapping, len(r.mappings))
	for k, v := range r.mappings {
		result[k] = v
	}
	return result
}

// GetMappingsByClientID 获取指定客户端的所有映射
func (r *DomainRegistry) GetMappingsByClientID(clientID int64) []*models.PortMapping {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*models.PortMapping
	for _, mapping := range r.mappings {
		if mapping.TargetClientID == clientID {
			result = append(result, mapping)
		}
	}
	return result
}

// IsSubdomainAvailable 检查子域名是否可用
func (r *DomainRegistry) IsSubdomainAvailable(subdomain, baseDomain string) bool {
	fullDomain := subdomain + "." + baseDomain

	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.mappings[fullDomain]
	return !exists
}
