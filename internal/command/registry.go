package command

import (
	"fmt"
	"sync"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// CommandRegistry 命令注册器
type CommandRegistry struct {
	handlers   map[packet.CommandType]CommandHandler
	categories map[CommandCategory][]CommandType
	mu         sync.RWMutex
}

// NewCommandRegistry 创建新的命令注册器
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		handlers:   make(map[packet.CommandType]CommandHandler),
		categories: make(map[CommandCategory][]CommandType),
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

	// 添加到分类映射
	category := handler.GetCategory()
	cmdType := CommandType{
		ID:          commandType,
		Category:    category,
		Direction:   handler.GetDirection(),
		Name:        fmt.Sprintf("cmd_%d", commandType),
		Description: fmt.Sprintf("Command type %d", commandType),
	}
	cr.categories[category] = append(cr.categories[category], cmdType)

	utils.Debugf("Registered command handler for type: %v, category: %v", commandType, category)
	return nil
}

// RegisterHandler 注册命令处理器（别名方法）
func (cr *CommandRegistry) RegisterHandler(cmdType packet.CommandType, handler CommandHandler) {
	cr.Register(handler)
}

// Unregister 注销命令处理器
func (cr *CommandRegistry) Unregister(commandType packet.CommandType) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if _, exists := cr.handlers[commandType]; !exists {
		return fmt.Errorf("handler for command type %v not found", commandType)
	}

	delete(cr.handlers, commandType)

	// 从分类映射中移除
	for category, commands := range cr.categories {
		for i, cmd := range commands {
			if cmd.ID == commandType {
				cr.categories[category] = append(commands[:i], commands[i+1:]...)
				break
			}
		}
	}

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

// RegisterCategory 注册命令分类
func (cr *CommandRegistry) RegisterCategory(category CommandCategory, commands []CommandType) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.categories[category] = commands
}

// GetCommandsByCategory 根据分类获取命令
func (cr *CommandRegistry) GetCommandsByCategory(category CommandCategory) []CommandType {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.categories[category]
}

// GetCategories 获取所有分类
func (cr *CommandRegistry) GetCategories() []CommandCategory {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	categories := make([]CommandCategory, 0, len(cr.categories))
	for category := range cr.categories {
		categories = append(categories, category)
	}
	return categories
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
