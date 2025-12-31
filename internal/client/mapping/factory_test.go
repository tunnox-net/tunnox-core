package mapping

import (
	"context"
	"strings"
	"testing"

	"tunnox-core/internal/config"
)

func TestCreateAdapter_TCP(t *testing.T) {
	cfg := config.MappingConfig{
		MappingID: "test-mapping",
		LocalPort: 8080,
	}

	adapter, err := CreateAdapter("tcp", cfg, context.Background())
	if err != nil {
		t.Fatalf("CreateAdapter(tcp) failed: %v", err)
	}
	if adapter == nil {
		t.Fatal("CreateAdapter(tcp) returned nil adapter")
	}
	if adapter.GetProtocol() != "tcp" {
		t.Errorf("Expected protocol 'tcp', got '%s'", adapter.GetProtocol())
	}

	// 清理
	adapter.Close()
}

func TestCreateAdapter_UDP(t *testing.T) {
	cfg := config.MappingConfig{
		MappingID: "test-mapping",
		LocalPort: 8081,
	}

	adapter, err := CreateAdapter("udp", cfg, context.Background())
	if err != nil {
		t.Fatalf("CreateAdapter(udp) failed: %v", err)
	}
	if adapter == nil {
		t.Fatal("CreateAdapter(udp) returned nil adapter")
	}
	if adapter.GetProtocol() != "udp" {
		t.Errorf("Expected protocol 'udp', got '%s'", adapter.GetProtocol())
	}

	// 清理
	adapter.Close()
}

func TestCreateAdapter_SOCKS5(t *testing.T) {
	cfg := config.MappingConfig{
		MappingID: "test-mapping",
		LocalPort: 1080,
	}

	adapter, err := CreateAdapter("socks5", cfg, context.Background())
	if err != nil {
		t.Fatalf("CreateAdapter(socks5) failed: %v", err)
	}
	if adapter == nil {
		t.Fatal("CreateAdapter(socks5) returned nil adapter")
	}
	if adapter.GetProtocol() != "socks5" {
		t.Errorf("Expected protocol 'socks5', got '%s'", adapter.GetProtocol())
	}

	// 清理
	adapter.Close()
}

func TestCreateAdapter_UnsupportedProtocol(t *testing.T) {
	cfg := config.MappingConfig{
		MappingID: "test-mapping",
		LocalPort: 8082,
	}

	adapter, err := CreateAdapter("unknown", cfg, context.Background())
	if err == nil {
		t.Fatal("CreateAdapter(unknown) should return error")
	}
	if adapter != nil {
		t.Error("CreateAdapter(unknown) should return nil adapter")
	}

	// 验证错误消息包含关键信息
	if !strings.Contains(err.Error(), "unsupported protocol: unknown") {
		t.Errorf("Expected error to contain 'unsupported protocol: unknown', got '%s'", err.Error())
	}
}
