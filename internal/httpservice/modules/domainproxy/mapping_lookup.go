// Package domainproxy 提供 HTTP 域名代理功能
package domainproxy

import (
	"context"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
)

// lookupMapping 查找域名映射
// 优先使用 HTTPDomainMappingRepository（持久化存储），回退到旧的 DomainRegistry（内存缓存）
func (m *DomainProxyModule) lookupMapping(host string) (*models.PortMapping, error) {
	// 移除端口号（如果有）
	domain := extractDomain(host)

	// 运行时从 deps 动态获取依赖（支持延迟绑定）
	var domainRepo repos.IHTTPDomainMappingRepository
	var registry *httpservice.DomainRegistry
	if m.deps != nil {
		domainRepo = m.deps.HTTPDomainMappingRepo
		registry = m.deps.DomainRegistry
	}

	// 1. 优先使用新的 HTTPDomainMappingRepository（持久化存储，O(1) 查找）
	if domainRepo != nil {
		mapping, err := m.lookupFromRepositoryWithRepo(domain, domainRepo)
		if err == nil {
			return mapping, nil
		}
		// 如果是找不到错误，继续尝试其他途径
		if !coreerrors.IsCode(err, coreerrors.CodeMappingNotFound) {
			return nil, err
		}
		corelog.Debugf("DomainProxyModule: domain not found in repository: %s", domain)
	}

	// 2. 回退到旧的本地注册表（内存缓存，兼容性保留）
	if registry != nil {
		mapping, found := registry.LookupByHost(host)
		if found {
			// 检查映射状态
			if mapping.Status != models.MappingStatusActive {
				return nil, coreerrors.Newf(coreerrors.CodeUnavailable, "mapping is not active: %s", mapping.Status)
			}
			if mapping.IsRevoked {
				return nil, coreerrors.New(coreerrors.CodeForbidden, "mapping has been revoked")
			}
			if mapping.ExpiresAt != nil && time.Now().After(*mapping.ExpiresAt) {
				return nil, coreerrors.New(coreerrors.CodeForbidden, "mapping has expired")
			}
			return mapping, nil
		}
	}

	// 3. 最后尝试从 CloudControl（数据库）查询
	// 这支持跨节点场景：映射在节点 A 创建，请求发到节点 B
	if m.deps != nil && m.deps.CloudControl != nil {
		mapping, err := m.deps.CloudControl.GetPortMappingByDomain(domain)
		if err != nil {
			corelog.Debugf("DomainProxyModule: domain not found in database: %s, err=%v", domain, err)
			return nil, httpservice.ErrDomainNotFound
		}

		// 检查映射状态
		if mapping.Status != models.MappingStatusActive {
			return nil, coreerrors.Newf(coreerrors.CodeUnavailable, "mapping is not active: %s", mapping.Status)
		}
		if mapping.IsRevoked {
			return nil, coreerrors.New(coreerrors.CodeForbidden, "mapping has been revoked")
		}
		if mapping.ExpiresAt != nil && time.Now().After(*mapping.ExpiresAt) {
			return nil, coreerrors.New(coreerrors.CodeForbidden, "mapping has expired")
		}

		// 缓存到本地注册表
		if registry != nil {
			if err := registry.Register(mapping); err != nil {
				corelog.Warnf("DomainProxyModule: failed to cache mapping to local registry: %v", err)
			} else {
				corelog.Infof("DomainProxyModule: cached mapping from database: %s -> client=%d",
					domain, mapping.TargetClientID)
			}
		}

		return mapping, nil
	}

	corelog.Debugf("DomainProxyModule: domain not found: %s", host)
	return nil, httpservice.ErrDomainNotFound
}

// lookupFromRepositoryWithRepo 从持久化仓库查找域名映射
// 使用 O(1) 时间复杂度的域名索引查找
// 参数 repo: 运行时传入的仓库实例（支持延迟绑定）
func (m *DomainProxyModule) lookupFromRepositoryWithRepo(fullDomain string, repo repos.IHTTPDomainMappingRepository) (*models.PortMapping, error) {
	ctx := context.Background()

	// 从 Repository 获取 HTTPDomainMapping
	httpMapping, err := repo.LookupByDomain(ctx, fullDomain)
	if err != nil {
		return nil, err
	}

	// 检查映射状态
	if !httpMapping.IsActive() {
		if httpMapping.IsExpired() {
			return nil, coreerrors.New(coreerrors.CodeForbidden, "mapping has expired")
		}
		return nil, coreerrors.Newf(coreerrors.CodeUnavailable, "mapping is not active: %s", httpMapping.Status)
	}

	// 转换为 PortMapping（保持与现有代理逻辑的兼容性）
	portMapping := convertHTTPDomainMappingToPortMapping(httpMapping)

	corelog.Debugf("DomainProxyModule: found mapping in repository: %s -> client=%d, target=%s:%d",
		fullDomain, portMapping.TargetClientID, portMapping.TargetHost, portMapping.TargetPort)

	return portMapping, nil
}

// convertHTTPDomainMappingToPortMapping 将 HTTPDomainMapping 转换为 PortMapping
// 用于保持与现有代理逻辑的兼容性
func convertHTTPDomainMappingToPortMapping(httpMapping *repos.HTTPDomainMapping) *models.PortMapping {
	var expiresAt *time.Time
	if httpMapping.ExpiresAt > 0 {
		t := time.Unix(httpMapping.ExpiresAt, 0)
		expiresAt = &t
	}

	return &models.PortMapping{
		ID:             httpMapping.ID,
		TargetClientID: httpMapping.ClientID,
		TargetHost:     httpMapping.TargetHost,
		TargetPort:     httpMapping.TargetPort,
		Protocol:       models.ProtocolHTTP,
		HTTPSubdomain:  httpMapping.Subdomain,
		HTTPBaseDomain: httpMapping.BaseDomain,
		Status:         convertHTTPDomainStatus(httpMapping.Status),
		ExpiresAt:      expiresAt,
		CreatedAt:      time.Unix(httpMapping.CreatedAt, 0),
		UpdatedAt:      time.Unix(httpMapping.UpdatedAt, 0),
		Description:    httpMapping.Description,
	}
}

// convertHTTPDomainStatus 将 HTTPDomainMappingStatus 转换为 MappingStatus
func convertHTTPDomainStatus(status repos.HTTPDomainMappingStatus) models.MappingStatus {
	switch status {
	case repos.HTTPDomainMappingStatusActive:
		return models.MappingStatusActive
	case repos.HTTPDomainMappingStatusInactive:
		return models.MappingStatusInactive
	case repos.HTTPDomainMappingStatusExpired:
		return models.MappingStatusError
	default:
		return models.MappingStatusInactive
	}
}

// extractDomain 从 host 中提取域名（移除端口号）
func extractDomain(host string) string {
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}
