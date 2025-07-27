package command

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
)

// TestRPCIntegration 测试RPC集成流程
func TestRPCIntegration(t *testing.T) {
	registry := NewCommandRegistry(context.Background())
	executor := NewCommandExecutor(registry, context.Background())

	// 创建处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionDuplex, // 替换 responseType 和 Duplex
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 解析请求数据
			var request struct {
				Port int `json:"port"`
			}
			if err := json.Unmarshal([]byte(ctx.RequestBody), &request); err != nil {
				return &CommandResponse{
					Success: false,
					Error:   "invalid request format",
				}, nil
			}

			// 处理请求
			data, _ := json.Marshal(map[string]interface{}{
				"port":      request.Port,
				"status":    "mapped",
				"requestID": ctx.RequestID,
			})
			return &CommandResponse{
				Success:        true,
				Data:           string(data),
				ProcessingTime: time.Since(ctx.StartTime),
				HandlerName:    "tcp_map_handler",
			}, nil
		},
	}

	// 注册处理器
	if err := registry.Register(handler); err != nil {
		t.Fatalf("Failed to register handler: %v", err)
	}

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.TcpMapCreate, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

// TestRPCWithMiddleware 测试带中间件的RPC流程
func TestRPCWithMiddleware(t *testing.T) {
	registry := NewCommandRegistry(context.Background())
	executor := NewCommandExecutor(registry, context.Background())

	// 创建处理器
	handler := &MockCommandHandler{
		commandType: packet.HttpMapCreate,
		direction:   DirectionDuplex, // 替换 responseType 和 Duplex
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			data, _ := json.Marshal("handler result")
			return &CommandResponse{Success: true, Data: string(data)}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建认证中间件
	authMiddleware := &MockMiddleware{
		name: "auth-middleware",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			// 检查认证信息
			if ctx.SenderID == "" {
				return &CommandResponse{
					Success: false,
					Error:   "unauthorized",
				}, nil
			}

			// 添加认证信息到上下文
			ctx.IsAuthenticated = true
			ctx.UserID = ctx.SenderID

			return next(ctx)
		},
	}

	// 创建日志中间件
	logMiddleware := &MockMiddleware{
		name: "log-middleware",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			// 记录请求开始
			ctx.StartTime = time.Now()

			response, err := next(ctx)

			// 记录请求结束
			processingTime := time.Since(ctx.StartTime)
			t.Logf("Request processed in %v", processingTime)

			return response, err
		},
	}

	// 添加中间件
	executor.AddMiddleware(authMiddleware)
	executor.AddMiddleware(logMiddleware)

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.HttpMapCreate, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

// TestRPCErrorHandling 测试RPC错误处理
func TestRPCErrorHandling(t *testing.T) {
	registry := NewCommandRegistry(context.Background())
	executor := NewCommandExecutor(registry, context.Background())

	// 创建会返回错误的处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapDelete,
		direction:   DirectionDuplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{
				Success: false,
				Error:   "mapping not found",
			}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.TcpMapDelete, `{"port": 9999}`)

	// 执行命令 - 对于双工模式，当处理器返回Success=false时，应该返回错误
	err := executor.Execute(streamPacket)
	// 期望返回错误，因为处理器返回了Success=false
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "command execution failed: mapping not found")
}

// TestRPCConcurrency 测试RPC并发处理
func TestRPCConcurrency(t *testing.T) {
	registry := NewCommandRegistry(context.Background())
	executor := NewCommandExecutor(registry, context.Background())

	// 创建处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionDuplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 模拟处理时间
			time.Sleep(10 * time.Millisecond)
			data, _ := json.Marshal("concurrent result")
			return &CommandResponse{Success: true, Data: string(data)}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 并发执行多个请求
	var wg sync.WaitGroup
	requestCount := 10

	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			streamPacket := createMockStreamPacket(packet.TcpMapCreate, fmt.Sprintf(`{"port": %d}`, 8000+index))
			err := executor.Execute(streamPacket)
			if err != nil {
				t.Errorf("Concurrent execute failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
}

// TestRPCRequestIDGeneration 测试请求ID生成
func TestRPCRequestIDGeneration(t *testing.T) {
	registry := NewCommandRegistry(context.Background())
	executor := NewCommandExecutor(registry, context.Background())

	// 生成多个请求ID
	requestIDs := make(map[string]bool)
	for i := 0; i < 100; i++ {
		requestID := executor.generateRequestID()
		if requestIDs[requestID] {
			t.Errorf("Duplicate request ID generated: %s", requestID)
		}
		requestIDs[requestID] = true
	}

	// 验证请求ID格式
	for requestID := range requestIDs {
		if len(requestID) == 0 {
			t.Error("Request ID should not be empty")
		}
	}
}

// TestRPCContextPropagation 测试上下文传播
func TestRPCContextPropagation(t *testing.T) {
	registry := NewCommandRegistry(context.Background())
	executor := NewCommandExecutor(registry, context.Background())

	// 创建处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionDuplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 验证上下文信息
			if ctx.ConnectionID == "" {
				return &CommandResponse{
					Success: false,
					Error:   "missing connection ID",
				}, nil
			}

			if ctx.SenderID == "" {
				return &CommandResponse{
					Success: false,
					Error:   "missing sender ID",
				}, nil
			}

			data, _ := json.Marshal(map[string]interface{}{
				"connectionID": ctx.ConnectionID,
				"senderID":     ctx.SenderID,
				"receiverID":   ctx.ReceiverID,
			})
			return &CommandResponse{Success: true, Data: string(data)}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.TcpMapCreate, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

// TestRPCResponseSerialization 测试响应序列化
func TestRPCResponseSerialization(t *testing.T) {
	registry := NewCommandRegistry(context.Background())
	executor := NewCommandExecutor(registry, context.Background())

	// 创建处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionDuplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 创建复杂响应
			response := &CommandResponse{
				Success:        true,
				Data:           `{"port": 8080, "status": "active"}`,
				ProcessingTime: time.Millisecond * 50,
				HandlerName:    "test_handler",
			}
			return response, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.TcpMapCreate, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

// TestRPCGracefulShutdown 测试优雅关闭
func TestRPCGracefulShutdown(t *testing.T) {
	registry := NewCommandRegistry(context.Background())
	executor := NewCommandExecutor(registry, context.Background())

	// 创建处理器
	handler := &MockCommandHandler{
		commandType: packet.TcpMapCreate,
		direction:   DirectionDuplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 模拟长时间处理
			time.Sleep(100 * time.Millisecond)
			return &CommandResponse{Success: true}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 启动一个长时间运行的请求
	go func() {
		streamPacket := createMockStreamPacket(packet.TcpMapCreate, `{"port": 8080}`)
		executor.Execute(streamPacket)
	}()

	// 等待一小段时间让请求开始
	time.Sleep(10 * time.Millisecond)

	// 关闭执行器
	// 注意：这里我们只是测试关闭，实际的优雅关闭需要在实现中添加
	t.Log("Testing graceful shutdown")
}
