package registry

import (
	"context"
	"testing"

	"tunnox-core/internal/protocol/adapter"
)

// mockProtocol 测试用的协议实现
type mockProtocol struct {
	name         string
	dependencies []string
	validateErr  error
	initErr      error
}

func (m *mockProtocol) Name() string {
	return m.name
}

func (m *mockProtocol) Dependencies() []string {
	return m.dependencies
}

func (m *mockProtocol) ValidateConfig(config *Config) error {
	return m.validateErr
}

func (m *mockProtocol) Initialize(ctx context.Context, container Container, config *Config) (adapter.Adapter, error) {
	return nil, m.initErr
}

func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()

	// 测试正常注册
	protocol := &mockProtocol{name: "test"}
	err := registry.Register(protocol)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// 测试重复注册
	err = registry.Register(protocol)
	if err == nil {
		t.Fatal("Expected error for duplicate registration")
	}

	// 测试空名称
	emptyProtocol := &mockProtocol{name: ""}
	err = registry.Register(emptyProtocol)
	if err == nil {
		t.Fatal("Expected error for empty protocol name")
	}
}

func TestRegistry_Get(t *testing.T) {
	registry := NewRegistry()
	protocol := &mockProtocol{name: "test"}

	// 测试获取不存在的协议
	_, err := registry.Get("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent protocol")
	}

	// 注册并获取
	_ = registry.Register(protocol)
	got, err := registry.Get("test")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if got.Name() != "test" {
		t.Fatalf("Expected protocol name 'test', got %s", got.Name())
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	// 空注册表
	list := registry.List()
	if len(list) != 0 {
		t.Fatalf("Expected empty list, got %v", list)
	}

	// 注册多个协议
	_ = registry.Register(&mockProtocol{name: "tcp"})
	_ = registry.Register(&mockProtocol{name: "udp"})

	list = registry.List()
	if len(list) != 2 {
		t.Fatalf("Expected 2 protocols, got %d", len(list))
	}
}

func TestRegistry_HasProtocol(t *testing.T) {
	registry := NewRegistry()

	// 测试不存在的协议
	if registry.HasProtocol("nonexistent") {
		t.Fatal("Expected false for nonexistent protocol")
	}

	// 注册并测试
	_ = registry.Register(&mockProtocol{name: "test"})
	if !registry.HasProtocol("test") {
		t.Fatal("Expected true for registered protocol")
	}
}

