package services

import (
	"context"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
)

// statsService 统计服务实现
type statsService struct {
	*dispose.ServiceBase
	userRepo    *repos.UserRepository
	clientRepo  *repos.ClientRepository
	mappingRepo *repos.PortMappingRepo
	nodeRepo    *repos.NodeRepository
}

// NewstatsService 创建新的统计服务实现
func NewstatsService(userRepo *repos.UserRepository, clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, nodeRepo *repos.NodeRepository, parentCtx context.Context) StatsService {
	service := &statsService{
		ServiceBase: dispose.NewService("statsService", parentCtx),
		userRepo:    userRepo,
		clientRepo:  clientRepo,
		mappingRepo: mappingRepo,
		nodeRepo:    nodeRepo,
	}
	return service
}

// GetSystemStats 获取系统统计信息
func (s *statsService) GetSystemStats() (*stats.SystemStats, error) {
	// 获取用户总数 - 暂时设为0，因为需要指定具体的UserType
	users := []*models.User{}

	// 获取客户端总数
	clients, err := s.clientRepo.ListClients()
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get clients")
	}

	// 获取节点总数
	nodes, err := s.nodeRepo.ListNodes()
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to get nodes")
	}

	return &stats.SystemStats{
		TotalUsers:    len(users),
		TotalClients:  len(clients),
		TotalMappings: 0, // 暂时设为0，因为没有ListMappings方法
		TotalNodes:    len(nodes),
	}, nil
}

// GetTrafficStats 获取流量统计
func (s *statsService) GetTrafficStats(timeRange string) ([]*stats.TrafficDataPoint, error) {
	// 简化实现：返回空数组，实际应该从数据库查询历史数据
	return []*stats.TrafficDataPoint{}, nil
}

// GetConnectionStats 获取连接统计
func (s *statsService) GetConnectionStats(timeRange string) ([]*stats.ConnectionDataPoint, error) {
	// 简化实现：返回空数组，实际应该从数据库查询历史数据
	return []*stats.ConnectionDataPoint{}, nil
}
