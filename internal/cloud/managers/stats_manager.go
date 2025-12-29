package managers

import (
	"context"
	"fmt"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage"
)

// StatsManager 统计管理器
// 通过 Service 接口访问数据，遵循 Manager -> Service -> Repository 架构
type StatsManager struct {
	*dispose.ManagerBase
	userService       services.UserService
	clientService     services.ClientService
	portMappingServic services.PortMappingService
	statsService      services.StatsService

	// 统计计数器
	counter    *stats.StatsCounter
	storage    storage.Storage
	useCounter bool // 是否使用计数器模式
}

// NewStatsManager 创建新的统计管理器
func NewStatsManager(
	userService services.UserService,
	clientService services.ClientService,
	portMappingService services.PortMappingService,
	statsService services.StatsService,
	storage storage.Storage,
	parentCtx context.Context,
) *StatsManager {
	manager := &StatsManager{
		ManagerBase:       dispose.NewManager("StatsManager", parentCtx),
		userService:       userService,
		clientService:     clientService,
		portMappingServic: portMappingService,
		statsService:      statsService,
		storage:           storage,
		useCounter:        true, // 默认使用计数器模式
	}

	// 创建统计计数器
	if manager.useCounter {
		counter, err := stats.NewStatsCounter(storage, parentCtx)
		if err != nil {
			dispose.Warnf("StatsManager: failed to create counter: %v", err)
			manager.useCounter = false // 降级到全量计算模式
		} else {
			manager.counter = counter
			// 初始化计数器
			if err := manager.counter.Initialize(); err != nil {
				dispose.Warnf("StatsManager: failed to initialize counter: %v", err)
				manager.useCounter = false // 降级到全量计算模式
			}
		}
	}

	return manager
}

// GetUserStats 获取用户统计信息
func (sm *StatsManager) GetUserStats(userID string) (*stats.UserStats, error) {
	return sm.userService.GetUserStats(userID)
}

// GetClientStats 获取客户端统计信息
func (sm *StatsManager) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	return sm.clientService.GetClientStats(clientID)
}

// GetSystemStats 获取系统整体统计 (优化版)
func (sm *StatsManager) GetSystemStats() (*stats.SystemStats, error) {
	// 1. 优先使用计数器模式 (<5ms)
	if sm.useCounter && sm.counter != nil {
		systemStats, err := sm.counter.GetGlobalStats()
		if err == nil {
			return systemStats, nil
		}

		// 计数器失败，记录日志并降级
		dispose.Warnf("StatsManager: counter mode failed: %v, falling back to full calculation", err)
	}

	// 2. 降级到通过 StatsService 获取
	return sm.statsService.GetSystemStats()
}

// RebuildStats 重建统计计数器（管理员手动触发）
func (sm *StatsManager) RebuildStats() error {
	if !sm.useCounter || sm.counter == nil {
		return fmt.Errorf("counter mode not enabled")
	}

	// 全量计算当前统计
	systemStats, err := sm.statsService.GetSystemStats()
	if err != nil {
		return fmt.Errorf("failed to calculate full stats: %w", err)
	}

	// 重建计数器
	return sm.counter.Rebuild(systemStats)
}

// GetCounter 获取统计计数器（供Service层使用）
func (sm *StatsManager) GetCounter() *stats.StatsCounter {
	return sm.counter
}

// GetTrafficStats 获取流量统计图表数据
func (sm *StatsManager) GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error) {
	return sm.statsService.GetTrafficStats(timeRange)
}

// GetConnectionStats 获取连接数统计图表数据
func (sm *StatsManager) GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error) {
	return sm.statsService.GetConnectionStats(timeRange)
}
