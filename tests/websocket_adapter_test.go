package tests

import (
	"context"
	"testing"
	"time"
	"tunnox-core/internal/protocol"
)

func TestWebSocketAdapterBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 测试基本功能
	adapter := protocol.NewWebSocketAdapter(ctx, nil)

	// 测试名称
	if adapter.Name() != "websocket" {
		t.Errorf("Expected name 'websocket', got '%s'", adapter.Name())
	}

	// 测试地址设置
	testAddr := "localhost:8080"
	adapter.ListenFrom(testAddr)
	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}

	// 测试启动服务器
	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("Failed to start WebSocket server: %v", err)
	}

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 测试停止服务器
	if err := adapter.Stop(); err != nil {
		t.Fatalf("Failed to stop WebSocket server: %v", err)
	}

	// 测试关闭
	adapter.Close()
}

func TestWebSocketAdapterName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := protocol.NewWebSocketAdapter(ctx, nil)

	if adapter.Name() != "websocket" {
		t.Errorf("Expected name 'websocket', got '%s'", adapter.Name())
	}
}

func TestWebSocketAdapterAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := protocol.NewWebSocketAdapter(ctx, nil)
	testAddr := "localhost:8080"

	adapter.ListenFrom(testAddr)

	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}
}
