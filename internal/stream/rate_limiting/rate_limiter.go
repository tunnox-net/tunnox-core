package rate_limiting

import (
	"time"
)

// RateLimiter 限流器接口
type RateLimiter interface {
	// SetRate 设置速率限制
	SetRate(bytesPerSecond int64) error

	// Read 读取数据（带限流）
	Read(p []byte) (n int, err error)

	// Write 写入数据（带限流）
	Write(p []byte) (n int, err error)

	// Close 关闭限流器
	Close()
}

// TokenBucketRateLimiter 令牌桶限流器
type TokenBucketRateLimiter struct {
	tokens     int64
	capacity   int64
	rate       int64
	lastRefill time.Time
}

// NewTokenBucketRateLimiter 创建新的令牌桶限流器
func NewTokenBucketRateLimiter(capacity, rate int64) *TokenBucketRateLimiter {
	return &TokenBucketRateLimiter{
		tokens:     capacity,
		capacity:   capacity,
		rate:       rate,
		lastRefill: time.Now(),
	}
}

// SetRate 设置速率限制
func (tb *TokenBucketRateLimiter) SetRate(bytesPerSecond int64) error {
	tb.rate = bytesPerSecond
	return nil
}

// Read 读取数据（带限流）
func (tb *TokenBucketRateLimiter) Read(p []byte) (n int, err error) {
	// 这里应该实现具体的读取限流逻辑
	return len(p), nil
}

// Write 写入数据（带限流）
func (tb *TokenBucketRateLimiter) Write(p []byte) (n int, err error) {
	// 这里应该实现具体的写入限流逻辑
	return len(p), nil
}

// Close 关闭限流器
func (tb *TokenBucketRateLimiter) Close() {
	// 令牌桶限流器不需要特殊关闭逻辑
}

// refill 补充令牌
func (tb *TokenBucketRateLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tokensToAdd := int64(elapsed * float64(tb.rate))

	if tokensToAdd > 0 {
		tb.tokens = min(tb.capacity, tb.tokens+tokensToAdd)
		tb.lastRefill = now
	}
}

// consume 消费令牌
func (tb *TokenBucketRateLimiter) consume(tokens int64) bool {
	tb.refill()
	if tb.tokens >= tokens {
		tb.tokens -= tokens
		return true
	}
	return false
}

// min 返回两个整数中的较小值
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// NoRateLimiter 无限流器
type NoRateLimiter struct{}

// NewNoRateLimiter 创建新的无限流器
func NewNoRateLimiter() *NoRateLimiter {
	return &NoRateLimiter{}
}

// SetRate 设置速率限制（无限制）
func (nr *NoRateLimiter) SetRate(bytesPerSecond int64) error {
	return nil
}

// Read 读取数据（无限制）
func (nr *NoRateLimiter) Read(p []byte) (n int, err error) {
	return len(p), nil
}

// Write 写入数据（无限制）
func (nr *NoRateLimiter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

// Close 关闭限流器
func (nr *NoRateLimiter) Close() {
	// 无限流器不需要特殊关闭逻辑
}
