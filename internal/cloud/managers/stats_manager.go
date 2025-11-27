package managers

import (
	"context"
	"fmt"
	"time"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/utils"
)

// StatsManager 统计管理器
type StatsManager struct {
	*dispose.ManagerBase
	userRepo    *repos.UserRepository
	clientRepo  *repos.ClientRepository
	mappingRepo *repos.PortMappingRepo
	nodeRepo    *repos.NodeRepository

	// 新增：统计计数器
	counter    *stats.StatsCounter
	storage    storage.Storage
	useCounter bool // 是否使用计数器模式
}

// NewStatsManager 创建新的统计管理器
func NewStatsManager(
	userRepo *repos.UserRepository,
	clientRepo *repos.ClientRepository,
	mappingRepo *repos.PortMappingRepo,
	nodeRepo *repos.NodeRepository,
	storage storage.Storage,
	parentCtx context.Context,
) *StatsManager {
	manager := &StatsManager{
		ManagerBase: dispose.NewManager("StatsManager", parentCtx),
		userRepo:    userRepo,
		clientRepo:  clientRepo,
		mappingRepo: mappingRepo,
		nodeRepo:    nodeRepo,
		storage:     storage,
		useCounter:  true, // 默认使用计数器模式
	}

	// 创建统计计数器
	if manager.useCounter {
		manager.counter = stats.NewStatsCounter(storage, parentCtx)

		// 初始化计数器
		if err := manager.counter.Initialize(); err != nil {
			dispose.Warnf("StatsManager: failed to initialize counter: %v", err)
			manager.useCounter = false // 降级到全量计算模式
		}
	}

	return manager
}

// GetUserStats 获取用户统计信息
func (sm *StatsManager) GetUserStats(userID string) (*stats.UserStats, error) {
	// 获取用户的客户端
	clients, err := sm.clientRepo.ListUserClients(userID)
	if err != nil {
		return nil, err
	}

	// 获取用户的端口映射
	mappings, err := sm.mappingRepo.GetUserPortMappings(userID)
	if err != nil {
		return nil, err
	}

	// 计算统计信息
	totalClients := len(clients)
	onlineClients := 0
	totalMappings := len(mappings)
	activeMappings := 0
	totalTraffic := int64(0)
	totalConnections := int64(0)
	var lastActive time.Time

	for _, client := range clients {
		if client.Status == models.ClientStatusOnline {
			onlineClients++
		}
		if client.LastSeen != nil && client.LastSeen.After(lastActive) {
			lastActive = *client.LastSeen
		}
	}

	for _, mapping := range mappings {
		if mapping.Status == models.MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
	}

	return &stats.UserStats{
		UserID:           userID,
		TotalClients:     totalClients,
		OnlineClients:    onlineClients,
		TotalMappings:    totalMappings,
		ActiveMappings:   activeMappings,
		TotalTraffic:     totalTraffic,
		TotalConnections: totalConnections,
		LastActive:       lastActive,
	}, nil
}

// GetClientStats 获取客户端统计信息
func (sm *StatsManager) GetClientStats(clientID int64) (*stats.ClientStats, error) {
	client, err := sm.clientRepo.GetClient(utils.Int64ToString(clientID))
	if err != nil {
		return nil, err
	}

	// 获取客户端的端口映射
	mappings, err := sm.mappingRepo.GetClientPortMappings(utils.Int64ToString(clientID))
	if err != nil {
		return nil, err
	}

	// 计算统计信息
	totalMappings := len(mappings)
	activeMappings := 0
	totalTraffic := int64(0)
	totalConnections := int64(0)
	uptime := int64(0)

	for _, mapping := range mappings {
		if mapping.Status == models.MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
	}

	// 计算在线时长
	if client.LastSeen != nil && client.Status == models.ClientStatusOnline {
		uptime = int64(time.Since(*client.LastSeen).Seconds())
	}

	return &stats.ClientStats{
		ClientID:         clientID,
		UserID:           client.UserID,
		TotalMappings:    totalMappings,
		ActiveMappings:   activeMappings,
		TotalTraffic:     totalTraffic,
		TotalConnections: totalConnections,
		Uptime:           uptime,
		LastSeen:         time.Now(),
	}, nil
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

	// 2. 降级到全量计算模式 (慢，但保证可用)
	return sm.getSystemStatsFull()
}

// getSystemStatsFull 全量计算系统统计 (旧实现，作为降级方案)
func (sm *StatsManager) getSystemStatsFull() (*stats.SystemStats, error) {
	// 获取所有用户
	users, err := sm.userRepo.ListAllUsers()
	if err != nil {
		return nil, err
	}

	// 获取所有客户端
	clients, err := sm.clientRepo.ListAllClients()
	if err != nil {
		return nil, err
	}

	// 获取所有端口映射
	mappings, err := sm.mappingRepo.ListAllMappings()
	if err != nil {
		return nil, err
	}

	// 获取所有节点
	nodes, err := sm.nodeRepo.ListNodes()
	if err != nil {
		return nil, err
	}

	// 计算统计信息
	totalUsers := len(users)
	totalClients := len(clients)
	onlineClients := 0
	totalMappings := len(mappings)
	activeMappings := 0
	totalNodes := len(nodes)
	onlineNodes := 0
	totalTraffic := int64(0)
	totalConnections := int64(0)
	anonymousUsers := 0

	for _, client := range clients {
		if client.Status == models.ClientStatusOnline {
			onlineClients++
		}
		if client.Type == models.ClientTypeAnonymous {
			anonymousUsers++
		}
	}

	for _, mapping := range mappings {
		if mapping.Status == models.MappingStatusActive {
			activeMappings++
		}
		totalTraffic += mapping.TrafficStats.BytesSent + mapping.TrafficStats.BytesReceived
		totalConnections += mapping.TrafficStats.Connections
	}

	// 简单假设所有节点都在线
	onlineNodes = totalNodes

	return &stats.SystemStats{
		TotalUsers:       totalUsers,
		TotalClients:     totalClients,
		OnlineClients:    onlineClients,
		TotalMappings:    totalMappings,
		ActiveMappings:   activeMappings,
		TotalNodes:       totalNodes,
		OnlineNodes:      onlineNodes,
		TotalTraffic:     totalTraffic,
		TotalConnections: totalConnections,
		AnonymousUsers:   anonymousUsers,
	}, nil
}

// RebuildStats 重建统计计数器（管理员手动触发）
func (sm *StatsManager) RebuildStats() error {
	if !sm.useCounter || sm.counter == nil {
		return fmt.Errorf("counter mode not enabled")
	}

	// 全量计算当前统计
	systemStats, err := sm.getSystemStatsFull()
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
	// 简单实现：返回空数组
	return []*stats.TrafficDataPoint{}, nil
}

// GetConnectionStats 获取连接数统计图表数据
func (sm *StatsManager) GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error) {
	// 简单实现：返回空数组
	return []*stats.ConnectionDataPoint{}, nil
}
