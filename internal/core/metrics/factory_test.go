package metrics

import (
	"context"
	"testing"
)

func TestMetricsFactory_CreateMetrics(t *testing.T) {
	ctx := context.Background()
	factory := NewMetricsFactory(ctx)

	// 测试创建 Memory Metrics
	metrics, err := factory.CreateMetrics(MetricsTypeMemory)
	if err != nil {
		t.Fatalf("CreateMetrics failed: %v", err)
	}
	if metrics == nil {
		t.Fatal("CreateMetrics returned nil")
	}
	defer metrics.Close()

	// 验证 Metrics 可以正常工作
	if err := metrics.IncrementCounter("test", nil); err != nil {
		t.Fatalf("IncrementCounter failed: %v", err)
	}
}

func TestMetricsFactory_UnsupportedType(t *testing.T) {
	ctx := context.Background()
	factory := NewMetricsFactory(ctx)

	// 测试不支持的类型
	_, err := factory.CreateMetrics("unsupported")
	if err == nil {
		t.Error("CreateMetrics should return error for unsupported type")
	}
}

func TestMetricsFactory_PrometheusNotImplemented(t *testing.T) {
	ctx := context.Background()
	factory := NewMetricsFactory(ctx)

	// 测试 Prometheus（未实现）
	_, err := factory.CreateMetrics(MetricsTypePrometheus)
	if err == nil {
		t.Error("CreateMetrics should return error for Prometheus (not implemented)")
	}
}
