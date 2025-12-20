package metrics

import (
	"context"
	"testing"
)

func TestMemoryMetrics_IncrementCounter(t *testing.T) {
	ctx := context.Background()
	metrics := NewMemoryMetrics(ctx)
	defer metrics.Close()

	// 测试增加计数器
	if err := metrics.IncrementCounter("test_counter", nil); err != nil {
		t.Fatalf("IncrementCounter failed: %v", err)
	}

	// 验证计数器值
	value, err := metrics.GetCounter("test_counter", nil)
	if err != nil {
		t.Fatalf("GetCounter failed: %v", err)
	}
	if value != 1.0 {
		t.Errorf("expected counter value 1.0, got %f", value)
	}

	// 再次增加
	if err := metrics.IncrementCounter("test_counter", nil); err != nil {
		t.Fatalf("IncrementCounter failed: %v", err)
	}

	value, err = metrics.GetCounter("test_counter", nil)
	if err != nil {
		t.Fatalf("GetCounter failed: %v", err)
	}
	if value != 2.0 {
		t.Errorf("expected counter value 2.0, got %f", value)
	}
}

func TestMemoryMetrics_AddCounter(t *testing.T) {
	ctx := context.Background()
	metrics := NewMemoryMetrics(ctx)
	defer metrics.Close()

	// 测试增加指定值
	if err := metrics.AddCounter("test_counter", 5.0, nil); err != nil {
		t.Fatalf("AddCounter failed: %v", err)
	}

	value, err := metrics.GetCounter("test_counter", nil)
	if err != nil {
		t.Fatalf("GetCounter failed: %v", err)
	}
	if value != 5.0 {
		t.Errorf("expected counter value 5.0, got %f", value)
	}
}

func TestMemoryMetrics_SetGauge(t *testing.T) {
	ctx := context.Background()
	metrics := NewMemoryMetrics(ctx)
	defer metrics.Close()

	// 测试设置 Gauge
	if err := metrics.SetGauge("test_gauge", 10.5, nil); err != nil {
		t.Fatalf("SetGauge failed: %v", err)
	}

	value, err := metrics.GetGauge("test_gauge", nil)
	if err != nil {
		t.Fatalf("GetGauge failed: %v", err)
	}
	if value != 10.5 {
		t.Errorf("expected gauge value 10.5, got %f", value)
	}

	// 更新 Gauge
	if err := metrics.SetGauge("test_gauge", 20.0, nil); err != nil {
		t.Fatalf("SetGauge failed: %v", err)
	}

	value, err = metrics.GetGauge("test_gauge", nil)
	if err != nil {
		t.Fatalf("GetGauge failed: %v", err)
	}
	if value != 20.0 {
		t.Errorf("expected gauge value 20.0, got %f", value)
	}
}

func TestMemoryMetrics_WithLabels(t *testing.T) {
	ctx := context.Background()
	metrics := NewMemoryMetrics(ctx)
	defer metrics.Close()

	labels1 := map[string]string{"env": "test", "service": "api"}
	labels2 := map[string]string{"env": "prod", "service": "api"}

	// 测试带标签的计数器
	if err := metrics.IncrementCounter("requests", labels1); err != nil {
		t.Fatalf("IncrementCounter failed: %v", err)
	}

	if err := metrics.IncrementCounter("requests", labels2); err != nil {
		t.Fatalf("IncrementCounter failed: %v", err)
	}

	// 验证不同标签的计数器是独立的
	value1, err := metrics.GetCounter("requests", labels1)
	if err != nil {
		t.Fatalf("GetCounter failed: %v", err)
	}
	if value1 != 1.0 {
		t.Errorf("expected counter value 1.0 for labels1, got %f", value1)
	}

	value2, err := metrics.GetCounter("requests", labels2)
	if err != nil {
		t.Fatalf("GetCounter failed: %v", err)
	}
	if value2 != 1.0 {
		t.Errorf("expected counter value 1.0 for labels2, got %f", value2)
	}
}

func TestMemoryMetrics_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	metrics := NewMemoryMetrics(ctx)
	defer metrics.Close()

	// 并发测试
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = metrics.IncrementCounter("concurrent_counter", nil)
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证最终值
	value, err := metrics.GetCounter("concurrent_counter", nil)
	if err != nil {
		t.Fatalf("GetCounter failed: %v", err)
	}
	if value != 1000.0 {
		t.Errorf("expected counter value 1000.0, got %f", value)
	}
}
