package security

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage"
)

// IPManager IP管理器
//
// 职责：
//   - 管理IP黑名单和白名单
//   - 支持CIDR网段匹配
//   - 持久化到Storage（跨节点共享）
//   - 提供查询和管理接口
//
// 设计：
//   - 使用Storage持久化黑白名单
//   - 使用内存缓存加速查询
//   - 白名单优先级高于黑名单
type IPManager struct {
	*dispose.ServiceBase
	
	// 存储
	storage storage.Storage
	
	// 内存缓存
	blacklist map[string]*IPRecord // IP黑名单
	whitelist map[string]*IPRecord // IP白名单
	mu        sync.RWMutex
}

// IPRecord IP记录
type IPRecord struct {
	IP        string    // IP地址或CIDR网段
	AddedAt   time.Time // 添加时间
	ExpiresAt time.Time // 过期时间（零值表示永久）
	Reason    string    // 原因
	AddedBy   string    // 添加者
}

// IPType IP类型
type IPType string

const (
	IPTypeBlacklist IPType = "blacklist" // 黑名单
	IPTypeWhitelist IPType = "whitelist" // 白名单
)

// NewIPManager 创建IP管理器
func NewIPManager(storage storage.Storage, ctx context.Context) *IPManager {
	manager := &IPManager{
		ServiceBase: dispose.NewService("IPManager", ctx),
		storage:     storage,
		blacklist:   make(map[string]*IPRecord),
		whitelist:   make(map[string]*IPRecord),
	}
	
	// 加载持久化的黑白名单
	if err := manager.loadFromStorage(); err != nil {
		corelog.Warnf("IPManager: failed to load from storage: %v", err)
	}
	
	// 启动后台清理任务
	go manager.cleanupTask(ctx)
	
	return manager
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 核心功能
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// IsAllowed 检查IP是否允许访问
//
// 逻辑：
//   1. 白名单优先：如果在白名单中，直接允许
//   2. 黑名单检查：如果在黑名单中，拒绝
//   3. 默认允许
func (m *IPManager) IsAllowed(ip string) (bool, string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// 1. 检查白名单（优先级最高）
	if m.isInList(ip, m.whitelist) {
		return true, ""
	}
	
	// 2. 检查黑名单
	if record := m.findInList(ip, m.blacklist); record != nil {
		// 检查是否过期
		if !record.ExpiresAt.IsZero() && time.Now().After(record.ExpiresAt) {
			// 已过期，异步移除
			go m.RemoveFromBlacklist(ip)
			return true, ""
		}
		return false, record.Reason
	}
	
	// 3. 默认允许
	return true, ""
}

// AddToBlacklist 添加到黑名单
func (m *IPManager) AddToBlacklist(ip string, duration time.Duration, reason string, addedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 验证IP格式
	if err := validateIPOrCIDR(ip); err != nil {
		return fmt.Errorf("invalid IP or CIDR: %w", err)
	}
	
	now := time.Now()
	var expiresAt time.Time
	if duration > 0 {
		expiresAt = now.Add(duration)
	}
	
	record := &IPRecord{
		IP:        ip,
		AddedAt:   now,
		ExpiresAt: expiresAt,
		Reason:    reason,
		AddedBy:   addedBy,
	}
	
	m.blacklist[ip] = record
	
	// 持久化到Storage
	if err := m.saveToStorage(IPTypeBlacklist, ip, record); err != nil {
		corelog.Warnf("IPManager: failed to save blacklist to storage: %v", err)
	}
	
	if duration > 0 {
		corelog.Infof("IPManager: added %s to blacklist (expires: %v, reason: %s)", 
			ip, expiresAt.Format(time.RFC3339), reason)
	} else {
		corelog.Warnf("IPManager: PERMANENTLY added %s to blacklist (reason: %s)", ip, reason)
	}
	
	return nil
}

// RemoveFromBlacklist 从黑名单移除
func (m *IPManager) RemoveFromBlacklist(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.blacklist[ip]; exists {
		delete(m.blacklist, ip)
		
		// 从Storage删除
		if err := m.removeFromStorage(IPTypeBlacklist, ip); err != nil {
			corelog.Warnf("IPManager: failed to remove blacklist from storage: %v", err)
		}
		
		corelog.Infof("IPManager: removed %s from blacklist", ip)
	}
}

// AddToWhitelist 添加到白名单
func (m *IPManager) AddToWhitelist(ip string, reason string, addedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// 验证IP格式
	if err := validateIPOrCIDR(ip); err != nil {
		return fmt.Errorf("invalid IP or CIDR: %w", err)
	}
	
	record := &IPRecord{
		IP:      ip,
		AddedAt: time.Now(),
		Reason:  reason,
		AddedBy: addedBy,
	}
	
	m.whitelist[ip] = record
	
	// 持久化到Storage
	if err := m.saveToStorage(IPTypeWhitelist, ip, record); err != nil {
		corelog.Warnf("IPManager: failed to save whitelist to storage: %v", err)
	}
	
	corelog.Infof("IPManager: added %s to whitelist (reason: %s)", ip, reason)
	
	return nil
}

// RemoveFromWhitelist 从白名单移除
func (m *IPManager) RemoveFromWhitelist(ip string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if _, exists := m.whitelist[ip]; exists {
		delete(m.whitelist, ip)
		
		// 从Storage删除
		if err := m.removeFromStorage(IPTypeWhitelist, ip); err != nil {
			corelog.Warnf("IPManager: failed to remove whitelist from storage: %v", err)
		}
		
		corelog.Infof("IPManager: removed %s from whitelist", ip)
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 查询方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetBlacklist 获取黑名单列表
func (m *IPManager) GetBlacklist() []*IPRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make([]*IPRecord, 0, len(m.blacklist))
	for _, record := range m.blacklist {
		recordCopy := *record
		result = append(result, &recordCopy)
	}
	
	return result
}

// GetWhitelist 获取白名单列表
func (m *IPManager) GetWhitelist() []*IPRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make([]*IPRecord, 0, len(m.whitelist))
	for _, record := range m.whitelist {
		recordCopy := *record
		result = append(result, &recordCopy)
	}
	
	return result
}

// GetStats 获取统计信息
func (m *IPManager) GetStats() *IPManagerStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return &IPManagerStats{
		BlacklistCount: len(m.blacklist),
		WhitelistCount: len(m.whitelist),
	}
}

