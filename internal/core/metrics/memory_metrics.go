package metrics

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"tunnox-core/internal/core/dispose"
)

// MemoryMetrics 内存指标实现（单文件运行，无外部依赖）
type MemoryMetrics struct {
	*dispose.ResourceBase

	counters map[string]*int64
	gauges   map[string]*float64
	mu       sync.RWMutex
}

// NewMemoryMetrics 创建内存指标收集器
func NewMemoryMetrics(parentCtx context.Context) *MemoryMetrics {
	metrics := &MemoryMetrics{
		ResourceBase: dispose.NewResourceBase("MemoryMetrics"),
		counters:     make(map[string]*int64),
		gauges:       make(map[string]*float64),
	}
	metrics.ResourceBase.Initialize(parentCtx)
	return metrics
}

// IncrementCounter 增加计数器
func (m *MemoryMetrics) IncrementCounter(name string, labels map[string]string) error {
	key := buildKey(name, labels)
	m.mu.Lock()
	counter, exists := m.counters[key]
	if !exists {
		var val int64
		counter = &val
		m.counters[key] = counter
	}
	m.mu.Unlock()
	atomic.AddInt64(counter, 1)
	return nil
}

// AddCounter 增加计数器指定值
func (m *MemoryMetrics) AddCounter(name string, value float64, labels map[string]string) error {
	key := buildKey(name, labels)
	m.mu.Lock()
	counter, exists := m.counters[key]
	if !exists {
		var val int64
		counter = &val
		m.counters[key] = counter
	}
	m.mu.Unlock()
	atomic.AddInt64(counter, int64(value))
	return nil
}

// GetCounter 获取计数器值
func (m *MemoryMetrics) GetCounter(name string, labels map[string]string) (float64, error) {
	key := buildKey(name, labels)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if counter, exists := m.counters[key]; exists {
		return float64(atomic.LoadInt64(counter)), nil
	}
	return 0, nil
}

// SetGauge 设置 Gauge 值
func (m *MemoryMetrics) SetGauge(name string, value float64, labels map[string]string) error {
	key := buildKey(name, labels)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[key] = &value
	return nil
}

// GetGauge 获取 Gauge 值
func (m *MemoryMetrics) GetGauge(name string, labels map[string]string) (float64, error) {
	key := buildKey(name, labels)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if gauge, exists := m.gauges[key]; exists {
		return *gauge, nil
	}
	return 0, nil
}

// ObserveHistogram 记录 Histogram 值（Memory 实现不支持，返回 nil）
func (m *MemoryMetrics) ObserveHistogram(name string, value float64, labels map[string]string) error {
	// Memory 实现不支持 Histogram，静默忽略
	return nil
}

// Close 关闭指标收集器
func (m *MemoryMetrics) Close() error {
	return m.ResourceBase.Close()
}

// buildKey 构建指标键名
func buildKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	// 按标签键名排序，确保相同标签集合生成相同的 key
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	key := name
	for _, k := range keys {
		key = fmt.Sprintf("%s{%s=%s}", key, k, labels[k])
	}
	return key
}
