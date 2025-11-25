package adapter

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

func TestSocksAdapterBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建无认证的 SOCKS5 adapter
	adapter := NewSocksAdapter(ctx, nil, nil)

	// 测试名称
	if adapter.Name() != "socks5" {
		t.Errorf("Expected name 'socks5', got '%s'", adapter.Name())
	}

	// 测试地址设置
	testAddr := "localhost:1080"
	err := adapter.Listen(testAddr)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	adapter.SetAddr(testAddr)
	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}

	// 等待一段时间让服务器启动
	time.Sleep(100 * time.Millisecond)

	// 测试关闭
	err = adapter.Close()
	if err != nil {
		t.Errorf("Failed to close adapter: %v", err)
	}
}

func TestSocksAdapterWithAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建带认证的 SOCKS5 adapter
	config := &SocksConfig{
		Username: "testuser",
		Password: "testpass",
	}
	adapter := NewSocksAdapter(ctx, nil, config)

	// 测试名称
	if adapter.Name() != "socks5" {
		t.Errorf("Expected name 'socks5', got '%s'", adapter.Name())
	}

	// 测试认证已启用
	if !adapter.authEnabled {
		t.Error("Expected authentication to be enabled")
	}

	// 测试凭据
	password, exists := adapter.credentials["testuser"]
	if !exists {
		t.Error("Expected credentials to be set")
	}
	if password != "testpass" {
		t.Errorf("Expected password 'testpass', got '%s'", password)
	}

	// 测试关闭
	err := adapter.Close()
	if err != nil {
		t.Errorf("Failed to close adapter: %v", err)
	}
}

func TestSocksAdapterDialNotSupported(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewSocksAdapter(ctx, nil, nil)

	// SOCKS5 adapter 不支持 Dial（仅服务器模式）
	_, err := adapter.Dial("localhost:8080")
	if err == nil {
		t.Error("Expected Dial to return error")
	}

	adapter.Close()
}

func TestSocksHandshakeNoAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建无认证的 SOCKS5 adapter
	adapter := NewSocksAdapter(ctx, nil, nil)

	testAddr := "localhost:1081"
	err := adapter.Listen(testAddr)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer adapter.Close()

	// 启动 accept 循环
	go func() {
		for {
			_, err := adapter.Accept()
			if err != nil {
				if adapter.IsClosed() {
					return
				}
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 连接到 SOCKS5 服务器
	conn, err := net.Dial("tcp", testAddr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// 发送握手请求（无认证）
	// VER: 5, NMETHODS: 1, METHODS: 0 (无认证)
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		t.Fatalf("Failed to send handshake: %v", err)
	}

	// 读取响应
	buf := make([]byte, 2)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = io.ReadFull(conn, buf)
	if err != nil {
		t.Fatalf("Failed to read handshake response: %v", err)
	}

	// 验证响应
	if buf[0] != 0x05 {
		t.Errorf("Expected SOCKS version 5, got %d", buf[0])
	}
	if buf[1] != 0x00 {
		t.Errorf("Expected auth method 0 (no auth), got %d", buf[1])
	}
}

func TestSocksHandshakeWithAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建带认证的 SOCKS5 adapter
	config := &SocksConfig{
		Username: "testuser",
		Password: "testpass",
	}
	adapter := NewSocksAdapter(ctx, nil, config)

	testAddr := "localhost:1082"
	err := adapter.Listen(testAddr)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer adapter.Close()

	// 启动 accept 循环
	go func() {
		for {
			_, err := adapter.Accept()
			if err != nil {
				if adapter.IsClosed() {
					return
				}
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 连接到 SOCKS5 服务器
	conn, err := net.Dial("tcp", testAddr)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// 发送握手请求（支持用户名/密码认证）
	// VER: 5, NMETHODS: 1, METHODS: 2 (用户名/密码)
	_, err = conn.Write([]byte{0x05, 0x01, 0x02})
	if err != nil {
		t.Fatalf("Failed to send handshake: %v", err)
	}

	// 读取响应
	buf := make([]byte, 2)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = io.ReadFull(conn, buf)
	if err != nil {
		t.Fatalf("Failed to read handshake response: %v", err)
	}

	// 验证响应
	if buf[0] != 0x05 {
		t.Errorf("Expected SOCKS version 5, got %d", buf[0])
	}
	if buf[1] != 0x02 {
		t.Errorf("Expected auth method 2 (username/password), got %d", buf[1])
	}

	// 发送认证信息
	// VER: 1, ULEN: 8, UNAME: testuser, PLEN: 8, PASSWD: testpass
	username := "testuser"
	password := "testpass"
	authReq := []byte{0x01}
	authReq = append(authReq, byte(len(username)))
	authReq = append(authReq, []byte(username)...)
	authReq = append(authReq, byte(len(password)))
	authReq = append(authReq, []byte(password)...)

	_, err = conn.Write(authReq)
	if err != nil {
		t.Fatalf("Failed to send auth: %v", err)
	}

	// 读取认证响应
	authResp := make([]byte, 2)
	_, err = io.ReadFull(conn, authResp)
	if err != nil {
		t.Fatalf("Failed to read auth response: %v", err)
	}

	// 验证认证成功
	if authResp[0] != 0x01 {
		t.Errorf("Expected auth version 1, got %d", authResp[0])
	}
	if authResp[1] != 0x00 {
		t.Errorf("Expected auth success (0), got %d", authResp[1])
	}
}

func TestSocksAdapterName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewSocksAdapter(ctx, nil, nil)
	defer adapter.Close()

	if adapter.Name() != "socks5" {
		t.Errorf("Expected name 'socks5', got '%s'", adapter.Name())
	}
}

func TestSocksAdapterAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewSocksAdapter(ctx, nil, nil)
	defer adapter.Close()

	testAddr := "localhost:1083"
	err := adapter.Listen(testAddr)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	adapter.SetAddr(testAddr)

	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}
}

