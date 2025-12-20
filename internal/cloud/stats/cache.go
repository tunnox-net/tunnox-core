package stats

import (
	"sync"
	"time"
)

// StatsCache 本地统计缓存
// 用于减少对存储层的访问频率
type StatsCache struct {
	data      *SystemStats
	expiresAt time.Time
	ttl       time.Duration
	mu        sync.RWMutex
}

// NewStatsCache 创建统计缓存
func NewStatsCache(ttl time.Duration) *StatsCache {
	return &StatsCache{
		ttl: ttl,
	}
}

// Get 获取缓存的统计数据
// 如果缓存过期或不存在，返回nil
func (c *StatsCache) Get() *SystemStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.data != nil && time.Now().Before(c.expiresAt) {
		return c.data
	}
	return nil
}

// Set 设置缓存的统计数据
func (c *StatsCache) Set(stats *SystemStats) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = stats
	c.expiresAt = time.Now().Add(c.ttl)
}

// Invalidate 使缓存失效
func (c *StatsCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = nil
}

// IsValid 检查缓存是否有效
func (c *StatsCache) IsValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.data != nil && time.Now().Before(c.expiresAt)
}
