package command

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	corelog "tunnox-core/internal/core/log"
)

// LoggingMiddleware 日志中间件
type LoggingMiddleware struct{}

// Process 实现Middleware接口
func (lm *LoggingMiddleware) Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
	start := time.Now()

	corelog.Debugf("Command started: %v, ConnectionID: %s, RequestID: %s",
		ctx.CommandType, ctx.ConnectionID, ctx.RequestID)

	response, err := next(ctx)

	duration := time.Since(start)
	if err != nil {
		corelog.Errorf("Command failed: %v, Duration: %v, Error: %v",
			ctx.CommandType, duration, err)
	} else {
		corelog.Debugf("Command completed: %v, Duration: %v, Success: %v",
			ctx.CommandType, duration, response.Success)
	}

	return response, err
}

// MetricsMiddleware 指标中间件
type MetricsMiddleware struct {
	metricsCollector MetricsCollector
}

// NewMetricsMiddleware 创建指标中间件
func NewMetricsMiddleware(collector MetricsCollector) *MetricsMiddleware {
	return &MetricsMiddleware{
		metricsCollector: collector,
	}
}

// Process 实现Middleware接口
func (mm *MetricsMiddleware) Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
	start := time.Now()

	// 记录命令开始
	mm.metricsCollector.IncCounter("command_started_total", map[string]string{
		"command_type": fmt.Sprintf("%d", ctx.CommandType),
	})

	response, err := next(ctx)

	duration := time.Since(start)

	// 记录命令完成
	if err != nil {
		mm.metricsCollector.IncCounter("command_failed_total", map[string]string{
			"command_type": fmt.Sprintf("%d", ctx.CommandType),
		})
	} else {
		mm.metricsCollector.IncCounter("command_completed_total", map[string]string{
			"command_type": fmt.Sprintf("%d", ctx.CommandType),
		})
	}

	// 记录执行时间
	mm.metricsCollector.RecordHistogram("command_duration_seconds", duration.Seconds(), map[string]string{
		"command_type": fmt.Sprintf("%d", ctx.CommandType),
	})

	return response, err
}

// RetryMiddleware 重试中间件
type RetryMiddleware struct {
	maxRetries int
	backoff    BackoffStrategy
	retryable  func(error) bool
}

// NewRetryMiddleware 创建重试中间件
func NewRetryMiddleware(maxRetries int, backoff BackoffStrategy, retryable func(error) bool) *RetryMiddleware {
	return &RetryMiddleware{
		maxRetries: maxRetries,
		backoff:    backoff,
		retryable:  retryable,
	}
}

// Process 实现Middleware接口
func (rm *RetryMiddleware) Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= rm.maxRetries; attempt++ {
		response, err := next(ctx)

		if err == nil {
			return response, nil
		}

		lastErr = err

		// 检查是否可重试
		if !rm.retryable(err) {
			return nil, err
		}

		// 等待后重试
		if attempt < rm.maxRetries {
			delay := rm.backoff.Delay(attempt)
			time.Sleep(delay)
		}
	}

	return nil, lastErr
}

// TimeoutMiddleware 超时中间件
type TimeoutMiddleware struct {
	timeout time.Duration
}

// NewTimeoutMiddleware 创建超时中间件
func NewTimeoutMiddleware(timeout time.Duration) *TimeoutMiddleware {
	return &TimeoutMiddleware{
		timeout: timeout,
	}
}

// Process 实现Middleware接口
func (tm *TimeoutMiddleware) Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
	// 创建带超时的上下文
	timeoutCtx := time.AfterFunc(tm.timeout, func() {
		// 超时处理
	})
	defer timeoutCtx.Stop()

	// 在goroutine中执行命令
	responseChan := make(chan *CommandResponse, 1)
	errorChan := make(chan error, 1)

	go func() {
		response, err := next(ctx)
		if err != nil {
			errorChan <- err
		} else {
			responseChan <- response
		}
	}()

	// 等待结果或超时
	select {
	case response := <-responseChan:
		return response, nil
	case err := <-errorChan:
		return nil, err
	case <-timeoutCtx.C:
		return nil, ErrTimeout
	}
}

// MetricsCollector 指标收集器接口
type MetricsCollector interface {
	IncCounter(name string, labels map[string]string)
	RecordHistogram(name string, value float64, labels map[string]string)
}

// BackoffStrategy 退避策略接口
type BackoffStrategy interface {
	Delay(attempt int) time.Duration
}

