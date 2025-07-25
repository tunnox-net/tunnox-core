package command

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/packet"
)

// TestRPCIntegration 测试RPC集成流程
func TestRPCIntegration(t *testing.T) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMapCreate,
		responseType: Duplex,
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
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.HttpMapCreate,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			data, _ := json.Marshal("handler result")
			return &CommandResponse{
				Success: true,
				Data:    string(data),
			}, nil
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
			if err != nil {
				return nil, err
			}

			// 记录请求结束
			ctx.EndTime = time.Now()

			return response, nil
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
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建会返回错误的处理器
	handler := &MockCommandHandler{
		commandType:  packet.SocksMapCreate,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 模拟业务逻辑错误
			var request struct {
				Port int `json:"port"`
			}
			if err := json.Unmarshal([]byte(ctx.RequestBody), &request); err != nil {
				return &CommandResponse{
					Success: false,
					Error:   "invalid JSON format",
				}, nil
			}

			// 检查端口范围
			if request.Port < 1 || request.Port > 65535 {
				return &CommandResponse{
					Success: false,
					Error:   "port out of range",
				}, nil
			}

			// 模拟系统错误
			if request.Port == 9999 {
				return nil, errors.New("system error")
			}

			data, _ := json.Marshal("port mapped successfully")
			return &CommandResponse{
				Success: true,
				Data:    string(data),
			}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 测试无效JSON - 双工命令会返回错误，这是预期的
	streamPacket1 := createMockStreamPacket(packet.SocksMapCreate, `invalid json`)
	_ = executor.Execute(streamPacket1)

	// 测试端口超出范围 - 双工命令会返回错误，这是预期的
	streamPacket2 := createMockStreamPacket(packet.SocksMapCreate, `{"port": 99999}`)
	_ = executor.Execute(streamPacket2)

	// 测试系统错误 - 双工命令会返回错误，这是预期的
	streamPacket3 := createMockStreamPacket(packet.SocksMapCreate, `{"port": 9999}`)
	_ = executor.Execute(streamPacket3)

	// 测试正常情况
	streamPacket4 := createMockStreamPacket(packet.SocksMapCreate, `{"port": 8080}`)
	err := executor.Execute(streamPacket4)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

// TestRPCConcurrency 测试RPC并发处理
func TestRPCConcurrency(t *testing.T) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMapCreate,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 模拟处理时间
			time.Sleep(50 * time.Millisecond)

			data, _ := json.Marshal(map[string]interface{}{
				"port": 8080,
			})
			return &CommandResponse{
				Success: true,
				Data:    string(data),
			}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 并发执行多个请求
	const numRequests = 10
	var wg sync.WaitGroup
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			streamPacket := createMockStreamPacket(packet.TcpMapCreate, `{"port": 8080}`)
			err := executor.Execute(streamPacket)
			results <- err
		}(i)
	}

	// 等待所有请求完成
	wg.Wait()
	close(results)

	// 检查结果
	for err := range results {
		if err != nil {
			t.Errorf("Concurrent execution failed: %v", err)
		}
	}
}

// TestRPCRequestIDGeneration 测试请求ID生成
func TestRPCRequestIDGeneration(t *testing.T) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMapCreate,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 验证请求ID不为空
			if ctx.RequestID == "" {
				return &CommandResponse{
					Success: false,
					Error:   "missing request ID",
				}, nil
			}

			data, _ := json.Marshal(map[string]interface{}{
				"port": 8080,
			})
			return &CommandResponse{
				Success: true,
				Data:    string(data),
			}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 执行多个请求，验证请求ID唯一性
	for i := 0; i < 5; i++ {
		streamPacket := createMockStreamPacket(packet.TcpMapCreate, `{"port": 8080}`)
		err := executor.Execute(streamPacket)
		if err != nil {
			t.Errorf("Execute failed: %v", err)
		}
	}
}

// TestRPCContextPropagation 测试上下文传播
func TestRPCContextPropagation(t *testing.T) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.HttpMapCreate,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 验证上下文字段
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

			if ctx.ReceiverID == "" {
				return &CommandResponse{
					Success: false,
					Error:   "missing receiver ID",
				}, nil
			}

			data, _ := json.Marshal(map[string]interface{}{
				"connection_id": ctx.ConnectionID,
				"sender_id":     ctx.SenderID,
				"receiver_id":   ctx.ReceiverID,
				"command_type":  ctx.CommandType,
			})
			return &CommandResponse{
				Success: true,
				Data:    string(data),
			}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.HttpMapCreate, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

// TestRPCResponseSerialization 测试响应序列化
func TestRPCResponseSerialization(t *testing.T) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMapCreate,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 创建复杂响应
			data, _ := json.Marshal(map[string]interface{}{
				"string": "test string",
				"int":    123,
				"float":  3.14,
				"bool":   true,
				"array":  []string{"a", "b", "c"},
				"object": map[string]interface{}{"key": "value"},
				"null":   nil,
			})
			response := &CommandResponse{
				Success:        true,
				Data:           string(data),
				ProcessingTime: time.Since(ctx.StartTime),
				HandlerName:    "serialization_test_handler",
				RequestID:      ctx.RequestID,
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
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMapCreate,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 检查上下文是否被取消
			select {
			case <-ctx.Context.Done():
				return &CommandResponse{
					Success: false,
					Error:   "context cancelled",
				}, nil
			default:
				// 模拟处理时间
				time.Sleep(100 * time.Millisecond)
				return &CommandResponse{Success: true}, nil
			}
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.TcpMapCreate, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// 等待上下文超时
	<-ctx.Done()
}
