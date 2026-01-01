package server

import (
	"context"
	"time"

	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/command"
	corelog "tunnox-core/internal/core/log"
)

// HTTPDomainRepositoryAdapter 将 IHTTPDomainMappingRepository 适配到 command 包的接口
//
// 实现以下接口：
//   - command.SubdomainChecker
//   - command.HTTPDomainCreator
//   - command.HTTPDomainLister
//   - command.HTTPDomainDeleter
type HTTPDomainRepositoryAdapter struct {
	repo repos.IHTTPDomainMappingRepository
}

// 编译时接口断言
var (
	_ command.SubdomainChecker  = (*HTTPDomainRepositoryAdapter)(nil)
	_ command.HTTPDomainCreator = (*HTTPDomainRepositoryAdapter)(nil)
	_ command.HTTPDomainLister  = (*HTTPDomainRepositoryAdapter)(nil)
	_ command.HTTPDomainDeleter = (*HTTPDomainRepositoryAdapter)(nil)
)

// NewHTTPDomainRepositoryAdapter 创建适配器
func NewHTTPDomainRepositoryAdapter(repo repos.IHTTPDomainMappingRepository) *HTTPDomainRepositoryAdapter {
	return &HTTPDomainRepositoryAdapter{repo: repo}
}

// IsSubdomainAvailable 检查子域名是否可用（实现 SubdomainChecker 接口）
func (a *HTTPDomainRepositoryAdapter) IsSubdomainAvailable(subdomain, baseDomain string) bool {
	ctx := context.Background()
	available, err := a.repo.CheckSubdomainAvailable(ctx, subdomain, baseDomain)
	if err != nil {
		corelog.Warnf("HTTPDomainRepositoryAdapter.IsSubdomainAvailable: check failed: %v", err)
		return false // 出错时返回不可用，避免冲突
	}
	return available
}

// IsBaseDomainAllowed 检查基础域名是否允许（实现 SubdomainChecker 接口）
func (a *HTTPDomainRepositoryAdapter) IsBaseDomainAllowed(baseDomain string) bool {
	baseDomains := a.repo.GetBaseDomains()
	for _, bd := range baseDomains {
		if bd == baseDomain {
			return true
		}
	}
	// 如果列表为空，允许所有域名（测试模式）
	return len(baseDomains) == 0
}

// CreateHTTPDomainMapping 创建 HTTP 域名映射（实现 HTTPDomainCreator 接口）
func (a *HTTPDomainRepositoryAdapter) CreateHTTPDomainMapping(
	clientID int64,
	targetHost string,
	targetPort int,
	subdomain, baseDomain, description string,
	ttlSeconds int,
) (mappingID, fullDomain, expiresAt string, err error) {
	ctx := context.Background()

	mapping, err := a.repo.CreateMapping(ctx, clientID, subdomain, baseDomain, targetHost, targetPort)
	if err != nil {
		return "", "", "", err
	}

	// 如果有 TTL，需要更新过期时间
	if ttlSeconds > 0 {
		mapping.ExpiresAt = time.Now().Unix() + int64(ttlSeconds)
		mapping.Description = description
		if updateErr := a.repo.UpdateMapping(ctx, mapping); updateErr != nil {
			corelog.Warnf("HTTPDomainRepositoryAdapter.CreateHTTPDomainMapping: failed to update TTL: %v", updateErr)
			// 不影响创建成功，继续返回
		}
	} else if description != "" {
		// 只更新描述
		mapping.Description = description
		if updateErr := a.repo.UpdateMapping(ctx, mapping); updateErr != nil {
			corelog.Warnf("HTTPDomainRepositoryAdapter.CreateHTTPDomainMapping: failed to update description: %v", updateErr)
		}
	}

	// 格式化过期时间
	if mapping.ExpiresAt > 0 {
		expiresAt = time.Unix(mapping.ExpiresAt, 0).Format(time.RFC3339)
	}

	corelog.Infof("HTTPDomainRepositoryAdapter: created mapping %s: %s -> %s:%d",
		mapping.ID, mapping.FullDomain, targetHost, targetPort)

	return mapping.ID, mapping.FullDomain, expiresAt, nil
}

// ListHTTPDomainMappings 列出客户端的 HTTP 域名映射（实现 HTTPDomainLister 接口）
func (a *HTTPDomainRepositoryAdapter) ListHTTPDomainMappings(clientID int64) ([]command.HTTPDomainMappingInfo, error) {
	ctx := context.Background()

	mappings, err := a.repo.GetMappingsByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}

	result := make([]command.HTTPDomainMappingInfo, 0, len(mappings))
	for _, m := range mappings {
		expiresAt := ""
		if m.ExpiresAt > 0 {
			expiresAt = time.Unix(m.ExpiresAt, 0).Format(time.RFC3339)
		}

		result = append(result, command.HTTPDomainMappingInfo{
			MappingID:  m.ID,
			FullDomain: m.FullDomain,
			TargetURL:  m.TargetURL(),
			Status:     string(m.Status),
			CreatedAt:  time.Unix(m.CreatedAt, 0).Format(time.RFC3339),
			ExpiresAt:  expiresAt,
		})
	}

	return result, nil
}

// DeleteHTTPDomainMapping 删除 HTTP 域名映射（实现 HTTPDomainDeleter 接口）
func (a *HTTPDomainRepositoryAdapter) DeleteHTTPDomainMapping(clientID int64, mappingID string) error {
	ctx := context.Background()
	err := a.repo.DeleteMapping(ctx, mappingID, clientID)
	if err != nil {
		return err
	}

	corelog.Infof("HTTPDomainRepositoryAdapter: deleted mapping %s", mappingID)
	return nil
}

// GetMapping 根据完整域名获取映射（用于 HTTP 代理）
func (a *HTTPDomainRepositoryAdapter) GetMapping(fullDomain string) *repos.HTTPDomainMapping {
	ctx := context.Background()
	mapping, err := a.repo.LookupByDomain(ctx, fullDomain)
	if err != nil {
		return nil
	}
	return mapping
}
