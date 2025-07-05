package tests

import (
	"context"
	"testing"
	"time"
	"tunnox-core/internal/protocol"
)

func TestUdpAdapterBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 测试基本功能
	adapter := protocol.NewUdpAdapter(ctx, nil)

	// 测试名称
	if adapter.Name() != "udp" {
		t.Errorf("Expected name 'udp', got '%s'", adapter.Name())
	}

	// 测试地址设置
	testAddr := "localhost:8080"
	adapter.ListenFrom(testAddr)
	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}

	// 测试启动服务器
	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("Failed to start UDP server: %v", err)
	}

	// 等待服务器启动
	time.Sleep(100 * time.Millisecond)

	// 测试停止服务器
	if err := adapter.Stop(); err != nil {
		t.Fatalf("Failed to stop UDP server: %v", err)
	}

	// 测试关闭
	adapter.Close()
}

func TestUdpAdapterName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := protocol.NewUdpAdapter(ctx, nil)

	if adapter.Name() != "udp" {
		t.Errorf("Expected name 'udp', got '%s'", adapter.Name())
	}
}

func TestUdpAdapterAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := protocol.NewUdpAdapter(ctx, nil)
	testAddr := "localhost:8080"

	adapter.ListenFrom(testAddr)

	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}
}
