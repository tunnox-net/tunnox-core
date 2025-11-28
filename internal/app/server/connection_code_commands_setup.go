package server

import (
	"fmt"

	"tunnox-core/internal/command"
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
			return fmt.Errorf("failed to set command executor: %w", err)
		}
		utils.Infof("Server: CommandExecutor created and set to SessionManager")
	} else {
		// ✅ 如果执行器已存在，也要确保设置了 session
		executor := s.session.GetCommandExecutor()
		if executorWithSession, ok := executor.(interface{ SetSession(types.Session) }); ok {
			executorWithSession.SetSession(s.session)
			utils.Infof("Server: Session set in existing CommandExecutor")
		}
	}

	// 创建连接码命令处理器
	cmdHandlers := NewConnectionCodeCommandHandlers(s.connCodeService, s.session)

	// 获取命令执行器
	executor := s.session.GetCommandExecutor()
	if executor == nil {
		return fmt.Errorf("command executor not initialized")
	}

	// 类型断言获取 CommandExecutor（GetRegistry 返回 types.CommandRegistry 接口）
	type ExecutorWithRegistry interface {
		GetRegistry() types.CommandRegistry
	}

	executorWithRegistry, ok := executor.(ExecutorWithRegistry)
	if !ok {
		return fmt.Errorf("command executor does not support GetRegistry")
	}

	registry := executorWithRegistry.GetRegistry()
	// 类型断言获取 *CommandRegistry
	cmdRegistry, ok := registry.(*command.CommandRegistry)
	if !ok {
		return fmt.Errorf("command registry is not *CommandRegistry, got %T", registry)
	}

	// 注册处理器
	if err := cmdHandlers.RegisterHandlers(cmdRegistry); err != nil {
		return fmt.Errorf("failed to register connection code command handlers: %w", err)
	}

	utils.Infof("Server: Connection code command handlers registered")
	return nil
}
