package adapter

import (
	"context"
	"testing"
)

func TestWebSocketAdapterBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 测试基本功能
	adapter := NewWebSocketAdapter(ctx, nil)

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

	// 立即关闭，避免启动服务器
	adapter.Close()
}

func TestWebSocketAdapterName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewWebSocketAdapter(ctx, nil)

	if adapter.Name() != "websocket" {
		t.Errorf("Expected name 'websocket', got '%s'", adapter.Name())
	}
}

func TestWebSocketAdapterAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewWebSocketAdapter(ctx, nil)
	testAddr := "localhost:8080"

	adapter.ListenFrom(testAddr)

	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}

	// 立即关闭，避免启动服务器
	adapter.Close()
}
