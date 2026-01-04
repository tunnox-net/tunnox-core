package quota

import (
	"tunnox-core/internal/cloud/models"
)

// QuotaChecker 配额检查器接口
// 用于在创建隧道时检查用户配额是否充足
type QuotaChecker interface {
	// CheckMappingQuota 检查是否可以创建新映射
	// userID: 用户ID（空字符串表示匿名用户，不受配额限制）
	// protocol: 映射协议（用于 HTTP 域名数单独检查）
	// 返回 nil 表示配额充足，返回 error 表示超限
	CheckMappingQuota(userID string, protocol models.Protocol) error

	// GetUserQuota 获取用户配额信息
	// 返回用户的配额限制
	GetUserQuota(userID string) (*models.UserQuota, error)

	// GetUserUsage 获取用户当前使用量
	// 返回用户当前的资源使用情况
	GetUserUsage(userID string) (*MappingUsage, error)
}

// MappingUsage 映射使用量统计
type MappingUsage struct {
	TotalMappings  int   // 总映射数
	HTTPMappings   int   // HTTP 映射数（用于 HTTP 域名配额检查）
	ActiveConns    int   // 活跃连接数
	MonthlyTraffic int64 // 当月流量（字节）
}
