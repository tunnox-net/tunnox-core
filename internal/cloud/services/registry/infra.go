package registry

import (
	"context"

	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/base"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
	storageCore "tunnox-core/internal/core/storage"
)

// ManagerFactories 管理器工厂函数集合
// 用于解决 services 和 managers 之间的循环依赖
// 注意: 工厂函数参数使用 any 是因为需要接受来自 DI 容器的动态类型，
// 具体实现在工厂函数内部进行类型断言
type ManagerFactories struct {
	// NewJWTProvider 创建 JWT 提供者的工厂函数
	NewJWTProvider func(config any, storage any, parentCtx context.Context) base.JWTProvider
	// NewStatsProvider 创建统计提供者的工厂函数
	NewStatsProvider func(userRepo, clientRepo, mappingRepo, nodeRepo any, storage any, parentCtx context.Context) base.StatsProvider
}

// RegisterInfrastructureServices 注册基础设施服务
// factories 参数包含创建 managers 实例的工厂函数，用于解决循环依赖
func RegisterInfrastructureServices(c *container.Container, config *configs.ControlConfig, storage storageCore.Storage, factories *ManagerFactories, parentCtx context.Context) error {
	// 注册存储服务
	c.RegisterSingleton("storage", func() (any, error) {
		if storage == nil {
			return nil, coreerrors.New(coreerrors.CodeNotConfigured, "storage is required")
		}
		return storage, nil
	})

	// 注册配置服务
	c.RegisterSingleton("config", func() (any, error) {
		if config == nil {
			return nil, coreerrors.New(coreerrors.CodeNotConfigured, "config is required")
		}
		return config, nil
	})

	// 注册ID管理器
	c.RegisterSingleton("id_manager", func() (any, error) {
		storageInstance, err := c.Resolve("storage")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve storage")
		}

		storageImpl, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "storage does not implement storage.Storage interface")
		}

		idManager := idgen.NewIDManager(storageImpl, parentCtx)
		return idManager, nil
	})

	// 注册Repository
	c.RegisterSingleton("repository", func() (any, error) {
		storageInstance, err := c.Resolve("storage")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve storage")
		}

		storageImpl, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "storage does not implement storage.Storage interface")
		}

		repo := repos.NewRepository(storageImpl)
		return repo, nil
	})

	// 注册基础 Repository
	if err := registerBaseRepositories(c, parentCtx); err != nil {
		return err
	}

	// 注册分离的客户端 Repository
	if err := registerClientRepositories(c, parentCtx); err != nil {
		return err
	}

	// 注册 JWT 管理器
	c.RegisterSingleton("jwt_manager", func() (any, error) {
		if factories == nil || factories.NewJWTProvider == nil {
			return nil, coreerrors.New(coreerrors.CodeNotConfigured, "JWT provider factory not configured")
		}

		configInstance, err := c.Resolve("config")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve config")
		}

		storageInstance, err := c.Resolve("storage")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve storage")
		}

		jwtProvider := factories.NewJWTProvider(configInstance, storageInstance, parentCtx)
		return jwtProvider, nil
	})

	// 注册统计提供者（简化版）
	c.RegisterSingleton("stats_manager", func() (any, error) {
		storageInstance, err := c.Resolve("storage")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve storage")
		}

		stor, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "storage is not of type storage.Storage")
		}

		statsProvider, err := base.NewSimpleStatsProvider(stor, parentCtx)
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to create stats provider")
		}

		return statsProvider, nil
	})

	corelog.Infof("Infrastructure services registered successfully")
	return nil
}

// registerBaseRepositories 注册基础 Repository
func registerBaseRepositories(c *container.Container, parentCtx context.Context) error {
	c.RegisterSingleton("user_repository", func() (any, error) {
		repoInstance, err := c.Resolve("repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve repository")
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "repository is not of type *repos.Repository")
		}

		userRepo := repos.NewUserRepository(repo)
		return userRepo, nil
	})

	c.RegisterSingleton("client_repository", func() (any, error) {
		repoInstance, err := c.Resolve("repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve repository")
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "repository is not of type *repos.Repository")
		}

		clientRepo := repos.NewClientRepository(repo)
		return clientRepo, nil
	})

	c.RegisterSingleton("mapping_repository", func() (any, error) {
		repoInstance, err := c.Resolve("repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve repository")
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "repository is not of type *repos.Repository")
		}

		mappingRepo := repos.NewPortMappingRepo(repo)
		return mappingRepo, nil
	})

	c.RegisterSingleton("node_repository", func() (any, error) {
		repoInstance, err := c.Resolve("repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve repository")
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "repository is not of type *repos.Repository")
		}

		nodeRepo := repos.NewNodeRepository(repo)
		return nodeRepo, nil
	})

	c.RegisterSingleton("connection_repository", func() (any, error) {
		repoInstance, err := c.Resolve("repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve repository")
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "repository is not of type *repos.Repository")
		}

		connRepo := repos.NewConnectionRepo(parentCtx, repo)
		return connRepo, nil
	})

	return nil
}

// registerClientRepositories 注册分离的客户端 Repository（配置、状态、Token）
func registerClientRepositories(c *container.Container, parentCtx context.Context) error {
	c.RegisterSingleton("client_config_repository", func() (any, error) {
		repoInstance, err := c.Resolve("repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve repository")
		}

		repo, ok := repoInstance.(*repos.Repository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "repository is not of type *repos.Repository")
		}

		configRepo := repos.NewClientConfigRepository(repo)
		return configRepo, nil
	})

	c.RegisterSingleton("client_state_repository", func() (any, error) {
		storageInstance, err := c.Resolve("storage")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve storage")
		}

		stor, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "storage is not of type storage.Storage")
		}

		stateRepo := repos.NewClientStateRepository(parentCtx, stor)
		return stateRepo, nil
	})

	c.RegisterSingleton("client_token_repository", func() (any, error) {
		storageInstance, err := c.Resolve("storage")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve storage")
		}

		stor, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "storage is not of type storage.Storage")
		}

		tokenRepo := repos.NewClientTokenRepository(parentCtx, stor)
		return tokenRepo, nil
	})

	return nil
}
