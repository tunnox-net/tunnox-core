package security

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"sync"
	"time"
	
	"tunnox-core/internal/core/dispose"
)

// RateLimiter 速率限制器
//
// 职责：
//   - 基于Token Bucket算法的速率限制
//   - 支持IP级别和隧道级别的速率限制
//   - 自动清理过期的bucket
//
// 设计：
//   - 使用Token Bucket算法（令牌桶）
//   - 分层限制：IP层（防止单IP滥用）和隧道层（防止带宽滥用）
type RateLimiter struct {
	*dispose.ServiceBase
	
	// IP级别限制（用于匿名客户端连接速率）
	ipBuckets map[string]*TokenBucket
	ipMu      sync.RWMutex
	ipConfig  *RateLimitConfig
	
	// 隧道级别限制（用于带宽控制）
	tunnelBuckets map[string]*TokenBucket
	tunnelMu      sync.RWMutex
	tunnelConfig  *RateLimitConfig
}

// RateLimitConfig 速率限制配置
type RateLimitConfig struct {
	Rate      int           // 速率（每秒令牌数）
	Burst     int           // 突发容量（桶大小）
	TTL       time.Duration // Bucket过期时间
}

// TokenBucket 令牌桶
type TokenBucket struct {
	tokens    float64   // 当前令牌数
	capacity  float64   // 桶容量
	rate      float64   // 填充速率（每秒）
	lastRefill time.Time // 上次填充时间
	mu        sync.Mutex
}

// DefaultIPRateLimitConfig 默认IP速率限制配置
func DefaultIPRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Rate:  10,            // 每秒10个连接
		Burst: 20,            // 最多突发20个
		TTL:   5 * time.Minute, // 5分钟后清理
	}
}

// DefaultTunnelRateLimitConfig 默认隧道速率限制配置
func DefaultTunnelRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Rate:  1024 * 1024,     // 1MB/s
		Burst: 10 * 1024 * 1024, // 最多突发10MB
		TTL:   10 * time.Minute,  // 10分钟后清理
	}
}

