package utils

import (
	"context"
	"sync"
	"time"
)

// RateLimiter 简单限流器
type RateLimiter struct {
	limit       int
	window      time.Duration
	tokens      map[string][]time.Time
	mutex       sync.RWMutex
	cleanupTick *time.Ticker
	done        chan bool
	Dispose
}

// NewRateLimiter 创建新的限流器
func NewRateLimiter(limit int, window time.Duration, parentCtx context.Context) *RateLimiter {
	limiter := &RateLimiter{
		limit:       limit,
		window:      window,
		tokens:      make(map[string][]time.Time),
		cleanupTick: time.NewTicker(window),
		done:        make(chan bool),
	}

	limiter.SetCtx(parentCtx, limiter.onClose)

	// 启动清理协程
	go limiter.cleanupRoutine()

	return limiter
}

// onClose 资源释放回调
func (r *RateLimiter) onClose() error {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if r.cleanupTick != nil {
		r.cleanupTick.Stop()
		r.cleanupTick = nil
	}

	close(r.done)
	r.tokens = nil
	return nil
}

// Allow 检查是否允许请求
func (r *RateLimiter) Allow() bool {
	return r.AllowWithKey("default")
}

// AllowWithKey 根据键检查是否允许请求
func (r *RateLimiter) AllowWithKey(key string) bool {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	now := time.Now()
	windowStart := now.Add(-r.window)

	// 获取或创建时间戳列表
	timestamps, exists := r.tokens[key]
	if !exists {
		timestamps = make([]time.Time, 0)
	}

	// 清理过期的令牌
	validTokens := make([]time.Time, 0)
	for _, ts := range timestamps {
		if ts.After(windowStart) {
			validTokens = append(validTokens, ts)
		}
	}

	// 检查是否超过限制
	if len(validTokens) >= r.limit {
		return false
	}

	// 添加新令牌
	validTokens = append(validTokens, now)
	r.tokens[key] = validTokens

	return true
}

// cleanupRoutine 清理协程
func (r *RateLimiter) cleanupRoutine() {
	for {
		r.mutex.RLock()
		tick := r.cleanupTick
		r.mutex.RUnlock()

		if tick == nil {
			return
		}

		select {
		case <-tick.C:
			r.cleanupTokens()
		case <-r.done:
			return
		case <-r.Ctx().Done():
			return
		}
	}
}

// cleanupTokens 清理过期的令牌
func (r *RateLimiter) cleanupTokens() {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	now := time.Now()
	windowStart := now.Add(-r.window)

	for key, timestamps := range r.tokens {
		validTokens := make([]time.Time, 0)
		for _, ts := range timestamps {
			if ts.After(windowStart) {
				validTokens = append(validTokens, ts)
			}
		}

		if len(validTokens) == 0 {
			delete(r.tokens, key)
		} else {
			r.tokens[key] = validTokens
		}
	}
}

// Close 方法由 utils.Dispose 提供，无需重复实现

// GetStats 获取统计信息
func (r *RateLimiter) GetStats() map[string]int {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	stats := make(map[string]int)
	for key, timestamps := range r.tokens {
		stats[key] = len(timestamps)
	}

	return stats
}
