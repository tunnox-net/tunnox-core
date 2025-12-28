package client

import (
	"math/rand"
	"sync"
	"time"

	corelog "tunnox-core/internal/core/log"
	timeutil "tunnox-core/internal/utils/time"
)

// ReconnectConfig 重连配置
type ReconnectConfig struct {
	Enabled      bool          // 是否启用重连
	InitialDelay time.Duration // 初始延迟（1秒）
	MaxDelay     time.Duration // 最大延迟（60秒）
	MaxAttempts  int           // 最大尝试次数（0=无限）
	Backoff      float64       // 退避因子（2.0=指数退避）
	JitterFactor float64       // 抖动因子（0.0-1.0，推荐0.3）
	// 熔断器配置
	CircuitBreakerEnabled   bool          // 是否启用熔断器
	CircuitBreakerThreshold int           // 熔断阈值（连续失败次数）
	CircuitBreakerTimeout   time.Duration // 熔断超时（熔断后多久重试）
}

// DefaultReconnectConfig 默认重连配置
var DefaultReconnectConfig = ReconnectConfig{
	Enabled:                 true,
	InitialDelay:            200 * time.Millisecond,
	MaxDelay:                60 * time.Second,
	MaxAttempts:             0,
	Backoff:                 2.0,
	JitterFactor:            0.3, // 30% 随机抖动
	CircuitBreakerEnabled:   true,
	CircuitBreakerThreshold: 10,               // 连续失败10次后熔断
	CircuitBreakerTimeout:   5 * time.Minute,  // 熔断5分钟后重试
}

// CircuitBreaker 熔断器
type CircuitBreaker struct {
	mu               sync.Mutex
	failureCount     int       // 连续失败次数
	lastFailureTime  time.Time // 最后失败时间
	state            string    // closed, open, half-open
	threshold        int       // 熔断阈值
	timeout          time.Duration
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     "closed",
		threshold: threshold,
		timeout:   timeout,
	}
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.state = "closed"
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount++
	cb.lastFailureTime = time.Now()
	if cb.failureCount >= cb.threshold {
		cb.state = "open"
		corelog.Warnf("CircuitBreaker: opened after %d consecutive failures", cb.failureCount)
	}
}

// AllowRequest 是否允许请求
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case "closed":
		return true
	case "open":
		// 检查是否超过熔断超时
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = "half-open"
			corelog.Infof("CircuitBreaker: transitioning to half-open state")
			return true
		}
		return false
	case "half-open":
		return true
	default:
		return true
	}
}

// addJitter 添加随机抖动
func addJitter(delay time.Duration, jitterFactor float64) time.Duration {
	if jitterFactor <= 0 {
		return delay
	}
	// 计算抖动范围: delay * (1 - jitterFactor) 到 delay * (1 + jitterFactor)
	jitter := float64(delay) * jitterFactor * (2*rand.Float64() - 1)
	return time.Duration(float64(delay) + jitter)
}

// shouldReconnect 判断是否应该重连
func (c *TunnoxClient) shouldReconnect() bool {
	// 被踢下线不重连
	if c.kicked {
		corelog.Infof("Client: not reconnecting (kicked by server)")
		return false
	}

	// 认证失败不重连
	if c.authFailed {
		corelog.Infof("Client: not reconnecting (authentication failed)")
		return false
	}

	// 主动关闭不重连
	select {
	case <-c.Ctx().Done():
		corelog.Infof("Client: not reconnecting (context cancelled)")
		return false
	default:
	}

	return true
}

// reconnect 重连逻辑
func (c *TunnoxClient) reconnect() {
	if !c.reconnecting.CompareAndSwap(false, true) {
		return
	}
	defer c.reconnecting.Store(false)

	// 获取重连配置
	reconnectConfig := c.getReconnectConfig()

	if !reconnectConfig.Enabled {
		corelog.Infof("Client: reconnect disabled")
		return
	}

	delay := reconnectConfig.InitialDelay
	attempts := 0

	// 创建熔断器
	var circuitBreaker *CircuitBreaker
	if reconnectConfig.CircuitBreakerEnabled {
		circuitBreaker = NewCircuitBreaker(
			reconnectConfig.CircuitBreakerThreshold,
			reconnectConfig.CircuitBreakerTimeout,
		)
	}

	// 使用 SafeTimer 避免循环中 time.After 内存泄漏
	timer := timeutil.NewSafeTimer(delay)
	defer timer.Stop()

	for {
		// 检查是否应该重连
		if !c.shouldReconnect() {
			return
		}

		// 检查最大尝试次数
		if reconnectConfig.MaxAttempts > 0 && attempts >= reconnectConfig.MaxAttempts {
			corelog.Errorf("Client: max reconnect attempts (%d) reached, giving up", reconnectConfig.MaxAttempts)
			return
		}

		// 检查熔断器状态
		if circuitBreaker != nil && !circuitBreaker.AllowRequest() {
			corelog.Warnf("Client: circuit breaker is open, waiting %v before retry", reconnectConfig.CircuitBreakerTimeout)
			timer.Reset(reconnectConfig.CircuitBreakerTimeout)
			select {
			case <-c.Ctx().Done():
				return
			case <-timer.C():
			}
			continue
		}

		// 应用 jitter 抖动
		actualDelay := addJitter(delay, reconnectConfig.JitterFactor)
		corelog.Debugf("Client: waiting %v before reconnect attempt %d (base delay: %v)", actualDelay, attempts+1, delay)

		timer.Reset(actualDelay)
		select {
		case <-c.Ctx().Done():
			return
		case <-timer.C():
		}

		if !c.shouldReconnect() {
			return
		}

		// 尝试重连
		if err := c.Connect(); err != nil {
			corelog.Errorf("Client: reconnect attempt %d failed: %v", attempts+1, err)

			// 记录熔断器失败
			if circuitBreaker != nil {
				circuitBreaker.RecordFailure()
			}

			// 增加延迟（指数退避）
			delay = time.Duration(float64(delay) * reconnectConfig.Backoff)
			if delay > reconnectConfig.MaxDelay {
				delay = reconnectConfig.MaxDelay
			}
			attempts++
			continue
		}

		// 重连成功，记录熔断器成功并重置
		if circuitBreaker != nil {
			circuitBreaker.RecordSuccess()
		}
		corelog.Infof("Client: reconnect successful after %d attempts", attempts+1)

		// 重连成功后不再主动请求映射配置
		// 服务端会在握手成功后通过 pushConfigToClient 主动推送配置
		// 详见：packet_handler_handshake.go:166

		return
	}
}

// getReconnectConfig 获取重连配置
func (c *TunnoxClient) getReconnectConfig() ReconnectConfig {
	// 使用默认配置
	// 注意：如果需要从配置文件读取，需要在 ClientConfig 中添加 Reconnect 字段
	return DefaultReconnectConfig
}
