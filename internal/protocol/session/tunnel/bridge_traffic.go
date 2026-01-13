// Package tunnel 提供隧道桥接和路由功能
package tunnel

import (
	"sync/atomic"
	"time"

	corelog "tunnox-core/internal/core/log"

	"golang.org/x/time/rate"
)

// ============================================================================
// 流量统计
// ============================================================================

// GetBytesSent 获取发送字节数
func (b *Bridge) GetBytesSent() int64 {
	return b.bytesSent.Load()
}

// GetBytesReceived 获取接收字节数
func (b *Bridge) GetBytesReceived() int64 {
	return b.bytesReceived.Load()
}

// AddBytesSent 增加发送字节数
func (b *Bridge) AddBytesSent(n int64) {
	b.bytesSent.Add(n)
}

// AddBytesReceived 增加接收字节数
func (b *Bridge) AddBytesReceived(n int64) {
	b.bytesReceived.Add(n)
}

// GetBytesSentPtr 获取发送字节计数器指针（用于跨节点场景）
func (b *Bridge) GetBytesSentPtr() *atomic.Int64 {
	return &b.bytesSent
}

// GetBytesReceivedPtr 获取接收字节计数器指针（用于跨节点场景）
func (b *Bridge) GetBytesReceivedPtr() *atomic.Int64 {
	return &b.bytesReceived
}

// GetRateLimiter 获取限速器
func (b *Bridge) GetRateLimiter() *rate.Limiter {
	return b.rateLimiter
}

// periodicTrafficReport 定期上报流量统计
func (b *Bridge) periodicTrafficReport() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.reportTrafficStats()
		case <-b.Ctx().Done():
			// 最终上报（带超时保护，避免阻塞导致 goroutine 泄漏）
			done := make(chan struct{})
			go func() {
				b.reportTrafficStats()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				corelog.Warnf("TunnelBridge[%s]: final traffic report timed out", b.tunnelID)
			}
			return
		}
	}
}

// reportTrafficStats 上报流量统计到CloudControl
func (b *Bridge) reportTrafficStats() {
	if b.cloudControl == nil {
		corelog.Debugf("TunnelBridge[%s]: cloudControl is nil, skipping traffic report", b.tunnelID)
		return
	}
	if b.mappingID == "" {
		corelog.Debugf("TunnelBridge[%s]: mappingID is empty, skipping traffic report", b.tunnelID)
		return
	}

	// 获取当前累计值
	currentSent := b.bytesSent.Load()
	currentReceived := b.bytesReceived.Load()

	// 获取上次上报的值
	lastSent := b.lastReportedSent.Load()
	lastReceived := b.lastReportedReceived.Load()

	// 计算增量
	deltaSent := currentSent - lastSent
	deltaReceived := currentReceived - lastReceived

	// 如果没有增量，不上报
	if deltaSent == 0 && deltaReceived == 0 {
		return
	}

	// 获取当前映射的统计数据
	mapping, err := b.cloudControl.GetPortMapping(b.mappingID)
	if err != nil {
		corelog.Errorf("TunnelBridge[%s]: failed to get mapping for traffic stats: %v", b.tunnelID, err)
		return
	}

	// 累加增量到映射统计
	trafficStats := mapping.TrafficStats
	trafficStats.BytesSent += deltaSent
	trafficStats.BytesReceived += deltaReceived
	trafficStats.LastUpdated = time.Now()

	// 更新映射统计
	if err := b.cloudControl.UpdatePortMappingStats(b.mappingID, &trafficStats); err != nil {
		corelog.Errorf("TunnelBridge[%s]: failed to update traffic stats: %v", b.tunnelID, err)
		return
	}

	// 更新上次上报的值
	b.lastReportedSent.Store(currentSent)
	b.lastReportedReceived.Store(currentReceived)

	corelog.Infof("TunnelBridge[%s]: traffic stats updated - mapping=%s, delta_sent=%d, delta_received=%d, total_sent=%d, total_received=%d",
		b.tunnelID, b.mappingID, deltaSent, deltaReceived, trafficStats.BytesSent, trafficStats.BytesReceived)
}

// StartPeriodicTrafficReport 启动定期流量上报
func (b *Bridge) StartPeriodicTrafficReport() {
	if b.cloudControl == nil {
		corelog.Warnf("TunnelBridge[%s]: cloudControl is nil, periodic traffic report disabled", b.tunnelID)
		return
	}
	if b.mappingID == "" {
		corelog.Warnf("TunnelBridge[%s]: mappingID is empty, periodic traffic report disabled", b.tunnelID)
		return
	}
	corelog.Infof("TunnelBridge[%s]: starting periodic traffic report, mappingID=%s", b.tunnelID, b.mappingID)
	go b.periodicTrafficReport()
}
