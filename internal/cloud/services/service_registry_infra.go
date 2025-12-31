package services

import (
	"context"

	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/services/base"
	"tunnox-core/internal/cloud/services/registry"
	storageCore "tunnox-core/internal/core/storage"
)

// registerInfrastructureServices 注册基础设施服务
// factories 参数包含创建 managers 实例的工厂函数，用于解决循环依赖
func registerInfrastructureServices(c *container.Container, config *configs.ControlConfig, storage storageCore.Storage, factories *ManagerFactories, parentCtx context.Context) error {
	// 转换 factories 到 registry.ManagerFactories
	var regFactories *registry.ManagerFactories
	if factories != nil {
		regFactories = &registry.ManagerFactories{
			NewJWTProvider: func(cfg interface{}, stor interface{}, ctx context.Context) base.JWTProvider {
				return factories.NewJWTProvider(cfg, stor, ctx)
			},
			NewStatsProvider: func(userRepo, clientRepo, mappingRepo, nodeRepo interface{}, stor interface{}, ctx context.Context) base.StatsProvider {
				return factories.NewStatsProvider(userRepo, clientRepo, mappingRepo, nodeRepo, stor, ctx)
			},
		}
	}

	return registry.RegisterInfrastructureServices(c, config, storage, regFactories, parentCtx)
}
