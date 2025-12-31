package services

import (
	"context"

	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/client"
	"tunnox-core/internal/core/idgen"
)

// clientService 客户端服务实现
// 向后兼容：别名到 client.Service
type clientService = client.Service

// NewClientService 创建客户端服务
// 向后兼容：委托到 client.NewService
func NewClientService(
	configRepo *repos.ClientConfigRepository,
	stateRepo *repos.ClientStateRepository,
	tokenRepo *repos.ClientTokenRepository,
	clientRepo *repos.ClientRepository,
	mappingRepo *repos.PortMappingRepo,
	idManager *idgen.IDManager,
	statsProvider StatsProvider,
	parentCtx context.Context,
) ClientService {
	return client.NewService(
		configRepo, stateRepo, tokenRepo,
		clientRepo, mappingRepo,
		idManager, statsProvider,
		parentCtx,
	)
}
