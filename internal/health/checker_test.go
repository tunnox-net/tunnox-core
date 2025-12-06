package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockHealthChecker 模拟健康检查器
type mockHealthChecker struct {
	health *ComponentHealth
	err    error
	delay  time.Duration
}

func (m *mockHealthChecker) Check(ctx context.Context) (*ComponentHealth, error) {
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.delay):
		}
	}
	return m.health, m.err
}

func TestNewCompositeHealthChecker(t *testing.T) {
	checker := NewCompositeHealthChecker(5 * time.Second)
	if checker == nil {
		t.Fatal("NewCompositeHealthChecker returned nil")
	}
	if checker.timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", checker.timeout)
	}
	if len(checker.checkers) != 0 {
		t.Errorf("expected empty checkers map, got %d", len(checker.checkers))
	}
}

func TestCompositeHealthChecker_RegisterChecker(t *testing.T) {
	checker := NewCompositeHealthChecker(5 * time.Second)
	mock := &mockHealthChecker{
		health: &ComponentHealth{
			Name:   "test",
			Status: ComponentStatusHealthy,
		},
	}

	checker.RegisterChecker("test", mock)
	if len(checker.checkers) != 1 {
		t.Errorf("expected 1 checker, got %d", len(checker.checkers))
	}
	if checker.checkers["test"] != mock {
		t.Error("checker not registered correctly")
	}
}

func TestCompositeHealthChecker_CheckAll(t *testing.T) {
	checker := NewCompositeHealthChecker(5 * time.Second)

	// 注册多个检查器
	checker.RegisterChecker("healthy", &mockHealthChecker{
		health: &ComponentHealth{
			Name:      "healthy",
			Status:    ComponentStatusHealthy,
			LastCheck: time.Now(),
		},
	})

	checker.RegisterChecker("degraded", &mockHealthChecker{
		health: &ComponentHealth{
			Name:      "degraded",
			Status:    ComponentStatusDegraded,
			Message:   "degraded message",
			LastCheck: time.Now(),
		},
	})

	checker.RegisterChecker("unhealthy", &mockHealthChecker{
		health: &ComponentHealth{
			Name:      "unhealthy",
			Status:    ComponentStatusUnhealthy,
			Message:   "unhealthy message",
			LastCheck: time.Now(),
		},
	})

	checker.RegisterChecker("error", &mockHealthChecker{
		err: errors.New("check failed"),
	})

	ctx := context.Background()
	results := checker.CheckAll(ctx)

	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
	}

	// 检查 healthy
	if results["healthy"].Status != ComponentStatusHealthy {
		t.Errorf("expected healthy status, got %s", results["healthy"].Status)
	}

	// 检查 degraded
	if results["degraded"].Status != ComponentStatusDegraded {
		t.Errorf("expected degraded status, got %s", results["degraded"].Status)
	}

	// 检查 unhealthy
	if results["unhealthy"].Status != ComponentStatusUnhealthy {
		t.Errorf("expected unhealthy status, got %s", results["unhealthy"].Status)
	}

	// 检查 error（应该被转换为 unhealthy）
	if results["error"].Status != ComponentStatusUnhealthy {
		t.Errorf("expected unhealthy status for error, got %s", results["error"].Status)
	}
	if results["error"].Message != "check failed" {
		t.Errorf("expected error message, got %s", results["error"].Message)
	}
}

func TestCompositeHealthChecker_CheckAll_Timeout(t *testing.T) {
	checker := NewCompositeHealthChecker(100 * time.Millisecond)

	// 注册一个会超时的检查器
	checker.RegisterChecker("slow", &mockHealthChecker{
		delay: 200 * time.Millisecond, // 超过超时时间
	})

	ctx := context.Background()
	results := checker.CheckAll(ctx)

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	// 超时的检查器应该返回 unhealthy
	if results["slow"].Status != ComponentStatusUnhealthy {
		t.Errorf("expected unhealthy status for timeout, got %s", results["slow"].Status)
	}
}

func TestCompositeHealthChecker_GetOverallStatus(t *testing.T) {
	tests := []struct {
		name     string
		checkers map[string]*mockHealthChecker
		expected ComponentStatus
	}{
		{
			name: "all healthy",
			checkers: map[string]*mockHealthChecker{
				"a": {health: &ComponentHealth{Status: ComponentStatusHealthy}},
				"b": {health: &ComponentHealth{Status: ComponentStatusHealthy}},
			},
			expected: ComponentStatusHealthy,
		},
		{
			name: "has degraded",
			checkers: map[string]*mockHealthChecker{
				"a": {health: &ComponentHealth{Status: ComponentStatusHealthy}},
				"b": {health: &ComponentHealth{Status: ComponentStatusDegraded}},
			},
			expected: ComponentStatusDegraded,
		},
		{
			name: "has unhealthy",
			checkers: map[string]*mockHealthChecker{
				"a": {health: &ComponentHealth{Status: ComponentStatusHealthy}},
				"b": {health: &ComponentHealth{Status: ComponentStatusUnhealthy}},
			},
			expected: ComponentStatusUnhealthy,
		},
		{
			name: "degraded and unhealthy",
			checkers: map[string]*mockHealthChecker{
				"a": {health: &ComponentHealth{Status: ComponentStatusDegraded}},
				"b": {health: &ComponentHealth{Status: ComponentStatusUnhealthy}},
			},
			expected: ComponentStatusUnhealthy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewCompositeHealthChecker(5 * time.Second)
			for name, mock := range tt.checkers {
				checker.RegisterChecker(name, mock)
			}

			ctx := context.Background()
			status := checker.GetOverallStatus(ctx)
			if status != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, status)
			}
		})
	}
}

