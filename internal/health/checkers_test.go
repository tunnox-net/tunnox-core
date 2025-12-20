package health

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockStorageChecker 模拟存储检查器
type mockStorageChecker struct {
	err error
}

func (m *mockStorageChecker) Ping(ctx context.Context) error {
	return m.err
}

// mockBrokerChecker 模拟消息代理检查器
type mockBrokerChecker struct {
	err error
}

func (m *mockBrokerChecker) Ping(ctx context.Context) error {
	return m.err
}

// mockSessionManagerChecker 模拟会话管理器检查器
type mockSessionManagerChecker struct {
	activeConns   int
	activeTunnels int
}

func (m *mockSessionManagerChecker) GetActiveConnections() int {
	return m.activeConns
}

func (m *mockSessionManagerChecker) GetActiveTunnels() int {
	return m.activeTunnels
}

func TestStorageHealthChecker(t *testing.T) {
	t.Run("nil storage", func(t *testing.T) {
		checker := NewStorageHealthChecker(nil)
		health, err := checker.Check(context.Background())
		if err != nil {
			t.Fatalf("Check should not return error, got: %v", err)
		}
		if health.Status != ComponentStatusUnhealthy {
			t.Errorf("expected unhealthy, got %s", health.Status)
		}
		if health.Message != "storage not configured" {
			t.Errorf("expected 'storage not configured', got %s", health.Message)
		}
	})

	t.Run("healthy storage", func(t *testing.T) {
		storage := &mockStorageChecker{err: nil}
		checker := NewStorageHealthChecker(storage)
		health, err := checker.Check(context.Background())
		if err != nil {
			t.Fatalf("Check should not return error, got: %v", err)
		}
		if health.Status != ComponentStatusHealthy {
			t.Errorf("expected healthy, got %s", health.Status)
		}
	})

	t.Run("unhealthy storage", func(t *testing.T) {
		storage := &mockStorageChecker{err: errors.New("connection failed")}
		checker := NewStorageHealthChecker(storage)
		health, err := checker.Check(context.Background())
		if err != nil {
			t.Fatalf("Check should not return error, got: %v", err)
		}
		if health.Status != ComponentStatusUnhealthy {
			t.Errorf("expected unhealthy, got %s", health.Status)
		}
		if health.Message != "connection failed" {
			t.Errorf("expected 'connection failed', got %s", health.Message)
		}
	})
}

func TestBrokerHealthChecker(t *testing.T) {
	t.Run("nil broker", func(t *testing.T) {
		checker := NewBrokerHealthChecker(nil)
		health, err := checker.Check(context.Background())
		if err != nil {
			t.Fatalf("Check should not return error, got: %v", err)
		}
		if health.Status != ComponentStatusUnhealthy {
			t.Errorf("expected unhealthy, got %s", health.Status)
		}
		if health.Message != "broker not configured" {
			t.Errorf("expected 'broker not configured', got %s", health.Message)
		}
	})

	t.Run("healthy broker", func(t *testing.T) {
		broker := &mockBrokerChecker{err: nil}
		checker := NewBrokerHealthChecker(broker)
		health, err := checker.Check(context.Background())
		if err != nil {
			t.Fatalf("Check should not return error, got: %v", err)
		}
		if health.Status != ComponentStatusHealthy {
			t.Errorf("expected healthy, got %s", health.Status)
		}
	})

	t.Run("unhealthy broker", func(t *testing.T) {
		broker := &mockBrokerChecker{err: errors.New("broker error")}
		checker := NewBrokerHealthChecker(broker)
		health, err := checker.Check(context.Background())
		if err != nil {
			t.Fatalf("Check should not return error, got: %v", err)
		}
		if health.Status != ComponentStatusUnhealthy {
			t.Errorf("expected unhealthy, got %s", health.Status)
		}
		if health.Message != "broker error" {
			t.Errorf("expected 'broker error', got %s", health.Message)
		}
	})
}

func TestProtocolHealthChecker(t *testing.T) {
	t.Run("nil session manager", func(t *testing.T) {
		checker := NewProtocolHealthChecker(nil)
		health, err := checker.Check(context.Background())
		if err != nil {
			t.Fatalf("Check should not return error, got: %v", err)
		}
		if health.Status != ComponentStatusDegraded {
			t.Errorf("expected degraded, got %s", health.Status)
		}
		if health.Message != "session manager not configured" {
			t.Errorf("expected 'session manager not configured', got %s", health.Message)
		}
	})

	t.Run("healthy with active connections", func(t *testing.T) {
		sessionMgr := &mockSessionManagerChecker{
			activeConns:   5,
			activeTunnels: 3,
		}
		checker := NewProtocolHealthChecker(sessionMgr)
		health, err := checker.Check(context.Background())
		if err != nil {
			t.Fatalf("Check should not return error, got: %v", err)
		}
		if health.Status != ComponentStatusHealthy {
			t.Errorf("expected healthy, got %s", health.Status)
		}
		if health.Message != "" {
			t.Errorf("expected empty message, got %s", health.Message)
		}
	})

	t.Run("healthy with no active connections", func(t *testing.T) {
		sessionMgr := &mockSessionManagerChecker{
			activeConns:   0,
			activeTunnels: 0,
		}
		checker := NewProtocolHealthChecker(sessionMgr)
		health, err := checker.Check(context.Background())
		if err != nil {
			t.Fatalf("Check should not return error, got: %v", err)
		}
		if health.Status != ComponentStatusHealthy {
			t.Errorf("expected healthy, got %s", health.Status)
		}
		if health.Message != "no active connections" {
			t.Errorf("expected 'no active connections', got %s", health.Message)
		}
	})
}

func TestHealthCheckers_Timeout(t *testing.T) {
	// 测试超时场景 - 使用 CompositeHealthChecker 的超时机制
	checker := NewCompositeHealthChecker(10 * time.Millisecond)

	// 创建一个会阻塞的检查器
	slowChecker := &mockHealthChecker{
		delay: 20 * time.Millisecond, // 超过超时时间
	}

	checker.RegisterChecker("slow", slowChecker)

	ctx := context.Background()
	results := checker.CheckAll(ctx)

	// 超时的检查器应该返回 unhealthy
	if results["slow"] == nil {
		t.Error("expected result for slow checker, got nil")
	} else if results["slow"].Status != ComponentStatusUnhealthy {
		t.Errorf("expected unhealthy status for timeout, got %s", results["slow"].Status)
	}
}
