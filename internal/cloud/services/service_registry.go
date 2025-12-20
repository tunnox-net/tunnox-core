package services

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"fmt"
	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/idgen"
	storageCore "tunnox-core/internal/core/storage"
)

// ServiceRegistry 服务注册器，提供依赖注入和错误处理
type ServiceRegistry struct {
	container   *container.Container
	baseService *BaseService
}

// NewServiceRegistry 创建服务注册器
func NewServiceRegistry(container *container.Container) *ServiceRegistry {
	return &ServiceRegistry{
		container:   container,
		baseService: NewBaseService(),
	}
}

// wrapResolveError 包装服务解析错误
func (r *ServiceRegistry) wrapResolveError(err error, serviceName string) error {
	return r.baseService.WrapError(err, fmt.Sprintf("resolve %s", serviceName))
}

// registerInfrastructureServices 注册基础设施服务
func registerInfrastructureServices(container *container.Container, config *managers.ControlConfig, storage storageCore.Storage, parentCtx context.Context) error {
	// 注册存储服务
	container.RegisterSingleton("storage", func() (interface{}, error) {
		if storage == nil {
			return nil, fmt.Errorf("storage is required")
		}
		return storage, nil
	})

	// 注册配置服务
	container.RegisterSingleton("config", func() (interface{}, error) {
		if config == nil {
			return nil, fmt.Errorf("config is required")
		}
		return config, nil
	})

	// 注册ID管理器
	container.RegisterSingleton("id_manager", func() (interface{}, error) {
		storageInstance, err := container.Resolve("storage")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve storage: %w", err)
		}

		storageImpl, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, fmt.Errorf("storage does not implement storage.Storage interface")
		}

		idManager := idgen.NewIDManager(storageImpl, parentCtx)
		return idManager, nil
	})

	// 注册Repository
	container.RegisterSingleton("repository", func() (interface{}, error) {
		storageInstance, err := container.Resolve("storage")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve storage: %w", err)
		}

		storageImpl, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, fmt.Errorf("storage does not implement storage.Storage interface")
		}

		repo := repos.NewRepository(storageImpl)
		return repo, nil
	})

	// 注册各个Repository
	container.RegisterSingleton("user_repository", func() (interface{}, error) {
		repoInstance, err := container.Resolve("repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve repository: %w", err)
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, fmt.Errorf("repository is not of type *repos.Repository")
		}

		userRepo := repos.NewUserRepository(repo)
		return userRepo, nil
	})

	container.RegisterSingleton("client_repository", func() (interface{}, error) {
		repoInstance, err := container.Resolve("repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve repository: %w", err)
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, fmt.Errorf("repository is not of type *repos.Repository")
		}

		clientRepo := repos.NewClientRepository(repo)
		return clientRepo, nil
	})

	container.RegisterSingleton("mapping_repository", func() (interface{}, error) {
		repoInstance, err := container.Resolve("repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve repository: %w", err)
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, fmt.Errorf("repository is not of type *repos.Repository")
		}

		mappingRepo := repos.NewPortMappingRepo(repo)
		return mappingRepo, nil
	})

	container.RegisterSingleton("node_repository", func() (interface{}, error) {
		repoInstance, err := container.Resolve("repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve repository: %w", err)
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, fmt.Errorf("repository is not of type *repos.Repository")
		}

		nodeRepo := repos.NewNodeRepository(repo)
		return nodeRepo, nil
	})

	container.RegisterSingleton("connection_repository", func() (interface{}, error) {
		repoInstance, err := container.Resolve("repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve repository: %w", err)
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, fmt.Errorf("repository is not of type *repos.Repository")
		}

		connRepo := repos.NewConnectionRepo(repo)
		return connRepo, nil
	})

	// ✅ 新增：注册分离的客户端Repository（配置、状态、Token）
	container.RegisterSingleton("client_config_repository", func() (interface{}, error) {
		repoInstance, err := container.Resolve("repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve repository: %w", err)
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, fmt.Errorf("repository is not of type *repos.Repository")
		}

		configRepo := repos.NewClientConfigRepository(repo)
		return configRepo, nil
	})

	container.RegisterSingleton("client_state_repository", func() (interface{}, error) {
		storageInstance, err := container.Resolve("storage")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve storage: %w", err)
		}

		stor, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, fmt.Errorf("storage is not of type storage.Storage")
		}

		stateRepo := repos.NewClientStateRepository(parentCtx, stor)
		return stateRepo, nil
	})

	container.RegisterSingleton("client_token_repository", func() (interface{}, error) {
		storageInstance, err := container.Resolve("storage")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve storage: %w", err)
		}

		stor, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, fmt.Errorf("storage is not of type storage.Storage")
		}

		tokenRepo := repos.NewClientTokenRepository(parentCtx, stor)
		return tokenRepo, nil
	})

	// 注册JWT管理器
	container.RegisterSingleton("jwt_manager", func() (interface{}, error) {
		configInstance, err := container.Resolve("config")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve config: %w", err)
		}

		repoInstance, err := container.Resolve("repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve repository: %w", err)
		}

		configImpl, ok := configInstance.(*managers.ControlConfig)
		if !ok {
			return nil, fmt.Errorf("config is not of type *managers.ControlConfig")
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, fmt.Errorf("repository is not of type *repos.Repository")
		}

		jwtManager := managers.NewJWTManager(configImpl, repo, parentCtx)
		return jwtManager, nil
	})

	// 注册统计管理器
	container.RegisterSingleton("stats_manager", func() (interface{}, error) {
		userRepoInstance, err := container.Resolve("user_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve user repository: %w", err)
		}

		clientRepoInstance, err := container.Resolve("client_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve client repository: %w", err)
		}

		mappingRepoInstance, err := container.Resolve("mapping_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve mapping repository: %w", err)
		}

		nodeRepoInstance, err := container.Resolve("node_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve node repository: %w", err)
		}

		userRepo, ok := userRepoInstance.(*repos.UserRepository)
		if !ok {
			return nil, fmt.Errorf("user repository is not of type *repos.UserRepository")
		}

		clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
		if !ok {
			return nil, fmt.Errorf("client repository is not of type *repos.ClientRepository")
		}

		mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
		if !ok {
			return nil, fmt.Errorf("mapping repository is not of type *repos.PortMappingRepo")
		}

		nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
		if !ok {
			return nil, fmt.Errorf("node repository is not of type *repos.NodeRepository")
		}

		storageInstance, err := container.Resolve("storage")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve storage: %w", err)
		}

		stor, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, fmt.Errorf("storage is not of type storage.Storage")
		}

		statsManager := managers.NewStatsManager(userRepo, clientRepo, mappingRepo, nodeRepo, stor, parentCtx)
		return statsManager, nil
	})

	corelog.Infof("Infrastructure services registered successfully")
	return nil
}

