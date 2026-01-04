package quota

import (
	"sync"
	"time"

	"tunnox-core/internal/cloud/models"
)

// QuotaCache 配额缓存
// 缓存用户配额信息，减少对 Platform 的调用次数
type QuotaCache struct {
	mu    sync.RWMutex
	items map[string]*cacheItem
	ttl   time.Duration
}

type cacheItem struct {
	quota     *models.UserQuota
	expiresAt time.Time
}

// NewQuotaCache 创建配额缓存
// ttl: 缓存有效期
func NewQuotaCache(ttl time.Duration) *QuotaCache {
	cache := &QuotaCache{
		items: make(map[string]*cacheItem),
		ttl:   ttl,
	}

	// 启动后台清理过期项
	go cache.cleanupLoop()

	return cache
}

// Get 获取缓存的配额
// 返回 (配额, 是否存在且未过期)
func (c *QuotaCache) Get(userID string) (*models.UserQuota, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[userID]
	if !exists {
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(item.expiresAt) {
		return nil, false
	}

	return item.quota, true
}

// Set 设置缓存
func (c *QuotaCache) Set(userID string, quota *models.UserQuota) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[userID] = &cacheItem{
		quota:     quota,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// SetWithTTL 设置缓存（自定义 TTL）
// 用于降级场景延长缓存时间
func (c *QuotaCache) SetWithTTL(userID string, quota *models.UserQuota, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[userID] = &cacheItem{
		quota:     quota,
		expiresAt: time.Now().Add(ttl),
	}
}

// Invalidate 使缓存失效
func (c *QuotaCache) Invalidate(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.items, userID)
}

// Clear 清空所有缓存
func (c *QuotaCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheItem)
}

// cleanupLoop 后台清理过期缓存
func (c *QuotaCache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup 清理过期项
func (c *QuotaCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for userID, item := range c.items {
		if now.After(item.expiresAt) {
			delete(c.items, userID)
		}
	}
}

// Stats 返回缓存统计
func (c *QuotaCache) Stats() (total, expired int) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	total = len(c.items)
	for _, item := range c.items {
		if now.After(item.expiresAt) {
			expired++
		}
	}
	return total, expired
}
