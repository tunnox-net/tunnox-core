package tests

import (
	"context"
	"errors"
	"testing"
	"tunnox-core/internal/command"
	"tunnox-core/internal/packet"
)

func TestMiddlewareFunc_Process(t *testing.T) {
	// 创建中间件函数
	middlewareFunc := command.MiddlewareFunc(func(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
		// 在调用前添加元数据
		ctx.Metadata["middleware_called"] = true

		// 调用下一个处理器
		response, err := next(ctx)
		if err != nil {
			return nil, err
		}

		// 修改响应
		response.Metadata = map[string]interface{}{"middleware": "func"}
		return response, nil
	})

	// 创建命令上下文
	ctx := &command.CommandContext{
		ConnectionID: "test-connection",
		CommandType:  packet.TcpMap,
		RequestID:    "test-request",
		Metadata:     make(map[string]interface{}),
		Context:      context.Background(),
	}

	// 创建下一个处理器
	next := func(ctx *command.CommandContext) (*command.CommandResponse, error) {
		return &command.CommandResponse{Success: true, Data: "next result"}, nil
	}

	// 执行中间件
	response, err := middlewareFunc.Process(ctx, next)
	if err != nil {
		t.Errorf("Middleware process failed: %v", err)
	}

	// 验证结果
	if !response.Success {
		t.Error("Response should be successful")
	}

	if response.Data != "next result" {
		t.Errorf("Expected data 'next result', got %v", response.Data)
	}

	if response.Metadata["middleware"] != "func" {
		t.Error("Middleware should add metadata to response")
	}

	if !ctx.Metadata["middleware_called"].(bool) {
		t.Error("Middleware should add metadata to context")
	}
}

