package metrics

// ProtocolMetrics 协议级别指标辅助函数
// 用于收集各种协议的连接数、错误数、RTT、重传率、分片命中率等指标

// ProtocolMetricsLabels 协议指标标签
type ProtocolMetricsLabels struct {
	Protocol string // 协议类型: tcp, websocket, quic
	Type     string // 连接类型: control, data
}

// ToMap 转换为标签 map
func (l *ProtocolMetricsLabels) ToMap() map[string]string {
	labels := make(map[string]string)
	if l.Protocol != "" {
		labels["protocol"] = l.Protocol
	}
	if l.Type != "" {
		labels["type"] = l.Type
	}
	return labels
}

// IncrementProtocolConnection 增加协议连接数
func IncrementProtocolConnection(protocol, connType string) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	labels := &ProtocolMetricsLabels{Protocol: protocol, Type: connType}
	return m.IncrementCounter("protocol_connections", labels.ToMap())
}

// DecrementProtocolConnection 减少协议连接数
func DecrementProtocolConnection(protocol, connType string) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	labels := &ProtocolMetricsLabels{Protocol: protocol, Type: connType}
	return m.AddCounter("protocol_connections", -1, labels.ToMap())
}

// SetProtocolConnections 设置协议连接数（Gauge）
func SetProtocolConnections(protocol, connType string, count float64) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	labels := &ProtocolMetricsLabels{Protocol: protocol, Type: connType}
	return m.SetGauge("protocol_connections", count, labels.ToMap())
}

// IncrementProtocolError 增加协议错误数
func IncrementProtocolError(protocol, connType, errorType string) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	labels := &ProtocolMetricsLabels{Protocol: protocol, Type: connType}
	labelMap := labels.ToMap()
	if errorType != "" {
		labelMap["error_type"] = errorType
	}
	return m.IncrementCounter("protocol_errors", labelMap)
}

// ObserveProtocolRTT 记录协议 RTT（往返时间）
func ObserveProtocolRTT(protocol, connType string, rttMs float64) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	labels := &ProtocolMetricsLabels{Protocol: protocol, Type: connType}
	return m.ObserveHistogram("protocol_rtt_ms", rttMs, labels.ToMap())
}

// IncrementProtocolRetransmission 增加协议重传次数
func IncrementProtocolRetransmission(protocol, connType string) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	labels := &ProtocolMetricsLabels{Protocol: protocol, Type: connType}
	return m.IncrementCounter("protocol_retransmissions", labels.ToMap())
}

// IncrementProtocolFragmentHit 增加协议分片命中次数（缓存命中）
func IncrementProtocolFragmentHit(protocol string) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	labels := map[string]string{"protocol": protocol}
	return m.IncrementCounter("protocol_fragment_hits", labels)
}

// IncrementProtocolFragmentMiss 增加协议分片未命中次数（缓存未命中）
func IncrementProtocolFragmentMiss(protocol string) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	labels := map[string]string{"protocol": protocol}
	return m.IncrementCounter("protocol_fragment_misses", labels)
}

// GetProtocolFragmentHitRate 获取协议分片命中率
func GetProtocolFragmentHitRate(protocol string) (float64, error) {
	m := GetGlobalMetrics()
	if m == nil {
		return 0, nil
	}
	labels := map[string]string{"protocol": protocol}
	hits, err := m.GetCounter("protocol_fragment_hits", labels)
	if err != nil {
		return 0, err
	}
	misses, err := m.GetCounter("protocol_fragment_misses", labels)
	if err != nil {
		return 0, err
	}
	total := hits + misses
	if total == 0 {
		return 0, nil
	}
	return hits / total, nil
}
