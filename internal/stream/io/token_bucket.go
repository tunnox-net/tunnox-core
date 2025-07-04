package io

import (
	"context"
	"sync"
	"time"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/errors"
)

// TokenBucket 通用令牌桶实现
type TokenBucket struct {
	rate      int64      // 令牌产生速率（字节/秒）
	burstSize int        // 突发大小
	tokens    int        // 当前令牌数
	lastTime  time.Time  // 上次更新时间
	mu        sync.Mutex // 保护并发访问
	ctx       context.Context
}

// NewTokenBucket 创建新的令牌桶
func NewTokenBucket(rate int64, ctx context.Context) (*TokenBucket, error) {
	if rate <= 0 {
		return nil, errors.ErrInvalidRate
	}

	// 计算突发大小
	burstSize := int(float64(rate) / float64(constants.DefaultBurstRatio))
	if burstSize < constants.MinBurstSize && int(rate) >= constants.MinBurstSize {
		burstSize = constants.MinBurstSize
	}
	if burstSize > int(rate) {
		burstSize = int(rate) // 突发大小不应超过速率
	}

	return &TokenBucket{
		rate:      rate,
		burstSize: burstSize,
		tokens:    0, // 初始令牌数为0，需要等待产生
		lastTime:  time.Now(),
		ctx:       ctx,
	}, nil
}

// WaitForTokens 等待足够的令牌
func (tb *TokenBucket) WaitForTokens(tokensNeeded int) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// 计算从上次到现在应该产生的令牌数
	elapsed := now.Sub(tb.lastTime)
	tokensToAdd := int(float64(tb.rate) * elapsed.Seconds())
	tb.tokens += tokensToAdd

	// 限制令牌数量不超过burst大小
	if tb.tokens > tb.burstSize {
		tb.tokens = tb.burstSize
	}

	// 如果令牌不足，需要等待
	if tb.tokens < tokensNeeded {
		tokensNeeded -= tb.tokens
		tb.tokens = 0

		// 计算需要等待的时间
		waitTime := time.Duration(float64(time.Second) * float64(tokensNeeded) / float64(tb.rate))
		if waitTime > 0 {
			// 释放锁，等待时间
			tb.mu.Unlock()

			select {
			case <-time.After(waitTime):
				// 重新获取锁
				tb.mu.Lock()
			case <-tb.ctx.Done():
				// 重新获取锁
				tb.mu.Lock()
				return errors.ErrContextCancelled
			}
		}
	} else {
		tb.tokens -= tokensNeeded
	}

	tb.lastTime = time.Now()
	return nil
}

// SetRate 设置令牌产生速率
func (tb *TokenBucket) SetRate(rate int64) error {
	if rate <= 0 {
		return errors.ErrInvalidRate
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.rate = rate

	// 重新计算突发大小
	burstSize := int(float64(rate) / float64(constants.DefaultBurstRatio))
	if burstSize < constants.MinBurstSize && int(rate) >= constants.MinBurstSize {
		burstSize = constants.MinBurstSize
	}
	if burstSize > int(rate) {
		burstSize = int(rate) // 突发大小不应超过速率
	}
	tb.burstSize = burstSize

	// 调整当前令牌数
	if tb.tokens > burstSize {
		tb.tokens = burstSize
	}

	return nil
}

// GetRate 获取当前速率
func (tb *TokenBucket) GetRate() int64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.rate
}

// GetBurstSize 获取突发大小
func (tb *TokenBucket) GetBurstSize() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.burstSize
}

// GetTokens 获取当前令牌数
func (tb *TokenBucket) GetTokens() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tokens
}
