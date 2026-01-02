package client

import (
	"fmt"
	"time"

	"tunnox-core/internal/cloud/models"
	corelog "tunnox-core/internal/core/log"
)

// CheckMappingQuota 检查映射配额
// 这个方法由BaseMappingHandler调用，用于在建立新连接前检查配额
func (c *TunnoxClient) CheckMappingQuota(mappingID string) error {
	// 获取用户配额
	_, err := c.GetUserQuota()
	if err != nil {
		// 获取配额失败，记录日志但不阻塞连接
		corelog.Warnf("Client: failed to get quota for mapping %s: %v", mappingID, err)
		return nil
	}

	// 检查月流量限制
	allowed, err := c.CheckMonthlyTrafficLimit()
	if err != nil {
		corelog.Warnf("Client: failed to check monthly traffic for mapping %s: %v", mappingID, err)
		// 检查失败不阻塞连接
	} else if !allowed {
		corelog.Errorf("Client: monthly traffic limit exceeded, rejecting connection for mapping %s", mappingID)
		return fmt.Errorf("monthly traffic limit exceeded")
	}

	// 检查带宽限制（已在MappingConfig中单独配置，这里不重复检查）
	// 检查存储限制（如果需要）
	// 注意：连接数限制已在BaseMappingHandler.checkConnectionQuota中检查

	corelog.Debugf("Client: quota check passed for mapping %s", mappingID)
	return nil
}

// TrackTraffic 上报流量统计
// 这个方法由BaseMappingHandler定期调用（每30秒）
func (c *TunnoxClient) TrackTraffic(mappingID string, bytesSent, bytesReceived int64) error {
	if bytesSent == 0 && bytesReceived == 0 {
		return nil // 无流量，不处理
	}

	// 1. 本地累计（用于月流量检查和统计）
	c.trafficStatsMu.Lock()
	stats, exists := c.localTrafficStats[mappingID]
	if !exists {
		stats = &localMappingStats{
			lastReportTime: time.Now(),
		}
		c.localTrafficStats[mappingID] = stats
	}
	stats.mu.Lock()
	stats.bytesSent += bytesSent
	stats.bytesReceived += bytesReceived
	stats.lastReportTime = time.Now()
	totalSent := stats.bytesSent
	totalReceived := stats.bytesReceived
	stats.mu.Unlock()
	c.trafficStatsMu.Unlock()

	// 2. 记录日志
	corelog.Debugf("Client: traffic stats for %s - period(sent=%d, recv=%d), total(sent=%d, recv=%d)",
		mappingID, bytesSent, bytesReceived, totalSent, totalReceived)

	// 3. 预留：可在此处将统计数据上报服务器
	// 可以通过控制连接发送JsonCommand类型的统计报告
	// 或者通过专门的统计上报接口

	return nil
}

