package server

import (
	"tunnox-core/internal/command"
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/utils"
)

// setupConnectionCodeCommands 设置连接码命令处理器
func (s *Server) setupConnectionCodeCommands() error {
	// 创建命令注册表和执行器（如果还没有）
	if s.session.GetCommandExecutor() == nil {
		registry := command.NewCommandRegistry(s.serviceManager.GetContext())
		executor := command.NewCommandExecutor(registry, s.serviceManager.GetContext())
		// ✅ 设置 session，以便 CommandExecutor 可以发送响应
		executor.SetSession(s.session)
		if err := s.session.SetCommandExecutor(executor); err != nil {
			return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to set command executor")
		}
	} else {
		// ✅ 如果执行器已存在，也要确保设置了 session
		executor := s.session.GetCommandExecutor()
		if executorWithSession, ok := executor.(interface{ SetSession(types.Session) }); ok {
			executorWithSession.SetSession(s.session)
		}
	}

	// 创建连接码命令处理器
	cmdHandlers := NewConnectionCodeCommandHandlers(s.connCodeService, s.session)

	// 获取命令执行器
	executor := s.session.GetCommandExecutor()
	if executor == nil {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "command executor not initialized")
	}

	// 类型断言获取 CommandExecutor（GetRegistry 返回 types.CommandRegistry 接口）
	type ExecutorWithRegistry interface {
		GetRegistry() types.CommandRegistry
	}

	executorWithRegistry, ok := executor.(ExecutorWithRegistry)
	if !ok {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "command executor does not support GetRegistry")
	}

	registry := executorWithRegistry.GetRegistry()
	// 类型断言获取 *CommandRegistry
	cmdRegistry, ok := registry.(*command.CommandRegistry)
	if !ok {
		return coreErrors.Newf(coreErrors.ErrorTypePermanent, "command registry is not *CommandRegistry, got %T", registry)
	}

	// 注册连接码处理器
	if err := cmdHandlers.RegisterHandlers(cmdRegistry); err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to register connection code command handlers")
	}
	// 注册配置命令处理器
	if s.authHandler == nil {
		utils.Warnf("Server: auth handler not set, skipping ConfigGet handler registration")
	} else {
		configHandlers := NewConfigCommandHandlers(s.authHandler, s.session)
		if err := configHandlers.RegisterHandlers(cmdRegistry); err != nil {
			return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to register config command handlers")
		}
	}

	// 注册映射命令处理器
	mappingHandlers := NewMappingCommandHandlers(s.connCodeService, s.session)
	if err := mappingHandlers.RegisterHandlers(cmdRegistry); err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to register mapping command handlers")
	}

	return nil
}