// IPManagerStats 统计信息
type IPManagerStats struct {
	BlacklistCount int
	WhitelistCount int
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 内部方法
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// isInList 检查IP是否在指定列表中
func (m *IPManager) isInList(ip string, list map[string]*IPRecord) bool {
	return m.findInList(ip, list) != nil
}

// findInList 在列表中查找IP（支持CIDR）
func (m *IPManager) findInList(ip string, list map[string]*IPRecord) *IPRecord {
	// 1. 精确匹配
	if record, exists := list[ip]; exists {
		return record
	}
	
	// 2. CIDR网段匹配
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return nil
	}
	
	for cidr, record := range list {
		// 跳过非CIDR的记录
		if !strings.Contains(cidr, "/") {
			continue
		}
		
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		
		if ipNet.Contains(parsedIP) {
			return record
		}
	}
	
	return nil
}

// cleanupTask 后台清理任务
func (m *IPManager) cleanupTask(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			corelog.Infof("IPManager: cleanup task stopped")
			return
		case <-ticker.C:
			m.cleanup()
		}
	}
}

// cleanup 清理过期的黑名单条目
func (m *IPManager) cleanup() {
	now := time.Now()
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	for ip, record := range m.blacklist {
		// 永久黑名单不清理
		if record.ExpiresAt.IsZero() {
			continue
		}
		
		// 已过期，移除
		if now.After(record.ExpiresAt) {
			delete(m.blacklist, ip)
			
			// 从Storage删除
			if err := m.removeFromStorage(IPTypeBlacklist, ip); err != nil {
				corelog.Warnf("IPManager: failed to remove expired blacklist from storage: %v", err)
			}
			
			corelog.Debugf("IPManager: removed expired blacklist entry: %s", ip)
		}
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 持久化
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	keyPrefixBlacklist = "tunnox:security:ip:blacklist:"
	keyPrefixWhitelist = "tunnox:security:ip:whitelist:"
	keyIndexBlacklist  = "tunnox:security:ip:blacklist:index"
	keyIndexWhitelist  = "tunnox:security:ip:whitelist:index"
)

// loadFromStorage 从Storage加载黑白名单
func (m *IPManager) loadFromStorage() error {
	if m.storage == nil {
		return fmt.Errorf("storage not available")
	}
	
	// 加载黑名单
	if err := m.loadListFromStorage(IPTypeBlacklist); err != nil {
		return fmt.Errorf("failed to load blacklist: %w", err)
	}
	
	// 加载白名单
	if err := m.loadListFromStorage(IPTypeWhitelist); err != nil {
		return fmt.Errorf("failed to load whitelist: %w", err)
	}
	
	corelog.Infof("IPManager: loaded %d blacklist and %d whitelist entries from storage",
		len(m.blacklist), len(m.whitelist))
	
	return nil
}

// loadListFromStorage 从Storage加载指定类型的列表
func (m *IPManager) loadListFromStorage(ipType IPType) error {
	var indexKey string
	var keyPrefix string
	var targetList map[string]*IPRecord
	
	switch ipType {
	case IPTypeBlacklist:
		indexKey = keyIndexBlacklist
		keyPrefix = keyPrefixBlacklist
		targetList = m.blacklist
	case IPTypeWhitelist:
		indexKey = keyIndexWhitelist
		keyPrefix = keyPrefixWhitelist
		targetList = m.whitelist
	default:
		return fmt.Errorf("invalid IP type: %s", ipType)
	}
	
	// 获取IP列表
	listStore, ok := m.storage.(storage.ListStore)
	if !ok {
		return fmt.Errorf("storage does not support list operations")
	}
	ips, err := listStore.GetList(indexKey)
	if err != nil {
		if err == storage.ErrKeyNotFound {
			return nil // 没有数据，不是错误
		}
		return err
	}
	
	// 加载每个IP的记录
	for _, ipInterface := range ips {
		ipStr, ok := ipInterface.(string)
		if !ok {
			continue
		}
		
		key := keyPrefix + ipStr
		data, err := m.storage.Get(key)
		if err != nil {
			continue
		}
		
		record := &IPRecord{}
		dataBytes, ok := data.([]byte)
		if !ok {
			if dataStr, ok := data.(string); ok {
				dataBytes = []byte(dataStr)
			} else {
				continue
			}
		}
		if err := json.Unmarshal(dataBytes, record); err != nil {
			continue
		}
		
		targetList[ipStr] = record
	}
	
	return nil
}

// saveToStorage 保存到Storage
func (m *IPManager) saveToStorage(ipType IPType, ip string, record *IPRecord) error {
	if m.storage == nil {
		return fmt.Errorf("storage not available")
	}
	
	var indexKey string
	var keyPrefix string
	
	switch ipType {
	case IPTypeBlacklist:
		indexKey = keyIndexBlacklist
		keyPrefix = keyPrefixBlacklist
	case IPTypeWhitelist:
		indexKey = keyIndexWhitelist
		keyPrefix = keyPrefixWhitelist
	default:
		return fmt.Errorf("invalid IP type: %s", ipType)
	}
	
	// 保存记录
	key := keyPrefix + ip
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to encode record: %w", err)
	}
	
	var ttl time.Duration
	if !record.ExpiresAt.IsZero() {
		ttl = time.Until(record.ExpiresAt)
		if ttl < 0 {
			ttl = 0
		}
	}
	
	if err := m.storage.Set(key, data, ttl); err != nil {
		return fmt.Errorf("failed to save record: %w", err)
	}
	
	// 添加到索引
	listStore, ok := m.storage.(storage.ListStore)
	if !ok {
		return fmt.Errorf("storage does not support list operations")
	}
	if err := listStore.AppendToList(indexKey, ip); err != nil {
		return fmt.Errorf("failed to add to index: %w", err)
	}
	
	return nil
}

