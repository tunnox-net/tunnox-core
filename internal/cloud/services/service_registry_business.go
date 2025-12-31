package services

import (
	"context"

	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/base"
	"tunnox-core/internal/cloud/stats"
	"tunnox-core/internal/core/idgen"
)

// registerBusinessServices 注册业务服务
// 委托给本地实现，保持向后兼容性
func registerBusinessServices(c *container.Container, parentCtx context.Context) error {
	// 创建服务构造器
	constructors := &serviceConstructors{
		newUserService:        NewUserService,
		newClientService:      NewClientService,
		newPortMappingService: NewPortMappingService,
		newNodeService:        NewNodeService,
		newAuthService:        NewauthService,
		newAnonymousService:   NewAnonymousService,
		newConnectionService:  NewConnectionService,
		newStatsService:       NewstatsService,
	}

	return registerBusinessServicesWithConstructors(c, constructors, parentCtx)
}

// serviceConstructors 服务构造器集合
type serviceConstructors struct {
	newUserService        func(userRepo *repos.UserRepository, idManager *idgen.IDManager, counter *stats.StatsCounter, parentCtx context.Context) UserService
	newClientService      func(configRepo *repos.ClientConfigRepository, stateRepo *repos.ClientStateRepository, tokenRepo *repos.ClientTokenRepository, clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) ClientService
	newPortMappingService func(mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, counter *stats.StatsCounter, parentCtx context.Context) PortMappingService
	newNodeService        func(nodeRepo *repos.NodeRepository, idManager *idgen.IDManager, parentCtx context.Context) NodeService
	newAuthService        func(clientRepo *repos.ClientRepository, nodeRepo *repos.NodeRepository, jwtProvider JWTProvider, parentCtx context.Context) AuthService
	newAnonymousService   func(clientRepo *repos.ClientRepository, configRepo *repos.ClientConfigRepository, mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, parentCtx context.Context) AnonymousService
	newConnectionService  func(connRepo *repos.ConnectionRepo, idManager *idgen.IDManager, parentCtx context.Context) ConnectionService
	newStatsService       func(userRepo *repos.UserRepository, clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, nodeRepo *repos.NodeRepository, parentCtx context.Context) StatsService
}
