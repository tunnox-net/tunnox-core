package services

import (
	"context"
	"fmt"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
)

// StatsServiceImpl 统计服务实现
type StatsServiceImpl struct {
	*dispose.ServiceBase
	userRepo    *repos.UserRepository
	clientRepo  *repos.ClientRepository
	mappingRepo *repos.PortMappingRepo
	nodeRepo    *repos.NodeRepository
}

// NewStatsServiceImpl 创建新的统计服务实现
func NewStatsServiceImpl(userRepo *repos.UserRepository, clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, nodeRepo *repos.NodeRepository, parentCtx context.Context) *StatsServiceImpl {
	service := &StatsServiceImpl{
		ServiceBase: dispose.NewService("StatsServiceImpl", parentCtx),
		userRepo:    userRepo,
		clientRepo:  clientRepo,
		mappingRepo: mappingRepo,
		nodeRepo:    nodeRepo,
	}
	return service
}

// GetSystemStats 获取系统统计信息
func (s *StatsServiceImpl) GetSystemStats() (*stats.SystemStats, error) {
	// 获取用户总数 - 暂时设为0，因为需要指定具体的UserType
	users := []*models.User{}

	// 获取客户端总数
	clients, err := s.clientRepo.ListClients()
	if err != nil {
		return nil, fmt.Errorf("failed to get clients: %w", err)
	}

	// 获取节点总数
	nodes, err := s.nodeRepo.ListNodes()
	if err != nil {
		return nil, fmt.Errorf("failed to get nodes: %w", err)
	}

	return &stats.SystemStats{
		TotalUsers:    len(users),
		TotalClients:  len(clients),
		TotalMappings: 0, // 暂时设为0，因为没有ListMappings方法
		TotalNodes:    len(nodes),
	}, nil
}

// GetTrafficStats 获取流量统计
func (s *StatsServiceImpl) GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error) {
	// 简化实现：返回空数组，实际应该从数据库查询历史数据
	return []*stats.TrafficDataPoint{}, nil
}

// GetConnectionStats 获取连接统计
func (s *StatsServiceImpl) GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error) {
	// 简化实现：返回空数组，实际应该从数据库查询历史数据
	return []*stats.ConnectionDataPoint{}, nil
}
