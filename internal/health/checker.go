package health

import (
	"context"
	"time"
)

// ComponentStatus 组件状态
type ComponentStatus string

const (
	ComponentStatusHealthy   ComponentStatus = "healthy"
	ComponentStatusDegraded  ComponentStatus = "degraded"  // 降级，部分功能不可用
	ComponentStatusUnhealthy ComponentStatus = "unhealthy" // 不健康，完全不可用
)

// ComponentHealth 组件健康信息
type ComponentHealth struct {
	Name      string          `json:"name"`
	Status    ComponentStatus `json:"status"`
	Message   string          `json:"message,omitempty"`
	LastCheck time.Time       `json:"last_check"`
}

// HealthChecker 健康检查器接口
type HealthChecker interface {
	// Check 执行健康检查，返回组件健康信息
	Check(ctx context.Context) (*ComponentHealth, error)
}

// CompositeHealthChecker 组合健康检查器
// 用于检查多个子系统的健康状态
type CompositeHealthChecker struct {
	checkers map[string]HealthChecker
	timeout  time.Duration
}

// NewCompositeHealthChecker 创建组合健康检查器
func NewCompositeHealthChecker(timeout time.Duration) *CompositeHealthChecker {
	return &CompositeHealthChecker{
		checkers: make(map[string]HealthChecker),
		timeout:  timeout,
	}
}

// RegisterChecker 注册健康检查器
func (c *CompositeHealthChecker) RegisterChecker(name string, checker HealthChecker) {
	c.checkers[name] = checker
}

// CheckAll 检查所有注册的组件
func (c *CompositeHealthChecker) CheckAll(ctx context.Context) map[string]*ComponentHealth {
	results := make(map[string]*ComponentHealth)

	for name, checker := range c.checkers {
		checkCtx, cancel := context.WithTimeout(ctx, c.timeout)
		health, err := checker.Check(checkCtx)
		cancel()

		if err != nil {
			health = &ComponentHealth{
				Name:      name,
				Status:    ComponentStatusUnhealthy,
				Message:   err.Error(),
				LastCheck: time.Now(),
			}
		}

		if health != nil {
			results[name] = health
		}
	}

	return results
}

// GetOverallStatus 获取整体健康状态
// 如果所有组件都健康，返回 healthy
// 如果有组件降级，返回 degraded
// 如果有组件不健康，返回 unhealthy
func (c *CompositeHealthChecker) GetOverallStatus(ctx context.Context) ComponentStatus {
	results := c.CheckAll(ctx)

	hasUnhealthy := false
	hasDegraded := false

	for _, health := range results {
		switch health.Status {
		case ComponentStatusUnhealthy:
			hasUnhealthy = true
		case ComponentStatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return ComponentStatusUnhealthy
	}
	if hasDegraded {
		return ComponentStatusDegraded
	}
	return ComponentStatusHealthy
}

