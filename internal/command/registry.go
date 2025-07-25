package command

import (
	"fmt"
	"sync"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// CommandRegistry 命令注册表
type CommandRegistry struct {
	handlers map[packet.CommandType]CommandHandler
	mu       sync.RWMutex
}

// NewCommandRegistry 创建命令注册表
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		handlers: make(map[packet.CommandType]CommandHandler),
	}
}

// Register 注册命令处理器
func (r *CommandRegistry) Register(handler CommandHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	commandType := handler.GetCommandType()
	if commandType == 0 {
		return fmt.Errorf("invalid command type: 0")
	}

	if _, exists := r.handlers[commandType]; exists {
		return fmt.Errorf("handler for command type %v already registered", commandType)
	}

	r.handlers[commandType] = handler
	utils.Debugf("Registered command handler for type: %v", commandType)
	return nil
}

// RegisterHandler 注册命令处理器（别名方法）
func (r *CommandRegistry) RegisterHandler(cmdType packet.CommandType, handler CommandHandler) {
	r.Register(handler)
}

// Unregister 注销命令处理器
func (r *CommandRegistry) Unregister(commandType packet.CommandType) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[commandType]; !exists {
		return fmt.Errorf("handler for command type %v not found", commandType)
	}

	delete(r.handlers, commandType)
	utils.Debugf("Unregistered command handler for type: %v", commandType)
	return nil
}

// GetHandler 获取命令处理器
func (r *CommandRegistry) GetHandler(commandType packet.CommandType) (CommandHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, exists := r.handlers[commandType]
	return handler, exists
}

// ListHandlers 列出所有已注册的命令类型
func (r *CommandRegistry) ListHandlers() []packet.CommandType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]packet.CommandType, 0, len(r.handlers))
	for commandType := range r.handlers {
		types = append(types, commandType)
	}
	return types
}

// GetHandlerCount 获取处理器数量
func (r *CommandRegistry) GetHandlerCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.handlers)
}
