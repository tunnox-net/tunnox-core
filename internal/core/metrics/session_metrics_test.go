package metrics

import (
	"context"
	"testing"
)

func TestSessionMetrics(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryMetrics(ctx)
	defer m.Close()
	SetGlobalMetrics(m)

	t.Run("IncrementActiveSession", func(t *testing.T) {
		err := IncrementActiveSession()
		if err != nil {
			t.Fatalf("IncrementActiveSession failed: %v", err)
		}

		value, err := m.GetCounter("session_active", nil)
		if err != nil {
			t.Fatalf("GetCounter failed: %v", err)
		}
		if value != 1.0 {
			t.Errorf("expected 1.0, got %f", value)
		}
	})

	t.Run("DecrementActiveSession", func(t *testing.T) {
		// 先获取当前值
		currentValue, _ := m.GetCounter("session_active", nil)

		// 先增加两次
		_ = IncrementActiveSession()
		_ = IncrementActiveSession()

		// 再减少一次
		err := DecrementActiveSession()
		if err != nil {
			t.Fatalf("DecrementActiveSession failed: %v", err)
		}

		value, err := m.GetCounter("session_active", nil)
		if err != nil {
			t.Fatalf("GetCounter failed: %v", err)
		}
		expected := currentValue + 1.0 // 增加2次，减少1次，净增加1
		if value != expected {
			t.Errorf("expected %f, got %f", expected, value)
		}
	})

	t.Run("SetActiveSessions", func(t *testing.T) {
		err := SetActiveSessions(10.0)
		if err != nil {
			t.Fatalf("SetActiveSessions failed: %v", err)
		}

		value, err := m.GetGauge("session_active", nil)
		if err != nil {
			t.Fatalf("GetGauge failed: %v", err)
		}
		if value != 10.0 {
			t.Errorf("expected 10.0, got %f", value)
		}
	})

	t.Run("IncrementTunnelRecovery", func(t *testing.T) {
		err := IncrementTunnelRecovery()
		if err != nil {
			t.Fatalf("IncrementTunnelRecovery failed: %v", err)
		}

		value, err := m.GetCounter("tunnel_recoveries", nil)
		if err != nil {
			t.Fatalf("GetCounter failed: %v", err)
		}
		if value != 1.0 {
			t.Errorf("expected 1.0, got %f", value)
		}
	})

	t.Run("SetActiveTunnels", func(t *testing.T) {
		err := SetActiveTunnels(5.0)
		if err != nil {
			t.Fatalf("SetActiveTunnels failed: %v", err)
		}

		value, err := m.GetGauge("tunnel_active", nil)
		if err != nil {
			t.Fatalf("GetGauge failed: %v", err)
		}
		if value != 5.0 {
			t.Errorf("expected 5.0, got %f", value)
		}
	})

	t.Run("IncrementTunnelCreated", func(t *testing.T) {
		err := IncrementTunnelCreated()
		if err != nil {
			t.Fatalf("IncrementTunnelCreated failed: %v", err)
		}

		value, err := m.GetCounter("tunnel_created", nil)
		if err != nil {
			t.Fatalf("GetCounter failed: %v", err)
		}
		if value != 1.0 {
			t.Errorf("expected 1.0, got %f", value)
		}
	})

	t.Run("IncrementTunnelClosed", func(t *testing.T) {
		err := IncrementTunnelClosed()
		if err != nil {
			t.Fatalf("IncrementTunnelClosed failed: %v", err)
		}

		value, err := m.GetCounter("tunnel_closed", nil)
		if err != nil {
			t.Fatalf("GetCounter failed: %v", err)
		}
		if value != 1.0 {
			t.Errorf("expected 1.0, got %f", value)
		}
	})

	t.Run("SetControlConnections", func(t *testing.T) {
		err := SetControlConnections(3.0)
		if err != nil {
			t.Fatalf("SetControlConnections failed: %v", err)
		}

		value, err := m.GetGauge("connection_control", nil)
		if err != nil {
			t.Fatalf("GetGauge failed: %v", err)
		}
		if value != 3.0 {
			t.Errorf("expected 3.0, got %f", value)
		}
	})

	t.Run("SetDataConnections", func(t *testing.T) {
		err := SetDataConnections(7.0)
		if err != nil {
			t.Fatalf("SetDataConnections failed: %v", err)
		}

		value, err := m.GetGauge("connection_data", nil)
		if err != nil {
			t.Fatalf("GetGauge failed: %v", err)
		}
		if value != 7.0 {
			t.Errorf("expected 7.0, got %f", value)
		}
	})
}

func TestSessionMetrics_NilMetrics(t *testing.T) {
	// 保存原来的 metrics
	originalMetrics := GetGlobalMetrics()

	t.Run("IncrementActiveSession with nil metrics", func(t *testing.T) {
		// 测试函数在 nil metrics 时的行为（不会 panic，返回 nil error）
		err := IncrementActiveSession()
		if err != nil {
			t.Errorf("IncrementActiveSession should not fail with nil metrics, got: %v", err)
		}
	})

	// 恢复测试环境
	if originalMetrics != nil {
		SetGlobalMetrics(originalMetrics)
	} else {
		ctx := context.Background()
		m := NewMemoryMetrics(ctx)
		defer m.Close()
		SetGlobalMetrics(m)
	}
}

