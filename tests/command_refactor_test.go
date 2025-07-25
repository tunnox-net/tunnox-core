package tests

import (
	"bytes"
	"context"
	"testing"
	"time"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/command"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/utils"
)

// MockCommandHandler 模拟命令处理器
type MockCommandHandler struct {
	commandType packet.CommandType
	handleCount int
}

func NewMockCommandHandler(cmdType packet.CommandType) *MockCommandHandler {
	return &MockCommandHandler{
		commandType: cmdType,
		handleCount: 0,
	}
}

func (h *MockCommandHandler) Handle(ctx *command.CommandContext) (*command.CommandResponse, error) {
	h.handleCount++
	return &command.CommandResponse{
		Success:   true,
		RequestID: ctx.RequestID,
		CommandId: ctx.CommandId,
	}, nil
}

func (h *MockCommandHandler) GetCommandType() packet.CommandType {
	return h.commandType
}

func (h *MockCommandHandler) GetCategory() command.CommandCategory {
	return command.CategoryMapping
}

func (h *MockCommandHandler) GetDirection() command.CommandDirection {
	return command.DirectionOneway
}

func (h *MockCommandHandler) GetResponseType() types.CommandResponseType {
	return types.ResponseOneway
}

func TestCommandRefactor(t *testing.T) {
	// 创建内存存储
	storage := storages.NewMemoryStorage(context.Background())

	// 创建ID管理器
	idManager := generators.NewIDManager(storage, context.Background())

	// 创建会话
	session := protocol.NewConnectionSession(idManager, context.Background())
	defer session.Close()

	t.Run("CommandRegistry_Basic", func(t *testing.T) {
		registry := command.NewCommandRegistry()

		// 注册处理器
		handler := NewMockCommandHandler(packet.TcpMap)
		if err := registry.Register(handler); err != nil {
			t.Fatalf("Failed to register handler: %v", err)
		}

		// 验证注册
		if registry.GetHandlerCount() != 1 {
			t.Errorf("Expected 1 handler, got %d", registry.GetHandlerCount())
		}

		// 获取处理器
		retrievedHandler, exists := registry.GetHandler(packet.TcpMap)
		if !exists {
			t.Fatal("Handler not found")
		}

		// 测试处理器
		ctx := &command.CommandContext{
			ConnectionID: "test-conn",
			CommandType:  packet.TcpMap,
			CommandId:    "test-cmd",
			RequestID:    "test-req",
			RequestBody:  "{}",
		}

		response, err := retrievedHandler.Handle(ctx)
		if err != nil {
			t.Fatalf("Handler execution failed: %v", err)
		}

		if !response.Success {
			t.Error("Expected successful response")
		}

		if handler.handleCount != 1 {
			t.Errorf("Expected handle count 1, got %d", handler.handleCount)
		}
	})

	t.Run("Session_CommandHandling", func(t *testing.T) {
		// 创建连接
		reader := bytes.NewReader([]byte("test data"))
		writer := &bytes.Buffer{}

		conn, err := session.CreateConnection(reader, writer)
		if err != nil {
			t.Fatalf("Failed to create connection: %v", err)
		}

		// 创建模拟数据包
		packet := &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.TcpMap,
				CommandId:   "test-cmd-1",
				Token:       "test-token",
				SenderId:    "test-sender",
				ReceiverId:  "test-receiver",
				CommandBody: "{}",
			},
		}

		// 处理数据包
		err = session.ProcessPacket(conn.ID, packet)
		if err != nil {
			t.Fatalf("Failed to process packet: %v", err)
		}

		// 验证连接状态
		updatedConn, exists := session.GetConnection(conn.ID)
		if !exists {
			t.Fatal("Connection not found after processing")
		}

		if updatedConn.State != types.StateActive {
			t.Errorf("Expected state %v, got %v", types.StateActive, updatedConn.State)
		}

		// 清理连接
		err = session.CloseConnection(conn.ID)
		if err != nil {
			t.Fatalf("Failed to close connection: %v", err)
		}
	})

	t.Run("Session_ConnectionManagement", func(t *testing.T) {
		// 测试连接创建
		reader := bytes.NewReader([]byte("test data"))
		writer := &bytes.Buffer{}

		conn, err := session.CreateConnection(reader, writer)
		if err != nil {
			t.Fatalf("Failed to create connection: %v", err)
		}

		// 验证连接信息
		if conn.ID == "" {
			t.Error("Connection ID should not be empty")
		}

		if conn.State != types.StateInitializing {
			t.Errorf("Expected state %v, got %v", types.StateInitializing, conn.State)
		}

		// 测试状态更新
		err = session.UpdateConnectionState(conn.ID, types.StateConnected)
		if err != nil {
			t.Fatalf("Failed to update connection state: %v", err)
		}

		updatedConn, exists := session.GetConnection(conn.ID)
		if !exists {
			t.Fatal("Connection not found after update")
		}

		if updatedConn.State != types.StateConnected {
			t.Errorf("Expected state %v, got %v", types.StateConnected, updatedConn.State)
		}

		// 测试连接列表
		connections := session.ListConnections()
		if len(connections) == 0 {
			t.Error("Expected at least one connection")
		}

		// 测试活跃连接数
		activeCount := session.GetActiveConnections()
		if activeCount == 0 {
			t.Error("Expected at least one active connection")
		}

		// 测试连接关闭
		err = session.CloseConnection(conn.ID)
		if err != nil {
			t.Fatalf("Failed to close connection: %v", err)
		}

		// 验证连接已关闭
		_, exists = session.GetConnection(conn.ID)
		if exists {
			t.Error("Connection should not exist after closing")
		}
	})

	t.Run("Session_ConcurrentOperations", func(t *testing.T) {
		const numConnections = 100
		const numGoroutines = 10

		// 并发创建连接
		done := make(chan bool, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				for j := 0; j < numConnections/numGoroutines; j++ {
					reader := bytes.NewReader([]byte("test data"))
					writer := &bytes.Buffer{}

					conn, err := session.CreateConnection(reader, writer)
					if err != nil {
						utils.Errorf("Failed to create connection: %v", err)
						return
					}

					// 更新状态
					if err := session.UpdateConnectionState(conn.ID, types.StateActive); err != nil {
						utils.Errorf("Failed to update state: %v", err)
					}

					// 关闭连接
					if err := session.CloseConnection(conn.ID); err != nil {
						utils.Errorf("Failed to close connection: %v", err)
					}
				}
				done <- true
			}(i)
		}

		// 等待所有goroutine完成
		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		// 验证最终状态 - 给一些时间让所有goroutine完成
		time.Sleep(200 * time.Millisecond)
		connections := session.ListConnections()
		if len(connections) != 0 {
			t.Errorf("Expected 0 connections after cleanup, got %d", len(connections))
		}
	})
}
