package adapter

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

func TestUdpSessionConn_OnHandshakeComplete(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 UDP 适配器并监听
	adapter := NewUdpAdapter(ctx, nil)
	testAddr := "localhost:0"
	adapter.ListenFrom(testAddr)
	defer adapter.Close()

	// 等待适配器启动
	time.Sleep(50 * time.Millisecond)

	// 创建一个有效的 UDP 会话连接
	// 需要有效的 net.PacketConn 和 net.Addr
	packetConn := adapter.conn
	if packetConn == nil {
		t.Skip("UDP adapter not properly initialized")
	}

	testAddrObj, err := net.ResolveUDPAddr("udp", "127.0.0.1:12345")
	require.NoError(t, err)

	session := newUdpSession(testAddrObj, packetConn, ctx)
	conn := session.sessionConn

	// 初始状态：不是控制连接
	conn.mu.Lock()
	initialState := conn.isControlConn
	conn.mu.Unlock()

	if initialState {
		t.Error("Initial state should not be control connection")
	}

	// 调用 OnHandshakeComplete
	conn.OnHandshakeComplete(123)

	// 验证已标记为控制连接
	conn.mu.Lock()
	isControlConn := conn.isControlConn
	conn.mu.Unlock()

	if !isControlConn {
		t.Error("Should be marked as control connection after OnHandshakeComplete")
	}
}

func TestUdpSessionConn_ToNetConn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建 UDP 适配器并监听
	adapter := NewUdpAdapter(ctx, nil)
	testAddr := "localhost:0"
	adapter.ListenFrom(testAddr)
	defer adapter.Close()

	// 等待适配器启动
	time.Sleep(50 * time.Millisecond)

	// 创建一个有效的 UDP 会话连接
	packetConn := adapter.conn
	if packetConn == nil {
		t.Skip("UDP adapter not properly initialized")
	}

	testAddrObj, err := net.ResolveUDPAddr("udp", "127.0.0.1:12345")
	require.NoError(t, err)

	session := newUdpSession(testAddrObj, packetConn, ctx)
	conn := session.sessionConn

	// 测试 ToNetConn
	netConn := conn.ToNetConn()
	if netConn == nil {
		t.Fatal("ToNetConn should return a non-nil net.Conn")
	}

	// 验证返回的是 udpConnWrapper
	// 注意：LocalAddr 可能为 nil（取决于实现），但 RemoteAddr 应该存在
	if netConn.RemoteAddr() == nil {
		t.Error("net.Conn should have RemoteAddr")
	}
}
