package protocols

import (
	"context"
	"testing"

	"tunnox-core/internal/cloud/container"
	"tunnox-core/internal/protocol/registry"
	"tunnox-core/internal/protocol/session"
)

func TestTCPProtocol_Name(t *testing.T) {
	protocol := NewTCPProtocol()
	if protocol.Name() != "tcp" {
		t.Fatalf("Expected protocol name 'tcp', got %s", protocol.Name())
	}
}

func TestTCPProtocol_Dependencies(t *testing.T) {
	protocol := NewTCPProtocol()
	deps := protocol.Dependencies()
	if len(deps) != 1 || deps[0] != "session_manager" {
		t.Fatalf("Expected dependencies ['session_manager'], got %v", deps)
	}
}

func TestTCPProtocol_ValidateConfig(t *testing.T) {
	protocol := NewTCPProtocol()

	// 测试有效配置
	config := &registry.Config{
		Port: 8080,
	}
	err := protocol.ValidateConfig(config)
	if err != nil {
		t.Fatalf("Expected no error for valid config, got %v", err)
	}

	// 测试无效端口
	config.Port = 0
	err = protocol.ValidateConfig(config)
	if err == nil {
		t.Fatal("Expected error for invalid port")
	}

	config.Port = 65536
	err = protocol.ValidateConfig(config)
	if err == nil {
		t.Fatal("Expected error for port out of range")
	}
}

func TestTCPProtocol_Initialize(t *testing.T) {
	protocol := NewTCPProtocol()

	// 创建测试容器
	ctx := context.Background()
	c := container.NewContainer(ctx)
	
	// 创建 SessionManager 并注册
	sessionMgr := session.NewSessionManager(nil, ctx)
	c.RegisterSingleton("session_manager", func() (interface{}, error) {
		return sessionMgr, nil
	})

	// 创建容器适配器
	containerAdapter := registry.NewContainerAdapter(c)

	// 测试初始化
	config := &registry.Config{
		Host: "0.0.0.0",
		Port: 8080,
	}

	adapter, err := protocol.Initialize(ctx, containerAdapter, config)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if adapter == nil {
		t.Fatal("Expected adapter, got nil")
	}
	if adapter.GetAddr() != "0.0.0.0:8080" {
		t.Fatalf("Expected address '0.0.0.0:8080', got %s", adapter.GetAddr())
	}
}

func TestTCPProtocol_Initialize_MissingDependency(t *testing.T) {
	protocol := NewTCPProtocol()

	// 创建空容器
	ctx := context.Background()
	c := container.NewContainer(ctx)
	containerAdapter := registry.NewContainerAdapter(c)

	config := &registry.Config{
		Host: "0.0.0.0",
		Port: 8080,
	}

	_, err := protocol.Initialize(ctx, containerAdapter, config)
	if err == nil {
		t.Fatal("Expected error for missing dependency")
	}
}

