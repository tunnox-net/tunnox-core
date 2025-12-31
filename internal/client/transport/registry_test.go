package transport

import (
	"context"
	"net"
	"testing"
)

func TestRegisterProtocol(t *testing.T) {
	// 注意：TCP 协议已经在 init() 中注册
	// 测试注册新协议

	testDialer := func(ctx context.Context, address string) (net.Conn, error) {
		return nil, nil
	}

	// 注册测试协议
	RegisterProtocol("test-protocol", 50, testDialer)

	// 验证注册成功
	info, ok := GetProtocol("test-protocol")
	if !ok {
		t.Fatal("test-protocol should be registered")
	}

	if info.Name != "test-protocol" {
		t.Errorf("Expected name 'test-protocol', got '%s'", info.Name)
	}

	if info.Priority != 50 {
		t.Errorf("Expected priority 50, got %d", info.Priority)
	}

	if info.Dialer == nil {
		t.Error("Dialer should not be nil")
	}
}

func TestGetProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		exists   bool
	}{
		{
			name:     "TCP protocol exists",
			protocol: "tcp",
			exists:   true,
		},
		{
			name:     "non-existent protocol",
			protocol: "non-existent",
			exists:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, ok := GetProtocol(tt.protocol)
			if ok != tt.exists {
				t.Errorf("GetProtocol(%s) exists = %v, want %v", tt.protocol, ok, tt.exists)
			}
			if tt.exists && info == nil {
				t.Error("info should not be nil when protocol exists")
			}
		})
	}
}

func TestGetRegisteredProtocols(t *testing.T) {
	protocols := GetRegisteredProtocols()

	// 至少应该有 TCP
	if len(protocols) == 0 {
		t.Fatal("Should have at least one registered protocol")
	}

	// 检查是否按优先级排序
	for i := 1; i < len(protocols); i++ {
		if protocols[i].Priority < protocols[i-1].Priority {
			t.Errorf("Protocols not sorted by priority: %s (%d) < %s (%d)",
				protocols[i].Name, protocols[i].Priority,
				protocols[i-1].Name, protocols[i-1].Priority)
		}
	}

	// 检查 TCP 是否存在
	foundTCP := false
	for _, p := range protocols {
		if p.Name == "tcp" {
			foundTCP = true
			break
		}
	}
	if !foundTCP {
		t.Error("TCP protocol should be in the list")
	}
}

func TestIsProtocolAvailable(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		available bool
	}{
		{
			name:      "TCP is available",
			protocol:  "tcp",
			available: true,
		},
		{
			name:      "unknown protocol not available",
			protocol:  "unknown-protocol",
			available: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			available := IsProtocolAvailable(tt.protocol)
			if available != tt.available {
				t.Errorf("IsProtocolAvailable(%s) = %v, want %v", tt.protocol, available, tt.available)
			}
		})
	}
}

func TestDial_UnavailableProtocol(t *testing.T) {
	ctx := context.Background()

	_, err := Dial(ctx, "unavailable-protocol", "127.0.0.1:8080")
	if err == nil {
		t.Error("Dial should return error for unavailable protocol")
	}
}

func TestDial_TCP(t *testing.T) {
	// 启动一个测试 TCP 服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	address := listener.Addr().String()

	// 接受连接的 goroutine
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	// 使用 Dial 连接
	ctx := context.Background()
	conn, err := Dial(ctx, "tcp", address)
	if err != nil {
		t.Fatalf("Dial(tcp) failed: %v", err)
	}
	defer conn.Close()

	if conn == nil {
		t.Error("conn should not be nil")
	}
}

func TestGetAvailableProtocolNames(t *testing.T) {
	names := GetAvailableProtocolNames()

	if len(names) == 0 {
		t.Fatal("Should have at least one protocol name")
	}

	// 检查 TCP 是否在列表中
	foundTCP := false
	for _, name := range names {
		if name == "tcp" {
			foundTCP = true
			break
		}
	}
	if !foundTCP {
		t.Error("TCP should be in protocol names")
	}
}

func TestProtocolInfo(t *testing.T) {
	info, ok := GetProtocol("tcp")
	if !ok {
		t.Fatal("TCP protocol should exist")
	}

	if info.Name != "tcp" {
		t.Errorf("Expected name 'tcp', got '%s'", info.Name)
	}

	if info.Priority <= 0 {
		t.Errorf("Priority should be positive, got %d", info.Priority)
	}

	if info.Dialer == nil {
		t.Error("Dialer should not be nil")
	}
}
