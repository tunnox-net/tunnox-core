package quota

import (
	"context"

	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/config/schema"
)

// NewQuotaChecker 创建配额检查器
// 根据配置选择合适的实现：
// - platform.enabled=true: 使用 RemoteQuotaChecker（调用 Platform API）
// - platform.enabled=false: 使用 DefaultQuotaChecker（无限配额）
func NewQuotaChecker(
	config *schema.PlatformConfig,
	platformClient PlatformClient,
	mappingRepo repos.IPortMappingRepository,
	parentCtx context.Context,
) QuotaChecker {
	if config == nil || !config.Enabled {
		// 单节点模式：使用默认配额检查器（无限制）
		return NewDefaultQuotaChecker(parentCtx)
	}

	// 云服务模式：使用远程配额检查器
	return NewRemoteQuotaChecker(platformClient, mappingRepo, parentCtx)
}

// MustNewQuotaChecker 创建配额检查器（单节点模式快捷方法）
// 用于不需要 Platform 集成的场景
func MustNewQuotaChecker(parentCtx context.Context) QuotaChecker {
	return NewDefaultQuotaChecker(parentCtx)
}
