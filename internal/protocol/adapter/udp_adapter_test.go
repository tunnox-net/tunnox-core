package adapter

import (
	"context"
	"testing"
	"time"
)

func TestUdpAdapterBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 测试基本功能
	adapter := NewUdpAdapter(ctx, nil)

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

	// 等待一段时间让服务器启动
	time.Sleep(100 * time.Millisecond)

	// 测试关闭
	adapter.Close()
}

func TestUdpAdapterName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewUdpAdapter(ctx, nil)

	if adapter.Name() != "udp" {
		t.Errorf("Expected name 'udp', got '%s'", adapter.Name())
	}
}

func TestUdpAdapterAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewUdpAdapter(ctx, nil)
	testAddr := "localhost:8080"

	adapter.ListenFrom(testAddr)

	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}
}