// GetUserQuota 获取用户配额信息
// 这个方法由BaseMappingHandler调用，用于获取当前用户的配额限制
// 使用缓存机制，每5分钟刷新一次
func (c *TunnoxClient) GetUserQuota() (*models.UserQuota, error) {
	const quotaCacheDuration = 5 * time.Minute

	// 检查缓存是否有效
	c.quotaCacheMu.RLock()
	if c.cachedQuota != nil && time.Since(c.quotaLastRefresh) < quotaCacheDuration {
		quota := c.cachedQuota
		c.quotaCacheMu.RUnlock()
		return quota, nil
	}
	c.quotaCacheMu.RUnlock()

	// 缓存失效，需要刷新
	var quota *models.UserQuota

	// 尝试通过 Management API 获取配额
	if c.apiClient != nil {
		quotaInfo, err := c.apiClient.GetQuota()
		if err != nil {
			corelog.Warnf("Client: failed to get quota from API: %v, using defaults", err)
		} else {
			quota = &models.UserQuota{
				MaxClientIDs:        quotaInfo.MaxClientIDs,
				MaxConnections:      quotaInfo.MaxConnections,
				BandwidthLimit:      quotaInfo.BandwidthLimit,
				StorageLimit:        quotaInfo.StorageLimit,
				MonthlyTrafficLimit: quotaInfo.MonthlyTrafficLimit,
				MonthlyTrafficUsed:  quotaInfo.MonthlyTrafficUsed,
				MonthlyResetDay:     quotaInfo.MonthlyResetDay,
			}
			corelog.Debugf("Client: quota retrieved from API - MaxConnections=%d, BandwidthLimit=%d, MonthlyTrafficLimit=%d, MonthlyTrafficUsed=%d",
				quota.MaxConnections, quota.BandwidthLimit, quota.MonthlyTrafficLimit, quota.MonthlyTrafficUsed)
		}
	}

	// 如果 API 获取失败或不可用，使用默认配额
	if quota == nil {
		quota = &models.UserQuota{
			MaxClientIDs:   10,
			MaxConnections: 100,
			BandwidthLimit: 0, // 0表示无限制
			StorageLimit:   0,
		}
		corelog.Debugf("Client: using default quota - MaxConnections=%d, BandwidthLimit=%d",
			quota.MaxConnections, quota.BandwidthLimit)
	}

	// 更新缓存
	c.quotaCacheMu.Lock()
	c.cachedQuota = quota
	c.quotaLastRefresh = time.Now()
	c.quotaCacheMu.Unlock()

	return quota, nil
}

// GetLocalTrafficStats 获取本地流量统计
// 用于调试和监控
func (c *TunnoxClient) GetLocalTrafficStats(mappingID string) (sent, received int64) {
	c.trafficStatsMu.RLock()
	defer c.trafficStatsMu.RUnlock()

	if stats, exists := c.localTrafficStats[mappingID]; exists {
		stats.mu.RLock()
		defer stats.mu.RUnlock()
		return stats.bytesSent, stats.bytesReceived
	}

	return 0, 0
}

// CheckMonthlyTrafficLimit 检查月流量是否超限
// 返回 true 表示未超限可以继续使用，false 表示已超限
func (c *TunnoxClient) CheckMonthlyTrafficLimit() (bool, error) {
	quota, err := c.GetUserQuota()
	if err != nil {
		// 获取配额失败，不阻塞使用
		corelog.Warnf("Client: failed to get quota for traffic check: %v, allowing traffic", err)
		return true, nil
	}

	// 无月流量限制
	if quota.MonthlyTrafficLimit == 0 {
		return true, nil
	}

	// 检查是否超限
	if quota.MonthlyTrafficUsed >= quota.MonthlyTrafficLimit {
		corelog.Warnf("Client: monthly traffic limit exceeded (used=%d, limit=%d)",
			quota.MonthlyTrafficUsed, quota.MonthlyTrafficLimit)
		return false, nil
	}

	// 计算剩余流量百分比
	remaining := quota.MonthlyTrafficLimit - quota.MonthlyTrafficUsed
	usagePercent := float64(quota.MonthlyTrafficUsed) / float64(quota.MonthlyTrafficLimit) * 100

	// 当使用量超过 80% 时发出警告
	if usagePercent >= 80 {
		corelog.Warnf("Client: monthly traffic usage at %.1f%% (remaining=%d bytes)",
			usagePercent, remaining)
	}

	return true, nil
}

// GetMonthlyTrafficUsage 获取月流量使用情况
// 返回已使用流量、总限制、使用百分比
func (c *TunnoxClient) GetMonthlyTrafficUsage() (used, limit int64, percent float64, err error) {
	quota, err := c.GetUserQuota()
	if err != nil {
		return 0, 0, 0, err
	}

	used = quota.MonthlyTrafficUsed
	limit = quota.MonthlyTrafficLimit

	if limit > 0 {
		percent = float64(used) / float64(limit) * 100
	}

	return used, limit, percent, nil
}
