package socks5

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// mockTunnelCreator 模拟的隧道创建器
type mockTunnelCreator struct {
	createError error
	called      bool
	targetHost  string
	targetPort  int
}

func (m *mockTunnelCreator) CreateSOCKS5Tunnel(
	userConn net.Conn,
	mappingID string,
	targetClientID int64,
	targetHost string,
	targetPort int,
	secretKey string,
	onSuccess func(),
) error {
	m.called = true
	m.targetHost = targetHost
	m.targetPort = targetPort

	if m.createError != nil {
		return m.createError
	}

	// 调用成功回调
	if onSuccess != nil {
		onSuccess()
	}

	return nil
}

func TestNewListener(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":11080",
		MappingID:      "test-mapping",
		TargetClientID: 123,
		SecretKey:      "secret",
	}

	listener := NewListener(ctx, config, nil)
	if listener == nil {
		t.Fatal("NewListener returned nil")
	}

	if listener.config != config {
		t.Error("config not set correctly")
	}

	if listener.listener != nil {
		t.Error("listener should be nil before Start")
	}
}

func TestListener_Start(t *testing.T) {
	ctx := context.Background()

	// 找一个可用端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	config := &ListenerConfig{
		ListenAddr:     "127.0.0.1:" + itoa(port),
		MappingID:      "test-mapping",
		TargetClientID: 123,
		SecretKey:      "secret",
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})
	defer listener.Close()

	err = listener.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if listener.listener == nil {
		t.Error("listener should not be nil after Start")
	}
}

func TestListener_GetListenAddr(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		startListener  bool
		expectedPrefix string
	}{
		{
			name:           "before start",
			startListener:  false,
			expectedPrefix: "127.0.0.1:",
		},
		{
			name:           "after start",
			startListener:  true,
			expectedPrefix: "127.0.0.1:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ln, _ := net.Listen("tcp", "127.0.0.1:0")
			port := ln.Addr().(*net.TCPAddr).Port
			ln.Close()

			config := &ListenerConfig{
				ListenAddr:     "127.0.0.1:" + itoa(port),
				MappingID:      "test-mapping",
				TargetClientID: 123,
			}

			listener := NewListener(ctx, config, &mockTunnelCreator{})
			defer listener.Close()

			if tt.startListener {
				listener.Start()
			}

			addr := listener.GetListenAddr()
			if len(addr) < len(tt.expectedPrefix) {
				t.Errorf("GetListenAddr returned too short: %s", addr)
			}
		})
	}
}

func TestListener_Handshake_Success(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		clientConn.Write([]byte{Version, 1, AuthNone})

		resp := make([]byte, 2)
		clientConn.Read(resp)

		request := []byte{
			Version, CmdConnect, 0x00, AddrIPv4,
			192, 168, 1, 1,
		}
		port := make([]byte, 2)
		binary.BigEndian.PutUint16(port, 8080)
		request = append(request, port...)
		clientConn.Write(request)
	}()

	serverConn.SetDeadline(time.Now().Add(5 * time.Second))

	result, err := listener.Handshake(serverConn)
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	if result.Command != CmdConnect {
		t.Errorf("Expected command CmdConnect, got %d", result.Command)
	}

	if result.TargetHost != "192.168.1.1" {
		t.Errorf("Expected host '192.168.1.1', got '%s'", result.TargetHost)
	}

	if result.TargetPort != 8080 {
		t.Errorf("Expected port 8080, got %d", result.TargetPort)
	}
}

func TestListener_Handshake_Domain(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		clientConn.Write([]byte{Version, 1, AuthNone})

		resp := make([]byte, 2)
		clientConn.Read(resp)

		domain := "example.com"
		request := []byte{
			Version, CmdConnect, 0x00, AddrDomain,
			byte(len(domain)),
		}
		request = append(request, []byte(domain)...)
		port := make([]byte, 2)
		binary.BigEndian.PutUint16(port, 443)
		request = append(request, port...)
		clientConn.Write(request)
	}()

	serverConn.SetDeadline(time.Now().Add(5 * time.Second))

	result, err := listener.Handshake(serverConn)
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	if result.TargetHost != "example.com" {
		t.Errorf("Expected host 'example.com', got '%s'", result.TargetHost)
	}

	if result.TargetPort != 443 {
		t.Errorf("Expected port 443, got %d", result.TargetPort)
	}
}

func TestListener_Handshake_IPv6(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		clientConn.Write([]byte{Version, 1, AuthNone})

		resp := make([]byte, 2)
		clientConn.Read(resp)

		ipv6 := net.ParseIP("::1").To16()
		request := []byte{
			Version, CmdConnect, 0x00, AddrIPv6,
		}
		request = append(request, ipv6...)
		port := make([]byte, 2)
		binary.BigEndian.PutUint16(port, 80)
		request = append(request, port...)
		clientConn.Write(request)
	}()

	serverConn.SetDeadline(time.Now().Add(5 * time.Second))

	result, err := listener.Handshake(serverConn)
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	if result.TargetHost != "::1" {
		t.Errorf("Expected host '::1', got '%s'", result.TargetHost)
	}

	if result.TargetPort != 80 {
		t.Errorf("Expected port 80, got %d", result.TargetPort)
	}
}

