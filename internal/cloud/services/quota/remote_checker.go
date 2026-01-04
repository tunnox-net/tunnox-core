package quota

import (
	"context"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// PlatformClient 平台客户端接口
// 用于调用 Platform API 获取用户配额信息
type PlatformClient interface {
	// GetUserQuota 获取用户配额
	// 返回用户在 Platform 中配置的配额限制
	GetUserQuota(userID string) (*models.UserQuota, error)
}

// RemoteQuotaChecker 远程配额检查器（云服务模式）
// 通过调用 Platform API 获取用户配额，并结合本地映射数据计算使用量
type RemoteQuotaChecker struct {
	*dispose.ServiceBase
	platformClient PlatformClient
	mappingRepo    repos.IPortMappingRepository
	cache          *QuotaCache
}

// NewRemoteQuotaChecker 创建远程配额检查器
func NewRemoteQuotaChecker(
	platformClient PlatformClient,
	mappingRepo repos.IPortMappingRepository,
	parentCtx context.Context,
) *RemoteQuotaChecker {
	checker := &RemoteQuotaChecker{
		ServiceBase:    dispose.NewService("RemoteQuotaChecker", parentCtx),
		platformClient: platformClient,
		mappingRepo:    mappingRepo,
		cache:          NewQuotaCache(5 * time.Minute),
	}

	return checker
}

// CheckMappingQuota 检查是否可以创建新映射
// 返回 nil 表示配额充足，返回 error 表示超限
func (c *RemoteQuotaChecker) CheckMappingQuota(userID string, protocol models.Protocol) error {
	// 匿名用户不受配额限制
	if userID == "" {
		return nil
	}

	// 1. 获取用户配额（带缓存）
	quota, err := c.GetUserQuota(userID)
	if err != nil {
		// Platform 不可用时，降级策略：允许创建但记录日志
		corelog.Warnf("QuotaChecker: failed to get quota for user %s, allowing: %v", userID, err)
		return nil
	}

	// 2. 获取当前使用量
	usage, err := c.GetUserUsage(userID)
	if err != nil {
		corelog.Warnf("QuotaChecker: failed to get usage for user %s, allowing: %v", userID, err)
		return nil
	}

	// 3. 校验总隧道数（0 表示无限制）
	if quota.MaxMappings > 0 && usage.TotalMappings >= quota.MaxMappings {
		return coreerrors.Newf(coreerrors.CodeQuotaExceeded,
			"tunnel quota exceeded: %d/%d", usage.TotalMappings, quota.MaxMappings)
	}

	// 4. 校验 HTTP 域名数（仅 HTTP 协议，0 表示无限制）
	if protocol == models.ProtocolHTTP {
		if quota.MaxHTTPDomains > 0 && usage.HTTPMappings >= quota.MaxHTTPDomains {
			return coreerrors.Newf(coreerrors.CodeQuotaExceeded,
				"HTTP domain quota exceeded: %d/%d", usage.HTTPMappings, quota.MaxHTTPDomains)
		}
	}

	return nil
}

// GetUserQuota 获取用户配额信息
// 优先从缓存获取，缓存未命中时调用 Platform API
func (c *RemoteQuotaChecker) GetUserQuota(userID string) (*models.UserQuota, error) {
	// 匿名用户返回无限配额
	if userID == "" {
		return &models.UserQuota{
			MaxMappings:    0, // 0 = 无限制
			MaxHTTPDomains: 0,
		}, nil
	}

	// 检查缓存
	if cached, ok := c.cache.Get(userID); ok {
		return cached, nil
	}

	// 调用 Platform API
	quota, err := c.platformClient.GetUserQuota(userID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to get quota from platform")
	}

	// 更新缓存
	c.cache.Set(userID, quota)
	return quota, nil
}

// GetUserUsage 获取用户当前使用量
// 通过查询本地映射数据计算
func (c *RemoteQuotaChecker) GetUserUsage(userID string) (*MappingUsage, error) {
	// 匿名用户返回空使用量
	if userID == "" {
		return &MappingUsage{}, nil
	}

	mappings, err := c.mappingRepo.GetUserPortMappings(userID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get user mappings")
	}

	usage := &MappingUsage{
		TotalMappings: len(mappings),
	}

	for _, m := range mappings {
		if m.Protocol == models.ProtocolHTTP {
			usage.HTTPMappings++
		}
	}

	return usage, nil
}

// InvalidateCache 使用户配额缓存失效
// 当用户套餐变更时调用
func (c *RemoteQuotaChecker) InvalidateCache(userID string) {
	c.cache.Invalidate(userID)
}

// RefreshQuota 刷新用户配额缓存
// 强制从 Platform 重新获取配额
func (c *RemoteQuotaChecker) RefreshQuota(userID string) (*models.UserQuota, error) {
	c.cache.Invalidate(userID)
	return c.GetUserQuota(userID)
}

// 确保实现接口
var _ QuotaChecker = (*RemoteQuotaChecker)(nil)