// ExponentialBackoff 指数退避策略
type ExponentialBackoff struct {
	initialDelay time.Duration
	maxDelay     time.Duration
}

// NewExponentialBackoff 创建指数退避策略
func NewExponentialBackoff(initialDelay, maxDelay time.Duration) *ExponentialBackoff {
	return &ExponentialBackoff{
		initialDelay: initialDelay,
		maxDelay:     maxDelay,
	}
}

// Delay 计算延迟时间
func (eb *ExponentialBackoff) Delay(attempt int) time.Duration {
	delay := eb.initialDelay * time.Duration(1<<attempt)
	if delay > eb.maxDelay {
		delay = eb.maxDelay
	}
	return delay
}

// CommandIdValidationMiddleware 命令ID验证中间件
type CommandIdValidationMiddleware struct {
	usedCommandIds map[string]time.Time
	mu             sync.RWMutex
	cleanupTicker  *time.Ticker
}

// NewCommandIdValidationMiddleware 创建命令ID验证中间件
func NewCommandIdValidationMiddleware() *CommandIdValidationMiddleware {
	middleware := &CommandIdValidationMiddleware{
		usedCommandIds: make(map[string]time.Time),
		cleanupTicker:  time.NewTicker(5 * time.Minute), // 每5分钟清理一次
	}

	// 启动清理协程
	go middleware.cleanupRoutine()

	return middleware
}

// Process 处理命令ID验证
func (cm *CommandIdValidationMiddleware) Process(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
	// 验证CommandId是否存在
	if ctx.CommandId == "" {
		return &CommandResponse{
			Success: false,
			Error:   "CommandId is required",
		}, nil
	}

	// 验证CommandId格式
	if !cm.isValidCommandId(ctx.CommandId) {
		return &CommandResponse{
			Success: false,
			Error:   "Invalid CommandId format",
		}, nil
	}

	// 检查CommandId是否已被使用（防重放攻击）
	if cm.isCommandIdUsed(ctx.CommandId) {
		return &CommandResponse{
			Success: false,
			Error:   "CommandId already used",
		}, nil
	}

	// 标记CommandId为已使用
	cm.markCommandIdAsUsed(ctx.CommandId)

	// 继续处理命令
	return next(ctx)
}

// isValidCommandId 验证命令ID格式
func (cm *CommandIdValidationMiddleware) isValidCommandId(commandId string) bool {
	// 基本格式验证：以"cmd_"开头，包含时间戳和连接ID
	if len(commandId) < 10 {
		return false
	}

	// 检查前缀
	if !strings.HasPrefix(commandId, "cmd_") {
		return false
	}

	// 检查是否包含时间戳和连接ID
	parts := strings.Split(commandId, "_")
	if len(parts) < 3 {
		return false
	}

	// 验证时间戳部分是否为数字
	if _, err := strconv.ParseInt(parts[1], 10, 64); err != nil {
		return false
	}

	return true
}

// isCommandIdUsed 检查命令ID是否已被使用
func (cm *CommandIdValidationMiddleware) isCommandIdUsed(commandId string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	_, exists := cm.usedCommandIds[commandId]
	return exists
}

// markCommandIdAsUsed 标记命令ID为已使用
func (cm *CommandIdValidationMiddleware) markCommandIdAsUsed(commandId string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.usedCommandIds[commandId] = time.Now()
}

// cleanupRoutine 清理过期的命令ID
func (cm *CommandIdValidationMiddleware) cleanupRoutine() {
	for range cm.cleanupTicker.C {
		cm.cleanupExpiredCommandIds()
	}
}

// cleanupExpiredCommandIds 清理过期的命令ID
func (cm *CommandIdValidationMiddleware) cleanupExpiredCommandIds() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	expireTime := time.Now().Add(-30 * time.Minute) // 30分钟过期
	for commandId, usedTime := range cm.usedCommandIds {
		if usedTime.Before(expireTime) {
			delete(cm.usedCommandIds, commandId)
		}
	}
}

// Close 关闭中间件
func (cm *CommandIdValidationMiddleware) Close() {
	if cm.cleanupTicker != nil {
		cm.cleanupTicker.Stop()
	}
}

// 错误定义
var ErrTimeout = &CommandError{Message: "command timeout"}
var ErrMaxRetriesExceeded = &CommandError{Message: "max retries exceeded"}

// CommandError 命令错误
type CommandError struct {
	Message string
}

func (ce *CommandError) Error() string {
	return ce.Message
}
