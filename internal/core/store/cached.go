package store

import (
	"context"
	"time"
)

// =============================================================================
// 缓存组合存储接口
// =============================================================================

// CachedPersistentStore 缓存 + 持久化组合存储
// 提供 Read-Through/Write-Through 缓存策略
type CachedPersistentStore[K comparable, V any] interface {
	Store[K, V]
	BatchStore[K, V]
	Closer

	// InvalidateCache 使缓存失效
	InvalidateCache(ctx context.Context, key K) error

	// RefreshCache 刷新缓存（从持久化层重新加载）
	RefreshCache(ctx context.Context, key K) error

	// GetFromPersistent 直接从持久化层获取（绕过缓存）
	GetFromPersistent(ctx context.Context, key K) (V, error)

	// GetCacheStats 获取缓存统计信息
	GetCacheStats() CacheStats
}

// CacheStats 缓存统计
type CacheStats struct {
	Hits              int64   // 缓存命中次数
	Misses            int64   // 缓存未命中次数
	NegativeHits      int64   // 负缓存命中次数
	BloomFilterRejects int64  // 布隆过滤器拒绝次数
	HitRate           float64 // 命中率
}

// =============================================================================
// 缓存配置
// =============================================================================

// CacheConfig 缓存配置
type CacheConfig struct {
	// TTL 缓存 TTL，默认 30 分钟
	TTL time.Duration

	// NegativeTTL 负缓存 TTL，默认 5 分钟
	// 用于缓存 "不存在" 的结果，防止缓存穿透
	NegativeTTL time.Duration

	// WritePolicy 写入策略，默认 WriteThrough
	WritePolicy WritePolicy

	// LoadOnMiss 缓存 miss 时自动从持久化层加载，默认 true
	LoadOnMiss bool

	// PenetrationProtection 启用穿透保护（负缓存 + 布隆过滤器），默认 true
	PenetrationProtection bool

	// MaxNegativeCacheSize 负缓存最大数量，默认 10000
	// 超过此数量时使用 LRU 淘汰
	MaxNegativeCacheSize int

	// BloomFilterSize 布隆过滤器大小，0 表示不启用
	// 建议设置为预期最大数据量的 10 倍
	BloomFilterSize int

	// BloomFilterFPRate 布隆过滤器误判率，默认 0.01 (1%)
	BloomFilterFPRate float64
}

// WritePolicy 写入策略
type WritePolicy int

const (
	// WriteThrough 同时写缓存和持久化
	// 优点：数据一致性好
	// 缺点：写入延迟较高
	WriteThrough WritePolicy = iota

	// WriteBehind 先写持久化，异步更新缓存
	// 优点：写入延迟低
	// 缺点：短暂不一致窗口
	WriteBehind
)

// String 返回写入策略名称
func (p WritePolicy) String() string {
	switch p {
	case WriteThrough:
		return "write_through"
	case WriteBehind:
		return "write_behind"
	default:
		return "unknown"
	}
}

// DefaultCacheConfig 默认缓存配置
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:                   30 * time.Minute,
		NegativeTTL:           5 * time.Minute,
		WritePolicy:           WriteThrough,
		LoadOnMiss:            true,
		PenetrationProtection: true,
		MaxNegativeCacheSize:  10000,
		BloomFilterSize:       0, // 默认不启用
		BloomFilterFPRate:     0.01,
	}
}

// Validate 验证配置
func (c *CacheConfig) Validate() error {
	if c.TTL <= 0 {
		c.TTL = 30 * time.Minute
	}
	if c.NegativeTTL <= 0 {
		c.NegativeTTL = 5 * time.Minute
	}
	if c.MaxNegativeCacheSize <= 0 {
		c.MaxNegativeCacheSize = 10000
	}
	if c.BloomFilterFPRate <= 0 || c.BloomFilterFPRate >= 1 {
		c.BloomFilterFPRate = 0.01
	}
	return nil
}

// WithTTL 设置 TTL
func (c CacheConfig) WithTTL(ttl time.Duration) CacheConfig {
	c.TTL = ttl
	return c
}

// WithNegativeTTL 设置负缓存 TTL
func (c CacheConfig) WithNegativeTTL(ttl time.Duration) CacheConfig {
	c.NegativeTTL = ttl
	return c
}

// WithWritePolicy 设置写入策略
func (c CacheConfig) WithWritePolicy(policy WritePolicy) CacheConfig {
	c.WritePolicy = policy
	return c
}

// WithPenetrationProtection 设置穿透保护
func (c CacheConfig) WithPenetrationProtection(enabled bool) CacheConfig {
	c.PenetrationProtection = enabled
	return c
}

// WithBloomFilter 启用布隆过滤器
func (c CacheConfig) WithBloomFilter(size int, fpRate float64) CacheConfig {
	c.BloomFilterSize = size
	c.BloomFilterFPRate = fpRate
	return c
}
