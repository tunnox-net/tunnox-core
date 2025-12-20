package command

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"tunnox-core/internal/core/types"
)

// CreateDefaultRegistry 创建默认的命令注册表
func CreateDefaultRegistry() types.CommandRegistry {
	registry := NewCommandRegistry(context.Background())
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
