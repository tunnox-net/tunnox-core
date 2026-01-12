package stream

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
)

// TrafficStats 流量统计
type TrafficStats struct {
	BytesSent     int64     `json:"bytes_sent"`
	BytesReceived int64     `json:"bytes_received"`
	LastUpdated   time.Time `json:"last_updated"`
}

// TrafficMeter 流量计量器
// 统计每个隧道/用户的流量，支持周期性上报
type TrafficMeter struct {
	// 标识
	userID    int64
	mappingID string
	tunnelID  string

	// 流量统计（原子操作）
	bytesSent     atomic.Int64
	bytesReceived atomic.Int64

	// 上报相关
	reportInterval  time.Duration
	lastReportTime  time.Time
	lastReportSent  int64
	lastReportRecv  int64
	reportCallback  TrafficReportCallback
	reportLock      sync.Mutex

	// 配额限制
	monthlyLimit    int64         // 月流量限制 (-1 = 无限制)
	bandwidthLimit  int64         // 带宽限制 bytes/s (-1 = 无限制)
	throttleRate    int64         // 降速后的速率 (超限后使用)
	quotaCallback   QuotaCallback // 配额变化回调

	// 日志
	logger corelog.Logger

	dispose.Dispose
}

// TrafficReportCallback 流量上报回调
type TrafficReportCallback func(userID int64, mappingID string, bytesSent, bytesReceived int64)

// QuotaCallback 配额状态回调
type QuotaCallback func(userID int64, status QuotaStatus)

// QuotaStatus 配额状态
type QuotaStatus struct {
	// 用量
	UsedBytes  int64 `json:"used_bytes"`
	LimitBytes int64 `json:"limit_bytes"` // -1 = 无限制

	// 状态
	Percentage float64 `json:"percentage"` // 使用百分比 (0-100+)
	Exceeded   bool    `json:"exceeded"`   // 是否超限
	Throttled  bool    `json:"throttled"`  // 是否降速中

	// 带宽
	BandwidthLimit int64 `json:"bandwidth_limit"` // bytes/s, -1 = 无限制
	CurrentRate    int64 `json:"current_rate"`    // 当前速率 (降速后)
}

// TrafficMeterConfig 流量计量器配置
type TrafficMeterConfig struct {
	UserID         int64
	MappingID      string
	TunnelID       string
	ReportInterval time.Duration         // 上报间隔，默认 30s
	ReportCallback TrafficReportCallback // 流量上报回调
	QuotaCallback  QuotaCallback         // 配额变化回调
	MonthlyLimit   int64                 // 月流量限制 bytes, -1 = 无限制
	BandwidthLimit int64                 // 带宽限制 bytes/s, -1 = 无限制
	ThrottleRate   int64                 // 超限后降速速率，默认 100KB/s
	Logger         corelog.Logger
}

// DefaultTrafficMeterConfig 返回默认配置
func DefaultTrafficMeterConfig() *TrafficMeterConfig {
	return &TrafficMeterConfig{
		ReportInterval: 30 * time.Second,
		MonthlyLimit:   -1,           // 无限制
		BandwidthLimit: -1,           // 无限制
		ThrottleRate:   100 * 1024,   // 100 KB/s
		Logger:         corelog.Default(),
	}
}

// NewTrafficMeter 创建流量计量器
func NewTrafficMeter(config *TrafficMeterConfig, parentCtx context.Context) *TrafficMeter {
	if config == nil {
		config = DefaultTrafficMeterConfig()
	}
	if config.Logger == nil {
		config.Logger = corelog.Default()
	}
	if config.ReportInterval <= 0 {
		config.ReportInterval = 30 * time.Second
	}
	if config.ThrottleRate <= 0 {
		config.ThrottleRate = 100 * 1024 // 100 KB/s
	}

	tm := &TrafficMeter{
		userID:          config.UserID,
		mappingID:       config.MappingID,
		tunnelID:        config.TunnelID,
		reportInterval:  config.ReportInterval,
		reportCallback:  config.ReportCallback,
		quotaCallback:   config.QuotaCallback,
		monthlyLimit:    config.MonthlyLimit,
		bandwidthLimit:  config.BandwidthLimit,
		throttleRate:    config.ThrottleRate,
		lastReportTime:  time.Now(),
		logger:          config.Logger,
	}

	tm.SetCtx(parentCtx, tm.onClose)

	// 启动周期性上报协程
	if config.ReportCallback != nil {
		go tm.reportLoop()
	}

	return tm
}

// AddSent 记录发送流量
func (tm *TrafficMeter) AddSent(bytes int64) {
	tm.bytesSent.Add(bytes)
}

// AddReceived 记录接收流量
func (tm *TrafficMeter) AddReceived(bytes int64) {
	tm.bytesReceived.Add(bytes)
}

