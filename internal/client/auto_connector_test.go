package client

import (
	"context"
	"net"
	"testing"
	"time"

	"tunnox-core/internal/core/dispose"
)

// TestNewAutoConnector 测试创建自动连接器
func TestNewAutoConnector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := &ClientConfig{
		Anonymous: true,
		DeviceID:  "test-device",
	}
	client := NewClient(ctx, config)
	defer client.Close()

	connector := NewAutoConnector(ctx, client)
	defer connector.Close()

	if connector == nil {
		t.Fatal("Expected non-nil AutoConnector")
	}

	if connector.client != client {
		t.Error("Expected connector to reference the client")
	}
}

// TestDefaultServerEndpoints 测试默认端点列表
func TestDefaultServerEndpoints(t *testing.T) {
	if len(DefaultServerEndpoints) == 0 {
		t.Fatal("Expected non-empty default endpoints")
	}

	expectedProtocols := map[string]bool{
		"tcp":        false,
		"udp":        false,
		"quic":       false,
		"websocket":  false,
	}

	for _, endpoint := range DefaultServerEndpoints {
		if _, exists := expectedProtocols[endpoint.Protocol]; !exists {
			t.Errorf("Unexpected protocol: %s", endpoint.Protocol)
		}
		expectedProtocols[endpoint.Protocol] = true

		if endpoint.Address == "" {
			t.Errorf("Expected non-empty address for protocol %s", endpoint.Protocol)
		}
	}

	for protocol, found := range expectedProtocols {
		if !found {
			t.Errorf("Expected protocol %s not found in default endpoints", protocol)
		}
	}
}

// TestAutoConnector_ConnectWithAutoDetection_AllFailures 测试所有连接都失败的情况
func TestAutoConnector_ConnectWithAutoDetection_AllFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	config := &ClientConfig{
		Anonymous: true,
		DeviceID:  "test-device",
	}
	client := NewClient(ctx, config)
	defer client.Close()

	// 使用无效的端点（本地回环地址的无效端口）
	originalEndpoints := DefaultServerEndpoints
	defer func() {
		DefaultServerEndpoints = originalEndpoints
	}()

	DefaultServerEndpoints = []ServerEndpoint{
		{Protocol: "tcp", Address: "127.0.0.1:1"}, // 无效端口
		{Protocol: "tcp", Address: "127.0.0.1:2"}, // 无效端口
	}

	connector := NewAutoConnector(ctx, client)
	defer connector.Close()

	endpoint, err := connector.ConnectWithAutoDetection(ctx)
	if err == nil {
		t.Error("Expected error when all connections fail")
	}
	if endpoint != nil {
		t.Error("Expected nil endpoint when all connections fail")
	}
}

// TestAutoConnector_ContextCancellation 测试 Context 取消
func TestAutoConnector_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	config := &ClientConfig{
		Anonymous: true,
		DeviceID:  "test-device",
	}
	client := NewClient(ctx, config)
	defer client.Close()

	connector := NewAutoConnector(ctx, client)
	defer connector.Close()

	// 立即取消 context
	cancel()

	endpoint, err := connector.ConnectWithAutoDetection(ctx)
	if err == nil {
		t.Error("Expected error when context is cancelled")
	}
	if endpoint != nil {
		t.Error("Expected nil endpoint when context is cancelled")
	}
}

// TestAutoConnector_Timeout 测试超时处理
func TestAutoConnector_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping network test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	config := &ClientConfig{
		Anonymous: true,
		DeviceID:  "test-device",
	}
	client := NewClient(ctx, config)
	defer client.Close()

	connector := NewAutoConnector(ctx, client)
	defer connector.Close()

	endpoint, err := connector.ConnectWithAutoDetection(ctx)
	if err == nil {
		t.Error("Expected error when context times out")
	}
	if endpoint != nil {
		t.Error("Expected nil endpoint when context times out")
	}
}

// TestAutoConnector_CloseAttempt 测试关闭连接尝试
func TestAutoConnector_CloseAttempt(t *testing.T) {
	connector := &AutoConnector{
		ServiceBase: dispose.NewService("TestAutoConnector", context.Background()),
	}

	// 创建一个模拟的连接尝试
	attempt := &ConnectionAttempt{
		Endpoint: ServerEndpoint{Protocol: "tcp", Address: "127.0.0.1:8080"},
	}

	// 测试关闭 nil 连接（不应该 panic）
	connector.closeAttempt(attempt)

	// 创建一个真实的 TCP 连接用于测试
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Failed to dial: %v", err)
	}

	attempt.Conn = conn
	connector.closeAttempt(attempt)

	// 验证连接已关闭
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Error("Expected connection to be closed")
	}
}

// TestServerEndpoint 测试 ServerEndpoint 结构
func TestServerEndpoint(t *testing.T) {
	endpoint := ServerEndpoint{
		Protocol: "tcp",
		Address:  "127.0.0.1:8080",
	}

	if endpoint.Protocol != "tcp" {
		t.Errorf("Expected protocol 'tcp', got '%s'", endpoint.Protocol)
	}

	if endpoint.Address != "127.0.0.1:8080" {
		t.Errorf("Expected address '127.0.0.1:8080', got '%s'", endpoint.Address)
	}
}

// TestConnectionAttempt 测试 ConnectionAttempt 结构
func TestConnectionAttempt(t *testing.T) {
	attempt := &ConnectionAttempt{
		Endpoint: ServerEndpoint{Protocol: "tcp", Address: "127.0.0.1:8080"},
		Err:      nil,
	}

	if attempt.Endpoint.Protocol != "tcp" {
		t.Errorf("Expected protocol 'tcp', got '%s'", attempt.Endpoint.Protocol)
	}

	if attempt.Err != nil {
		t.Error("Expected nil error")
	}
}

