package stream

import (
	"context"
	"sync"
	"time"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/errors"
)

// TokenBucket 令牌桶限流器
type TokenBucket struct {
	capacity   int64
	rate       int64
	tokens     int64
	lastRefill time.Time
	mu         sync.Mutex
	dispose.Dispose
}

// NewTokenBucket 创建新的令牌桶
func NewTokenBucket(rate int64, parentCtx context.Context) (*TokenBucket, error) {
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

	tokenBucket := &TokenBucket{
		rate:       rate,
		capacity:   int64(burstSize),
		tokens:     0, // 初始令牌数为0，需要等待产生
		lastRefill: time.Now(),
	}

	// 使用Dispose的context管理
	tokenBucket.SetCtxWithNoOpOnClose(parentCtx)

	return tokenBucket, nil
}

// WaitForTokens 等待足够的令牌
func (tb *TokenBucket) WaitForTokens(tokensNeeded int) error {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	// 计算从上次到现在应该产生的令牌数
	elapsed := now.Sub(tb.lastRefill)
	tokensToAdd := int64(float64(tb.rate) * elapsed.Seconds())
	tb.tokens += tokensToAdd

	// 限制令牌数量不超过burst大小
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	// 如果令牌不足，需要等待
	if tb.tokens < int64(tokensNeeded) {
		tokensNeeded -= int(tb.tokens)
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
			case <-tb.Ctx().Done():
				// 重新获取锁
				tb.mu.Lock()
				return errors.ErrContextCancelled
			}
		}
	} else {
		tb.tokens -= int64(tokensNeeded)
	}

	tb.lastRefill = time.Now()
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
	tb.capacity = int64(burstSize)

	// 调整当前令牌数
	if tb.tokens > int64(burstSize) {
		tb.tokens = int64(burstSize)
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
	return int(tb.capacity)
}

// GetTokens 获取当前令牌数
func (tb *TokenBucket) GetTokens() int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return int(tb.tokens)
}

// Close 方法由 utils.Dispose 提供，无需重复实现
// TokenBucket 使用独立的 context 管理，在 onClose 中处理清理
