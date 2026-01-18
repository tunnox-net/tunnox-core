package factories

import (
	"context"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/core/storage/postgres"
)

// CreateBuiltinCloudControlDeps 创建内置云控所需的完整依赖
// 用于创建完整功能的 BuiltinCloudControl（包含所有 Services）
func CreateBuiltinCloudControlDeps(stor storage.Storage, parentCtx context.Context) *managers.CloudControlDeps {
	// 创建基础 Repository
	repo := repos.NewRepository(stor)
	return CreateBuiltinCloudControlDepsWithRepo(stor, repo, parentCtx)
}

// CreateBuiltinCloudControlDepsWithRepo 使用指定的 Repository 创建内置云控所需的完整依赖
// 用于确保 CloudControl 与其他组件共享同一个 Repository 实例
func CreateBuiltinCloudControlDepsWithRepo(stor storage.Storage, repo *repos.Repository, parentCtx context.Context) *managers.CloudControlDeps {
	// 创建各个 Repository
	userRepo := repos.NewUserRepository(repo)
	clientRepo := repos.NewClientRepository(repo)
	mappingRepo := repos.NewPortMappingRepo(repo)
	nodeRepo := repos.NewNodeRepository(repo)
	connRepo := repos.NewConnectionRepo(parentCtx, repo)

	// 创建分离的客户端 Repository
	configRepo := repos.NewClientConfigRepository(repo)
	stateRepo := repos.NewClientStateRepository(parentCtx, stor)
	tokenRepo := repos.NewClientTokenRepository(parentCtx, stor)

	// 创建 ID 管理器
	idManager := idgen.NewIDManager(stor, parentCtx)

	// 创建简化版统计提供者
	statsProvider, _ := services.NewSimpleStatsProvider(stor, parentCtx)

	// 创建 Services
	userService := services.NewUserService(userRepo, idManager, statsProvider.GetCounter(), parentCtx)
	clientService := services.NewClientService(
		configRepo, stateRepo, tokenRepo,
		clientRepo, mappingRepo,
		idManager, statsProvider, parentCtx,
	)
	mappingService := services.NewPortMappingService(mappingRepo, idManager, statsProvider.GetCounter(), parentCtx)
	nodeService := services.NewNodeService(nodeRepo, idManager, parentCtx)
	connService := services.NewConnectionService(connRepo, idManager, parentCtx)
	anonymousService := services.NewAnonymousService(clientRepo, configRepo, mappingRepo, idManager, parentCtx)
	statsService := services.NewstatsService(userRepo, clientRepo, mappingRepo, nodeRepo, parentCtx)

	return &managers.CloudControlDeps{
		UserService:        userService,
		ClientService:      clientService,
		PortMappingService: mappingService,
		NodeService:        nodeService,
		ConnectionService:  connService,
		AnonymousService:   anonymousService,
		StatsService:       statsService,
	}
}

// NewBuiltinCloudControlWithServices 创建带完整 Services 的内置云控实例
// 这是推荐的创建方式，提供完整的功能支持
func NewBuiltinCloudControlWithServices(parentCtx context.Context, config *managers.ControlConfig) *managers.BuiltinCloudControl {
	stor := storage.NewMemoryStorage(parentCtx)
	deps := CreateBuiltinCloudControlDeps(stor, parentCtx)
	return managers.NewBuiltinCloudControlWithDeps(parentCtx, config, stor, deps)
}

// NewBuiltinCloudControlWithStorageAndServices 使用指定存储创建带完整 Services 的内置云控实例
func NewBuiltinCloudControlWithStorageAndServices(parentCtx context.Context, config *managers.ControlConfig, stor storage.Storage) *managers.BuiltinCloudControl {
	deps := CreateBuiltinCloudControlDeps(stor, parentCtx)
	return managers.NewBuiltinCloudControlWithDeps(parentCtx, config, stor, deps)
}

// NewBuiltinCloudControlWithRepo 使用指定存储和 Repository 创建带完整 Services 的内置云控实例
// 这确保 CloudControl 与其他组件（如 Management API）共享同一个 Repository 实例
func NewBuiltinCloudControlWithRepo(parentCtx context.Context, config *managers.ControlConfig, stor storage.Storage, repo *repos.Repository) *managers.BuiltinCloudControl {
	deps := CreateBuiltinCloudControlDepsWithRepo(stor, repo, parentCtx)
	return managers.NewBuiltinCloudControlWithDeps(parentCtx, config, stor, deps)
}

func CreateBuiltinCloudControlDepsWithPostgres(stor storage.Storage, pg *postgres.Storage, parentCtx context.Context) *managers.CloudControlDeps {
	repo := repos.NewRepository(stor)

	userRepo := repos.NewUserRepository(repo)
	clientRepo := repos.NewClientRepository(repo)
	connRepo := repos.NewConnectionRepo(parentCtx, repo)

	configRepo := repos.NewPgClientConfigRepository(pg)
	mappingRepo := repos.NewPgPortMappingRepository(pg)
	nodeRepo := repos.NewPgNodeRepository(pg)

	stateRepo := repos.NewClientStateRepository(parentCtx, stor)
	tokenRepo := repos.NewClientTokenRepository(parentCtx, stor)

	idManager := idgen.NewIDManager(stor, parentCtx)
	statsProvider, _ := services.NewSimpleStatsProvider(stor, parentCtx)

	userService := services.NewUserService(userRepo, idManager, statsProvider.GetCounter(), parentCtx)
	clientService := services.NewClientService(
		configRepo, stateRepo, tokenRepo,
		clientRepo, mappingRepo,
		idManager, statsProvider, parentCtx,
	)
	portMappingService := services.NewPortMappingService(mappingRepo, idManager, statsProvider.GetCounter(), parentCtx)
	nodeService := services.NewNodeService(nodeRepo, idManager, parentCtx)
	connService := services.NewConnectionService(connRepo, idManager, parentCtx)
	anonymousService := services.NewAnonymousService(clientRepo, configRepo, mappingRepo, idManager, parentCtx)
	statsService := services.NewstatsService(userRepo, clientRepo, mappingRepo, nodeRepo, parentCtx)

	return &managers.CloudControlDeps{
		UserService:        userService,
		ClientService:      clientService,
		PortMappingService: portMappingService,
		NodeService:        nodeService,
		ConnectionService:  connService,
		AnonymousService:   anonymousService,
		StatsService:       statsService,
	}
}

func NewBuiltinCloudControlWithPostgres(parentCtx context.Context, config *managers.ControlConfig, stor storage.Storage, pg *postgres.Storage) *managers.BuiltinCloudControl {
	deps := CreateBuiltinCloudControlDepsWithPostgres(stor, pg, parentCtx)
	return managers.NewBuiltinCloudControlWithDeps(parentCtx, config, stor, deps)
}
