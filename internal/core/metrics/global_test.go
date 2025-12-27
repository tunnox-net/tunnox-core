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
	if err := SetGlobalMetrics(metrics); err != nil {
		t.Fatalf("SetGlobalMetrics failed: %v", err)
	}

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

func TestTryGetGlobalMetrics_Error(t *testing.T) {
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

	// 应该返回 ErrNotInitialized
	_, err := TryGetGlobalMetrics()
	if err != ErrNotInitialized {
		t.Errorf("TryGetGlobalMetrics should return ErrNotInitialized, got: %v", err)
	}
}

func TestSetGlobalMetrics_NilError(t *testing.T) {
	err := SetGlobalMetrics(nil)
	if err != ErrNilMetrics {
		t.Errorf("SetGlobalMetrics(nil) should return ErrNilMetrics, got: %v", err)
	}
}
