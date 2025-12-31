package services

import (
	"context"

	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/repos"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
)

// registerBusinessServicesWithConstructors 使用构造器注册业务服务
func registerBusinessServicesWithConstructors(c *container.Container, constructors *serviceConstructors, parentCtx context.Context) error {
	// 注册用户服务
	c.RegisterSingleton("user_service", func() (any, error) {
		userRepo, idManager, statsProvider, err := resolveUserServiceDeps(c)
		if err != nil {
			return nil, err
		}
		return constructors.newUserService(userRepo, idManager, statsProvider.GetCounter(), parentCtx), nil
	})

	// 注册客户端服务
	c.RegisterSingleton("client_service", func() (any, error) {
		configRepo, stateRepo, tokenRepo, clientRepo, mappingRepo, idManager, statsProvider, err := resolveClientServiceDeps(c)
		if err != nil {
			return nil, err
		}
		return constructors.newClientService(configRepo, stateRepo, tokenRepo, clientRepo, mappingRepo, idManager, statsProvider, parentCtx), nil
	})

	// 注册端口映射服务
	c.RegisterSingleton("mapping_service", func() (any, error) {
		mappingRepo, idManager, statsProvider, err := resolveMappingServiceDeps(c)
		if err != nil {
			return nil, err
		}
		return constructors.newPortMappingService(mappingRepo, idManager, statsProvider.GetCounter(), parentCtx), nil
	})

	// 注册节点服务
	c.RegisterSingleton("node_service", func() (any, error) {
		nodeRepo, idManager, err := resolveNodeServiceDeps(c)
		if err != nil {
			return nil, err
		}
		return constructors.newNodeService(nodeRepo, idManager, parentCtx), nil
	})

	// 注册认证服务
	c.RegisterSingleton("auth_service", func() (any, error) {
		clientRepo, nodeRepo, jwtProvider, err := resolveAuthServiceDeps(c)
		if err != nil {
			return nil, err
		}
		return constructors.newAuthService(clientRepo, nodeRepo, jwtProvider, parentCtx), nil
	})

	// 注册匿名服务
	c.RegisterSingleton("anonymous_service", func() (any, error) {
		clientRepo, configRepo, mappingRepo, idManager, err := resolveAnonymousServiceDeps(c)
		if err != nil {
			return nil, err
		}
		return constructors.newAnonymousService(clientRepo, configRepo, mappingRepo, idManager, parentCtx), nil
	})

	// 注册连接服务
	c.RegisterSingleton("connection_service", func() (any, error) {
		connRepo, idManager, err := resolveConnectionServiceDeps(c)
		if err != nil {
			return nil, err
		}
		return constructors.newConnectionService(connRepo, idManager, parentCtx), nil
	})

	// 注册统计服务
	c.RegisterSingleton("stats_service", func() (any, error) {
		userRepo, clientRepo, mappingRepo, nodeRepo, err := resolveStatsServiceDeps(c)
		if err != nil {
			return nil, err
		}
		return constructors.newStatsService(userRepo, clientRepo, mappingRepo, nodeRepo, parentCtx), nil
	})

	corelog.Infof("Business services registered successfully")
	return nil
}

// 以下是依赖解析辅助函数

func resolveUserServiceDeps(c *container.Container) (*repos.UserRepository, *idgen.IDManager, StatsProvider, error) {
	userRepoInstance, err := c.Resolve("user_repository")
	if err != nil {
		return nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve user repository")
	}
	userRepo, ok := userRepoInstance.(*repos.UserRepository)
	if !ok {
		return nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "user repository is not of type *repos.UserRepository")
	}

	idManagerInstance, err := c.Resolve("id_manager")
	if err != nil {
		return nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve id manager")
	}
	idManager, ok := idManagerInstance.(*idgen.IDManager)
	if !ok {
		return nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "id manager is not of type *idgen.IDManager")
	}

	statsManagerInstance, err := c.Resolve("stats_manager")
	if err != nil {
		return nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve stats manager")
	}
	statsProvider, ok := statsManagerInstance.(StatsProvider)
	if !ok {
		return nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "stats manager does not implement StatsProvider interface")
	}

	return userRepo, idManager, statsProvider, nil
}

