package command

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"tunnox-core/internal/packet"
)

// MockCommandHandler 模拟命令处理器
type MockCommandHandler struct {
	commandType packet.CommandType
	direction   CommandDirection // 替换 responseType
	handleFunc  func(*CommandContext) (*CommandResponse, error)
}

func (m *MockCommandHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	if m.handleFunc != nil {
		return m.handleFunc(ctx)
	}
	data, _ := json.Marshal("registry result")
	return &CommandResponse{Success: true, Data: string(data)}, nil
}

func (m *MockCommandHandler) GetDirection() CommandDirection {
	return m.direction
}

func (m *MockCommandHandler) GetCommandType() packet.CommandType {
	return m.commandType
}

func (m *MockCommandHandler) GetCategory() CommandCategory {
	return CategoryMapping
}

// GetRequestType 获取请求类型（向后兼容，返回nil）
func (m *MockCommandHandler) GetRequestType() reflect.Type { return nil }

// GetResponseType 获取响应类型（向后兼容，返回nil）
func (m *MockCommandHandler) GetResponseType() reflect.Type { return nil }

func TestNewCommandRegistry(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	if cr == nil {
		t.Fatal("NewCommandRegistry returned nil")
	}

	if cr.handlers == nil {
		t.Error("handlers map should be initialized")
	}

	if cr.GetHandlerCount() != 0 {
		t.Error("New registry should have 0 handlers")
	}
}

func TestCommandRegistry_Register(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	// 创建有效的处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionOneway, // 替换 Oneway
	}

	// 注册处理器
	err := cr.Register(handler)
	if err != nil {
		t.Errorf("Failed to register handler: %v", err)
	}

	// 验证注册成功
	if cr.GetHandlerCount() != 1 {
		t.Errorf("Expected 1 handler, got %d", cr.GetHandlerCount())
	}

	// 验证处理器存在
	registeredHandler, exists := cr.GetHandler(packet.TcpMapCreate)
	if !exists {
		t.Error("Handler should exist after registration")
	}

	if registeredHandler != handler {
		t.Error("Retrieved handler should be the same as registered handler")
	}
}

func TestCommandRegistry_RegisterInvalidCommandType(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	// 创建无效的处理器（命令类型为0）
	handler := &MockCommandHandler{
		commandType: 0,
		direction:   DirectionOneway,
	}

	// 尝试注册无效处理器
	err := cr.Register(handler)
	if err == nil {
		t.Error("Should return error for invalid command type")
	}

	// 验证没有注册
	if cr.GetHandlerCount() != 0 {
		t.Error("Invalid handler should not be registered")
	}
}

func TestCommandRegistry_RegisterDuplicate(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	// 创建第一个处理器
	handler1 := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionOneway,
	}

	// 创建第二个处理器（相同命令类型）
	handler2 := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionDuplex,
	}

	// 注册第一个处理器
	err := cr.Register(handler1)
	if err != nil {
		t.Errorf("Failed to register first handler: %v", err)
	}

	// 尝试注册重复的处理器
	err = cr.Register(handler2)
	if err == nil {
		t.Error("Should return error for duplicate registration")
	}

	// 验证只有第一个处理器存在
	if cr.GetHandlerCount() != 1 {
		t.Errorf("Expected 1 handler, got %d", cr.GetHandlerCount())
	}

	// 验证第一个处理器仍然存在
	registeredHandler, exists := cr.GetHandler(packet.TcpMapCreate)
	if !exists {
		t.Error("First handler should still exist")
	}

	if registeredHandler != handler1 {
		t.Error("First handler should be the registered handler")
	}
}

func TestCommandRegistry_Unregister(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	// 创建处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionOneway,
	}

	// 注册处理器
	err := cr.Register(handler)
	if err != nil {
		t.Errorf("Failed to register handler: %v", err)
	}

	// 验证注册成功
	if cr.GetHandlerCount() != 1 {
		t.Error("Expected 1 handler after registration")
	}

	// 注销处理器
	err = cr.Unregister(packet.TcpMapCreate)
	if err != nil {
		t.Errorf("Failed to unregister handler: %v", err)
	}

	// 验证注销成功
	if cr.GetHandlerCount() != 0 {
		t.Error("Expected 0 handlers after unregistration")
	}

	// 验证处理器不存在
	_, exists := cr.GetHandler(packet.TcpMapCreate)
	if exists {
		t.Error("Handler should not exist after unregistration")
	}
}

