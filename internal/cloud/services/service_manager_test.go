package services

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
	"tunnox-core/internal/protocol"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

// MockService 模拟服务
type MockService struct {
	name     string
	started  bool
	stopped  bool
	startErr error
	stopErr  error
	mu       sync.Mutex
}

// MockResource 模拟资源
type MockResource struct {
	name      string
	disposed  bool
	disposeMu sync.Mutex
}

func NewMockResource(name string) *MockResource {
	return &MockResource{name: name}
}

func (mr *MockResource) Dispose() error {
	mr.disposeMu.Lock()
	defer mr.disposeMu.Unlock()
	mr.disposed = true
	return nil
}

func (mr *MockResource) IsDisposed() bool {
	mr.disposeMu.Lock()
	defer mr.disposeMu.Unlock()
	return mr.disposed
}

func NewMockService(name string) *MockService {
	return &MockService{name: name}
}

func (ms *MockService) Name() string {
	return ms.name
}

func (ms *MockService) Start(ctx context.Context) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.started {
		return fmt.Errorf("service %s already started", ms.name)
	}

	ms.started = true
	utils.Infof("Mock service started: %s", ms.name)
	return ms.startErr
}

func (ms *MockService) Stop(ctx context.Context) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.stopped {
		return fmt.Errorf("service %s already stopped", ms.name)
	}

	ms.stopped = true
	utils.Infof("Mock service stopped: %s", ms.name)
	return ms.stopErr
}

func (ms *MockService) IsStarted() bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.started
}

func (ms *MockService) IsStopped() bool {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.stopped
}

// TestServiceManagerBasic 测试服务管理器基本功能
func TestServiceManagerBasic(t *testing.T) {
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false // 禁用信号处理以便测试
	manager := utils.NewServiceManager(config)

	// 测试初始状态
	if manager.GetServiceCount() != 0 {
		t.Errorf("Expected 0 services, got %d", manager.GetServiceCount())
	}

	// 注册服务
	service1 := NewMockService("service-1")
	service2 := NewMockService("service-2")

	if err := manager.RegisterService(service1); err != nil {
		t.Fatalf("Failed to register service1: %v", err)
	}

	if err := manager.RegisterService(service2); err != nil {
		t.Fatalf("Failed to register service2: %v", err)
	}

	// 验证服务数量
	if manager.GetServiceCount() != 2 {
		t.Errorf("Expected 2 services, got %d", manager.GetServiceCount())
	}

	// 验证服务列表
	services := manager.ListServices()
	if len(services) != 2 {
		t.Errorf("Expected 2 services in list, got %d", len(services))
	}

	// 测试重复注册
	if err := manager.RegisterService(service1); err == nil {
		t.Error("Expected error when registering duplicate service")
	}

	// 测试获取服务
	if s, exists := manager.GetService("service-1"); !exists {
		t.Error("Service service-1 should exist")
	} else if s != service1 {
		t.Error("Retrieved service should be the same as registered service")
	}

	// 测试注销服务
	if err := manager.UnregisterService("service-1"); err != nil {
		t.Fatalf("Failed to unregister service: %v", err)
	}

	if manager.GetServiceCount() != 1 {
		t.Errorf("Expected 1 service after unregister, got %d", manager.GetServiceCount())
	}
}

// TestServiceManagerStartStop 测试服务启动和停止
func TestServiceManagerStartStop(t *testing.T) {
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false
	manager := utils.NewServiceManager(config)

	// 创建测试服务
	service1 := NewMockService("service-1")
	service2 := NewMockService("service-2")

	manager.RegisterService(service1)
	manager.RegisterService(service2)

	// 启动所有服务
	if err := manager.StartAllServices(); err != nil {
		t.Fatalf("Failed to start services: %v", err)
	}

	// 验证服务已启动
	if !service1.IsStarted() {
		t.Error("Service1 should be started")
	}
	if !service2.IsStarted() {
		t.Error("Service2 should be started")
	}

	// 停止所有服务
	if err := manager.StopAllServices(); err != nil {
		t.Fatalf("Failed to stop services: %v", err)
	}

	// 验证服务已停止
	if !service1.IsStopped() {
		t.Error("Service1 should be stopped")
	}
	if !service2.IsStopped() {
		t.Error("Service2 should be stopped")
	}
}

// TestServiceManagerWithHTTPService 测试HTTP服务集成
func TestServiceManagerWithHTTPService(t *testing.T) {
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false
	manager := utils.NewServiceManager(config)

	// 创建HTTP服务
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello from test"))
	})
	httpService := utils.NewHTTPService(":0", handler) // 使用端口0让系统自动分配

	manager.RegisterService(httpService)

	// 启动服务
	if err := manager.StartAllServices(); err != nil {
		t.Fatalf("Failed to start HTTP service: %v", err)
	}

	// 给服务一点时间启动
	time.Sleep(100 * time.Millisecond)

	// 停止服务
	if err := manager.StopAllServices(); err != nil {
		t.Fatalf("Failed to stop HTTP service: %v", err)
	}
}

