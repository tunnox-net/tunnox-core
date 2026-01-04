package quota

import (
	"context"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
)

// DefaultQuotaChecker 默认配额检查器（单节点模式）
// 返回无限配额，不限制用户操作
// 当 platform.enabled=false 时使用此实现
type DefaultQuotaChecker struct {
	*dispose.ServiceBase
}

// NewDefaultQuotaChecker 创建默认配额检查器
func NewDefaultQuotaChecker(parentCtx context.Context) *DefaultQuotaChecker {
	return &DefaultQuotaChecker{
		ServiceBase: dispose.NewService("DefaultQuotaChecker", parentCtx),
	}
}

// CheckMappingQuota 单节点模式始终允许创建映射
func (c *DefaultQuotaChecker) CheckMappingQuota(userID string, protocol models.Protocol) error {
	// 单节点模式不限制配额
	return nil
}

// GetUserQuota 返回无限配额
// 0 表示无限制
func (c *DefaultQuotaChecker) GetUserQuota(userID string) (*models.UserQuota, error) {
	return &models.UserQuota{
		MaxClientIDs:        0, // 0 = 无限制
		MaxConnections:      0,
		BandwidthLimit:      0,
		StorageLimit:        0,
		MonthlyTrafficLimit: 0,
		MonthlyTrafficUsed:  0,
		MonthlyResetDay:     1,
		MaxMappings:         0,
		MaxHTTPDomains:      0,
	}, nil
}

// GetUserUsage 返回空使用量
func (c *DefaultQuotaChecker) GetUserUsage(userID string) (*MappingUsage, error) {
	return &MappingUsage{
		TotalMappings:  0,
		HTTPMappings:   0,
		ActiveConns:    0,
		MonthlyTraffic: 0,
	}, nil
}

// 确保实现接口
var _ QuotaChecker = (*DefaultQuotaChecker)(nil)
