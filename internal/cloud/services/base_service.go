package services

import (
	"context"

	"tunnox-core/internal/cloud/services/base"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/storage"
)

// BaseService 基础服务结构，提供通用的错误处理工具
// 向后兼容：重新导出 base.Service
type BaseService = base.Service

// NewBaseService 创建基础服务实例
func NewBaseService() *BaseService {
	return base.NewService()
}

// StatsProvider 统计数据提供者接口
// 向后兼容：重新导出 base.StatsProvider
type StatsProvider = base.StatsProvider

// NewSimpleStatsProvider 创建简化版统计提供者
func NewSimpleStatsProvider(stor storage.Storage, parentCtx context.Context) (StatsProvider, error) {
	return base.NewSimpleStatsProvider(stor, parentCtx)
}

// JWTProvider JWT令牌提供者接口
// 向后兼容：重新导出 base.JWTProvider
type JWTProvider = base.JWTProvider

// JWTTokenResult JWT令牌生成结果接口
type JWTTokenResult = base.JWTTokenResult

// JWTClaimsResult JWT声明结果接口
type JWTClaimsResult = base.JWTClaimsResult

// RefreshTokenClaimsResult 刷新Token声明结果接口
type RefreshTokenClaimsResult = base.RefreshTokenClaimsResult

// GetStatsCounter 从统计提供者获取计数器
func GetStatsCounter(provider StatsProvider) *stats.StatsCounter {
	return provider.GetCounter()
}
