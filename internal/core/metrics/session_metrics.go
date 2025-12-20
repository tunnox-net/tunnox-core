package metrics

// SessionMetrics Session 级别指标辅助函数
// 用于收集活跃 session 数、恢复的 tunnel 数等指标

// IncrementActiveSession 增加活跃 session 数
func IncrementActiveSession() error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	return m.IncrementCounter("session_active", nil)
}

// DecrementActiveSession 减少活跃 session 数
func DecrementActiveSession() error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	return m.AddCounter("session_active", -1, nil)
}

// SetActiveSessions 设置活跃 session 数（Gauge）
func SetActiveSessions(count float64) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	return m.SetGauge("session_active", count, nil)
}

// IncrementTunnelRecovery 增加恢复的 tunnel 数
func IncrementTunnelRecovery() error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	return m.IncrementCounter("tunnel_recoveries", nil)
}

// SetActiveTunnels 设置活跃 tunnel 数（Gauge）
func SetActiveTunnels(count float64) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	return m.SetGauge("tunnel_active", count, nil)
}

// IncrementTunnelCreated 增加创建的 tunnel 数
func IncrementTunnelCreated() error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	return m.IncrementCounter("tunnel_created", nil)
}

// IncrementTunnelClosed 增加关闭的 tunnel 数
func IncrementTunnelClosed() error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	return m.IncrementCounter("tunnel_closed", nil)
}

// SetControlConnections 设置控制连接数（Gauge）
func SetControlConnections(count float64) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	return m.SetGauge("connection_control", count, nil)
}

// SetDataConnections 设置数据连接数（Gauge）
func SetDataConnections(count float64) error {
	m := GetGlobalMetrics()
	if m == nil {
		return nil
	}
	return m.SetGauge("connection_data", count, nil)
}
