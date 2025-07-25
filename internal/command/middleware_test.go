package command

import (
	"context"
	"errors"
	"testing"
	"time"
	"tunnox-core/internal/packet"
)

func TestMiddlewareFunc_Process(t *testing.T) {
	// 创建中间件函数
	middlewareFunc := MiddlewareFunc(func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
		// 在调用前设置开始时间
		ctx.StartTime = time.Now()

		// 调用下一个处理器
		response, err := next(ctx)
		if err != nil {
			return nil, err
		}

		// 设置结束时间和处理时间
		ctx.EndTime = time.Now()
		response.ProcessingTime = ctx.EndTime.Sub(ctx.StartTime)
		response.HandlerName = "middleware_func"
		return response, nil
	})

	// 创建命令上下文
	ctx := &CommandContext{
		ConnectionID: "test-connection",
		CommandType:  packet.TcpMap,
		RequestID:    "test-request",
		Context:      context.Background(),
	}

	// 创建下一个处理器
	next := func(ctx *CommandContext) (*CommandResponse, error) {
		return &CommandResponse{Success: true, Data: "next result"}, nil
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

	if response.HandlerName != "middleware_func" {
		t.Error("Middleware should set handler name")
	}

	if response.ProcessingTime <= 0 {
		t.Error("Middleware should set processing time")
	}

	if ctx.StartTime.IsZero() {
		t.Error("Middleware should set start time")
	}

	if ctx.EndTime.IsZero() {
		t.Error("Middleware should set end time")
	}
}

func TestMiddlewareChain_Execution(t *testing.T) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true, Data: "handler result"}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建多个中间件
	middleware1 := &MockMiddleware{
		name: "middleware1",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			ctx.StartTime = time.Now()
			response, err := next(ctx)
			if err != nil {
				return nil, err
			}
			response.HandlerName = "middleware1"
			response.ProcessingTime = time.Since(ctx.StartTime)
			return response, nil
		},
	}

	middleware2 := &MockMiddleware{
		name: "middleware2",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			ctx.StartTime = time.Now()
			response, err := next(ctx)
			if err != nil {
				return nil, err
			}
			response.HandlerName = "middleware2"
			response.ProcessingTime = time.Since(ctx.StartTime)
			return response, nil
		},
	}

	middleware3 := &MockMiddleware{
		name: "middleware3",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			ctx.StartTime = time.Now()
			response, err := next(ctx)
			if err != nil {
				return nil, err
			}
			response.HandlerName = "middleware3"
			response.ProcessingTime = time.Since(ctx.StartTime)
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
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建会返回错误的处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return nil, errors.New("handler error")
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建会捕获错误的中间件
	middleware := &MockMiddleware{
		name: "error-handling-middleware",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			ctx.StartTime = time.Now()

			response, err := next(ctx)
			if err != nil {
				// 捕获错误并返回错误响应
				return &CommandResponse{
					Success:        false,
					Error:          "middleware caught: " + err.Error(),
					HandlerName:    "error-handling-middleware",
					ProcessingTime: time.Since(ctx.StartTime),
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
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true, Data: "handler result"}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建会短路执行的中间件
	middleware := &MockMiddleware{
		name: "short-circuit-middleware",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			// 检查请求体，如果是特定值则短路
			if ctx.RequestBody == `{"short_circuit": true}` {
				return &CommandResponse{
					Success:     true,
					Data:        "short circuit response",
					HandlerName: "short-circuit-middleware",
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
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			// 验证中间件修改的上下文
			if !ctx.IsAuthenticated {
				t.Error("Context should be authenticated by middleware")
			}
			if ctx.UserID == "" {
				t.Error("Context should have user ID set by middleware")
			}
			return &CommandResponse{Success: true, Data: ctx.UserID}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建会修改上下文的中间件
	middleware := &MockMiddleware{
		name: "context-modification-middleware",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			// 修改上下文
			ctx.IsAuthenticated = true
			ctx.UserID = "modified_user_id"

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
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true, Data: "original data"}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建会修改响应的中间件
	middleware := &MockMiddleware{
		name: "response-modification-middleware",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			response, err := next(ctx)
			if err != nil {
				return nil, err
			}

			// 修改响应
			response.Data = "modified data"
			response.HandlerName = "response-modification-middleware"
			response.ProcessingTime = 100 * time.Millisecond

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
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true, Data: "concurrent result"}, nil
		},
	}

	// 注册处理器
	registry.Register(handler)

	// 创建中间件
	middleware := &MockMiddleware{
		name: "concurrent-middleware",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			ctx.StartTime = time.Now()
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
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建处理器
	handler := &MockCommandHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true, Data: "no middleware"}, nil
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
