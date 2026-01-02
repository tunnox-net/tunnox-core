package store

import "context"

// =============================================================================
// 特化存储接口
// =============================================================================

// SharedStore 共享存储接口（Redis/miniredis）
// 用于跨节点共享的数据存储，如缓存、会话、索引等
type SharedStore[K comparable, V any] interface {
	TTLStore[K, V]
	BatchStore[K, V]
	AtomicStore[K, V]
	HealthChecker
	Closer

	// Pipeline 返回管道用于批量操作
	Pipeline() Pipeline[K, V]
}

// PersistentStore 持久化存储接口（gRPC/PostgreSQL）
// 用于需要持久化的数据，如用户配置、客户端配置等
type PersistentStore[K comparable, V any] interface {
	Store[K, V]
	BatchStore[K, V]
	HealthChecker
	Closer

	// List 列出所有键值对（用于索引重建）
	// prefix 为空时列出所有，否则按前缀过滤
	List(ctx context.Context, prefix string) (map[K]V, error)
}

// MemoryStore 内存存储接口
// 用于本地临时数据，如连接状态、活跃会话等
type MemoryStore[K comparable, V any] interface {
	TTLStore[K, V]
	BatchStore[K, V]
	AtomicStore[K, V]
	Closer

	// Clear 清空所有数据
	Clear(ctx context.Context) error

	// Keys 获取所有键（按前缀过滤）
	Keys(ctx context.Context, pattern string) ([]K, error)
}

// =============================================================================
// 存储类型标识
// =============================================================================

// StoreType 存储类型
type StoreType string

const (
	// StoreTypeMemory 内存存储
	StoreTypeMemory StoreType = "memory"

	// StoreTypeRedis Redis 存储
	StoreTypeRedis StoreType = "redis"

	// StoreTypeEmbedded 内嵌 Redis (miniredis)
	StoreTypeEmbedded StoreType = "embedded"

	// StoreTypeGRPC gRPC 持久化存储
	StoreTypeGRPC StoreType = "grpc"

	// StoreTypeJSON JSON 文件存储
	StoreTypeJSON StoreType = "json"
)