func TestCommandRegistry_UnregisterNonExistent(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	// 尝试注销不存在的处理器
	err := cr.Unregister(packet.TcpMapCreate)
	if err == nil {
		t.Error("Should return error for unregistering non-existent handler")
	}
}

func TestCommandRegistry_GetHandler(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	// 创建处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionOneway,
	}

	// 注册处理器
	err := cr.Register(handler)
	if err != nil {
		t.Errorf("Failed to register handler: %v", err)
	}

	// 获取存在的处理器
	retrievedHandler, exists := cr.GetHandler(packet.TcpMapCreate)
	if !exists {
		t.Error("Handler should exist")
	}

	if retrievedHandler != handler {
		t.Error("Retrieved handler should be the same as registered handler")
	}

	// 获取不存在的处理器
	_, exists = cr.GetHandler(packet.TcpMapDelete)
	if exists {
		t.Error("Non-existent handler should not exist")
	}
}

func TestCommandRegistry_ListHandlers(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	// 创建多个处理器
	handler1 := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionOneway,
	}

	handler2 := &MockCommandHandler{
		commandType: packet.TcpMapDelete,
		direction:   DirectionDuplex,
	}

	handler3 := &MockCommandHandler{
		commandType: packet.HttpMapCreate,
		direction:   DirectionOneway,
	}

	// 注册处理器
	err := cr.Register(handler1)
	if err != nil {
		t.Errorf("Failed to register handler1: %v", err)
	}

	err = cr.Register(handler2)
	if err != nil {
		t.Errorf("Failed to register handler2: %v", err)
	}

	err = cr.Register(handler3)
	if err != nil {
		t.Errorf("Failed to register handler3: %v", err)
	}

	// 获取处理器列表
	handlers := cr.ListHandlers()

	// 验证列表长度
	if len(handlers) != 3 {
		t.Errorf("Expected 3 handlers, got %d", len(handlers))
	}

	// 验证所有处理器都在列表中
	expectedTypes := map[packet.CommandType]bool{
		packet.TcpMapCreate:  true,
		packet.TcpMapDelete:  true,
		packet.HttpMapCreate: true,
	}

	for _, handlerType := range handlers {
		if !expectedTypes[handlerType] {
			t.Errorf("Unexpected handler type: %v", handlerType)
		}
	}
}

func TestCommandRegistry_GetHandlerCount(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	// 初始状态
	if cr.GetHandlerCount() != 0 {
		t.Error("New registry should have 0 handlers")
	}

	// 注册一个处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionOneway,
	}

	err := cr.Register(handler)
	if err != nil {
		t.Errorf("Failed to register handler: %v", err)
	}

	if cr.GetHandlerCount() != 1 {
		t.Error("Registry should have 1 handler after registration")
	}

	// 注册另一个处理器
	handler2 := &MockCommandHandler{
		commandType: packet.TcpMapDelete,
		direction:   DirectionDuplex,
	}

	err = cr.Register(handler2)
	if err != nil {
		t.Errorf("Failed to register handler2: %v", err)
	}

	if cr.GetHandlerCount() != 2 {
		t.Error("Registry should have 2 handlers after second registration")
	}

	// 注销一个处理器
	err = cr.Unregister(packet.TcpMapCreate)
	if err != nil {
		t.Errorf("Failed to unregister handler: %v", err)
	}

	if cr.GetHandlerCount() != 1 {
		t.Error("Registry should have 1 handler after unregistration")
	}
}

func TestCommandRegistry_ConcurrentAccess(t *testing.T) {
	cr := NewCommandRegistry(context.Background())

	// 并发注册处理器
	var wg sync.WaitGroup
	handlerCount := 10

	for i := 0; i < handlerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			handler := &MockCommandHandler{
				commandType: packet.CommandType(id + 1),
				direction:   DirectionOneway,
			}

			err := cr.Register(handler)
			if err != nil {
				t.Errorf("Failed to register handler %d: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// 验证所有处理器都被注册
	if cr.GetHandlerCount() != handlerCount {
		t.Errorf("Expected %d handlers, got %d", handlerCount, cr.GetHandlerCount())
	}

	// 并发获取处理器
	for i := 0; i < handlerCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			_, exists := cr.GetHandler(packet.CommandType(id + 1))
			if !exists {
				t.Errorf("Handler %d should exist", id)
			}
		}(i)
	}

	wg.Wait()
}
