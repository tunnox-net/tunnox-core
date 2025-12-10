package constants

import "time"

// TTL (Time To Live) Constants
// TTL 常量定义，用于存储层的数据过期时间管理
// 这些常量是跨层共享的，kernel 层和 cloud 层都可以使用

// ============================================================================
// 默认 TTL 常量
// ============================================================================

const (
	// DefaultDataTTL 默认数据过期时间（24小时）
	// 用于一般性数据的默认过期时间
	DefaultDataTTL = 24 * time.Hour

	// DefaultUserDataTTL 用户数据过期时间（0 表示永不过期）
	// 用户数据需要持久保存，不应自动过期
	DefaultUserDataTTL = 0

	// DefaultClientDataTTL 客户端数据过期时间（24小时）
	// 客户端配置数据的默认过期时间
	DefaultClientDataTTL = 24 * time.Hour

	// DefaultMappingDataTTL 端口映射数据过期时间（24小时）
	// 端口映射配置的默认过期时间
	DefaultMappingDataTTL = 24 * time.Hour

	// DefaultNodeDataTTL 节点数据过期时间（24小时）
	// 节点信息的默认过期时间
	DefaultNodeDataTTL = 24 * time.Hour

	// DefaultConnectionTTL 连接数据过期时间（24小时）
	// 连接记录的默认过期时间
	DefaultConnectionTTL = 24 * time.Hour

	// DefaultSessionTTL 会话数据过期时间（1小时）
	// 会话数据的默认过期时间
	DefaultSessionTTL = 1 * time.Hour

	// DefaultCacheTTL 缓存数据过期时间（5分钟）
	// 临时缓存数据的默认过期时间
	DefaultCacheTTL = 5 * time.Minute
)

// ============================================================================
// 清理相关常量
// ============================================================================

const (
	// DefaultCleanupInterval 默认清理间隔（5分钟）
	// 用于定期清理过期数据的时间间隔
	DefaultCleanupInterval = 5 * time.Minute
)
