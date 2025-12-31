package socks5

import (
	"context"
	"net"
	"testing"

	"tunnox-core/internal/cloud/models"
)

func TestNewManager(t *testing.T) {
	ctx := context.Background()
	clientID := int64(123)
	tunnelCreator := &mockTunnelCreator{}

	manager := NewManager(ctx, clientID, tunnelCreator)
	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	if manager.clientID != clientID {
		t.Errorf("Expected clientID %d, got %d", clientID, manager.clientID)
	}

	if manager.listeners == nil {
		t.Error("listeners map should not be nil")
	}

	if manager.tunnelCreator != tunnelCreator {
		t.Error("tunnelCreator not set correctly")
	}
}

func TestManager_AddMapping_NonSOCKS5(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})
	defer manager.Close()

	// 添加非 SOCKS5 映射
	mapping := &models.PortMapping{
		ID:             "test-mapping",
		Protocol:       models.ProtocolTCP, // 非 SOCKS5
		ListenClientID: 123,
	}

	err := manager.AddMapping(mapping)
	if err != nil {
		t.Errorf("AddMapping should not return error for non-SOCKS5 mapping: %v", err)
	}

	// 不应该创建监听器
	if len(manager.listeners) != 0 {
		t.Error("Should not create listener for non-SOCKS5 mapping")
	}
}

func TestManager_AddMapping_WrongClient(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})
	defer manager.Close()

	// 添加不属于本客户端的映射
	mapping := &models.PortMapping{
		ID:             "test-mapping",
		Protocol:       models.ProtocolSOCKS,
		ListenClientID: 456, // 不同的客户端ID
	}

	err := manager.AddMapping(mapping)
	if err != nil {
		t.Errorf("AddMapping should not return error: %v", err)
	}

	// 不应该创建监听器
	if len(manager.listeners) != 0 {
		t.Error("Should not create listener for wrong client")
	}
}

func TestManager_AddMapping_Success(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})
	defer manager.Close()

	// 找一个可用端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// 添加 SOCKS5 映射
	mapping := &models.PortMapping{
		ID:             "test-mapping",
		Protocol:       models.ProtocolSOCKS,
		ListenClientID: 123,
		TargetClientID: 456,
		SourcePort:     port,
		SecretKey:      "secret",
	}

	err = manager.AddMapping(mapping)
	if err != nil {
		t.Fatalf("AddMapping failed: %v", err)
	}

	// 应该创建监听器
	if len(manager.listeners) != 1 {
		t.Errorf("Expected 1 listener, got %d", len(manager.listeners))
	}

	if _, exists := manager.listeners["test-mapping"]; !exists {
		t.Error("Listener not found for mapping ID")
	}
}

func TestManager_AddMapping_Duplicate(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})
	defer manager.Close()

	// 找一个可用端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	mapping := &models.PortMapping{
		ID:             "test-mapping",
		Protocol:       models.ProtocolSOCKS,
		ListenClientID: 123,
		TargetClientID: 456,
		SourcePort:     port,
	}

	// 第一次添加
	err = manager.AddMapping(mapping)
	if err != nil {
		t.Fatalf("First AddMapping failed: %v", err)
	}

	// 第二次添加（重复）
	err = manager.AddMapping(mapping)
	if err != nil {
		t.Errorf("Duplicate AddMapping should not return error: %v", err)
	}

	// 应该仍然只有一个监听器
	if len(manager.listeners) != 1 {
		t.Errorf("Expected 1 listener, got %d", len(manager.listeners))
	}
}

func TestManager_RemoveMapping(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})
	defer manager.Close()

	// 找一个可用端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	mapping := &models.PortMapping{
		ID:             "test-mapping",
		Protocol:       models.ProtocolSOCKS,
		ListenClientID: 123,
		TargetClientID: 456,
		SourcePort:     port,
	}

	// 添加映射
	err = manager.AddMapping(mapping)
	if err != nil {
		t.Fatalf("AddMapping failed: %v", err)
	}

	// 移除映射
	manager.RemoveMapping("test-mapping")

	// 验证已移除
	if len(manager.listeners) != 0 {
		t.Errorf("Expected 0 listeners after remove, got %d", len(manager.listeners))
	}
}

