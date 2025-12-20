package security

import (
	"context"
	"fmt"
	"sync"
	"time"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/core/dispose"
)

// BruteForceProtector 暴力破解防护器
//
// 职责：
//   - 跟踪IP级别的失败次数
//   - 自动封禁超过阈值的IP
//   - 自动解封过期的IP
//   - 提供查询和管理接口
//
// 设计：
//   - 使用内存缓存存储失败记录（考虑性能）
//   - 使用Redis存储封禁列表（跨节点共享）
//   - 定期清理过期数据
type BruteForceProtector struct {
	*dispose.ServiceBase

	// 配置
	config *BruteForceConfig

	// 失败记录（内存缓存，按IP）
	failures map[string]*FailureRecord
	mu       sync.RWMutex

	// 封禁列表（可选：使用Redis共享）
	bannedIPs map[string]*BanRecord
	banMu     sync.RWMutex
}

// BruteForceConfig 暴力破解防护配置
type BruteForceConfig struct {
	// 失败阈值
	MaxFailures int           // 最大失败次数（默认: 5）
	TimeWindow  time.Duration // 时间窗口（默认: 5分钟）

	// 封禁设置
	BanDuration    time.Duration // 封禁时长（默认: 30分钟）
	PermanentBanAt int           // 永久封禁阈值（默认: 20次）

	// 清理设置
	CleanupInterval time.Duration // 清理间隔（默认: 1分钟）
}

// DefaultBruteForceConfig 默认配置
func DefaultBruteForceConfig() *BruteForceConfig {
	return &BruteForceConfig{
		MaxFailures:     5,
		TimeWindow:      5 * time.Minute,
		BanDuration:     30 * time.Minute,
		PermanentBanAt:  20,
		CleanupInterval: 1 * time.Minute,
	}
}

// FailureRecord 失败记录
type FailureRecord struct {
	IP           string      // IP地址
	Failures     []time.Time // 失败时间列表
	TotalCount   int         // 累计失败次数（包括已清理的）
	FirstFailure time.Time   // 首次失败时间
	LastFailure  time.Time   // 最后失败时间
}

// BanRecord 封禁记录
type BanRecord struct {
	IP        string    // IP地址
	BannedAt  time.Time // 封禁时间
	ExpiresAt time.Time // 过期时间（零值表示永久封禁）
	Reason    string    // 封禁原因
	Count     int       // 失败次数
}

// NewBruteForceProtector 创建暴力破解防护器
func NewBruteForceProtector(config *BruteForceConfig, ctx context.Context) *BruteForceProtector {
	if config == nil {
		config = DefaultBruteForceConfig()
	}

	protector := &BruteForceProtector{
		ServiceBase: dispose.NewService("BruteForceProtector", ctx),
		config:      config,
		failures:    make(map[string]*FailureRecord),
		bannedIPs:   make(map[string]*BanRecord),
	}

	// 启动后台清理任务
	go protector.cleanupTask(ctx)

	return protector
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 核心功能
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// RecordFailure 记录一次失败尝试
//
// 返回：是否应该封禁此IP
func (p *BruteForceProtector) RecordFailure(ip string) bool {
	p.mu.Lock()

	now := time.Now()

	// 获取或创建失败记录
	record, exists := p.failures[ip]
	if !exists {
		record = &FailureRecord{
			IP:           ip,
			Failures:     make([]time.Time, 0),
			FirstFailure: now,
		}
		p.failures[ip] = record
	}

	// 记录失败
	record.Failures = append(record.Failures, now)
	record.TotalCount++
	record.LastFailure = now

	// 清理时间窗口外的失败记录
	p.cleanupOldFailures(record)

	// 检查是否需要封禁
	recentFailures := len(record.Failures)
	totalCount := record.TotalCount

	// 释放锁，避免在 banIP 中死锁
	p.mu.Unlock()

	// 永久封禁判断
	if totalCount >= p.config.PermanentBanAt {
		p.banIP(ip, 0, fmt.Sprintf("累计失败 %d 次", totalCount), totalCount)
		corelog.Warnf("BruteForce: IP %s permanently banned after %d total failures", ip, totalCount)
		return true
	}

	// 临时封禁判断
	if recentFailures >= p.config.MaxFailures {
		p.banIP(ip, p.config.BanDuration, fmt.Sprintf("时间窗口内失败 %d 次", recentFailures), totalCount)
		corelog.Warnf("BruteForce: IP %s banned for %v after %d failures in time window",
			ip, p.config.BanDuration, recentFailures)
		return true
	}

	corelog.Debugf("BruteForce: IP %s failed %d/%d times (total: %d)",
		ip, recentFailures, p.config.MaxFailures, totalCount)

	return false
}

// RecordSuccess 记录一次成功（清除失败记录）
func (p *BruteForceProtector) RecordSuccess(ip string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.failures, ip)
	corelog.Debugf("BruteForce: IP %s cleared failures after success", ip)
}

// IsBanned 检查IP是否被封禁
func (p *BruteForceProtector) IsBanned(ip string) (bool, string) {
	p.banMu.RLock()
	defer p.banMu.RUnlock()

	record, exists := p.bannedIPs[ip]
	if !exists {
		return false, ""
	}

	// 检查是否过期
	if !record.ExpiresAt.IsZero() && time.Now().After(record.ExpiresAt) {
		// 已过期，异步解封
		go p.UnbanIP(ip)
		return false, ""
	}

	return true, record.Reason
}

