package httppoll

import (
	"sync"
	"time"
	"tunnox-core/internal/utils"
)

// PollRateLimiter 限制每个连接的并发poll请求数量
// 避免大量poll请求同时冲击服务器,导致资源耗尽
type PollRateLimiter struct {
	maxConcurrent int           // 每个连接最大并发poll数
	semaphore     chan struct{} // 使用channel作为信号量
	mu            sync.Mutex
}

// NewPollRateLimiter 创建poll限流器
func NewPollRateLimiter(maxConcurrent int) *PollRateLimiter {
	if maxConcurrent <= 0 {
		maxConcurrent = 5 // 默认每个连接最多5个并发poll
	}
	return &PollRateLimiter{
		maxConcurrent: maxConcurrent,
		semaphore:     make(chan struct{}, maxConcurrent),
	}
}

// TryAcquire 尝试获取poll许可(非阻塞)
// 返回true表示获取成功,false表示已达到限制
func (p *PollRateLimiter) TryAcquire() bool {
	select {
	case p.semaphore <- struct{}{}:
		return true
	default:
		// 信号量已满,达到并发限制
		return false
	}
}

// Acquire 获取poll许可(阻塞,带超时)
// 返回true表示获取成功,false表示超时
func (p *PollRateLimiter) Acquire(timeout time.Duration) bool {
	select {
	case p.semaphore <- struct{}{}:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Release 释放poll许可
func (p *PollRateLimiter) Release() {
	select {
	case <-p.semaphore:
		// 成功释放
	default:
		// 不应该发生,说明Release被多次调用
		utils.Warnf("PollRateLimiter: Release called but no permit to release")
	}
}

// GetCurrentCount 获取当前并发数
func (p *PollRateLimiter) GetCurrentCount() int {
	return len(p.semaphore)
}

// GetMaxConcurrent 获取最大并发数
func (p *PollRateLimiter) GetMaxConcurrent() int {
	return p.maxConcurrent
}
