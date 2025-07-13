package command

import (
	"fmt"
	"sync"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// CommandRegistry 命令注册器
type CommandRegistry struct {
	handlers map[packet.CommandType]CommandHandler
	mu       sync.RWMutex
}

// NewCommandRegistry 创建新的命令注册器
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		handlers: make(map[packet.CommandType]CommandHandler),
	}
}

// Register 注册命令处理器
func (cr *CommandRegistry) Register(handler CommandHandler) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	commandType := handler.GetCommandType()
	if commandType == 0 {
		return fmt.Errorf("invalid command type: 0")
	}

	if _, exists := cr.handlers[commandType]; exists {
		return fmt.Errorf("handler for command type %v already registered", commandType)
	}

	cr.handlers[commandType] = handler
	utils.Debugf("Registered command handler for type: %v", commandType)
	return nil
}

// Unregister 注销命令处理器
func (cr *CommandRegistry) Unregister(commandType packet.CommandType) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if _, exists := cr.handlers[commandType]; !exists {
		return fmt.Errorf("handler for command type %v not found", commandType)
	}

	delete(cr.handlers, commandType)
	utils.Debugf("Unregistered command handler for type: %v", commandType)
	return nil
}

// GetHandler 获取命令处理器
func (cr *CommandRegistry) GetHandler(commandType packet.CommandType) (CommandHandler, bool) {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	handler, exists := cr.handlers[commandType]
	return handler, exists
}

// ListHandlers 列出所有已注册的命令类型
func (cr *CommandRegistry) ListHandlers() []packet.CommandType {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	types := make([]packet.CommandType, 0, len(cr.handlers))
	for commandType := range cr.handlers {
		types = append(types, commandType)
	}
	return types
}

// GetHandlerCount 获取处理器数量
func (cr *CommandRegistry) GetHandlerCount() int {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return len(cr.handlers)
}
