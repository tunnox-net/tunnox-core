package utils

import (
	"context"
	"sync"
	"time"
)

// RateLimiter 速率限制器
type RateLimiter struct {
	Dispose
	rate     float64
	capacity int64
	tokens   int64
	lastTime time.Time
	mu       sync.RWMutex
}

// NewRateLimiter 创建新的速率限制器
func NewRateLimiter(rate float64, capacity int64, parentCtx context.Context) *RateLimiter {
	limiter := &RateLimiter{
		rate:     rate,
		capacity: capacity,
		tokens:   capacity,
		lastTime: time.Now(),
	}
	limiter.SetCtx(parentCtx, limiter.onClose)
	return limiter
}

// onClose 资源释放回调
func (r *RateLimiter) onClose() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tokens = 0
	r.lastTime = time.Time{}
	return nil
}

// Allow 检查是否允许请求
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastTime)
	generatedTokens := int64(float64(elapsed) * r.rate)

	if generatedTokens > 0 {
		r.tokens = min(r.tokens+generatedTokens, r.capacity)
		r.lastTime = now
	}

	if r.tokens <= 0 {
		return false
	}

	r.tokens--
	return true
}

// AllowWithKey 根据键检查是否允许请求
func (r *RateLimiter) AllowWithKey(key string) bool {
	// 简化实现，忽略 key 参数
	return r.Allow()
}

// GetStats 获取统计信息
func (r *RateLimiter) GetStats() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]int)
	stats["tokens"] = int(r.tokens)
	stats["capacity"] = int(r.capacity)
	stats["rate"] = int(r.rate)
	return stats
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
