package metrics

import (
	"sync"
)

var (
	globalMetrics Metrics
	globalMu      sync.RWMutex
)

// SetGlobalMetrics 设置全局 Metrics 实例
func SetGlobalMetrics(m Metrics) {
	if m == nil {
		panic("metrics: SetGlobalMetrics called with nil")
	}
	globalMu.Lock()
	defer globalMu.Unlock()
	globalMetrics = m
}

// RegisterDisposeCounter 注册释放计数器到 dispose 包（由应用层调用，避免循环依赖）
// 这个函数应该在 SetGlobalMetrics 之后调用
// setter 函数接收一个 func() 类型的参数
func RegisterDisposeCounter(setter interface{}) {
	if setter == nil {
		return
	}
	// 使用类型断言，支持不同的函数签名
	// 使用函数闭包捕获 Metrics 实例
	if fn, ok := setter.(func(func())); ok {
		fn(func() {
			if m := GetGlobalMetrics(); m != nil {
				_ = m.IncrementCounter("dispose_count", nil)
			}
		})
	}
}

// GetGlobalMetrics 获取全局 Metrics 实例
func GetGlobalMetrics() Metrics {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalMetrics
}

// MustGetGlobalMetrics 获取全局 Metrics 实例，未初始化时 panic
func MustGetGlobalMetrics() Metrics {
	m := GetGlobalMetrics()
	if m == nil {
		panic("metrics: global metrics not initialized, call SetGlobalMetrics first")
	}
	return m
}

// IncrementCounter 全局便捷方法：增加计数器
func IncrementCounter(name string, labels map[string]string) error {
	return MustGetGlobalMetrics().IncrementCounter(name, labels)
}

// AddCounter 全局便捷方法：增加计数器指定值
func AddCounter(name string, value float64, labels map[string]string) error {
	return MustGetGlobalMetrics().AddCounter(name, value, labels)
}

// SetGauge 全局便捷方法：设置 Gauge 值
func SetGauge(name string, value float64, labels map[string]string) error {
	return MustGetGlobalMetrics().SetGauge(name, value, labels)
}

// GetCounter 全局便捷方法：获取计数器值
func GetCounter(name string, labels map[string]string) (float64, error) {
	return MustGetGlobalMetrics().GetCounter(name, labels)
}

// GetGauge 全局便捷方法：获取 Gauge 值
func GetGauge(name string, labels map[string]string) (float64, error) {
	return MustGetGlobalMetrics().GetGauge(name, labels)
}
