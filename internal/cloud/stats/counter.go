package stats

import (
	"context"
	"fmt"
	"time"

	"tunnox-core/internal/core/storage"
)

const (
	// PersistentStatsKey 持久化统计数据的key
	PersistentStatsKey = "tunnox:stats:persistent:global"
	// RuntimeStatsKey 运行时统计数据的key
	RuntimeStatsKey = "tunnox:stats:runtime:global"
)

// StatsCounter 统计计数器
// 自动适配不同存储后端（Memory/Redis/Hybrid）
type StatsCounter struct {
	storage storage.Storage
	ctx     context.Context

	// 缓存层（减少Storage访问）
	localCache   *StatsCache
	cacheEnabled bool
	cacheTTL     time.Duration
}

// getHashStore 获取 HashStore 接口（如果支持）
func (sc *StatsCounter) getHashStore() (interface {
	SetHash(key string, field string, value interface{}) error
	GetHash(key string, field string) (interface{}, error)
	GetAllHash(key string) (map[string]interface{}, error)
	DeleteHash(key string, field string) error
}, error) {
	// 优先使用 FullStorage
	if fullStorage, ok := sc.storage.(storage.FullStorage); ok {
		return fullStorage, nil
	}
	// 尝试直接类型断言
	if hs, ok := sc.storage.(interface {
		SetHash(key string, field string, value interface{}) error
		GetHash(key string, field string) (interface{}, error)
		GetAllHash(key string) (map[string]interface{}, error)
		DeleteHash(key string, field string) error
	}); ok {
		return hs, nil
	}
	return nil, fmt.Errorf("storage does not support hash operations")
}

// NewStatsCounter 创建统计计数器
func NewStatsCounter(storage storage.Storage, ctx context.Context) *StatsCounter {
	// 验证存储支持 HashStore 接口（通过方法检查）
	if _, ok := storage.(interface {
		SetHash(key string, field string, value interface{}) error
		GetHash(key string, field string) (interface{}, error)
		GetAllHash(key string) (map[string]interface{}, error)
		DeleteHash(key string, field string) error
	}); !ok {
		panic("storage does not support hash operations (required for StatsCounter)")
	}

	counter := &StatsCounter{
		storage:      storage,
		ctx:          ctx,
		cacheEnabled: true,
		cacheTTL:     30 * time.Second,
	}

	// 初始化本地缓存
	counter.localCache = NewStatsCache(counter.cacheTTL)

	return counter
}

// ═══════════════════════════════════════════════════════════════════
// 持久化统计操作 (tunnox:stats:persistent:*)
// ═══════════════════════════════════════════════════════════════════

// IncrUser 增加/减少用户计数 (持久化)
func (sc *StatsCounter) IncrUser(delta int64) error {
	if err := sc.incrHashField(PersistentStatsKey, "total_users", delta); err != nil {
		return fmt.Errorf("failed to increment user count: %w", err)
	}

	sc.invalidateCache()
	return nil
}

// IncrClient 增加/减少客户端计数 (持久化)
func (sc *StatsCounter) IncrClient(delta int64) error {
	if err := sc.incrHashField(PersistentStatsKey, "total_clients", delta); err != nil {
		return fmt.Errorf("failed to increment client count: %w", err)
	}

	sc.invalidateCache()
	return nil
}

// IncrMapping 增加/减少映射计数 (持久化)
func (sc *StatsCounter) IncrMapping(delta int64) error {
	if err := sc.incrHashField(PersistentStatsKey, "total_mappings", delta); err != nil {
		return fmt.Errorf("failed to increment mapping count: %w", err)
	}

	sc.invalidateCache()
	return nil
}

// IncrNode 增加/减少节点计数 (持久化)
func (sc *StatsCounter) IncrNode(delta int64) error {
	if err := sc.incrHashField(PersistentStatsKey, "total_nodes", delta); err != nil {
		return fmt.Errorf("failed to increment node count: %w", err)
	}

	sc.invalidateCache()
	return nil
}

// ═══════════════════════════════════════════════════════════════════
// 运行时统计操作 (tunnox:stats:runtime:*)
// ═══════════════════════════════════════════════════════════════════

// SetOnlineClients 设置在线客户端数 (运行时，非持久化)
func (sc *StatsCounter) SetOnlineClients(count int64) error {
	hs, err := sc.getHashStore()
	if err != nil {
		return err
	}
	return hs.SetHash(RuntimeStatsKey, "online_clients", count)
}

