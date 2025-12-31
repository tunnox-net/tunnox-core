package registry

import (
	"context"

	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/cloud/services/base"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
)

// ServiceConstructors 服务构造器集合
// 用于注册业务服务
type ServiceConstructors struct {
	NewUserService       func(userRepo *repos.UserRepository, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) interface{}
	NewClientService     func(configRepo, stateRepo, tokenRepo, clientRepo, mappingRepo interface{}, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) interface{}
	NewPortMappingService func(mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, statsProvider base.StatsProvider, parentCtx context.Context) interface{}
	NewNodeService       func(nodeRepo *repos.NodeRepository, idManager *idgen.IDManager, parentCtx context.Context) interface{}
	NewAuthService       func(clientRepo *repos.ClientRepository, nodeRepo *repos.NodeRepository, jwtProvider base.JWTProvider, parentCtx context.Context) interface{}
	NewAnonymousService  func(clientRepo *repos.ClientRepository, configRepo *repos.ClientConfigRepository, mappingRepo *repos.PortMappingRepo, idManager *idgen.IDManager, parentCtx context.Context) interface{}
	NewConnectionService func(connRepo *repos.ConnectionRepo, idManager *idgen.IDManager, parentCtx context.Context) interface{}
	NewStatsService      func(userRepo *repos.UserRepository, clientRepo *repos.ClientRepository, mappingRepo *repos.PortMappingRepo, nodeRepo *repos.NodeRepository, parentCtx context.Context) interface{}
}

// RegisterBusinessServices 注册业务服务
func RegisterBusinessServices(c *container.Container, constructors *ServiceConstructors, parentCtx context.Context) error {
	// 注册用户服务
	if err := registerUserService(c, constructors, parentCtx); err != nil {
		return err
	}

	// 注册客户端服务
	if err := registerClientService(c, constructors, parentCtx); err != nil {
		return err
	}

	// 注册端口映射服务
	if err := registerMappingService(c, constructors, parentCtx); err != nil {
		return err
	}

	// 注册节点服务
	if err := registerNodeService(c, constructors, parentCtx); err != nil {
		return err
	}

	// 注册认证服务
	if err := registerAuthService(c, constructors, parentCtx); err != nil {
		return err
	}

	// 注册匿名服务
	if err := registerAnonymousService(c, constructors, parentCtx); err != nil {
		return err
	}

	// 注册连接服务
	if err := registerConnectionService(c, constructors, parentCtx); err != nil {
		return err
	}

	// 注册统计服务
	if err := registerStatsService(c, constructors, parentCtx); err != nil {
		return err
	}

	corelog.Infof("Business services registered successfully")
	return nil
}

// registerUserService 注册用户服务
func registerUserService(c *container.Container, constructors *ServiceConstructors, parentCtx context.Context) error {
	c.RegisterSingleton("user_service", func() (interface{}, error) {
		userRepoInstance, err := c.Resolve("user_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve user repository")
		}

		idManagerInstance, err := c.Resolve("id_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve id manager")
		}

		userRepo, ok := userRepoInstance.(*repos.UserRepository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "user repository is not of type *repos.UserRepository")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "id manager is not of type *idgen.IDManager")
		}

		statsManagerInstance, err := c.Resolve("stats_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve stats manager")
		}

		statsProvider, ok := statsManagerInstance.(base.StatsProvider)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "stats manager does not implement StatsProvider interface")
		}

		if constructors != nil && constructors.NewUserService != nil {
			return constructors.NewUserService(userRepo, idManager, statsProvider, parentCtx), nil
		}

		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "user service constructor not provided")
	})
	return nil
}

// registerClientService 注册客户端服务（使用分离的Repository）
func registerClientService(c *container.Container, constructors *ServiceConstructors, parentCtx context.Context) error {
	c.RegisterSingleton("client_service", func() (interface{}, error) {
		// 新Repository
		configRepoInstance, err := c.Resolve("client_config_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve client config repository")
		}

		stateRepoInstance, err := c.Resolve("client_state_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve client state repository")
		}

		tokenRepoInstance, err := c.Resolve("client_token_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve client token repository")
		}

		// 旧Repository（兼容性）
		clientRepoInstance, err := c.Resolve("client_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve client repository")
		}

		mappingRepoInstance, err := c.Resolve("mapping_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve mapping repository")
		}

		idManagerInstance, err := c.Resolve("id_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve id manager")
		}

		statsManagerInstance, err := c.Resolve("stats_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve stats manager")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "id manager is not of type *idgen.IDManager")
		}

		statsProvider, ok := statsManagerInstance.(base.StatsProvider)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "stats manager does not implement StatsProvider interface")
		}

		if constructors != nil && constructors.NewClientService != nil {
			return constructors.NewClientService(
				configRepoInstance, stateRepoInstance, tokenRepoInstance,
				clientRepoInstance, mappingRepoInstance,
				idManager, statsProvider, parentCtx,
			), nil
		}

		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "client service constructor not provided")
	})
	return nil
}

// registerMappingService 注册端口映射服务
func registerMappingService(c *container.Container, constructors *ServiceConstructors, parentCtx context.Context) error {
	c.RegisterSingleton("mapping_service", func() (interface{}, error) {
		mappingRepoInstance, err := c.Resolve("mapping_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve mapping repository")
		}

		idManagerInstance, err := c.Resolve("id_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve id manager")
		}

		mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "mapping repository is not of type *repos.PortMappingRepo")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "id manager is not of type *idgen.IDManager")
		}

		statsManagerInstance, err := c.Resolve("stats_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve stats manager")
		}

		statsProvider, ok := statsManagerInstance.(base.StatsProvider)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "stats manager does not implement StatsProvider interface")
		}

		if constructors != nil && constructors.NewPortMappingService != nil {
			return constructors.NewPortMappingService(mappingRepo, idManager, statsProvider, parentCtx), nil
		}

		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "port mapping service constructor not provided")
	})
	return nil
}

