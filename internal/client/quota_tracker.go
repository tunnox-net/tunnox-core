package client

import (
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/utils"
)

// CheckMappingQuota 检查映射配额
// 这个方法由BaseMappingHandler调用，用于在建立新连接前检查配额
func (c *TunnoxClient) CheckMappingQuota(mappingID string) error {
	// 获取用户配额
	_, err := c.GetUserQuota()
	if err != nil {
		// 获取配额失败，记录日志但不阻塞连接
		utils.Warnf("Client: failed to get quota for mapping %s: %v", mappingID, err)
		return nil
	}

	// 检查带宽限制（已在MappingConfig中单独配置，这里不重复检查）
	// 检查存储限制（如果需要）
	// 注意：连接数限制已在BaseMappingHandler.checkConnectionQuota中检查

	// 未来可以在这里添加更多业务限制检查
	// 例如：月流量限制、特定时段限制等

	utils.Debugf("Client: quota check passed for mapping %s", mappingID)
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
	utils.Debugf("Client: traffic stats for %s - period(sent=%d, recv=%d), total(sent=%d, recv=%d)",
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
	// 预留：未来可通过 JsonCommand 发送 QuotaQuery 请求，从服务器获取配额信息

	// 暂时使用默认配额
	defaultQuota := &models.UserQuota{
		MaxClientIDs:   10,
		MaxConnections: 100,
		BandwidthLimit: 0, // 0表示无限制
		StorageLimit:   0,
	}

	// 更新缓存
	c.quotaCacheMu.Lock()
	c.cachedQuota = defaultQuota
	c.quotaLastRefresh = time.Now()
	c.quotaCacheMu.Unlock()

	utils.Debugf("Client: quota refreshed - MaxConnections=%d, BandwidthLimit=%d",
		defaultQuota.MaxConnections, defaultQuota.BandwidthLimit)

	return defaultQuota, nil
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