// GetStats 获取当前统计
func (tm *TrafficMeter) GetStats() TrafficStats {
	return TrafficStats{
		BytesSent:     tm.bytesSent.Load(),
		BytesReceived: tm.bytesReceived.Load(),
		LastUpdated:   time.Now(),
	}
}

// GetTotalBytes 获取总流量
func (tm *TrafficMeter) GetTotalBytes() int64 {
	return tm.bytesSent.Load() + tm.bytesReceived.Load()
}

// GetQuotaStatus 获取配额状态
func (tm *TrafficMeter) GetQuotaStatus() QuotaStatus {
	total := tm.GetTotalBytes()
	status := QuotaStatus{
		UsedBytes:      total,
		LimitBytes:     tm.monthlyLimit,
		BandwidthLimit: tm.bandwidthLimit,
		CurrentRate:    tm.bandwidthLimit,
	}

	if tm.monthlyLimit > 0 {
		status.Percentage = float64(total) / float64(tm.monthlyLimit) * 100
		status.Exceeded = total >= tm.monthlyLimit
		// 超过 100% 时降速
		if status.Percentage >= 100 {
			status.Throttled = true
			status.CurrentRate = tm.throttleRate
		}
	}

	return status
}

// IsExceeded 检查是否超限
func (tm *TrafficMeter) IsExceeded() bool {
	if tm.monthlyLimit <= 0 {
		return false // 无限制
	}
	return tm.GetTotalBytes() >= tm.monthlyLimit
}

// ShouldThrottle 检查是否需要降速
func (tm *TrafficMeter) ShouldThrottle() bool {
	if tm.monthlyLimit <= 0 {
		return false
	}
	// 达到 100% 时降速
	return tm.GetTotalBytes() >= tm.monthlyLimit
}

// GetEffectiveBandwidth 获取有效带宽限制
// 如果超限则返回降速后的速率
func (tm *TrafficMeter) GetEffectiveBandwidth() int64 {
	if tm.ShouldThrottle() {
		return tm.throttleRate
	}
	return tm.bandwidthLimit
}

// SetMonthlyLimit 设置月流量限制
func (tm *TrafficMeter) SetMonthlyLimit(limit int64) {
	tm.monthlyLimit = limit
	tm.checkAndNotifyQuota()
}

// SetBandwidthLimit 设置带宽限制
func (tm *TrafficMeter) SetBandwidthLimit(limit int64) {
	tm.bandwidthLimit = limit
}

// SetThrottleRate 设置超限后的降速速率
func (tm *TrafficMeter) SetThrottleRate(rate int64) {
	if rate > 0 {
		tm.throttleRate = rate
	}
}

// ForceReport 强制立即上报
func (tm *TrafficMeter) ForceReport() {
	tm.doReport()
}

// reportLoop 周期性上报协程
func (tm *TrafficMeter) reportLoop() {
	ticker := time.NewTicker(tm.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tm.doReport()
		case <-tm.Ctx().Done():
			// 关闭前最后一次上报
			tm.doReport()
			return
		}
	}
}

// doReport 执行上报
func (tm *TrafficMeter) doReport() {
	tm.reportLock.Lock()
	defer tm.reportLock.Unlock()

	if tm.reportCallback == nil {
		return
	}

	currentSent := tm.bytesSent.Load()
	currentRecv := tm.bytesReceived.Load()

	// 计算增量
	deltaSent := currentSent - tm.lastReportSent
	deltaRecv := currentRecv - tm.lastReportRecv

	// 只有有增量才上报
	if deltaSent > 0 || deltaRecv > 0 {
		tm.reportCallback(tm.userID, tm.mappingID, deltaSent, deltaRecv)
		tm.lastReportSent = currentSent
		tm.lastReportRecv = currentRecv
		tm.lastReportTime = time.Now()

		tm.logger.Debugf("TrafficMeter: reported user=%d mapping=%s sent=%d recv=%d",
			tm.userID, tm.mappingID, deltaSent, deltaRecv)
	}

	// 检查配额
	tm.checkAndNotifyQuota()
}

// checkAndNotifyQuota 检查配额并通知
func (tm *TrafficMeter) checkAndNotifyQuota() {
	if tm.quotaCallback == nil {
		return
	}

	status := tm.GetQuotaStatus()
	tm.quotaCallback(tm.userID, status)
}

// onClose 关闭时清理
func (tm *TrafficMeter) onClose() error {
	// 最后一次上报已在 reportLoop 中处理
	tm.logger.Debugf("TrafficMeter: closed user=%d mapping=%s total=%d",
		tm.userID, tm.mappingID, tm.GetTotalBytes())
	return nil
}

// UserID 获取用户 ID
func (tm *TrafficMeter) UserID() int64 {
	return tm.userID
}

// MappingID 获取映射 ID
func (tm *TrafficMeter) MappingID() string {
	return tm.mappingID
}

// TunnelID 获取隧道 ID
func (tm *TrafficMeter) TunnelID() string {
	return tm.tunnelID
}
