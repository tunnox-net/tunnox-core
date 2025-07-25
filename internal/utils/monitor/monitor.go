package monitor

import (
	"runtime"
	"sync"
	"time"
)

// SystemMetrics 系统指标
type SystemMetrics struct {
	Timestamp      time.Time `json:"timestamp"`
	CPUUsage       float64   `json:"cpu_usage"`
	MemoryUsage    uint64    `json:"memory_usage"`
	MemoryTotal    uint64    `json:"memory_total"`
	GoroutineCount int       `json:"goroutine_count"`
	HeapAlloc      uint64    `json:"heap_alloc"`
	HeapSys        uint64    `json:"heap_sys"`
	HeapIdle       uint64    `json:"heap_idle"`
	HeapInuse      uint64    `json:"heap_inuse"`
	HeapReleased   uint64    `json:"heap_released"`
	HeapObjects    uint64    `json:"heap_objects"`
}

// Monitor 监控接口
type Monitor interface {
	// Start 开始监控
	Start() error

	// Stop 停止监控
	Stop() error

	// GetMetrics 获取当前指标
	GetMetrics() *SystemMetrics

	// GetMetricsHistory 获取指标历史
	GetMetricsHistory() []*SystemMetrics

	// SetInterval 设置监控间隔
	SetInterval(interval time.Duration)

	// IsRunning 是否正在运行
	IsRunning() bool
}

// DefaultMonitor 默认监控实现
type DefaultMonitor struct {
	interval   time.Duration
	history    []*SystemMetrics
	maxHistory int
	running    bool
	stopChan   chan struct{}
	mutex      sync.RWMutex
}

// NewDefaultMonitor 创建新的默认监控器
func NewDefaultMonitor(interval time.Duration, maxHistory int) *DefaultMonitor {
	return &DefaultMonitor{
		interval:   interval,
		maxHistory: maxHistory,
		history:    make([]*SystemMetrics, 0, maxHistory),
		stopChan:   make(chan struct{}),
	}
}

// Start 开始监控
func (m *DefaultMonitor) Start() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return nil
	}

	m.running = true
	go m.monitorLoop()
	return nil
}

// Stop 停止监控
func (m *DefaultMonitor) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	m.running = false
	close(m.stopChan)
	return nil
}

// GetMetrics 获取当前指标
func (m *DefaultMonitor) GetMetrics() *SystemMetrics {
	return m.collectMetrics()
}

// GetMetricsHistory 获取指标历史
func (m *DefaultMonitor) GetMetricsHistory() []*SystemMetrics {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	history := make([]*SystemMetrics, len(m.history))
	copy(history, m.history)
	return history
}

// SetInterval 设置监控间隔
func (m *DefaultMonitor) SetInterval(interval time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.interval = interval
}

// IsRunning 是否正在运行
func (m *DefaultMonitor) IsRunning() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.running
}

// monitorLoop 监控循环
func (m *DefaultMonitor) monitorLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			metrics := m.collectMetrics()
			m.addMetrics(metrics)
		case <-m.stopChan:
			return
		}
	}
}

// collectMetrics 收集系统指标
func (m *DefaultMonitor) collectMetrics() *SystemMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &SystemMetrics{
		Timestamp:      time.Now(),
		CPUUsage:       0.0, // 需要外部实现CPU使用率计算
		MemoryUsage:    memStats.Alloc,
		MemoryTotal:    memStats.Sys,
		GoroutineCount: runtime.NumGoroutine(),
		HeapAlloc:      memStats.HeapAlloc,
		HeapSys:        memStats.HeapSys,
		HeapIdle:       memStats.HeapIdle,
		HeapInuse:      memStats.HeapInuse,
		HeapReleased:   memStats.HeapReleased,
		HeapObjects:    memStats.HeapObjects,
	}
}

// addMetrics 添加指标到历史记录
func (m *DefaultMonitor) addMetrics(metrics *SystemMetrics) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.history = append(m.history, metrics)

	// 保持历史记录数量限制
	if len(m.history) > m.maxHistory {
		m.history = m.history[1:]
	}
}
