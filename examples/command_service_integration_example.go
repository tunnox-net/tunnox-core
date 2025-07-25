package main

import (
	"context"
	"log"
	"time"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/command"
	"tunnox-core/internal/protocol"
)

func ExampleCommandServiceIntegration() {
	// 创建上下文
	ctx := context.Background()

	// 创建存储和ID管理器
	storage := storages.NewMemoryStorage(ctx)
	idManager := generators.NewIDManager(storage, ctx)

	// 创建会话
	session := protocol.NewConnectionSession(idManager, ctx)

	// 创建并配置命令服务
	commandService := command.CreateDefaultService(ctx)

	// 添加中间件
	commandService.Use(&command.LoggingMiddleware{})
	commandService.Use(command.NewMetricsMiddleware(nil)) // 可以传入metrics收集器

	// 设置命令服务到会话
	session.SetCommandService(commandService)

	log.Println("Command service integrated successfully!")

	// 获取统计信息
	stats := commandService.GetStats()
	log.Printf("Command service stats: %+v", stats.GetStats())

	// 这里可以继续使用 session 进行其他操作
	// 例如：处理连接、处理命令等

	// 模拟一些命令执行
	simulateCommandExecution(session, commandService)

	// 等待一段时间后查看统计信息
	time.Sleep(2 * time.Second)
	stats = commandService.GetStats()
	log.Printf("Updated command service stats: %+v", stats.GetStats())
}

func simulateCommandExecution(session protocol.Session, commandService command.CommandService) {
	// 创建模拟的命令上下文
	ctx := &command.CommandContext{
		ConnectionID:    "test-connection-123",
		CommandType:     1, // 假设是某个命令类型
		CommandId:       "cmd-123",
		RequestID:       "req-456",
		SenderID:        "sender-123",
		ReceiverID:      "receiver-456",
		RequestBody:     `{"test": "data"}`,
		Session:         session,
		Context:         context.Background(),
		IsAuthenticated: true,
		UserID:          "user-123",
		StartTime:       time.Now(),
	}

	// 异步执行命令
	responseChan, errorChan := commandService.ExecuteAsync(ctx)

	// 等待结果
	select {
	case response := <-responseChan:
		log.Printf("Command executed successfully: %+v", response)
	case err := <-errorChan:
		log.Printf("Command execution failed: %v", err)
	case <-time.After(5 * time.Second):
		log.Println("Command execution timeout")
	}
}

func ExampleCustomMiddleware() {
	ctx := context.Background()
	commandService := command.CreateDefaultService(ctx)

	// 添加自定义中间件
	commandService.Use(&CustomMiddleware{})

	log.Println("Custom middleware added successfully!")
}

// CustomMiddleware 自定义中间件示例
type CustomMiddleware struct{}

func (cm *CustomMiddleware) Process(ctx *command.CommandContext, next func(*command.CommandContext) (*command.CommandResponse, error)) (*command.CommandResponse, error) {
	log.Printf("Custom middleware: processing command %v for connection %s", ctx.CommandType, ctx.ConnectionID)

	// 前置处理
	startTime := time.Now()

	// 调用下一个处理器
	response, err := next(ctx)

	// 后置处理
	duration := time.Since(startTime)
	log.Printf("Custom middleware: command completed in %v", duration)

	return response, err
}