func TestListener_Handshake_InvalidVersion(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		clientConn.Write([]byte{0x04, 1, AuthNone})
	}()

	serverConn.SetDeadline(time.Now().Add(1 * time.Second))

	_, err := listener.Handshake(serverConn)
	if err == nil {
		t.Error("Expected error for invalid version")
	}
}

func TestListener_Handshake_NoAuthMethods(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		clientConn.Write([]byte{Version, 0})
	}()

	serverConn.SetDeadline(time.Now().Add(1 * time.Second))

	_, err := listener.Handshake(serverConn)
	if err == nil {
		t.Error("Expected error for no auth methods")
	}
}

func TestListener_Handshake_UnsupportedCommand(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		clientConn.Write([]byte{Version, 1, AuthNone})

		resp := make([]byte, 2)
		clientConn.Read(resp)

		request := []byte{
			Version, CmdBind, 0x00, AddrIPv4,
			127, 0, 0, 1,
			0x00, 0x50,
		}
		clientConn.Write(request)

		errResp := make([]byte, 10)
		clientConn.Read(errResp)
	}()

	serverConn.SetDeadline(time.Now().Add(1 * time.Second))

	_, err := listener.Handshake(serverConn)
	if err == nil {
		t.Error("Expected error for unsupported command")
	}
}

func TestListener_Handshake_UnsupportedAddressType(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		clientConn.Write([]byte{Version, 1, AuthNone})

		resp := make([]byte, 2)
		clientConn.Read(resp)

		request := []byte{
			Version, CmdConnect, 0x00, 0x05,
			127, 0, 0, 1,
			0x00, 0x50,
		}
		clientConn.Write(request)

		errResp := make([]byte, 10)
		clientConn.Read(errResp)
	}()

	serverConn.SetDeadline(time.Now().Add(1 * time.Second))

	_, err := listener.Handshake(serverConn)
	if err == nil {
		t.Error("Expected error for unsupported address type")
	}
}

func TestListener_Handshake_UDPAssociate(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	go func() {
		clientConn.Write([]byte{Version, 1, AuthNone})

		resp := make([]byte, 2)
		clientConn.Read(resp)

		request := []byte{
			Version, CmdUDPAssoc, 0x00, AddrIPv4,
			0, 0, 0, 0,
			0x00, 0x00,
		}
		clientConn.Write(request)
	}()

	serverConn.SetDeadline(time.Now().Add(5 * time.Second))

	result, err := listener.Handshake(serverConn)
	if err != nil {
		t.Fatalf("Handshake failed: %v", err)
	}

	if result.Command != CmdUDPAssoc {
		t.Errorf("Expected command CmdUDPAssoc, got %d", result.Command)
	}
}

func TestListener_SendSuccess(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	// 创建管道连接
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// 读取响应
	go func() {
		resp := make([]byte, 10)
		n, _ := clientConn.Read(resp)
		if n < 10 {
			t.Errorf("Response too short: %d bytes", n)
		}
		if resp[0] != Version {
			t.Errorf("Wrong version: %d", resp[0])
		}
		if resp[1] != RepSuccess {
			t.Errorf("Wrong reply code: %d", resp[1])
		}
	}()

	listener.SendSuccess(serverConn)
	time.Sleep(100 * time.Millisecond)
}

func TestListener_SendError(t *testing.T) {
	ctx := context.Background()
	config := &ListenerConfig{
		ListenAddr:     ":0",
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	// 创建管道连接
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// 读取响应
	go func() {
		resp := make([]byte, 10)
		n, _ := clientConn.Read(resp)
		if n < 10 {
			t.Errorf("Response too short: %d bytes", n)
		}
		if resp[0] != Version {
			t.Errorf("Wrong version: %d", resp[0])
		}
		if resp[1] != RepFailure {
			t.Errorf("Wrong reply code: %d", resp[1])
		}
	}()

	listener.SendError(serverConn, RepFailure)
	time.Sleep(100 * time.Millisecond)
}

func TestListener_Close(t *testing.T) {
	ctx := context.Background()

	// 找一个可用端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	config := &ListenerConfig{
		ListenAddr:     "127.0.0.1:" + itoa(port),
		MappingID:      "test-mapping",
		TargetClientID: 123,
	}

	listener := NewListener(ctx, config, &mockTunnelCreator{})

	err = listener.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 关闭监听器
	listener.Close()

	// 验证关闭状态
	if !listener.IsClosed() {
		t.Error("Listener should be closed")
	}
}

// itoa 简单的整数转字符串
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	result := ""
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
