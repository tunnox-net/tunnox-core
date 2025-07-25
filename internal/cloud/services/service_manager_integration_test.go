package services

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/utils"
)

// TestServiceManagerResourceRegistration 测试服务管理器的资源注册功能
func TestServiceManagerResourceRegistration(t *testing.T) {
	// 创建服务管理器，使用独立的资源管理器
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false                 // 禁用信号处理以便测试
	config.ResourceManager = utils.NewResourceManager() // 使用独立的资源管理器
	manager := utils.NewServiceManager(config)

	// 创建一些测试资源
	resource1 := &MockResource{name: "test-resource-1"}
	resource2 := &MockResource{name: "test-resource-2"}

	// 注册资源
	if err := manager.RegisterResource("resource-1", resource1); err != nil {
		t.Fatalf("Failed to register resource 1: %v", err)
	}

	if err := manager.RegisterResource("resource-2", resource2); err != nil {
		t.Fatalf("Failed to register resource 2: %v", err)
	}

	// 验证资源数量
	if count := manager.GetResourceCount(); count != 2 {
		t.Errorf("Expected 2 resources, got %d", count)
	}

	// 验证资源列表
	resources := manager.ListResources()
	if len(resources) != 2 {
		t.Errorf("Expected 2 resources in list, got %d", len(resources))
	}

	// 检查资源是否在列表中
	found1, found2 := false, false
	for _, name := range resources {
		if name == "resource-1" {
			found1 = true
		}
		if name == "resource-2" {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("Not all resources found in list")
	}

	// 取消注册资源
	if err := manager.UnregisterResource("resource-1"); err != nil {
		t.Fatalf("Failed to unregister resource 1: %v", err)
	}

	if count := manager.GetResourceCount(); count != 1 {
		t.Errorf("Expected 1 resource after unregister, got %d", count)
	}
}

// TestServiceManagerResourceDisposal 测试服务管理器的资源释放功能
func TestServiceManagerResourceDisposal(t *testing.T) {
	// 创建服务管理器，使用独立的资源管理器
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false
	config.ResourceDisposeTimeout = 5 * time.Second
	config.ResourceManager = utils.NewResourceManager() // 使用独立的资源管理器
	manager := utils.NewServiceManager(config)

	// 创建测试资源
	resource1 := &MockResource{name: "dispose-test-1"}
	resource2 := &MockResource{name: "dispose-test-2"}

	// 注册资源
	if err := manager.RegisterResource("dispose-1", resource1); err != nil {
		t.Fatalf("Failed to register resource 1: %v", err)
	}

	if err := manager.RegisterResource("dispose-2", resource2); err != nil {
		t.Fatalf("Failed to register resource 2: %v", err)
	}

	// 验证资源已注册
	if count := manager.GetResourceCount(); count != 2 {
		t.Errorf("Expected 2 resources, got %d", count)
	}

	// 手动触发资源释放
	result := manager.GetDisposeResult()
	if result != nil {
		t.Error("Dispose result should be nil before disposal")
	}

	// 直接调用资源管理器的DisposeAll方法
	disposeResult := config.ResourceManager.DisposeAll()

	// 验证释放结果
	if disposeResult.HasErrors() {
		t.Errorf("Resource disposal failed: %v", disposeResult.Error())
	}

	// 验证资源已被释放
	if !resource1.IsDisposed() {
		t.Error("Resource 1 should be disposed")
	}

	if !resource2.IsDisposed() {
		t.Error("Resource 2 should be disposed")
	}
}

// TestServiceManagerWithContext 测试带上下文的服务管理器运行
func TestServiceManagerWithContext(t *testing.T) {
	// 创建服务管理器，使用独立的资源管理器
	config := utils.DefaultServiceConfig()
	config.EnableSignalHandling = false
	config.ResourceManager = utils.NewResourceManager() // 使用独立的资源管理器
	manager := utils.NewServiceManager(config)

	// 创建测试资源
	resource := &MockResource{name: "context-test"}

	// 注册资源
	if err := manager.RegisterResource("context-resource", resource); err != nil {
		t.Fatalf("Failed to register resource: %v", err)
	}

	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 在goroutine中运行服务管理器
	go func() {
		if err := manager.RunWithContext(ctx); err != nil {
			t.Errorf("RunWithContext failed: %v", err)
		}
	}()

	// 等待上下文取消
	<-ctx.Done()

	// 等待一段时间让资源释放完成
	time.Sleep(100 * time.Millisecond)

	// 验证资源已被释放
	if !resource.IsDisposed() {
		t.Error("Resource should be disposed after context cancellation")
	}
}
