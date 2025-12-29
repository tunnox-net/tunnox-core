package command

import (
	"context"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
)

// CreateDefaultRegistry 创建默认的命令注册表
// 注意：此函数使用 context.Background() 作为根 context，
// 仅在程序入口点或测试中使用。其他场景请使用 CreateDefaultRegistryWithContext
func CreateDefaultRegistry() types.CommandRegistry {
	registry := NewCommandRegistry(context.Background())
	RegisterDefaultHandlers(registry)
	return registry
}

// CreateDefaultRegistryWithContext 创建默认的命令注册表（支持 context 传递）
func CreateDefaultRegistryWithContext(parentCtx context.Context) types.CommandRegistry {
	registry := NewCommandRegistry(parentCtx)
	RegisterDefaultHandlers(registry)
	return registry
}

// CreateDefaultService 创建默认的命令服务
func CreateDefaultService(parentCtx context.Context) CommandService {
	service := NewCommandService(parentCtx)
	RegisterDefaultHandlersToService(service)
	return service
}

// RegisterDefaultHandlers 注册默认命令处理器
func RegisterDefaultHandlers(registry types.CommandRegistry) {
	// 注册所有默认命令处理器
	handlers := []types.CommandHandler{
		NewTcpMapHandler(),
		NewHttpMapHandler(),
		NewSocksMapHandler(),
		NewDataInHandler(),
		NewDataOutHandler(),
		NewForwardHandler(),
		NewDisconnectHandler(),
		NewRpcInvokeHandler(),
		NewDefaultHandler(),
	}

	for _, handler := range handlers {
		if err := registry.Register(handler); err != nil {
			corelog.Errorf("Failed to register handler for command type %v: %v", handler.GetCommandType(), err)
		} else {
			corelog.Infof("Registered command handler for type: %v", handler.GetCommandType())
		}
	}

	corelog.Infof("Registered %d default command handlers", len(handlers))
}

// RegisterDefaultHandlersToService 注册默认命令处理器到服务
func RegisterDefaultHandlersToService(service CommandService) {
	// 注册所有默认命令处理器
	handlers := []types.CommandHandler{
		NewTcpMapHandler(),
		NewHttpMapHandler(),
		NewSocksMapHandler(),
		NewDataInHandler(),
		NewDataOutHandler(),
		NewForwardHandler(),
		NewDisconnectHandler(),
		NewRpcInvokeHandler(),
		NewDefaultHandler(),
	}

	for _, handler := range handlers {
		if err := service.RegisterHandler(handler); err != nil {
			corelog.Errorf("Failed to register handler for command type %v: %v", handler.GetCommandType(), err)
		} else {
			corelog.Infof("Registered command handler for type: %v", handler.GetCommandType())
		}
	}

	corelog.Infof("Registered %d default command handlers to service", len(handlers))
}
