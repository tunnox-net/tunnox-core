package metrics

import (
	"context"
	"fmt"
)

// MetricsType 指标类型
type MetricsType string

const (
	// MetricsTypeMemory 内存指标（单文件运行）
	MetricsTypeMemory MetricsType = "memory"
	// MetricsTypePrometheus Prometheus 指标（未来扩展）
	MetricsTypePrometheus MetricsType = "prometheus"
)

// MetricsFactory 指标工厂
type MetricsFactory struct {
	ctx context.Context
}

// NewMetricsFactory 创建指标工厂
func NewMetricsFactory(ctx context.Context) *MetricsFactory {
	return &MetricsFactory{
		ctx: ctx,
	}
}

// CreateMetrics 创建指标收集器实例
func (f *MetricsFactory) CreateMetrics(metricsType MetricsType) (Metrics, error) {
	switch metricsType {
	case MetricsTypeMemory:
		return f.createMemoryMetrics()
	case MetricsTypePrometheus:
		return nil, fmt.Errorf("prometheus metrics not implemented yet")
	default:
		return nil, fmt.Errorf("unsupported metrics type: %s", metricsType)
	}
}

// createMemoryMetrics 创建内存指标收集器
func (f *MetricsFactory) createMemoryMetrics() (Metrics, error) {
	metrics := NewMemoryMetrics(f.ctx)
	return metrics, nil
}

