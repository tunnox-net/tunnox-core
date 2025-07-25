package services

import (
	"context"
	"fmt"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/utils"
)

// StatsServiceImpl 统计服务实现
type StatsServiceImpl struct {
	statsMgr *managers.StatsManager
	utils.Dispose
}

// NewStatsService 创建统计服务
func NewStatsService(statsMgr *managers.StatsManager, parentCtx context.Context) StatsService {
	service := &StatsServiceImpl{
		statsMgr: statsMgr,
	}
	service.SetCtx(parentCtx, service.onClose)
	return service
}

// onClose 资源清理回调
func (s *StatsServiceImpl) onClose() error {
	utils.Infof("Stats service resources cleaned up")
	return nil
}

// GetSystemStats 获取系统统计信息
func (s *StatsServiceImpl) GetSystemStats() (*stats.SystemStats, error) {
	if s.statsMgr == nil {
		return nil, fmt.Errorf("stats manager not available")
	}

	systemStats, err := s.statsMgr.GetSystemStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get system stats: %w", err)
	}
	return systemStats, nil
}

// GetTrafficStats 获取流量统计
func (s *StatsServiceImpl) GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error) {
	if s.statsMgr == nil {
		return nil, fmt.Errorf("stats manager not available")
	}

	trafficStats, err := s.statsMgr.GetTrafficStats(timeRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get traffic stats: %w", err)
	}
	return trafficStats, nil
}

// GetConnectionStats 获取连接统计
func (s *StatsServiceImpl) GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error) {
	if s.statsMgr == nil {
		return nil, fmt.Errorf("stats manager not available")
	}

	connectionStats, err := s.statsMgr.GetConnectionStats(timeRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection stats: %w", err)
	}
	return connectionStats, nil
}
