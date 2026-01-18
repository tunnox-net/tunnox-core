package services

import (
	"context"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/conncode"
	cloudstats "tunnox-core/internal/cloud/stats"
)

// ConnectionCodeService 连接码服务
// 向后兼容：别名到 conncode.Service
type ConnectionCodeService = conncode.Service

// ConnectionCodeServiceConfig 连接码服务配置
// 向后兼容：别名到 conncode.Config
type ConnectionCodeServiceConfig = conncode.Config

// DefaultConnectionCodeServiceConfig 默认配置
func DefaultConnectionCodeServiceConfig() *ConnectionCodeServiceConfig {
	return conncode.DefaultConfig()
}

// CreateConnectionCodeRequest 创建连接码请求
// 向后兼容：别名到 conncode.CreateRequest
type CreateConnectionCodeRequest = conncode.CreateRequest

// ActivateConnectionCodeRequest 激活连接码请求
// 向后兼容：别名到 conncode.ActivateRequest
type ActivateConnectionCodeRequest = conncode.ActivateRequest

// NewConnectionCodeService 创建连接码服务
func NewConnectionCodeService(
	connCodeRepo *repos.ConnectionCodeRepository,
	portMappingService PortMappingService,
	portMappingRepo repos.IPortMappingRepository,
	config *ConnectionCodeServiceConfig,
	ctx context.Context,
) *ConnectionCodeService {
	// 创建适配器以满足 conncode.PortMappingService 接口
	adapter := &portMappingServiceAdapter{svc: portMappingService}
	return conncode.NewService(connCodeRepo, adapter, portMappingRepo, config, ctx)
}

// ConnectionCodeGenerator 连接码生成器
// 向后兼容：别名到 conncode.Generator
type ConnectionCodeGenerator = conncode.Generator

// NewConnectionCodeGenerator 创建连接码生成器
func NewConnectionCodeGenerator(config *models.ConnectionCodeGenerator) *ConnectionCodeGenerator {
	return conncode.NewGenerator(config)
}

// portMappingServiceAdapter 端口映射服务适配器
// 将 PortMappingService 接口适配到 conncode.PortMappingService
type portMappingServiceAdapter struct {
	svc PortMappingService
}

func (a *portMappingServiceAdapter) CreatePortMapping(mapping *models.PortMapping) (*models.PortMapping, error) {
	return a.svc.CreatePortMapping(mapping)
}

func (a *portMappingServiceAdapter) GetPortMapping(mappingID string) (*models.PortMapping, error) {
	return a.svc.GetPortMapping(mappingID)
}

func (a *portMappingServiceAdapter) UpdatePortMapping(mapping *models.PortMapping) error {
	return a.svc.UpdatePortMapping(mapping)
}

func (a *portMappingServiceAdapter) DeletePortMapping(mappingID string) error {
	return a.svc.DeletePortMapping(mappingID)
}

func (a *portMappingServiceAdapter) UpdatePortMappingStats(mappingID string, statsData interface{}) error {
	// 类型断言将 interface{} 转为具体类型
	if trafficStats, ok := statsData.(*models.TrafficStats); ok {
		// stats.TrafficStats 是 models.TrafficStats 的别名
		return a.svc.UpdatePortMappingStats(mappingID, trafficStats)
	}
	// 尝试从 cloudstats.TrafficStats 转换（它也是 models.TrafficStats 的别名）
	if trafficStats, ok := statsData.(*cloudstats.TrafficStats); ok {
		return a.svc.UpdatePortMappingStats(mappingID, trafficStats)
	}
	return nil
}
