package command

import (
	"encoding/json"
	"testing"
	"tunnox-core/internal/packet"
)

// MockCommandHandler 模拟命令处理器
type MockCommandHandler struct {
	commandType  packet.CommandType
	responseType CommandResponseType
	handleFunc   func(*CommandContext) (*CommandResponse, error)
}

func (m *MockCommandHandler) Handle(ctx *CommandContext) (*CommandResponse, error) {
	if m.handleFunc != nil {
		return m.handleFunc(ctx)
	}
	data, _ := json.Marshal("registry result")
	return &CommandResponse{Success: true, Data: string(data)}, nil
}

func (m *MockCommandHandler) GetResponseType() CommandResponseType {
	return m.responseType
}

func (m *MockCommandHandler) GetCommandType() packet.CommandType {
	return m.commandType
}

func (m *MockCommandHandler) GetCategory() CommandCategory {
	return CategoryMapping
}

func (m *MockCommandHandler) GetDirection() CommandDirection {
	return DirectionOneway
}

func TestNewCommandRegistry(t *testing.T) {
	cr := NewCommandRegistry()

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
	cr := NewCommandRegistry()

	// 创建有效的处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Oneway,
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
	registeredHandler, exists := cr.GetHandler(packet.TcpMap)
	if !exists {
		t.Error("Handler should exist after registration")
	}

	if registeredHandler != handler {
		t.Error("Retrieved handler should be the same as registered handler")
	}
}

func TestCommandRegistry_RegisterInvalidCommandType(t *testing.T) {
	cr := NewCommandRegistry()

	// 创建无效的处理器（命令类型为0）
	handler := &MockCommandHandler{
		commandType:  0,
		responseType: Oneway,
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
	cr := NewCommandRegistry()

	// 创建第一个处理器
	handler1 := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Oneway,
	}

	// 创建第二个处理器（相同命令类型）
	handler2 := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
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

	// 验证获取到的是第一个处理器
	registeredHandler, exists := cr.GetHandler(packet.TcpMap)
	if !exists {
		t.Error("Handler should exist")
	}

	if registeredHandler != handler1 {
		t.Error("Should get the first registered handler")
	}
}

func TestCommandRegistry_Unregister(t *testing.T) {
	cr := NewCommandRegistry()

	// 注册处理器
	handler := &MockCommandHandler{
		commandType:  packet.HttpMap,
		responseType: Duplex,
	}

	err := cr.Register(handler)
	if err != nil {
		t.Errorf("Failed to register handler: %v", err)
	}

	// 验证注册成功
	if cr.GetHandlerCount() != 1 {
		t.Error("Handler should be registered")
	}

	// 注销处理器
	err = cr.Unregister(packet.HttpMap)
	if err != nil {
		t.Errorf("Failed to unregister handler: %v", err)
	}

	// 验证注销成功
	if cr.GetHandlerCount() != 0 {
		t.Error("Handler should be unregistered")
	}

	// 验证处理器不存在
	_, exists := cr.GetHandler(packet.HttpMap)
	if exists {
		t.Error("Handler should not exist after unregistration")
	}
}

func TestCommandRegistry_UnregisterNonExistent(t *testing.T) {
	cr := NewCommandRegistry()

	// 尝试注销不存在的处理器
	err := cr.Unregister(packet.SocksMap)
	if err == nil {
		t.Error("Should return error for non-existent handler")
	}
}

func TestCommandRegistry_GetHandler(t *testing.T) {
	cr := NewCommandRegistry()

	// 注册多个处理器
	handlers := map[packet.CommandType]*MockCommandHandler{
		packet.TcpMap:   {commandType: packet.TcpMap, responseType: Oneway},
		packet.HttpMap:  {commandType: packet.HttpMap, responseType: Duplex},
		packet.SocksMap: {commandType: packet.SocksMap, responseType: Oneway},
	}

	for _, handler := range handlers {
		err := cr.Register(handler)
		if err != nil {
			t.Errorf("Failed to register handler: %v", err)
		}
	}

	// 测试获取存在的处理器
	for commandType, expectedHandler := range handlers {
		handler, exists := cr.GetHandler(commandType)
		if !exists {
			t.Errorf("Handler for %v should exist", commandType)
		}

		if handler != expectedHandler {
			t.Errorf("Expected handler %v, got %v", expectedHandler, handler)
		}
	}

	// 测试获取不存在的处理器
	_, exists := cr.GetHandler(packet.DataIn)
	if exists {
		t.Error("Non-existent handler should not exist")
	}
}

func TestCommandRegistry_ListHandlers(t *testing.T) {
	cr := NewCommandRegistry()

	// 注册多个处理器
	expectedTypes := []packet.CommandType{
		packet.TcpMap,
		packet.HttpMap,
		packet.SocksMap,
	}

	for _, commandType := range expectedTypes {
		handler := &MockCommandHandler{
			commandType:  commandType,
			responseType: Oneway,
		}
		err := cr.Register(handler)
		if err != nil {
			t.Errorf("Failed to register handler: %v", err)
		}
	}

	// 获取所有处理器类型
	types := cr.ListHandlers()

	// 验证数量
	if len(types) != len(expectedTypes) {
		t.Errorf("Expected %d types, got %d", len(expectedTypes), len(types))
	}

	// 验证所有期望的类型都存在
	typeMap := make(map[packet.CommandType]bool)
	for _, t := range types {
		typeMap[t] = true
	}

	for _, expectedType := range expectedTypes {
		if !typeMap[expectedType] {
			t.Errorf("Expected type %v not found in list", expectedType)
		}
	}
}

func TestCommandRegistry_GetHandlerCount(t *testing.T) {
	cr := NewCommandRegistry()

	// 初始数量应该为0
	count := cr.GetHandlerCount()
	if count != 0 {
		t.Errorf("Expected initial count 0, got %d", count)
	}

	// 注册处理器
	handler1 := &MockCommandHandler{commandType: packet.TcpMap, responseType: Oneway}
	handler2 := &MockCommandHandler{commandType: packet.HttpMap, responseType: Duplex}

	cr.Register(handler1)
	cr.Register(handler2)

	// 验证数量
	count = cr.GetHandlerCount()
	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}

	// 注销一个处理器
	cr.Unregister(packet.TcpMap)

	// 验证数量
	count = cr.GetHandlerCount()
	if count != 1 {
		t.Errorf("Expected count 1, got %d", count)
	}
}

func TestCommandRegistry_ConcurrentAccess(t *testing.T) {
	cr := NewCommandRegistry()
	done := make(chan bool, 10)

	// 并发注册和注销处理器
	for i := 0; i < 10; i++ {
		go func(id int) {
			commandType := packet.CommandType(id + 1)
			handler := &MockCommandHandler{
				commandType:  commandType,
				responseType: Oneway,
			}

			// 注册
			err := cr.Register(handler)
			if err != nil {
				t.Errorf("Failed to register handler: %v", err)
			}

			// 验证注册成功
			_, exists := cr.GetHandler(commandType)
			if !exists {
				t.Errorf("Handler for %v should exist", commandType)
			}

			// 注销
			err = cr.Unregister(commandType)
			if err != nil {
				t.Errorf("Failed to unregister handler: %v", err)
			}

			done <- true
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证最终数量为0
	count := cr.GetHandlerCount()
	if count != 0 {
		t.Errorf("Expected final count 0, got %d", count)
	}
}