func TestMiddlewareChain_Execution(t *testing.T) {
	registry := command.NewCommandRegistry()
	executor := command.NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: command.Duplex,
		handleFunc: func(ctx *command.CommandContext) (*command.CommandResponse, error) {
			return &command.CommandResponse{Success: true, Data: "handler result"}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建多个中间件
	middleware1 := &MockMiddleware{
		name: "middleware1",
		processFunc: func(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
			ctx.Metadata["middleware1"] = true
			response, err := next(ctx)
			if err != nil {
				return nil, err
			}
			if response.Metadata == nil {
				response.Metadata = make(map[string]interface{})
			}
			response.Metadata["middleware1"] = "processed"
			return response, nil
		},
	}

	middleware2 := &MockMiddleware{
		name: "middleware2",
		processFunc: func(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
			ctx.Metadata["middleware2"] = true
			response, err := next(ctx)
			if err != nil {
				return nil, err
			}
			if response.Metadata == nil {
				response.Metadata = make(map[string]interface{})
			}
			response.Metadata["middleware2"] = "processed"
			return response, nil
		},
	}

	middleware3 := &MockMiddleware{
		name: "middleware3",
		processFunc: func(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
			ctx.Metadata["middleware3"] = true
			response, err := next(ctx)
			if err != nil {
				return nil, err
			}
			if response.Metadata == nil {
				response.Metadata = make(map[string]interface{})
			}
			response.Metadata["middleware3"] = "processed"
			return response, nil
		},
	}

	// 添加中间件（按添加顺序执行）
	executor.AddMiddleware(middleware1)
	executor.AddMiddleware(middleware2)
	executor.AddMiddleware(middleware3)

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.TcpMap, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

func TestMiddlewareChain_ErrorHandling(t *testing.T) {
	registry := command.NewCommandRegistry()
	executor := command.NewCommandExecutor(registry)

	// 创建会返回错误的处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: command.Duplex,
		handleFunc: func(ctx *command.CommandContext) (*command.CommandResponse, error) {
			return nil, errors.New("handler error")
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建会捕获错误的中间件
	middleware := &MockMiddleware{
		name: "error-handling-middleware",
		processFunc: func(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
			ctx.Metadata["middleware_called"] = true

			response, err := next(ctx)
			if err != nil {
				// 捕获错误并返回错误响应
				return &command.CommandResponse{
					Success: false,
					Error:   "middleware caught: " + err.Error(),
				}, nil
			}

			return response, nil
		},
	}

	// 添加中间件
	executor.AddMiddleware(middleware)

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.TcpMap, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

func TestMiddlewareChain_ShortCircuit(t *testing.T) {
	registry := command.NewCommandRegistry()
	executor := command.NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: command.Duplex,
		handleFunc: func(ctx *command.CommandContext) (*command.CommandResponse, error) {
			return &command.CommandResponse{Success: true, Data: "handler result"}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建会短路执行的中间件
	middleware := &MockMiddleware{
		name: "short-circuit-middleware",
		processFunc: func(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
			// 检查请求体，如果是特定值则短路
			if ctx.RequestBody == `{"short_circuit": true}` {
				return &command.CommandResponse{
					Success: true,
					Data:    "short circuit response",
				}, nil
			}

			// 否则继续执行
			return next(ctx)
		},
	}

	// 添加中间件
	executor.AddMiddleware(middleware)

	// 测试短路情况
	streamPacket1 := createMockStreamPacket(packet.TcpMap, `{"short_circuit": true}`)
	err := executor.Execute(streamPacket1)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// 测试正常执行情况
	streamPacket2 := createMockStreamPacket(packet.TcpMap, `{"port": 8080}`)
	err = executor.Execute(streamPacket2)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

func TestMiddlewareChain_ContextModification(t *testing.T) {
	registry := command.NewCommandRegistry()
	executor := command.NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: command.Duplex,
		handleFunc: func(ctx *command.CommandContext) (*command.CommandResponse, error) {
			// 验证中间件修改的上下文
			if ctx.Metadata["modified"] != true {
				t.Error("Context should be modified by middleware")
			}
			return &command.CommandResponse{Success: true, Data: ctx.Metadata["value"]}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建会修改上下文的中间件
	middleware := &MockMiddleware{
		name: "context-modification-middleware",
		processFunc: func(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
			// 修改上下文
			ctx.Metadata["modified"] = true
			ctx.Metadata["value"] = "modified value"

			return next(ctx)
		},
	}

	// 添加中间件
	executor.AddMiddleware(middleware)

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.TcpMap, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

func TestMiddlewareChain_ResponseModification(t *testing.T) {
	registry := command.NewCommandRegistry()
	executor := command.NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: command.Duplex,
		handleFunc: func(ctx *command.CommandContext) (*command.CommandResponse, error) {
			return &command.CommandResponse{Success: true, Data: "original data"}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建会修改响应的中间件
	middleware := &MockMiddleware{
		name: "response-modification-middleware",
		processFunc: func(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
			response, err := next(ctx)
			if err != nil {
				return nil, err
			}

			// 修改响应
			response.Data = "modified data"
			response.Metadata = map[string]interface{}{"modified": true}

			return response, nil
		},
	}

	// 添加中间件
	executor.AddMiddleware(middleware)

	// 创建流数据包
	streamPacket := createMockStreamPacket(packet.TcpMap, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

func TestMiddlewareChain_ConcurrentAccess(t *testing.T) {
	registry := command.NewCommandRegistry()
	executor := command.NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: command.Duplex,
		handleFunc: func(ctx *command.CommandContext) (*command.CommandResponse, error) {
			return &command.CommandResponse{Success: true, Data: "concurrent result"}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建中间件
	middleware := &MockMiddleware{
		name: "concurrent-middleware",
		processFunc: func(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
			ctx.Metadata["concurrent"] = true
			return next(ctx)
		},
	}

	// 添加中间件
	executor.AddMiddleware(middleware)

	// 并发执行
	done := make(chan bool, 3)

	for i := 0; i < 3; i++ {
		go func(id int) {
			streamPacket := createMockStreamPacket(packet.TcpMap, `{"port": 8080}`)
			err := executor.Execute(streamPacket)
			if err != nil {
				t.Errorf("Execute failed: %v", err)
			}
			done <- true
		}(i)
	}

	// 等待所有执行完成
	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestMiddlewareChain_EmptyChain(t *testing.T) {
	registry := command.NewCommandRegistry()
	executor := command.NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: command.Duplex,
		handleFunc: func(ctx *command.CommandContext) (*command.CommandResponse, error) {
			return &command.CommandResponse{Success: true, Data: "no middleware"}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 不添加任何中间件
	streamPacket := createMockStreamPacket(packet.TcpMap, `{"port": 8080}`)

	// 执行命令
	err := executor.Execute(streamPacket)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}
