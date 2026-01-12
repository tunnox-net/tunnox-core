package stream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
)

// QuotaEnforcer 配额执行器
// 与 platform 通信检查配额，执行限速/断开策略
type QuotaEnforcer struct {
	platformURL string
	apiToken    string
	httpClient  *http.Client

	// 用户配额缓存
	userQuotas   map[int64]*UserQuotaCache
	userQuotasMu sync.RWMutex

	// 流量计量器注册表
	meters   map[string]*TrafficMeter // key: mappingID or tunnelID
	metersMu sync.RWMutex

	// 上报配置
	reportInterval time.Duration
	syncInterval   time.Duration // 配额同步间隔

	// 配额变化通知
	quotaChangeCh chan QuotaChangeEvent

	// 状态
	running atomic.Bool

	logger corelog.Logger
	dispose.Dispose
}

// UserQuotaCache 用户配额缓存
type UserQuotaCache struct {
	UserID         int64
	Plan           string
	MonthlyLimit   int64 // bytes, -1 = 无限制
	BandwidthLimit int64 // bytes/s, -1 = 无限制
	UsedBytes      int64 // 当月已用
	LastSync       time.Time
	Exceeded       bool
	Throttled      bool
}

// QuotaChangeEvent 配额变化事件
type QuotaChangeEvent struct {
	UserID    int64
	Type      QuotaEventType
	OldStatus QuotaStatus
	NewStatus QuotaStatus
}

// QuotaEventType 配额事件类型
type QuotaEventType string

const (
	QuotaEventWarning80  QuotaEventType = "warning_80"
	QuotaEventWarning100 QuotaEventType = "warning_100"
	QuotaEventExceeded   QuotaEventType = "exceeded"
	QuotaEventThrottled  QuotaEventType = "throttled"
	QuotaEventReset      QuotaEventType = "reset"
)

// QuotaEnforcerConfig 配额执行器配置
type QuotaEnforcerConfig struct {
	PlatformURL    string
	APIToken       string
	ReportInterval time.Duration // 流量上报间隔，默认 30s
	SyncInterval   time.Duration // 配额同步间隔，默认 60s
	Logger         corelog.Logger
}

// DefaultQuotaEnforcerConfig 默认配置
func DefaultQuotaEnforcerConfig() *QuotaEnforcerConfig {
	return &QuotaEnforcerConfig{
		ReportInterval: 30 * time.Second,
		SyncInterval:   60 * time.Second,
		Logger:         corelog.Default(),
	}
}

// NewQuotaEnforcer 创建配额执行器
func NewQuotaEnforcer(config *QuotaEnforcerConfig, parentCtx context.Context) *QuotaEnforcer {
	if config == nil {
		config = DefaultQuotaEnforcerConfig()
	}
	if config.Logger == nil {
		config.Logger = corelog.Default()
	}
	if config.ReportInterval <= 0 {
		config.ReportInterval = 30 * time.Second
	}
	if config.SyncInterval <= 0 {
		config.SyncInterval = 60 * time.Second
	}

	qe := &QuotaEnforcer{
		platformURL:    config.PlatformURL,
		apiToken:       config.APIToken,
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		userQuotas:     make(map[int64]*UserQuotaCache),
		meters:         make(map[string]*TrafficMeter),
		reportInterval: config.ReportInterval,
		syncInterval:   config.SyncInterval,
		quotaChangeCh:  make(chan QuotaChangeEvent, 100),
		logger:         config.Logger,
	}

	qe.SetCtx(parentCtx, qe.onClose)

	return qe
}

// Start 启动配额执行器
func (qe *QuotaEnforcer) Start() {
	if qe.running.Swap(true) {
		return // 已经在运行
	}

	go qe.syncLoop()
	go qe.reportLoop()

	qe.logger.Info("QuotaEnforcer: started")
}

// RegisterMeter 注册流量计量器
func (qe *QuotaEnforcer) RegisterMeter(meter *TrafficMeter) {
	qe.metersMu.Lock()
	defer qe.metersMu.Unlock()

	key := meter.MappingID()
	if key == "" {
		key = meter.TunnelID()
	}
	qe.meters[key] = meter

	qe.logger.Debugf("QuotaEnforcer: registered meter for user=%d mapping=%s",
		meter.UserID(), meter.MappingID())
}

// UnregisterMeter 注销流量计量器
func (qe *QuotaEnforcer) UnregisterMeter(mappingID string) {
	qe.metersMu.Lock()
	defer qe.metersMu.Unlock()

	if meter, ok := qe.meters[mappingID]; ok {
		// 最后一次上报
		meter.ForceReport()
		delete(qe.meters, mappingID)
		qe.logger.Debugf("QuotaEnforcer: unregistered meter mapping=%s", mappingID)
	}
}