// IncrOnlineClients 增加/减少在线客户端数 (运行时)
func (sc *StatsCounter) IncrOnlineClients(delta int64) error {
	if err := sc.incrHashField(RuntimeStatsKey, "online_clients", delta); err != nil {
		return fmt.Errorf("failed to increment online clients: %w", err)
	}

	sc.invalidateCache()
	return nil
}

// SetActiveMappings 设置活跃映射数 (运行时)
func (sc *StatsCounter) SetActiveMappings(count int64) error {
	hs, err := sc.getHashStore()
	if err != nil {
		return err
	}
	return hs.SetHash(RuntimeStatsKey, "active_mappings", count)
}

// IncrActiveMappings 增加/减少活跃映射数 (运行时)
func (sc *StatsCounter) IncrActiveMappings(delta int64) error {
	if err := sc.incrHashField(RuntimeStatsKey, "active_mappings", delta); err != nil {
		return fmt.Errorf("failed to increment active mappings: %w", err)
	}

	sc.invalidateCache()
	return nil
}

// SetOnlineNodes 设置在线节点数 (运行时)
func (sc *StatsCounter) SetOnlineNodes(count int64) error {
	hs, err := sc.getHashStore()
	if err != nil {
		return err
	}
	return hs.SetHash(RuntimeStatsKey, "online_nodes", count)
}

// IncrAnonymousUsers 增加/减少匿名用户数 (运行时)
func (sc *StatsCounter) IncrAnonymousUsers(delta int64) error {
	if err := sc.incrHashField(RuntimeStatsKey, "anonymous_users", delta); err != nil {
		return fmt.Errorf("failed to increment anonymous users: %w", err)
	}

	sc.invalidateCache()
	return nil
}

// ═══════════════════════════════════════════════════════════════════
// 获取统计数据
// ═══════════════════════════════════════════════════════════════════

// GetGlobalStats 获取全局统计 (带缓存)
func (sc *StatsCounter) GetGlobalStats() (*SystemStats, error) {
	// 1. 尝试从本地缓存获取
	if sc.cacheEnabled {
		if cached := sc.localCache.Get(); cached != nil {
			return cached, nil
		}
	}

	// 2. 从存储获取
	stats, err := sc.getStatsFromStorage()
	if err != nil {
		return nil, err
	}

	// 3. 写入本地缓存
	if sc.cacheEnabled {
		sc.localCache.Set(stats)
	}

	return stats, nil
}

// getStatsFromStorage 从存储获取统计数据
func (sc *StatsCounter) getStatsFromStorage() (*SystemStats, error) {
	// 获取持久化统计
	hs, err := sc.getHashStore()
	if err != nil {
		return nil, err
	}
	persistent, err := hs.GetAllHash(PersistentStatsKey)
	if err != nil && err != storage.ErrKeyNotFound {
		return nil, fmt.Errorf("failed to get persistent stats: %w", err)
	}

	// 获取运行时统计
	runtime, err := hs.GetAllHash(RuntimeStatsKey)
	if err != nil && err != storage.ErrKeyNotFound {
		return nil, fmt.Errorf("failed to get runtime stats: %w", err)
	}

	// 合并统计数据
	stats := &SystemStats{
		TotalUsers:       getInt(persistent, "total_users"),
		TotalClients:     getInt(persistent, "total_clients"),
		TotalMappings:    getInt(persistent, "total_mappings"),
		TotalNodes:       getInt(persistent, "total_nodes"),
		OnlineClients:    getInt(runtime, "online_clients"),
		ActiveMappings:   getInt(runtime, "active_mappings"),
		OnlineNodes:      getInt(runtime, "online_nodes"),
		AnonymousUsers:   getInt(runtime, "anonymous_users"),
		TotalTraffic:     getInt64(runtime, "total_traffic"),
		TotalConnections: getInt64(runtime, "total_connections"),
	}

	return stats, nil
}

// getInt 从map安全获取int值
func getInt(m map[string]interface{}, key string) int {
	if m == nil {
		return 0
	}
	if val, ok := m[key]; ok {
		if intVal, ok := val.(int64); ok {
			return int(intVal)
		}
		if intVal, ok := val.(int); ok {
			return intVal
		}
	}
	return 0
}

// getInt64 从map安全获取int64值
func getInt64(m map[string]interface{}, key string) int64 {
	if m == nil {
		return 0
	}
	if val, ok := m[key]; ok {
		if intVal, ok := val.(int64); ok {
			return intVal
		}
		if intVal, ok := val.(int); ok {
			return int64(intVal)
		}
	}
	return 0
}

