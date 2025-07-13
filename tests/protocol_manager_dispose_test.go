package tests

import (
	"context"
	"io"
	"testing"

	"tunnox-core/internal/protocol"
	"tunnox-core/internal/utils"
)

// TestProtocolManagerDispose 测试协议管理器的Dispose功能
func TestProtocolManagerDispose(t *testing.T) {
	// 创建协议管理器
	ctx := context.Background()
	manager := protocol.NewManager(ctx)

	// 创建一些测试适配器
	adapter1 := &MockAdapter{name: "test-adapter-1"}
	adapter2 := &MockAdapter{name: "test-adapter-2"}

	// 注册适配器
	manager.Register(adapter1)
	manager.Register(adapter2)

	// 测试Dispose功能
	if err := manager.Dispose(); err != nil {
		t.Errorf("Failed to dispose protocol manager: %v", err)
	}

	// 验证适配器已被关闭
	if !adapter1.closed {
		t.Error("Adapter 1 should be closed")
	}
	if !adapter2.closed {
		t.Error("Adapter 2 should be closed")
	}
}

// TestProtocolManagerWithResourceManager 测试协议管理器与资源管理器的集成
func TestProtocolManagerWithResourceManager(t *testing.T) {
	// 创建资源管理器
	resourceMgr := utils.NewResourceManager()

	// 创建协议管理器
	ctx := context.Background()
	manager := protocol.NewManager(ctx)

	// 注册到资源管理器
	if err := resourceMgr.Register("protocol-manager", manager); err != nil {
		t.Fatalf("Failed to register protocol manager: %v", err)
	}

	// 验证注册成功
	if count := resourceMgr.GetResourceCount(); count != 1 {
		t.Errorf("Expected 1 resource, got %d", count)
	}

	// 释放所有资源
	result := resourceMgr.DisposeAll()

	// 验证释放结果
	if result.HasErrors() {
		t.Errorf("Resource disposal failed: %v", result.Error())
	}

	// 验证资源已被释放
	if count := resourceMgr.GetResourceCount(); count != 0 {
		t.Errorf("Expected 0 resources after disposal, got %d", count)
	}
}

// MockAdapter 模拟适配器
type MockAdapter struct {
	name   string
	closed bool
}

func (a *MockAdapter) Name() string {
	return a.name
}

func (a *MockAdapter) GetAddr() string {
	return "localhost:8080"
}

func (a *MockAdapter) SetAddr(addr string) {
	// 模拟实现
}

func (a *MockAdapter) ConnectTo(serverAddr string) error {
	return nil
}

func (a *MockAdapter) ListenFrom(serverAddr string) error {
	return nil
}

func (a *MockAdapter) GetReader() io.Reader {
	return nil
}

func (a *MockAdapter) GetWriter() io.Writer {
	return nil
}

func (a *MockAdapter) Close() error {
	a.closed = true
	return nil
}
