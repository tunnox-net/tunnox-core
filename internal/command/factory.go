package command

import (
	"tunnox-core/internal/common"
	"tunnox-core/internal/utils"
)

// CreateDefaultRegistry 创建默认的命令注册表
func CreateDefaultRegistry() common.CommandRegistry {
	registry := NewCommandRegistry()
	RegisterDefaultHandlers(registry)
	return registry
}

// RegisterDefaultHandlers 注册默认命令处理器
func RegisterDefaultHandlers(registry common.CommandRegistry) {
	// 注册所有默认命令处理器
	handlers := []common.CommandHandler{
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
			utils.Errorf("Failed to register handler for command type %v: %v", handler.GetCommandType(), err)
		} else {
			utils.Infof("Registered command handler for type: %v", handler.GetCommandType())
		}
	}

	utils.Infof("Registered %d default command handlers", len(handlers))
}