func resolveClientServiceDeps(c *container.Container) (*repos.ClientConfigRepository, *repos.ClientStateRepository, *repos.ClientTokenRepository, *repos.ClientRepository, *repos.PortMappingRepo, *idgen.IDManager, StatsProvider, error) {
	configRepoInstance, err := c.Resolve("client_config_repository")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve client config repository")
	}
	configRepo, ok := configRepoInstance.(*repos.ClientConfigRepository)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "client config repository is not of type *repos.ClientConfigRepository")
	}

	stateRepoInstance, err := c.Resolve("client_state_repository")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve client state repository")
	}
	stateRepo, ok := stateRepoInstance.(*repos.ClientStateRepository)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "client state repository is not of type *repos.ClientStateRepository")
	}

	tokenRepoInstance, err := c.Resolve("client_token_repository")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve client token repository")
	}
	tokenRepo, ok := tokenRepoInstance.(*repos.ClientTokenRepository)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "client token repository is not of type *repos.ClientTokenRepository")
	}

	clientRepoInstance, err := c.Resolve("client_repository")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve client repository")
	}
	clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "client repository is not of type *repos.ClientRepository")
	}

	mappingRepoInstance, err := c.Resolve("mapping_repository")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve mapping repository")
	}
	mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "mapping repository is not of type *repos.PortMappingRepo")
	}

	idManagerInstance, err := c.Resolve("id_manager")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve id manager")
	}
	idManager, ok := idManagerInstance.(*idgen.IDManager)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "id manager is not of type *idgen.IDManager")
	}

	statsManagerInstance, err := c.Resolve("stats_manager")
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve stats manager")
	}
	statsProvider, ok := statsManagerInstance.(StatsProvider)
	if !ok {
		return nil, nil, nil, nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "stats manager does not implement StatsProvider interface")
	}

	return configRepo, stateRepo, tokenRepo, clientRepo, mappingRepo, idManager, statsProvider, nil
}

func resolveMappingServiceDeps(c *container.Container) (*repos.PortMappingRepo, *idgen.IDManager, StatsProvider, error) {
	mappingRepoInstance, err := c.Resolve("mapping_repository")
	if err != nil {
		return nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve mapping repository")
	}
	mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
	if !ok {
		return nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "mapping repository is not of type *repos.PortMappingRepo")
	}

	idManagerInstance, err := c.Resolve("id_manager")
	if err != nil {
		return nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve id manager")
	}
	idManager, ok := idManagerInstance.(*idgen.IDManager)
	if !ok {
		return nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "id manager is not of type *idgen.IDManager")
	}

	statsManagerInstance, err := c.Resolve("stats_manager")
	if err != nil {
		return nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve stats manager")
	}
	statsProvider, ok := statsManagerInstance.(StatsProvider)
	if !ok {
		return nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "stats manager does not implement StatsProvider interface")
	}

	return mappingRepo, idManager, statsProvider, nil
}

func resolveNodeServiceDeps(c *container.Container) (*repos.NodeRepository, *idgen.IDManager, error) {
	nodeRepoInstance, err := c.Resolve("node_repository")
	if err != nil {
		return nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve node repository")
	}
	nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
	if !ok {
		return nil, nil, coreerrors.New(coreerrors.CodeInternalError, "node repository is not of type *repos.NodeRepository")
	}

	idManagerInstance, err := c.Resolve("id_manager")
	if err != nil {
		return nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve id manager")
	}
	idManager, ok := idManagerInstance.(*idgen.IDManager)
	if !ok {
		return nil, nil, coreerrors.New(coreerrors.CodeInternalError, "id manager is not of type *idgen.IDManager")
	}

	return nodeRepo, idManager, nil
}

