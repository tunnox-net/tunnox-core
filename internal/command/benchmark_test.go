package command

import (
	"testing"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// BenchmarkRPCManager_RegisterRequest 基准测试RPC管理器注册请求
func BenchmarkRPCManager_RegisterRequest(b *testing.B) {
	rm := NewRPCManager()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requestID := string(rune(i))
		responseChan := make(chan *CommandResponse, 1)
		rm.RegisterRequest(requestID, responseChan)
	}
}

// BenchmarkRPCManager_GetRequest 基准测试RPC管理器获取请求
func BenchmarkRPCManager_GetRequest(b *testing.B) {
	rm := NewRPCManager()

	// 预先注册一些请求
	for i := 0; i < 1000; i++ {
		requestID := string(rune(i))
		responseChan := make(chan *CommandResponse, 1)
		rm.RegisterRequest(requestID, responseChan)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		requestID := string(rune(i % 1000))
		rm.GetRequest(requestID)
	}
}

// BenchmarkCommandRegistry_Register 基准测试命令注册器注册
func BenchmarkCommandRegistry_Register(b *testing.B) {
	cr := NewCommandRegistry()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler := &simpleMockHandler{
			commandType:  packet.CommandType(i % 10),
			responseType: Oneway,
		}
		cr.Register(handler)
	}
}

// BenchmarkCommandRegistry_GetHandler 基准测试命令注册器获取处理器
func BenchmarkCommandRegistry_GetHandler(b *testing.B) {
	cr := NewCommandRegistry()

	// 预先注册一些处理器
	for i := 0; i < 100; i++ {
		handler := &simpleMockHandler{
			commandType:  packet.CommandType(i),
			responseType: Oneway,
		}
		cr.Register(handler)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		commandType := packet.CommandType(i % 100)
		cr.GetHandler(commandType)
	}
}

// BenchmarkCommandExecutor_ExecuteOneway 基准测试单向命令执行
func BenchmarkCommandExecutor_ExecuteOneway(b *testing.B) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 注册单向处理器
	handler := &simpleMockHandler{
		commandType:  packet.TcpMap,
		responseType: Oneway,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true}, nil
		},
	}
	registry.Register(handler)

	// 创建流数据包
	streamPacket := &types.StreamPacket{
		ConnectionID: "benchmark-connection",
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.TcpMap,
				Token:       "benchmark-token",
				SenderId:    "benchmark-sender",
				ReceiverId:  "benchmark-receiver",
				CommandBody: `{"port": 8080}`,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.Execute(streamPacket)
	}
}

// BenchmarkCommandExecutor_ExecuteDuplex 基准测试双工命令执行
func BenchmarkCommandExecutor_ExecuteDuplex(b *testing.B) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 注册双工处理器
	handler := &simpleMockHandler{
		commandType:  packet.HttpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true}, nil
		},
	}
	registry.Register(handler)

	// 创建流数据包
	streamPacket := &types.StreamPacket{
		ConnectionID: "benchmark-connection",
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.HttpMap,
				Token:       "benchmark-token",
				SenderId:    "benchmark-sender",
				ReceiverId:  "benchmark-receiver",
				CommandBody: `{"port": 8080}`,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.Execute(streamPacket)
	}
}

// BenchmarkCommandExecutor_ExecuteWithMiddleware 基准测试带中间件的命令执行
func BenchmarkCommandExecutor_ExecuteWithMiddleware(b *testing.B) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 注册处理器
	handler := &simpleMockHandler{
		commandType:  packet.SocksMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true}, nil
		},
	}
	registry.Register(handler)

	// 添加中间件
	middleware := &simpleMockMiddleware{
		name: "benchmark-middleware",
		processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
			return next(ctx)
		},
	}
	executor.AddMiddleware(middleware)

	// 创建流数据包
	streamPacket := &types.StreamPacket{
		ConnectionID: "benchmark-connection",
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.SocksMap,
				Token:       "benchmark-token",
				SenderId:    "benchmark-sender",
				ReceiverId:  "benchmark-receiver",
				CommandBody: `{"port": 8080}`,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.Execute(streamPacket)
	}
}

// BenchmarkCommandExecutor_ExecuteMultipleMiddleware 基准测试多中间件命令执行
func BenchmarkCommandExecutor_ExecuteMultipleMiddleware(b *testing.B) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 注册处理器
	handler := &simpleMockHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true}, nil
		},
	}
	registry.Register(handler)

	// 添加多个中间件
	for i := 0; i < 5; i++ {
		middleware := &simpleMockMiddleware{
			name: "benchmark-middleware",
			processFunc: func(ctx *CommandContext, next func(*CommandContext) (*CommandResponse, error)) (*CommandResponse, error) {
				return next(ctx)
			},
		}
		executor.AddMiddleware(middleware)
	}

	// 创建流数据包
	streamPacket := &types.StreamPacket{
		ConnectionID: "benchmark-connection",
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.TcpMap,
				Token:       "benchmark-token",
				SenderId:    "benchmark-sender",
				ReceiverId:  "benchmark-receiver",
				CommandBody: `{"port": 8080}`,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.Execute(streamPacket)
	}
}

// BenchmarkConcurrentExecution 基准测试并发执行
func BenchmarkConcurrentExecution(b *testing.B) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 注册处理器
	handler := &simpleMockHandler{
		commandType:  packet.TcpMap,
		responseType: Duplex,
		handleFunc: func(ctx *CommandContext) (*CommandResponse, error) {
			return &CommandResponse{Success: true}, nil
		},
	}
	registry.Register(handler)

	// 创建流数据包
	streamPacket := &types.StreamPacket{
		ConnectionID: "benchmark-connection",
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.TcpMap,
				Token:       "benchmark-token",
				SenderId:    "benchmark-sender",
				ReceiverId:  "benchmark-receiver",
				CommandBody: `{"port": 8080}`,
			},
		},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			executor.Execute(streamPacket)
		}
	})
}

// BenchmarkRequestIDGeneration 基准测试请求ID生成
func BenchmarkRequestIDGeneration(b *testing.B) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.generateRequestID()
	}
}

// BenchmarkContextCreation 基准测试上下文创建
func BenchmarkContextCreation(b *testing.B) {
	registry := NewCommandRegistry()
	executor := NewCommandExecutor(registry)

	// 创建流数据包
	streamPacket := &types.StreamPacket{
		ConnectionID: "benchmark-connection",
		Packet: &packet.TransferPacket{
			PacketType: packet.JsonCommand,
			CommandPacket: &packet.CommandPacket{
				CommandType: packet.TcpMap,
				Token:       "benchmark-token",
				SenderId:    "benchmark-sender",
				ReceiverId:  "benchmark-receiver",
				CommandBody: `{"port": 8080}`,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.createCommandContext(streamPacket)
	}
}