// NewRateLimiter 创建速率限制器
func NewRateLimiter(ipConfig *RateLimitConfig, tunnelConfig *RateLimitConfig, ctx context.Context) *RateLimiter {
	if ipConfig == nil {
		ipConfig = DefaultIPRateLimitConfig()
	}
	if tunnelConfig == nil {
		tunnelConfig = DefaultTunnelRateLimitConfig()
	}
	
	limiter := &RateLimiter{
		ServiceBase:   dispose.NewService("RateLimiter", ctx),
		ipBuckets:     make(map[string]*TokenBucket),
		tunnelBuckets: make(map[string]*TokenBucket),
		ipConfig:      ipConfig,
		tunnelConfig:  tunnelConfig,
	}
	
	// 启动后台清理任务
	go limiter.cleanupTask(ctx)
	
	return limiter
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// IP级别速率限制
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// AllowIP 检查IP是否允许连接（用于匿名客户端）
func (r *RateLimiter) AllowIP(ip string) bool {
	return r.allow(ip, 1, &r.ipMu, r.ipBuckets, r.ipConfig)
}

// AllowIPBurst 检查IP是否允许突发连接
func (r *RateLimiter) AllowIPBurst(ip string, n int) bool {
	return r.allow(ip, n, &r.ipMu, r.ipBuckets, r.ipConfig)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 隧道级别速率限制
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// AllowTunnel 检查隧道是否允许传输（用于带宽控制）
func (r *RateLimiter) AllowTunnel(tunnelID string, bytes int) bool {
	return r.allow(tunnelID, bytes, &r.tunnelMu, r.tunnelBuckets, r.tunnelConfig)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 核心逻辑
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// allow 通用速率限制检查
func (r *RateLimiter) allow(key string, tokens int, mu *sync.RWMutex, buckets map[string]*TokenBucket, config *RateLimitConfig) bool {
	// 获取或创建bucket
	mu.RLock()
	bucket, exists := buckets[key]
	mu.RUnlock()
	
	if !exists {
		mu.Lock()
		// 双重检查
		bucket, exists = buckets[key]
		if !exists {
			bucket = newTokenBucket(config.Rate, config.Burst)
			buckets[key] = bucket
		}
		mu.Unlock()
	}
	
	return bucket.Take(tokens)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 清理任务
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cleanupTask 后台清理任务
func (r *RateLimiter) cleanupTask(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			corelog.Infof("RateLimiter: cleanup task stopped")
			return
		case <-ticker.C:
			r.cleanup()
		}
	}
}

// cleanup 清理长时间未使用的bucket
func (r *RateLimiter) cleanup() {
	now := time.Now()
	
	// 清理IP buckets
	r.ipMu.Lock()
	for key, bucket := range r.ipBuckets {
		bucket.mu.Lock()
		if now.Sub(bucket.lastRefill) > r.ipConfig.TTL {
			delete(r.ipBuckets, key)
		}
		bucket.mu.Unlock()
	}
	r.ipMu.Unlock()
	
	// 清理隧道buckets
	r.tunnelMu.Lock()
	for key, bucket := range r.tunnelBuckets {
		bucket.mu.Lock()
		if now.Sub(bucket.lastRefill) > r.tunnelConfig.TTL {
			delete(r.tunnelBuckets, key)
		}
		bucket.mu.Unlock()
	}
	r.tunnelMu.Unlock()
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TokenBucket 实现
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// newTokenBucket 创建令牌桶
func newTokenBucket(rate int, capacity int) *TokenBucket {
	return &TokenBucket{
		tokens:    float64(capacity), // 初始填满
		capacity:  float64(capacity),
		rate:      float64(rate),
		lastRefill: time.Now(),
	}
}

// Take 尝试消耗令牌
func (b *TokenBucket) Take(n int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// 填充令牌
	b.refill()
	
	// 检查是否有足够的令牌
	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}
	
	return false
}

// refill 填充令牌（调用者需持有锁）
func (b *TokenBucket) refill() {
	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	
	// 根据时间填充令牌
	tokensToAdd := elapsed * b.rate
	b.tokens = min(b.tokens+tokensToAdd, b.capacity)
	b.lastRefill = now
}

// min 返回较小值
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 查询和统计
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetStats 获取统计信息
func (r *RateLimiter) GetStats() *RateLimiterStats {
	r.ipMu.RLock()
	r.tunnelMu.RLock()
	defer r.ipMu.RUnlock()
	defer r.tunnelMu.RUnlock()
	
	return &RateLimiterStats{
		IPBucketCount:     len(r.ipBuckets),
		TunnelBucketCount: len(r.tunnelBuckets),
	}
}

// RateLimiterStats 统计信息
type RateLimiterStats struct {
	IPBucketCount     int
	TunnelBucketCount int
}

// Reset 重置所有bucket（仅用于测试）
func (r *RateLimiter) Reset() {
	r.ipMu.Lock()
	r.tunnelMu.Lock()
	defer r.ipMu.Unlock()
	defer r.tunnelMu.Unlock()
	
	r.ipBuckets = make(map[string]*TokenBucket)
	r.tunnelBuckets = make(map[string]*TokenBucket)
	
	corelog.Infof("RateLimiter: all buckets reset")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// WaitIP 等待直到IP允许连接（阻塞式）
func (r *RateLimiter) WaitIP(ctx context.Context, ip string) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		if r.AllowIP(ip) {
			return nil
		}
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			continue
		}
	}
}

// WaitTunnel 等待直到隧道允许传输（阻塞式）
func (r *RateLimiter) WaitTunnel(ctx context.Context, tunnelID string, bytes int) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		if r.AllowTunnel(tunnelID, bytes) {
			return nil
		}
		
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			continue
		}
	}
}

// SetIPRateLimit 动态调整IP速率限制
func (r *RateLimiter) SetIPRateLimit(rate int, burst int) {
	r.ipMu.Lock()
	defer r.ipMu.Unlock()
	
	r.ipConfig.Rate = rate
	r.ipConfig.Burst = burst
	
	// 清空现有buckets，让它们重新创建
	r.ipBuckets = make(map[string]*TokenBucket)
	
	corelog.Infof("RateLimiter: IP rate limit updated to %d/s (burst: %d)", rate, burst)
}

// SetTunnelRateLimit 动态调整隧道速率限制
func (r *RateLimiter) SetTunnelRateLimit(rate int, burst int) {
	r.tunnelMu.Lock()
	defer r.tunnelMu.Unlock()
	
	r.tunnelConfig.Rate = rate
	r.tunnelConfig.Burst = burst
	
	// 清空现有buckets，让它们重新创建
	r.tunnelBuckets = make(map[string]*TokenBucket)
	
	corelog.Infof("RateLimiter: Tunnel rate limit updated to %d bytes/s (burst: %d bytes)", rate, burst)
}

