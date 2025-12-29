package services

import (
	"context"
	"fmt"

	"tunnox-core/internal/cloud/configs"
	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
	storageCore "tunnox-core/internal/core/storage"
)

// registerInfrastructureServices 注册基础设施服务
// factories 参数包含创建 managers 实例的工厂函数，用于解决循环依赖
func registerInfrastructureServices(container *container.Container, config *configs.ControlConfig, storage storageCore.Storage, factories *ManagerFactories, parentCtx context.Context) error {
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

	// 注册基础 Repository
	if err := registerBaseRepositories(container, parentCtx); err != nil {
		return err
	}

	// 注册分离的客户端 Repository
	if err := registerClientRepositories(container, parentCtx); err != nil {
		return err
	}

	// 注册 JWT 管理器
	container.RegisterSingleton("jwt_manager", func() (interface{}, error) {
		if factories == nil || factories.NewJWTProvider == nil {
			return nil, fmt.Errorf("JWT provider factory not configured")
		}

		configInstance, err := container.Resolve("config")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve config: %w", err)
		}

		storageInstance, err := container.Resolve("storage")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve storage: %w", err)
		}

		jwtProvider := factories.NewJWTProvider(configInstance, storageInstance, parentCtx)
		return jwtProvider, nil
	})

	// 注册统计提供者（简化版）
	container.RegisterSingleton("stats_manager", func() (interface{}, error) {
		storageInstance, err := container.Resolve("storage")
		if err != nil {
			return nil, fmt.Errorf("failed to resolve storage: %w", err)
		}

		stor, ok := storageInstance.(storageCore.Storage)
		if !ok {
			return nil, fmt.Errorf("storage is not of type storage.Storage")
		}

		statsProvider, err := NewSimpleStatsProvider(stor, parentCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to create stats provider: %w", err)
		}

		return statsProvider, nil
	})

	corelog.Infof("Infrastructure services registered successfully")
	return nil
}

// registerBaseRepositories 注册基础 Repository
func registerBaseRepositories(container *container.Container, parentCtx context.Context) error {
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

		connRepo := repos.NewConnectionRepo(parentCtx, repo)
		return connRepo, nil
	})

	return nil
}

// registerClientRepositories 注册分离的客户端 Repository（配置、状态、Token）
func registerClientRepositories(container *container.Container, parentCtx context.Context) error {
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

	return nil
}
