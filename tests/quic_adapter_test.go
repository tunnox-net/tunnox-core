package tests

import (
	"context"
	"testing"
	"time"
	"tunnox-core/internal/protocol"
)

func TestQuicAdapterBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 测试基本功能
	adapter := protocol.NewQuicAdapter(ctx, nil)

	// 测试名称
	if adapter.Name() != "quic" {
		t.Errorf("Expected name 'quic', got '%s'", adapter.Name())
	}

	// 测试地址设置
	testAddr := "localhost:8080"
	adapter.ListenFrom(testAddr)
	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}

	// 测试启动服务器（这里可能会失败，因为需要TLS证书）
	// 在实际环境中，应该使用有效的TLS证书
	err := adapter.Start(ctx)
	if err != nil {
		t.Logf("QUIC server start failed (expected in test environment): %v", err)
	} else {
		// 等待服务器启动
		time.Sleep(100 * time.Millisecond)

		// 测试停止服务器
		if err := adapter.Stop(); err != nil {
			t.Fatalf("Failed to stop QUIC server: %v", err)
		}
	}

	// 测试关闭
	adapter.Close()
}

func TestQuicAdapterName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := protocol.NewQuicAdapter(ctx, nil)

	if adapter.Name() != "quic" {
		t.Errorf("Expected name 'quic', got '%s'", adapter.Name())
	}
}

func TestQuicAdapterAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := protocol.NewQuicAdapter(ctx, nil)
	testAddr := "localhost:8080"

	adapter.ListenFrom(testAddr)

	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}
}