// BanIP 封禁IP
func (p *BruteForceProtector) BanIP(ip string, duration time.Duration, reason string) {
	// 获取失败次数
	p.mu.RLock()
	count := 0
	if record, exists := p.failures[ip]; exists {
		count = record.TotalCount
	}
	p.mu.RUnlock()

	// banIP 内部会获取锁，这里直接调用
	p.banIP(ip, duration, reason, count)
}

// banIP 内部封禁方法（需要获取 banMu 锁）
// count: 失败次数，由调用者提供以避免死锁
func (p *BruteForceProtector) banIP(ip string, duration time.Duration, reason string, count int) {
	// 获取封禁锁
	p.banMu.Lock()
	defer p.banMu.Unlock()

	now := time.Now()
	var expiresAt time.Time

	if duration > 0 {
		expiresAt = now.Add(duration)
	}

	p.bannedIPs[ip] = &BanRecord{
		IP:        ip,
		BannedAt:  now,
		ExpiresAt: expiresAt,
		Reason:    reason,
		Count:     count,
	}

	if duration > 0 {
		corelog.Infof("BruteForce: IP %s banned until %v (%s)", ip, expiresAt.Format(time.RFC3339), reason)
	} else {
		corelog.Warnf("BruteForce: IP %s PERMANENTLY banned (%s)", ip, reason)
	}
}

// UnbanIP 解封IP
func (p *BruteForceProtector) UnbanIP(ip string) {
	p.banMu.Lock()
	defer p.banMu.Unlock()

	if _, exists := p.bannedIPs[ip]; exists {
		delete(p.bannedIPs, ip)
		corelog.Infof("BruteForce: IP %s unbanned", ip)
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 查询和统计
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetFailureCount 获取IP的失败次数
func (p *BruteForceProtector) GetFailureCount(ip string) int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if record, exists := p.failures[ip]; exists {
		return len(record.Failures)
	}
	return 0
}

// GetBannedIPs 获取所有被封禁的IP列表
func (p *BruteForceProtector) GetBannedIPs() []*BanRecord {
	p.banMu.RLock()
	defer p.banMu.RUnlock()

	result := make([]*BanRecord, 0, len(p.bannedIPs))
	for _, record := range p.bannedIPs {
		// 复制记录，避免外部修改
		recordCopy := *record
		result = append(result, &recordCopy)
	}

	return result
}

// GetStats 获取统计信息
func (p *BruteForceProtector) GetStats() *BruteForceStats {
	p.mu.RLock()
	p.banMu.RLock()
	defer p.mu.RUnlock()
	defer p.banMu.RUnlock()

	stats := &BruteForceStats{
		TotalFailureRecords: len(p.failures),
		TotalBannedIPs:      len(p.bannedIPs),
		PermanentBans:       0,
		TemporaryBans:       0,
	}

	for _, record := range p.bannedIPs {
		if record.ExpiresAt.IsZero() {
			stats.PermanentBans++
		} else {
			stats.TemporaryBans++
		}
	}

	return stats
}

// BruteForceStats 统计信息
type BruteForceStats struct {
	TotalFailureRecords int // 失败记录数
	TotalBannedIPs      int // 封禁IP数
	PermanentBans       int // 永久封禁数
	TemporaryBans       int // 临时封禁数
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 清理任务
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cleanupTask 后台清理任务
func (p *BruteForceProtector) cleanupTask(ctx context.Context) {
	ticker := time.NewTicker(p.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			corelog.Infof("BruteForce: cleanup task stopped")
			return
		case <-ticker.C:
			p.cleanup()
		}
	}
}

// cleanup 执行清理
func (p *BruteForceProtector) cleanup() {
	now := time.Now()

	// 清理失败记录
	p.mu.Lock()
	for ip, record := range p.failures {
		p.cleanupOldFailures(record)

		// 如果没有剩余失败记录，删除整个记录
		if len(record.Failures) == 0 {
			delete(p.failures, ip)
		}
	}
	p.mu.Unlock()

	// 清理过期的封禁
	p.banMu.Lock()
	for ip, record := range p.bannedIPs {
		// 永久封禁不清理
		if record.ExpiresAt.IsZero() {
			continue
		}

		// 已过期，解封
		if now.After(record.ExpiresAt) {
			delete(p.bannedIPs, ip)
			corelog.Debugf("BruteForce: IP %s unbanned (expired)", ip)
		}
	}
	p.banMu.Unlock()
}

// cleanupOldFailures 清理时间窗口外的失败记录（调用者需持有锁）
func (p *BruteForceProtector) cleanupOldFailures(record *FailureRecord) {
	if len(record.Failures) == 0 {
		return
	}

	cutoff := time.Now().Add(-p.config.TimeWindow)

	// 保留时间窗口内的失败记录
	validFailures := make([]time.Time, 0)
	for _, failTime := range record.Failures {
		if failTime.After(cutoff) {
			validFailures = append(validFailures, failTime)
		}
	}

	record.Failures = validFailures
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Reset 重置所有数据（仅用于测试）
func (p *BruteForceProtector) Reset() {
	p.mu.Lock()
	p.banMu.Lock()
	defer p.mu.Unlock()
	defer p.banMu.Unlock()

	p.failures = make(map[string]*FailureRecord)
	p.bannedIPs = make(map[string]*BanRecord)

	corelog.Infof("BruteForce: all data reset")
}