// TestServiceManagerWithProtocolService 测试协议服务集成
func TestServiceManagerWithProtocolService(t *testing.T) {
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false
	manager := utils.NewServiceManager(config)

	// 创建协议服务
	ctx := context.Background()
	protocolManager := protocol.NewProtocolManager(ctx)
	protocolService := protocol.NewProtocolService("test-protocol", protocolManager)

	manager.RegisterService(protocolService)

	// 启动服务
	if err := manager.StartAllServices(); err != nil {
		t.Fatalf("Failed to start protocol service: %v", err)
	}

	// 停止服务
	if err := manager.StopAllServices(); err != nil {
		t.Fatalf("Failed to stop protocol service: %v", err)
	}
}

// TestServiceManagerWithStreamService 测试流服务集成
func TestServiceManagerWithStreamService(t *testing.T) {
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false
	manager := utils.NewServiceManager(config)

	// 创建流服务
	ctx := context.Background()
	streamFactory := stream.NewDefaultStreamFactory(ctx)
	streamManager := stream.NewStreamManager(streamFactory, ctx)
	streamService := stream.NewStreamService("test-stream", streamManager)

	manager.RegisterService(streamService)

	// 启动服务
	if err := manager.StartAllServices(); err != nil {
		t.Fatalf("Failed to start stream service: %v", err)
	}

	// 停止服务
	if err := manager.StopAllServices(); err != nil {
		t.Fatalf("Failed to stop stream service: %v", err)
	}
}

// TestServiceManagerResourceManagement 测试资源管理
func TestServiceManagerResourceManagement(t *testing.T) {
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false
	manager := utils.NewServiceManager(config)

	// 注册资源
	resource1 := &MockResource{name: "resource-1"}
	resource2 := &MockResource{name: "resource-2"}

	if err := manager.RegisterResource("resource-1", resource1); err != nil {
		t.Fatalf("Failed to register resource1: %v", err)
	}

	if err := manager.RegisterResource("resource-2", resource2); err != nil {
		t.Fatalf("Failed to register resource2: %v", err)
	}

	// 验证资源数量
	if manager.GetResourceCount() != 2 {
		t.Errorf("Expected 2 resources, got %d", manager.GetResourceCount())
	}

	// 验证资源列表
	resources := manager.ListResources()
	if len(resources) != 2 {
		t.Errorf("Expected 2 resources in list, got %d", len(resources))
	}

	// 测试资源释放 - 通过ServiceManager的Dispose方法
	if err := manager.Close(); err != nil {
		t.Errorf("Service manager disposal failed: %v", err)
	}

	// 验证资源已被释放
	if !resource1.IsDisposed() {
		t.Error("Resource1 should be disposed")
	}
	if !resource2.IsDisposed() {
		t.Error("Resource2 should be disposed")
	}
}

// TestServiceManagerGracefulShutdown 测试优雅关闭
func TestServiceManagerGracefulShutdown(t *testing.T) {
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false
	config.GracefulShutdownTimeout = 5 * time.Second
	config.ResourceDisposeTimeout = 2 * time.Second

	manager := utils.NewServiceManager(config)

	// 注册服务和资源
	service := NewMockService("test-service")
	resource := &MockResource{name: "test-resource"}

	manager.RegisterService(service)
	manager.RegisterResource("test-resource", resource)

	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 运行服务管理器（会等待上下文取消）
	go func() {
		if err := manager.RunWithContext(ctx); err != nil {
			t.Errorf("Service manager run error: %v", err)
		}
	}()

	// 等待上下文取消
	<-ctx.Done()

	// 给优雅关闭一点时间完成
	time.Sleep(200 * time.Millisecond)

	// 验证服务已停止
	if !service.IsStopped() {
		t.Error("Service should be stopped after graceful shutdown")
	}

	// 验证资源已释放
	if !resource.IsDisposed() {
		t.Error("Resource should be disposed after graceful shutdown")
	}
}

// TestServiceManagerConcurrent 测试并发操作
func TestServiceManagerConcurrent(t *testing.T) {
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false
	manager := utils.NewServiceManager(config)

	var wg sync.WaitGroup
	serviceCount := 10

	// 并发注册服务
	for i := 0; i < serviceCount; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			service := NewMockService(fmt.Sprintf("service-%d", id))
			if err := manager.RegisterService(service); err != nil {
				t.Errorf("Failed to register service %d: %v", id, err)
			}
		}(i)
	}

	wg.Wait()

	// 验证所有服务都已注册
	if manager.GetServiceCount() != serviceCount {
		t.Errorf("Expected %d services, got %d", serviceCount, manager.GetServiceCount())
	}

	// 并发启动服务
	if err := manager.StartAllServices(); err != nil {
		t.Fatalf("Failed to start services: %v", err)
	}

	// 并发停止服务
	if err := manager.StopAllServices(); err != nil {
		t.Fatalf("Failed to stop services: %v", err)
	}
}