// GetUserQuota 获取用户配额（从缓存或同步）
func (qe *QuotaEnforcer) GetUserQuota(userID int64) (*UserQuotaCache, error) {
	qe.userQuotasMu.RLock()
	cached, ok := qe.userQuotas[userID]
	qe.userQuotasMu.RUnlock()

	// 缓存有效（60秒内）
	if ok && time.Since(cached.LastSync) < qe.syncInterval {
		return cached, nil
	}

	// 从 platform 同步
	return qe.syncUserQuota(userID)
}

// CheckQuota 检查配额是否允许操作
func (qe *QuotaEnforcer) CheckQuota(userID int64, action string, params map[string]interface{}) (*QuotaCheckResult, error) {
	// 先检查本地缓存
	quota, err := qe.GetUserQuota(userID)
	if err != nil {
		return nil, err
	}

	result := &QuotaCheckResult{
		Allowed: true,
		Quota:   quota,
	}

	// 本地快速检查
	if quota.MonthlyLimit > 0 && quota.UsedBytes >= quota.MonthlyLimit {
		result.Allowed = false
		result.Reason = "monthly traffic limit exceeded"
		result.Throttled = true
	}

	return result, nil
}

// QuotaCheckResult 配额检查结果
type QuotaCheckResult struct {
	Allowed   bool
	Reason    string
	Throttled bool
	Quota     *UserQuotaCache
}

// QuotaChangeChan 获取配额变化通知通道
func (qe *QuotaEnforcer) QuotaChangeChan() <-chan QuotaChangeEvent {
	return qe.quotaChangeCh
}

// syncLoop 配额同步循环
func (qe *QuotaEnforcer) syncLoop() {
	ticker := time.NewTicker(qe.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			qe.syncAllUserQuotas()
		case <-qe.Ctx().Done():
			return
		}
	}
}

// reportLoop 流量上报循环
func (qe *QuotaEnforcer) reportLoop() {
	ticker := time.NewTicker(qe.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			qe.reportAllTraffic()
		case <-qe.Ctx().Done():
			// 关闭前最后一次上报
			qe.reportAllTraffic()
			return
		}
	}
}

// syncAllUserQuotas 同步所有用户配额
func (qe *QuotaEnforcer) syncAllUserQuotas() {
	qe.metersMu.RLock()
	userIDs := make(map[int64]bool)
	for _, meter := range qe.meters {
		userIDs[meter.UserID()] = true
	}
	qe.metersMu.RUnlock()

	for userID := range userIDs {
		if _, err := qe.syncUserQuota(userID); err != nil {
			qe.logger.Warnf("QuotaEnforcer: failed to sync quota for user=%d: %v", userID, err)
		}
	}
}