// removeFromStorage 从Storage删除
func (m *IPManager) removeFromStorage(ipType IPType, ip string) error {
	if m.storage == nil {
		return fmt.Errorf("storage not available")
	}
	
	var indexKey string
	var keyPrefix string
	
	switch ipType {
	case IPTypeBlacklist:
		indexKey = keyIndexBlacklist
		keyPrefix = keyPrefixBlacklist
	case IPTypeWhitelist:
		indexKey = keyIndexWhitelist
		keyPrefix = keyPrefixWhitelist
	default:
		return fmt.Errorf("invalid IP type: %s", ipType)
	}
	
	// 删除记录
	key := keyPrefix + ip
	if err := m.storage.Delete(key); err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}
	
	// 从索引移除
	listStore, ok := m.storage.(storage.ListStore)
	if !ok {
		return fmt.Errorf("storage does not support list operations")
	}
	if err := listStore.RemoveFromList(indexKey, ip); err != nil {
		return fmt.Errorf("failed to remove from index: %w", err)
	}
	
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 辅助函数
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// validateIPOrCIDR 验证IP或CIDR格式
func validateIPOrCIDR(ip string) error {
	// 尝试解析为IP
	if parsedIP := net.ParseIP(ip); parsedIP != nil {
		return nil
	}
	
	// 尝试解析为CIDR
	if _, _, err := net.ParseCIDR(ip); err == nil {
		return nil
	}
	
	return fmt.Errorf("invalid IP or CIDR format: %s", ip)
}