func TestManager_RemoveMapping_NotExists(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})
	defer manager.Close()

	// 移除不存在的映射（不应该 panic）
	manager.RemoveMapping("non-existent-mapping")
}

func TestManager_GetMapping(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})
	defer manager.Close()

	// 找一个可用端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	mapping := &models.PortMapping{
		ID:             "test-mapping",
		Protocol:       models.ProtocolSOCKS,
		ListenClientID: 123,
		TargetClientID: 456,
		SourcePort:     port,
	}

	// 添加映射
	err = manager.AddMapping(mapping)
	if err != nil {
		t.Fatalf("AddMapping failed: %v", err)
	}

	// 获取存在的映射
	listener, exists := manager.GetMapping("test-mapping")
	if !exists {
		t.Error("GetMapping should return true for existing mapping")
	}
	if listener == nil {
		t.Error("GetMapping should return non-nil listener")
	}

	// 获取不存在的映射
	_, exists = manager.GetMapping("non-existent")
	if exists {
		t.Error("GetMapping should return false for non-existent mapping")
	}
}

func TestManager_ListMappings(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})
	defer manager.Close()

	// 空列表
	ids := manager.ListMappings()
	if len(ids) != 0 {
		t.Errorf("Expected empty list, got %d items", len(ids))
	}

	// 找可用端口
	ports := make([]int, 3)
	for i := 0; i < 3; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to find available port: %v", err)
		}
		ports[i] = ln.Addr().(*net.TCPAddr).Port
		ln.Close()
	}

	// 添加多个映射
	for i := 0; i < 3; i++ {
		mapping := &models.PortMapping{
			ID:             "mapping-" + itoa(i),
			Protocol:       models.ProtocolSOCKS,
			ListenClientID: 123,
			TargetClientID: 456,
			SourcePort:     ports[i],
		}
		err := manager.AddMapping(mapping)
		if err != nil {
			t.Fatalf("AddMapping failed: %v", err)
		}
	}

	// 列出映射
	ids = manager.ListMappings()
	if len(ids) != 3 {
		t.Errorf("Expected 3 mappings, got %d", len(ids))
	}
}

func TestManager_SetTunnelCreator(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, nil)
	defer manager.Close()

	if manager.tunnelCreator != nil {
		t.Error("tunnelCreator should be nil initially")
	}

	creator := &mockTunnelCreator{}
	manager.SetTunnelCreator(creator)

	if manager.tunnelCreator != creator {
		t.Error("SetTunnelCreator did not set the creator")
	}
}

func TestManager_SetClientID(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})
	defer manager.Close()

	if manager.clientID != 123 {
		t.Errorf("Expected clientID 123, got %d", manager.clientID)
	}

	manager.SetClientID(456)

	if manager.clientID != 456 {
		t.Errorf("Expected clientID 456, got %d", manager.clientID)
	}
}

func TestManager_Close(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, 123, &mockTunnelCreator{})

	// 找可用端口
	ports := make([]int, 2)
	for i := 0; i < 2; i++ {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("Failed to find available port: %v", err)
		}
		ports[i] = ln.Addr().(*net.TCPAddr).Port
		ln.Close()
	}

	// 添加多个映射
	for i := 0; i < 2; i++ {
		mapping := &models.PortMapping{
			ID:             "mapping-" + itoa(i),
			Protocol:       models.ProtocolSOCKS,
			ListenClientID: 123,
			TargetClientID: 456,
			SourcePort:     ports[i],
		}
		err := manager.AddMapping(mapping)
		if err != nil {
			t.Fatalf("AddMapping failed: %v", err)
		}
	}

	// 关闭管理器
	manager.Close()

	// 验证所有监听器都已关闭
	if len(manager.listeners) != 0 {
		t.Errorf("Expected 0 listeners after close, got %d", len(manager.listeners))
	}
}