// syncUserQuota 同步单个用户配额
func (qe *QuotaEnforcer) syncUserQuota(userID int64) (*UserQuotaCache, error) {
	if qe.platformURL == "" {
		// 无 platform 配置，返回默认无限制
		return &UserQuotaCache{
			UserID:         userID,
			MonthlyLimit:   -1,
			BandwidthLimit: -1,
			LastSync:       time.Now(),
		}, nil
	}

	// 调用 platform API
	url := fmt.Sprintf("%s/api/v1/internal/user/%d/quota", qe.platformURL, userID)
	req, err := http.NewRequestWithContext(qe.Ctx(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if qe.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+qe.apiToken)
	}

	resp, err := qe.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("platform returned status %d", resp.StatusCode)
	}

	var quotaResp struct {
		Plan           string `json:"plan"`
		MonthlyLimit   int64  `json:"monthly_limit"`
		BandwidthLimit int64  `json:"bandwidth_limit"`
		UsedBytes      int64  `json:"used_bytes"`
		Exceeded       bool   `json:"exceeded"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&quotaResp); err != nil {
		return nil, err
	}

	qe.userQuotasMu.Lock()
	oldCache := qe.userQuotas[userID]
	newCache := &UserQuotaCache{
		UserID:         userID,
		Plan:           quotaResp.Plan,
		MonthlyLimit:   quotaResp.MonthlyLimit,
		BandwidthLimit: quotaResp.BandwidthLimit,
		UsedBytes:      quotaResp.UsedBytes,
		LastSync:       time.Now(),
		Exceeded:       quotaResp.Exceeded,
		Throttled:      quotaResp.Exceeded, // 超限即降速
	}
	qe.userQuotas[userID] = newCache
	qe.userQuotasMu.Unlock()

	// 检查并发送配额变化事件
	qe.checkQuotaChange(userID, oldCache, newCache)

	// 更新关联的流量计量器
	qe.updateMetersForUser(userID, newCache)

	return newCache, nil
}

// checkQuotaChange 检查配额变化并发送事件
func (qe *QuotaEnforcer) checkQuotaChange(userID int64, oldCache, newCache *UserQuotaCache) {
	if oldCache == nil {
		return
	}

	// 计算使用百分比
	var oldPercent, newPercent float64
	if oldCache.MonthlyLimit > 0 {
		oldPercent = float64(oldCache.UsedBytes) / float64(oldCache.MonthlyLimit) * 100
	}
	if newCache.MonthlyLimit > 0 {
		newPercent = float64(newCache.UsedBytes) / float64(newCache.MonthlyLimit) * 100
	}

	// 80% 警告
	if oldPercent < 80 && newPercent >= 80 && newPercent < 100 {
		qe.sendQuotaEvent(userID, QuotaEventWarning80, oldCache, newCache)
	}

	// 100% 警告
	if oldPercent < 100 && newPercent >= 100 {
		qe.sendQuotaEvent(userID, QuotaEventWarning100, oldCache, newCache)
	}

	// 超限事件
	if !oldCache.Exceeded && newCache.Exceeded {
		qe.sendQuotaEvent(userID, QuotaEventExceeded, oldCache, newCache)
	}

	// 降速事件
	if !oldCache.Throttled && newCache.Throttled {
		qe.sendQuotaEvent(userID, QuotaEventThrottled, oldCache, newCache)
	}

	// 重置事件
	if oldCache.Exceeded && !newCache.Exceeded {
		qe.sendQuotaEvent(userID, QuotaEventReset, oldCache, newCache)
	}
}

// sendQuotaEvent 发送配额事件
func (qe *QuotaEnforcer) sendQuotaEvent(userID int64, eventType QuotaEventType, oldCache, newCache *UserQuotaCache) {
	event := QuotaChangeEvent{
		UserID: userID,
		Type:   eventType,
	}

	if oldCache != nil {
		event.OldStatus = QuotaStatus{
			UsedBytes:      oldCache.UsedBytes,
			LimitBytes:     oldCache.MonthlyLimit,
			BandwidthLimit: oldCache.BandwidthLimit,
			Exceeded:       oldCache.Exceeded,
			Throttled:      oldCache.Throttled,
		}
	}

	event.NewStatus = QuotaStatus{
		UsedBytes:      newCache.UsedBytes,
		LimitBytes:     newCache.MonthlyLimit,
		BandwidthLimit: newCache.BandwidthLimit,
		Exceeded:       newCache.Exceeded,
		Throttled:      newCache.Throttled,
	}

	select {
	case qe.quotaChangeCh <- event:
	default:
		qe.logger.Warnf("QuotaEnforcer: quota change channel full, dropping event for user=%d", userID)
	}

	qe.logger.Infof("QuotaEnforcer: quota event user=%d type=%s", userID, eventType)
}

// updateMetersForUser 更新用户的流量计量器配置
func (qe *QuotaEnforcer) updateMetersForUser(userID int64, quota *UserQuotaCache) {
	qe.metersMu.RLock()
	defer qe.metersMu.RUnlock()

	for _, meter := range qe.meters {
		if meter.UserID() == userID {
			meter.SetMonthlyLimit(quota.MonthlyLimit)
			meter.SetBandwidthLimit(quota.BandwidthLimit)
		}
	}
}

// reportAllTraffic 上报所有流量
func (qe *QuotaEnforcer) reportAllTraffic() {
	if qe.platformURL == "" {
		return
	}

	// 收集所有用户的流量增量
	type trafficReport struct {
		UserID    int64  `json:"user_id"`
		MappingID string `json:"mapping_id"`
		BytesSent int64  `json:"bytes_sent"`
		BytesRecv int64  `json:"bytes_received"`
	}

	var reports []trafficReport

	qe.metersMu.RLock()
	for _, meter := range qe.meters {
		stats := meter.GetStats()
		if stats.BytesSent > 0 || stats.BytesReceived > 0 {
			reports = append(reports, trafficReport{
				UserID:    meter.UserID(),
				MappingID: meter.MappingID(),
				BytesSent: stats.BytesSent,
				BytesRecv: stats.BytesReceived,
			})
		}
	}
	qe.metersMu.RUnlock()

	if len(reports) == 0 {
		return
	}

	// 批量上报
	url := fmt.Sprintf("%s/api/v1/internal/traffic/report", qe.platformURL)
	body, _ := json.Marshal(map[string]interface{}{
		"reports": reports,
	})

	req, err := http.NewRequestWithContext(qe.Ctx(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		qe.logger.Warnf("QuotaEnforcer: failed to create report request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if qe.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+qe.apiToken)
	}

	resp, err := qe.httpClient.Do(req)
	if err != nil {
		qe.logger.Warnf("QuotaEnforcer: failed to report traffic: %v", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		qe.logger.Warnf("QuotaEnforcer: traffic report returned status %d", resp.StatusCode)
	} else {
		qe.logger.Debugf("QuotaEnforcer: reported traffic for %d meters", len(reports))
	}
}

// onClose 关闭清理
func (qe *QuotaEnforcer) onClose() error {
	qe.running.Store(false)

	// 最后一次上报
	qe.reportAllTraffic()

	// 关闭通道
	close(qe.quotaChangeCh)

	qe.logger.Info("QuotaEnforcer: stopped")
	return nil
}
