package metrics

import (
	"context"
	"testing"
)

func TestSetGlobalMetrics(t *testing.T) {
	// 清理全局状态
	globalMu.Lock()
	oldMetrics := globalMetrics
	globalMetrics = nil
	globalMu.Unlock()

	defer func() {
		globalMu.Lock()
		globalMetrics = oldMetrics
		globalMu.Unlock()
	}()

	ctx := context.Background()
	metrics := NewMemoryMetrics(ctx)
	defer metrics.Close()

	// 测试设置全局 Metrics
	SetGlobalMetrics(metrics)

	// 验证全局 Metrics 已设置
	if GetGlobalMetrics() != metrics {
		t.Error("GetGlobalMetrics() did not return the set metrics")
	}

	// 测试全局便捷方法
	if err := IncrementCounter("test", nil); err != nil {
		t.Fatalf("IncrementCounter failed: %v", err)
	}

	value, err := GetCounter("test", nil)
	if err != nil {
		t.Fatalf("GetCounter failed: %v", err)
	}
	if value != 1.0 {
		t.Errorf("expected counter value 1.0, got %f", value)
	}
}

func TestMustGetGlobalMetrics_Panic(t *testing.T) {
	// 清理全局状态
	globalMu.Lock()
	oldMetrics := globalMetrics
	globalMetrics = nil
	globalMu.Unlock()

	defer func() {
		globalMu.Lock()
		globalMetrics = oldMetrics
		globalMu.Unlock()

		// 恢复 panic
		if r := recover(); r == nil {
			t.Error("MustGetGlobalMetrics should panic when metrics not initialized")
		}
	}()

	// 应该 panic
	_ = MustGetGlobalMetrics()
}

func TestSetGlobalMetrics_NilPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("SetGlobalMetrics should panic when called with nil")
		}
	}()

	SetGlobalMetrics(nil)
}