// registerNodeService 注册节点服务
func registerNodeService(c *container.Container, constructors *ServiceConstructors, parentCtx context.Context) error {
	c.RegisterSingleton("node_service", func() (interface{}, error) {
		nodeRepoInstance, err := c.Resolve("node_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve node repository")
		}

		idManagerInstance, err := c.Resolve("id_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve id manager")
		}

		nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "node repository is not of type *repos.NodeRepository")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "id manager is not of type *idgen.IDManager")
		}

		if constructors != nil && constructors.NewNodeService != nil {
			return constructors.NewNodeService(nodeRepo, idManager, parentCtx), nil
		}

		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "node service constructor not provided")
	})
	return nil
}

// registerAuthService 注册认证服务
func registerAuthService(c *container.Container, constructors *ServiceConstructors, parentCtx context.Context) error {
	c.RegisterSingleton("auth_service", func() (interface{}, error) {
		clientRepoInstance, err := c.Resolve("client_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve client repository")
		}

		nodeRepoInstance, err := c.Resolve("node_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve node repository")
		}

		jwtManagerInstance, err := c.Resolve("jwt_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve jwt manager")
		}

		clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "client repository is not of type *repos.ClientRepository")
		}

		nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "node repository is not of type *repos.NodeRepository")
		}

		jwtProvider, ok := jwtManagerInstance.(base.JWTProvider)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "jwt manager does not implement JWTProvider interface")
		}

		if constructors != nil && constructors.NewAuthService != nil {
			return constructors.NewAuthService(clientRepo, nodeRepo, jwtProvider, parentCtx), nil
		}

		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "auth service constructor not provided")
	})
	return nil
}

// registerAnonymousService 注册匿名服务
func registerAnonymousService(c *container.Container, constructors *ServiceConstructors, parentCtx context.Context) error {
	c.RegisterSingleton("anonymous_service", func() (interface{}, error) {
		clientRepoInstance, err := c.Resolve("client_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve client repository")
		}

		configRepoInstance, err := c.Resolve("client_config_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve client config repository")
		}

		mappingRepoInstance, err := c.Resolve("mapping_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve mapping repository")
		}

		idManagerInstance, err := c.Resolve("id_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve id manager")
		}

		clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "client repository is not of type *repos.ClientRepository")
		}

		configRepo, ok := configRepoInstance.(*repos.ClientConfigRepository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "client config repository is not of type *repos.ClientConfigRepository")
		}

		mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "mapping repository is not of type *repos.PortMappingRepo")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "id manager is not of type *idgen.IDManager")
		}

		if constructors != nil && constructors.NewAnonymousService != nil {
			return constructors.NewAnonymousService(clientRepo, configRepo, mappingRepo, idManager, parentCtx), nil
		}

		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "anonymous service constructor not provided")
	})
	return nil
}

// registerConnectionService 注册连接服务
func registerConnectionService(c *container.Container, constructors *ServiceConstructors, parentCtx context.Context) error {
	c.RegisterSingleton("connection_service", func() (interface{}, error) {
		connRepoInstance, err := c.Resolve("connection_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve connection repository")
		}

		idManagerInstance, err := c.Resolve("id_manager")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve id manager")
		}

		connRepo, ok := connRepoInstance.(*repos.ConnectionRepo)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "connection repository is not of type *repos.ConnectionRepo")
		}

		idManager, ok := idManagerInstance.(*idgen.IDManager)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "id manager is not of type *idgen.IDManager")
		}

		if constructors != nil && constructors.NewConnectionService != nil {
			return constructors.NewConnectionService(connRepo, idManager, parentCtx), nil
		}

		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "connection service constructor not provided")
	})
	return nil
}

// registerStatsService 注册统计服务
func registerStatsService(c *container.Container, constructors *ServiceConstructors, parentCtx context.Context) error {
	c.RegisterSingleton("stats_service", func() (interface{}, error) {
		userRepoInstance, err := c.Resolve("user_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve user repository")
		}

		clientRepoInstance, err := c.Resolve("client_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve client repository")
		}

		mappingRepoInstance, err := c.Resolve("mapping_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve mapping repository")
		}

		nodeRepoInstance, err := c.Resolve("node_repository")
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to resolve node repository")
		}

		userRepo, ok := userRepoInstance.(*repos.UserRepository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "user repository is not of type *repos.UserRepository")
		}

		clientRepo, ok := clientRepoInstance.(*repos.ClientRepository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "client repository is not of type *repos.ClientRepository")
		}

		mappingRepo, ok := mappingRepoInstance.(*repos.PortMappingRepo)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "mapping repository is not of type *repos.PortMappingRepo")
		}

		nodeRepo, ok := nodeRepoInstance.(*repos.NodeRepository)
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeInternal, "node repository is not of type *repos.NodeRepository")
		}

		if constructors != nil && constructors.NewStatsService != nil {
			return constructors.NewStatsService(userRepo, clientRepo, mappingRepo, nodeRepo, parentCtx), nil
		}

		return nil, coreerrors.New(coreerrors.CodeNotConfigured, "stats service constructor not provided")
	})
	return nil
}
