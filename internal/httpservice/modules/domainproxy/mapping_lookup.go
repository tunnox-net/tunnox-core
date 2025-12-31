// Package domainproxy 提供 HTTP 域名代理功能
package domainproxy

import (
	"time"

	"tunnox-core/internal/cloud/models"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
)

// lookupMapping 查找域名映射
func (m *DomainProxyModule) lookupMapping(host string) (*models.PortMapping, error) {
	// 1. 先从本地注册表查找
	if m.registry != nil {
		mapping, found := m.registry.LookupByHost(host)
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

	// 2. 本地找不到，从 CloudControl（数据库）查询
	// 这支持跨节点场景：映射在节点 A 创建，请求发到节点 B
	if m.deps != nil && m.deps.CloudControl != nil {
		// 移除端口号（如果有）
		domain := host
		for i := len(host) - 1; i >= 0; i-- {
			if host[i] == ':' {
				domain = host[:i]
				break
			}
		}

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
		if m.registry != nil {
			if err := m.registry.Register(mapping); err != nil {
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
