package metrics

// Metrics 指标收集接口
// 设计目标：单文件运行使用简单实现，可无缝迁移到 Prometheus
type Metrics interface {
	// Counter 操作
	IncrementCounter(name string, labels map[string]string) error
	AddCounter(name string, value float64, labels map[string]string) error
	GetCounter(name string, labels map[string]string) (float64, error)

	// Gauge 操作
	SetGauge(name string, value float64, labels map[string]string) error
	GetGauge(name string, labels map[string]string) (float64, error)

	// Histogram 操作（可选，Prometheus 实现）
	ObserveHistogram(name string, value float64, labels map[string]string) error

	// 关闭指标收集器
	Close() error
}

