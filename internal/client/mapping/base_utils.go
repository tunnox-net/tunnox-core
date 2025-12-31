package mapping

import (
	"context"
	"fmt"
	"io"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/stream/transform"

	"golang.org/x/time/rate"
)

// determineCloseReason 根据错误类型判断关闭原因
func (h *BaseMappingHandler) determineCloseReason(sendErr, recvErr error) string {
	// 无错误，正常关闭
	if sendErr == nil && recvErr == nil {
		return "normal"
	}

	// 检查常见错误类型
	for _, err := range []error{sendErr, recvErr} {
		if err == nil {
			continue
		}
		errStr := err.Error()
		// EOF 表示对端正常关闭
		if errStr == "EOF" || errStr == "io: read/write on closed pipe" {
			return "peer_closed"
		}
		// 网络错误
		if contains(errStr, "connection reset") || contains(errStr, "broken pipe") {
			return "network_error"
		}
		if contains(errStr, "timeout") || contains(errStr, "deadline exceeded") {
			return "timeout"
		}
		if contains(errStr, "use of closed") {
			return "closed"
		}
	}

	return "error"
}

// contains 检查字符串是否包含子串（避免导入 strings 包）
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// checkConnectionQuota 检查连接数配额
func (h *BaseMappingHandler) checkConnectionQuota() error {
	// 优先使用mapping配置的连接数限制
	maxConn := h.config.MaxConnections
	if maxConn <= 0 {
		// 如果mapping未设置，使用用户配额的全局限制
		quota, err := h.client.GetUserQuota()
		if err != nil {
			// 如果获取配额失败，记录日志但不阻塞连接
			corelog.Warnf("BaseMappingHandler[%s]: failed to get quota: %v", h.config.MappingID, err)
			return nil
		}
		maxConn = quota.MaxConnections
	}

	// 检查连接数限制
	if maxConn > 0 {
		if int(h.activeConnCount.Load()) >= maxConn {
			return coreerrors.Newf(coreerrors.CodeResourceExhausted, "max connections reached: %d/%d", h.activeConnCount.Load(), maxConn)
		}
	}

	return nil
}

// wrapConnectionForControl 包装连接以进行速率限制和流量统计
func (h *BaseMappingHandler) wrapConnectionForControl(
	conn io.ReadWriteCloser,
	direction string,
) io.ReadWriteCloser {
	return &controlledConn{
		ReadWriteCloser: conn,
		rateLimiter:     h.rateLimiter,
		stats:           h.trafficStats,
		direction:       direction,
		ctx:             h.Ctx(), // 使用 handler 的 context，确保能接收退出信号
	}
}

// createTransformer 创建流转换器
// 注意：压缩和加密已移至StreamProcessor，Transform只处理限速
func (h *BaseMappingHandler) createTransformer() error {
	transformConfig := &transform.TransformConfig{
		BandwidthLimit: h.config.BandwidthLimit,
	}

	transformer, err := transform.NewTransformer(transformConfig)
	if err != nil {
		return err
	}

	h.transformer = transformer
	corelog.Debugf("BaseMappingHandler[%s]: transformer created, bandwidth_limit=%d bytes/s",
		h.config.MappingID, h.config.BandwidthLimit)
	return nil
}

// generateTunnelID 生成隧道ID
func (h *BaseMappingHandler) generateTunnelID() string {
	return fmt.Sprintf("%s-tunnel-%d-%d",
		h.adapter.GetProtocol(),
		time.Now().UnixNano(),
		h.config.LocalPort,
	)
}

// reportStatsLoop 定期上报流量统计
func (h *BaseMappingHandler) reportStatsLoop() {
	for {
		select {
		case <-h.Ctx().Done():
			return
		case <-h.statsReportTicker.C:
			h.reportStats()
		}
	}
}

// reportStats 上报流量统计
func (h *BaseMappingHandler) reportStats() {
	bytesSent := h.trafficStats.BytesSent.Swap(0)
	bytesReceived := h.trafficStats.BytesReceived.Swap(0)

	if bytesSent > 0 || bytesReceived > 0 {
		if err := h.client.TrackTraffic(h.config.MappingID, bytesSent, bytesReceived); err != nil {
			corelog.Warnf("BaseMappingHandler[%s]: failed to report stats: %v", h.config.MappingID, err)
			// 回滚计数（避免丢失）
			h.trafficStats.BytesSent.Add(bytesSent)
			h.trafficStats.BytesReceived.Add(bytesReceived)
		} else {
			corelog.Debugf("BaseMappingHandler[%s]: reported stats - sent=%d, received=%d",
				h.config.MappingID, bytesSent, bytesReceived)
		}
	}
}

// controlledConn 包装的连接（带速率限制和流量统计）
type controlledConn struct {
	io.ReadWriteCloser
	rateLimiter *rate.Limiter
	stats       *TrafficStats
	direction   string          // "local" or "tunnel"
	ctx         context.Context // context 用于接收退出信号
}

func (c *controlledConn) Read(p []byte) (n int, err error) {
	// 速率限制（如果启用）
	if c.rateLimiter != nil {
		if err := c.rateLimiter.WaitN(c.ctx, len(p)); err != nil {
			return 0, err
		}
	}

	// 读取数据
	n, err = c.ReadWriteCloser.Read(p)

	// 流量统计
	if n > 0 {
		if c.direction == "tunnel" {
			c.stats.BytesReceived.Add(int64(n))
		} else {
			c.stats.BytesSent.Add(int64(n))
		}
	}

	return n, err
}

func (c *controlledConn) Write(p []byte) (n int, err error) {
	// 速率限制（如果启用）
	if c.rateLimiter != nil {
		if err := c.rateLimiter.WaitN(c.ctx, len(p)); err != nil {
			return 0, err
		}
	}

	// 写入数据
	n, err = c.ReadWriteCloser.Write(p)

	// 流量统计
	if n > 0 {
		if c.direction == "tunnel" {
			c.stats.BytesSent.Add(int64(n))
		} else {
			c.stats.BytesReceived.Add(int64(n))
		}
	}

	return n, err
}