// registerBusinessServices 注册业务服务
func registerBusinessServices(container *container.Container, parentCtx context.Context) error {
	// 注册用户服务
	container.RegisterSingleton("user_service", func() (interface{}, error) {
		userRepoInstance, err := container.Resolve("user_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve user repository: %w", err)
		}

		idManagerInstance, err := container.Resolve("id_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve id manager: %w", err)
		}

		userRepo, ok := userRepoInstance.(*repos.UserRepository)
		if !ok {
			return nil, fmt.Errorf("user repository is not of type *repos.UserRepository")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, fmt.Errorf("id manager is not of type *idgen.IDManager")
		}

		statsManagerInstance, err := container.Resolve("stats_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve stats manager: %w", err)
		}

		statsManager, ok := statsManagerInstance.(*managers.StatsManager)
		if !ok {
			return nil, fmt.Errorf("stats manager is not of type *managers.StatsManager")
		}

		userService := NewUserService(userRepo, idManager, statsManager.GetCounter(), parentCtx)
		return userService, nil
	})

	// ✅ 重构：注册新的ClientService（使用分离的Repository）
	container.RegisterSingleton("client_service", func() (interface{}, error) {
		// 新Repository
		configRepoInstance, err := container.Resolve("client_config_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve client config repository: %w", err)
		}

		stateRepoInstance, err := container.Resolve("client_state_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve client state repository: %w", err)
		}

		tokenRepoInstance, err := container.Resolve("client_token_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve client token repository: %w", err)
		}

		// 旧Repository（兼容性）
		clientRepoInstance, err := container.Resolve("client_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve client repository: %w", err)
		}

		mappingRepoInstance, err := container.Resolve("mapping_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve mapping repository: %w", err)
		}

		idManagerInstance, err := container.Resolve("id_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve id manager: %w", err)
		}

		statsManagerInstance, err := container.Resolve("stats_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve stats manager: %w", err)
		}

		// 类型断言
		configRepo, ok := configRepoInstance.(*repos.ClientConfigRepository)
		if !ok {
			return nil, fmt.Errorf("client config repository is not of type *repos.ClientConfigRepository")
		}

		stateRepo, ok := stateRepoInstance.(*repos.ClientStateRepository)
		if !ok {
			return nil, fmt.Errorf("client state repository is not of type *repos.ClientStateRepository")
		}

		tokenRepo, ok := tokenRepoInstance.(*repos.ClientTokenRepository)
		if !ok {
			return nil, fmt.Errorf("client token repository is not of type *repos.ClientTokenRepository")
		}

		clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
		if !ok {
			return nil, fmt.Errorf("client repository is not of type *repos.ClientRepository")
		}

		mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
		if !ok {
			return nil, fmt.Errorf("mapping repository is not of type *repos.PortMappingRepo")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, fmt.Errorf("id manager is not of type *idgen.IDManager")
		}

		statsManager, ok := statsManagerInstance.(*managers.StatsManager)
		if !ok {
			return nil, fmt.Errorf("stats manager is not of type *managers.StatsManager")
		}

		// ✅ 使用新的构造函数
		clientService := NewClientService(
			configRepo, stateRepo, tokenRepo,
			clientRepo, mappingRepo,
			idManager, statsManager, parentCtx,
		)
		return clientService, nil
	})

	// 注册端口映射服务
	container.RegisterSingleton("mapping_service", func() (interface{}, error) {
		mappingRepoInstance, err := container.Resolve("mapping_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve mapping repository: %w", err)
		}

		idManagerInstance, err := container.Resolve("id_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve id manager: %w", err)
		}

		mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
		if !ok {
			return nil, fmt.Errorf("mapping repository is not of type *repos.PortMappingRepo")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, fmt.Errorf("id manager is not of type *idgen.IDManager")
		}

		statsManagerInstance, err := container.Resolve("stats_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve stats manager: %w", err)
		}

		statsManager, ok := statsManagerInstance.(*managers.StatsManager)
		if !ok {
			return nil, fmt.Errorf("stats manager is not of type *managers.StatsManager")
		}

		mappingService := NewPortMappingService(mappingRepo, idManager, statsManager.GetCounter(), parentCtx)
		return mappingService, nil
	})

	// 注册节点服务
	container.RegisterSingleton("node_service", func() (interface{}, error) {
		nodeRepoInstance, err := container.Resolve("node_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve node repository: %w", err)
		}

		idManagerInstance, err := container.Resolve("id_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve id manager: %w", err)
		}

		nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
		if !ok {
			return nil, fmt.Errorf("node repository is not of type *repos.NodeRepository")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, fmt.Errorf("id manager is not of type *idgen.IDManager")
		}

		nodeService := NewNodeService(nodeRepo, idManager, parentCtx)
		return nodeService, nil
	})

	// 注册认证服务
	container.RegisterSingleton("auth_service", func() (interface{}, error) {
		clientRepoInstance, err := container.Resolve("client_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve client repository: %w", err)
		}

		nodeRepoInstance, err := container.Resolve("node_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve node repository: %w", err)
		}

		jwtManagerInstance, err := container.Resolve("jwt_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve jwt manager: %w", err)
		}

		clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
		if !ok {
			return nil, fmt.Errorf("client repository is not of type *repos.ClientRepository")
		}

		nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
		if !ok {
			return nil, fmt.Errorf("node repository is not of type *repos.NodeRepository")
		}

		jwtManager, ok := jwtManagerInstance.(*managers.JWTManager)
		if !ok {
			return nil, fmt.Errorf("jwt manager is not of type *managers.JWTManager")
		}

		authService := NewauthService(clientRepo, nodeRepo, jwtManager, parentCtx)
		return authService, nil
	})

	// 注册匿名服务
	container.RegisterSingleton("anonymous_service", func() (interface{}, error) {
		clientRepoInstance, err := container.Resolve("client_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve client repository: %w", err)
		}

		mappingRepoInstance, err := container.Resolve("mapping_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve mapping repository: %w", err)
		}

		idManagerInstance, err := container.Resolve("id_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve id manager: %w", err)
		}

		clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
		if !ok {
			return nil, fmt.Errorf("client repository is not of type *repos.ClientRepository")
		}

		mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
		if !ok {
			return nil, fmt.Errorf("mapping repository is not of type *repos.PortMappingRepo")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, fmt.Errorf("id manager is not of type *idgen.IDManager")
		}

		anonymousService := NewAnonymousService(clientRepo, mappingRepo, idManager, parentCtx)
		return anonymousService, nil
	})

	// 注册连接服务
	container.RegisterSingleton("connection_service", func() (interface{}, error) {
		connRepoInstance, err := container.Resolve("connection_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve connection repository: %w", err)
		}

		idManagerInstance, err := container.Resolve("id_manager")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve id manager: %w", err)
		}

		connRepo, ok := connRepoInstance.(*repos.ConnectionRepo)
		if !ok {
			return nil, fmt.Errorf("connection repository is not of type *repos.ConnectionRepo")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, fmt.Errorf("id manager is not of type *idgen.IDManager")
		}

		connectionService := NewConnectionService(connRepo, idManager, parentCtx)
		return connectionService, nil
	})

	// 注册统计服务
	container.RegisterSingleton("stats_service", func() (interface{}, error) {
		userRepoInstance, err := container.Resolve("user_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve user repository: %w", err)
		}

		clientRepoInstance, err := container.Resolve("client_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve client repository: %w", err)
		}

		mappingRepoInstance, err := container.Resolve("mapping_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve mapping repository: %w", err)
		}

		nodeRepoInstance, err := container.Resolve("node_repository")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve node repository: %w", err)
		}

		userRepo, ok := userRepoInstance.(*repos.UserRepository)
		if !ok {
			return nil, fmt.Errorf("user repository is not of type *repos.UserRepository")
		}

		clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
		if !ok {
			return nil, fmt.Errorf("client repository is not of type *repos.ClientRepository")
		}

		mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
		if !ok {
			return nil, fmt.Errorf("mapping repository is not of type *repos.PortMappingRepo")
		}

		nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
		if !ok {
			return nil, fmt.Errorf("node repository is not of type *repos.NodeRepository")
		}

		statsService := NewstatsService(userRepo, clientRepo, mappingRepo, nodeRepo, parentCtx)
		return statsService, nil
	})

	corelog.Infof("Business services registered successfully")
	return nil
}
