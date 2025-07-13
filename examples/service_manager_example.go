package main

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

func runServiceManagerExample() {
	// 创建服务配置
	config := utils.DefaultServiceConfig()
	config.GracefulShutdownTimeout = 30 * time.Second
	config.ResourceDisposeTimeout = 10 * time.Second

	// 创建服务管理器
	serviceManager := utils.NewServiceManager(config)

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. 创建并注册HTTP服务
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello from Tunnox Core HTTP Service!")
	})
	httpService := utils.NewHTTPService(":8080", httpHandler)
	serviceManager.RegisterService(httpService)

	// 2. 创建并注册协议服务
	protocolManager := protocol.NewManager(ctx)
	protocolService := protocol.NewProtocolService("Protocol-Service", protocolManager)
	serviceManager.RegisterService(protocolService)

	// 3. 创建并注册流服务
	streamFactory := stream.NewDefaultStreamFactory(ctx)
	streamManager := stream.NewStreamManager(streamFactory, ctx)
	streamService := stream.NewStreamService("Stream-Service", streamManager)
	serviceManager.RegisterService(streamService)

	// 4. 注册一些资源
	serviceManager.RegisterResource("database-connection", &MockDatabaseConnection{})
	serviceManager.RegisterResource("redis-client", &MockRedisClient{})
	serviceManager.RegisterResource("file-handler", &MockFileHandler{})

	// 5. 运行服务管理器
	fmt.Println("Starting Tunnox Core with multiple services...")
	fmt.Printf("Registered services: %v\n", serviceManager.ListServices())
	fmt.Printf("Registered resources: %v\n", serviceManager.ListResources())

	if err := serviceManager.RunWithContext(ctx); err != nil {
		fmt.Printf("Service manager error: %v\n", err)
	}

	fmt.Println("Tunnox Core shutdown completed")
}

// MockDatabaseConnection 模拟数据库连接
type MockDatabaseConnection struct {
	closed bool
}

func (m *MockDatabaseConnection) Dispose() error {
	if m.closed {
		return fmt.Errorf("database connection already closed")
	}
	m.closed = true
	fmt.Println("Database connection disposed")
	return nil
}

// MockRedisClient 模拟Redis客户端
type MockRedisClient struct {
	closed bool
}

func (m *MockRedisClient) Dispose() error {
	if m.closed {
		return fmt.Errorf("redis client already closed")
	}
	m.closed = true
	fmt.Println("Redis client disposed")
	return nil
}

// MockFileHandler 模拟文件处理器
type MockFileHandler struct {
	closed bool
}

func (m *MockFileHandler) Dispose() error {
	if m.closed {
		return fmt.Errorf("file handler already closed")
	}
	m.closed = true
	fmt.Println("File handler disposed")
	return nil
}