func resolveAuthServiceDeps(c *container.Container) (*repos.ClientRepository, *repos.NodeRepository, JWTProvider, error) {
	clientRepoInstance, err := c.Resolve("client_repository")
	if err != nil {
		return nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve client repository")
	}
	clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
	if !ok {
		return nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "client repository is not of type *repos.ClientRepository")
	}

	nodeRepoInstance, err := c.Resolve("node_repository")
	if err != nil {
		return nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve node repository")
	}
	nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
	if !ok {
		return nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "node repository is not of type *repos.NodeRepository")
	}

	jwtManagerInstance, err := c.Resolve("jwt_manager")
	if err != nil {
		return nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve jwt manager")
	}
	jwtProvider, ok := jwtManagerInstance.(JWTProvider)
	if !ok {
		return nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "jwt manager does not implement JWTProvider interface")
	}

	return clientRepo, nodeRepo, jwtProvider, nil
}

func resolveAnonymousServiceDeps(c *container.Container) (*repos.ClientRepository, *repos.ClientConfigRepository, *repos.PortMappingRepo, *idgen.IDManager, error) {
	clientRepoInstance, err := c.Resolve("client_repository")
	if err != nil {
		return nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve client repository")
	}
	clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
	if !ok {
		return nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "client repository is not of type *repos.ClientRepository")
	}

	configRepoInstance, err := c.Resolve("client_config_repository")
	if err != nil {
		return nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve client config repository")
	}
	configRepo, ok := configRepoInstance.(*repos.ClientConfigRepository)
	if !ok {
		return nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "client config repository is not of type *repos.ClientConfigRepository")
	}

	mappingRepoInstance, err := c.Resolve("mapping_repository")
	if err != nil {
		return nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve mapping repository")
	}
	mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
	if !ok {
		return nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "mapping repository is not of type *repos.PortMappingRepo")
	}

	idManagerInstance, err := c.Resolve("id_manager")
	if err != nil {
		return nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve id manager")
	}
	idManager, ok := idManagerInstance.(*idgen.IDManager)
	if !ok {
		return nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "id manager is not of type *idgen.IDManager")
	}

	return clientRepo, configRepo, mappingRepo, idManager, nil
}

func resolveConnectionServiceDeps(c *container.Container) (*repos.ConnectionRepo, *idgen.IDManager, error) {
	connRepoInstance, err := c.Resolve("connection_repository")
	if err != nil {
		return nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve connection repository")
	}
	connRepo, ok := connRepoInstance.(*repos.ConnectionRepo)
	if !ok {
		return nil, nil, coreerrors.New(coreerrors.CodeInternalError, "connection repository is not of type *repos.ConnectionRepo")
	}

	idManagerInstance, err := c.Resolve("id_manager")
	if err != nil {
		return nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve id manager")
	}
	idManager, ok := idManagerInstance.(*idgen.IDManager)
	if !ok {
		return nil, nil, coreerrors.New(coreerrors.CodeInternalError, "id manager is not of type *idgen.IDManager")
	}

	return connRepo, idManager, nil
}

func resolveStatsServiceDeps(c *container.Container) (*repos.UserRepository, *repos.ClientRepository, *repos.PortMappingRepo, *repos.NodeRepository, error) {
	userRepoInstance, err := c.Resolve("user_repository")
	if err != nil {
		return nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve user repository")
	}
	userRepo, ok := userRepoInstance.(*repos.UserRepository)
	if !ok {
		return nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "user repository is not of type *repos.UserRepository")
	}

	clientRepoInstance, err := c.Resolve("client_repository")
	if err != nil {
		return nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve client repository")
	}
	clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
	if !ok {
		return nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "client repository is not of type *repos.ClientRepository")
	}

	mappingRepoInstance, err := c.Resolve("mapping_repository")
	if err != nil {
		return nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve mapping repository")
	}
	mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
	if !ok {
		return nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "mapping repository is not of type *repos.PortMappingRepo")
	}

	nodeRepoInstance, err := c.Resolve("node_repository")
	if err != nil {
		return nil, nil, nil, nil, coreerrors.Wrap(err, coreerrors.CodeNotConfigured, "failed to resolve node repository")
	}
	nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
	if !ok {
		return nil, nil, nil, nil, coreerrors.New(coreerrors.CodeInternalError, "node repository is not of type *repos.NodeRepository")
	}

	return userRepo, clientRepo, mappingRepo, nodeRepo, nil
}