// ═══════════════════════════════════════════════════════════════════
// 初始化和重建
// ═══════════════════════════════════════════════════════════════════

// Initialize 初始化计数器（系统启动时调用）
func (sc *StatsCounter) Initialize() error {
	// 检查持久化计数器是否存在
	exists, _ := sc.storage.Exists(PersistentStatsKey)

	if !exists {
		// 初始化持久化统计为0
		persistentCounters := map[string]interface{}{
			"total_users":    int64(0),
			"total_clients":  int64(0),
			"total_mappings": int64(0),
			"total_nodes":    int64(0),
		}

		for field, value := range persistentCounters {
			hs, err := sc.getHashStore()
			if err != nil {
				return err
			}
			if err := hs.SetHash(PersistentStatsKey, field, value); err != nil {
				return fmt.Errorf("failed to initialize persistent counter %s: %w", field, err)
			}
		}
	}

	// 初始化运行时统计为0（每次启动都重置）
	runtimeCounters := map[string]interface{}{
		"online_clients":    int64(0),
		"active_mappings":   int64(0),
		"online_nodes":      int64(0),
		"anonymous_users":   int64(0),
		"total_traffic":     int64(0),
		"total_connections": int64(0),
	}

	for field, value := range runtimeCounters {
		hs, err := sc.getHashStore()
		if err != nil {
			return err
		}
		if err := hs.SetHash(RuntimeStatsKey, field, value); err != nil {
			return fmt.Errorf("failed to initialize runtime counter %s: %w", field, err)
		}
	}

	return nil
}

// Rebuild 重建计数器（从数据库全量计算，管理员手动触发）
func (sc *StatsCounter) Rebuild(stats *SystemStats) error {
	// 重建持久化统计
	persistentCounters := map[string]interface{}{
		"total_users":    int64(stats.TotalUsers),
		"total_clients":  int64(stats.TotalClients),
		"total_mappings": int64(stats.TotalMappings),
		"total_nodes":    int64(stats.TotalNodes),
	}

	for field, value := range persistentCounters {
		hs, err := sc.getHashStore()
		if err != nil {
			return err
		}
		if err := hs.SetHash(PersistentStatsKey, field, value); err != nil {
			return fmt.Errorf("failed to rebuild persistent counter %s: %w", field, err)
		}
	}

	// 重建运行时统计
	runtimeCounters := map[string]interface{}{
		"online_clients":    int64(stats.OnlineClients),
		"active_mappings":   int64(stats.ActiveMappings),
		"online_nodes":      int64(stats.OnlineNodes),
		"anonymous_users":   int64(stats.AnonymousUsers),
		"total_traffic":     stats.TotalTraffic,
		"total_connections": stats.TotalConnections,
	}

	for field, value := range runtimeCounters {
		hs, err := sc.getHashStore()
		if err != nil {
			return err
		}
		if err := hs.SetHash(RuntimeStatsKey, field, value); err != nil {
			return fmt.Errorf("failed to rebuild runtime counter %s: %w", field, err)
		}
	}

	sc.invalidateCache()
	return nil
}

// ═══════════════════════════════════════════════════════════════════
// 缓存管理
// ═══════════════════════════════════════════════════════════════════

// invalidateCache 使缓存失效
func (sc *StatsCounter) invalidateCache() {
	if sc.localCache != nil {
		sc.localCache.Invalidate()
	}
}

// DisableCache 禁用本地缓存
func (sc *StatsCounter) DisableCache() {
	sc.cacheEnabled = false
}

// EnableCache 启用本地缓存
func (sc *StatsCounter) EnableCache() {
	sc.cacheEnabled = true
}

// ═══════════════════════════════════════════════════════════════════
// 辅助方法
// ═══════════════════════════════════════════════════════════════════

// incrHashField 增加Hash字段的值
func (sc *StatsCounter) incrHashField(key, field string, delta int64) error {
	// 获取当前值
	hs, err := sc.getHashStore()
	if err != nil {
		return err
	}
	val, err := hs.GetHash(key, field)
	if err != nil && err != storage.ErrKeyNotFound {
		return err
	}

	// 计算新值
	var currentVal int64
	if val != nil {
		if intVal, ok := val.(int64); ok {
			currentVal = intVal
		} else if intVal, ok := val.(int); ok {
			currentVal = int64(intVal)
		}
	}

	newVal := currentVal + delta

	// 设置新值
	return hs.SetHash(key, field, newVal)
}
