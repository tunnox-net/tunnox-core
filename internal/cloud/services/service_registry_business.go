package services

import (
	"context"
	"fmt"

	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
)

// registerBusinessServices 注册业务服务
func registerBusinessServices(container *container.Container, parentCtx context.Context) error {
	// 注册用户服务
	if err := registerUserService(container, parentCtx); err != nil {
		return err
	}

	// 注册客户端服务
	if err := registerClientService(container, parentCtx); err != nil {
		return err
	}

	// 注册端口映射服务
	if err := registerMappingService(container, parentCtx); err != nil {
		return err
	}

	// 注册节点服务
	if err := registerNodeService(container, parentCtx); err != nil {
		return err
	}

	// 注册认证服务
	if err := registerAuthService(container, parentCtx); err != nil {
		return err
	}

	// 注册匿名服务
	if err := registerAnonymousService(container, parentCtx); err != nil {
		return err
	}

	// 注册连接服务
	if err := registerConnectionService(container, parentCtx); err != nil {
		return err
	}

	// 注册统计服务
	if err := registerStatsService(container, parentCtx); err != nil {
		return err
	}

	corelog.Infof("Business services registered successfully")
	return nil
}

// registerUserService 注册用户服务
func registerUserService(container *container.Container, parentCtx context.Context) error {
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

		statsProvider, ok := statsManagerInstance.(StatsProvider)
		if !ok {
			return nil, fmt.Errorf("stats manager does not implement StatsProvider interface")
		}

		userService := NewUserService(userRepo, idManager, statsProvider.GetCounter(), parentCtx)
		return userService, nil
	})
	return nil
}

// registerClientService 注册客户端服务（使用分离的Repository）
func registerClientService(container *container.Container, parentCtx context.Context) error {
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

		statsProvider, ok := statsManagerInstance.(StatsProvider)
		if !ok {
			return nil, fmt.Errorf("stats manager does not implement StatsProvider interface")
		}

		// 使用新的构造函数
		clientService := NewClientService(
			configRepo, stateRepo, tokenRepo,
			clientRepo, mappingRepo,
			idManager, statsProvider, parentCtx,
		)
		return clientService, nil
	})
	return nil
}

// registerMappingService 注册端口映射服务
func registerMappingService(container *container.Container, parentCtx context.Context) error {
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

		statsProvider, ok := statsManagerInstance.(StatsProvider)
		if !ok {
			return nil, fmt.Errorf("stats manager does not implement StatsProvider interface")
		}

		mappingService := NewPortMappingService(mappingRepo, idManager, statsProvider.GetCounter(), parentCtx)
		return mappingService, nil
	})
	return nil
}

// registerNodeService 注册节点服务
func registerNodeService(container *container.Container, parentCtx context.Context) error {
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
	return nil
}

// registerAuthService 注册认证服务
func registerAuthService(container *container.Container, parentCtx context.Context) error {
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

		jwtProvider, ok := jwtManagerInstance.(JWTProvider)
		if !ok {
			return nil, fmt.Errorf("jwt manager does not implement JWTProvider interface")
		}

		authService := NewauthService(clientRepo, nodeRepo, jwtProvider, parentCtx)
		return authService, nil
	})
	return nil
}

// registerAnonymousService 注册匿名服务
func registerAnonymousService(container *container.Container, parentCtx context.Context) error {
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
	return nil
}

// registerConnectionService 注册连接服务
func registerConnectionService(container *container.Container, parentCtx context.Context) error {
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
	return nil
}

// registerStatsService 注册统计服务
func registerStatsService(container *container.Container, parentCtx context.Context) error {
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
	return nil
}
