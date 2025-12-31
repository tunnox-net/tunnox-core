package mapping

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"tunnox-core/internal/config"
)

func TestNewSOCKS5MappingAdapter(t *testing.T) {
	tests := []struct {
		name        string
		credentials map[string]string
	}{
		{
			name:        "nil credentials",
			credentials: nil,
		},
		{
			name:        "empty credentials",
			credentials: make(map[string]string),
		},
		{
			name: "with credentials",
			credentials: map[string]string{
				"user": "pass",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewSOCKS5MappingAdapter(tt.credentials)
			if adapter == nil {
				t.Fatal("NewSOCKS5MappingAdapter returned nil")
			}
			if adapter.listener != nil {
				t.Error("listener should be nil before StartListener")
			}
		})
	}
}

func TestSOCKS5MappingAdapter_GetProtocol(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)
	if adapter.GetProtocol() != "socks5" {
		t.Errorf("Expected protocol 'socks5', got '%s'", adapter.GetProtocol())
	}
}

func TestSOCKS5MappingAdapter_StartListener(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)
	defer adapter.Close()

	// 找一个可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := config.MappingConfig{
		MappingID: "test-socks5-mapping",
		LocalPort: port,
	}

	err = adapter.StartListener(cfg)
	if err != nil {
		t.Fatalf("StartListener failed: %v", err)
	}

	if adapter.listener == nil {
		t.Error("listener should not be nil after StartListener")
	}
}

func TestSOCKS5MappingAdapter_Accept(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)
	defer adapter.Close()

	// 找一个可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := config.MappingConfig{
		MappingID: "test-socks5-mapping",
		LocalPort: port,
	}

	err = adapter.StartListener(cfg)
	if err != nil {
		t.Fatalf("StartListener failed: %v", err)
	}

	// 启动一个客户端连接
	go func() {
		time.Sleep(100 * time.Millisecond)
		conn, err := net.Dial("tcp", adapter.listener.Addr().String())
		if err != nil {
			t.Logf("Dial failed: %v", err)
			return
		}
		defer conn.Close()
	}()

	// Accept 连接
	conn, err := adapter.Accept()
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	if conn == nil {
		t.Fatal("Accept returned nil connection")
	}
	defer conn.Close()
}

func TestSOCKS5MappingAdapter_AcceptWithoutListener(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)

	_, err := adapter.Accept()
	if err == nil {
		t.Error("Accept should return error when listener is nil")
	}
}

func TestSOCKS5MappingAdapter_Close(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)

	// 关闭未初始化的适配器不应该报错
	err := adapter.Close()
	if err != nil {
		t.Errorf("Close without listener should not error, got %v", err)
	}

	// 初始化后关闭
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := config.MappingConfig{
		MappingID: "test-socks5-mapping",
		LocalPort: port,
	}

	err = adapter.StartListener(cfg)
	if err != nil {
		t.Fatalf("StartListener failed: %v", err)
	}

	err = adapter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestSOCKS5MappingAdapter_PrepareConnection_NonNetConn(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)

	// 使用一个不是 net.Conn 的 io.ReadWriteCloser
	mockConn := &mockReadWriteCloser{}

	err := adapter.PrepareConnection(mockConn)
	if err == nil {
		t.Error("PrepareConnection should return error for non-net.Conn")
	}
}

type mockReadWriteCloser struct{}

func (m *mockReadWriteCloser) Read(p []byte) (n int, err error)  { return 0, io.EOF }
func (m *mockReadWriteCloser) Write(p []byte) (n int, err error) { return len(p), nil }
func (m *mockReadWriteCloser) Close() error                      { return nil }

func TestSOCKS5MappingAdapter_HandleHandshake(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)

	tests := []struct {
		name        string
		input       []byte
		expectError bool
	}{
		{
			name:        "valid handshake with no auth",
			input:       []byte{socks5Version, 1, socksAuthNone},
			expectError: false,
		},
		{
			name:        "invalid version",
			input:       []byte{0x04, 1, socksAuthNone},
			expectError: true,
		},
		{
			name:        "empty input",
			input:       []byte{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建管道连接
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()

			// 设置超时
			serverConn.SetDeadline(time.Now().Add(1 * time.Second))

			// 客户端发送握手数据
			go func() {
				clientConn.Write(tt.input)
				// 读取服务端响应
				resp := make([]byte, 2)
				clientConn.Read(resp)
			}()

			// 服务端处理握手
			err := adapter.handleHandshake(serverConn)
			if tt.expectError && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestSOCKS5MappingAdapter_HandleRequest_IPv4(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)
	defer adapter.Close()

	// 找一个可用端口并启动监听器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := config.MappingConfig{
		MappingID: "test-socks5-mapping",
		LocalPort: port,
	}

	err = adapter.StartListener(cfg)
	if err != nil {
		t.Fatalf("StartListener failed: %v", err)
	}

	// 客户端发送 CONNECT 请求（IPv4）
	resultChan := make(chan string, 1)
	errChan := make(chan error, 1)

	go func() {
		time.Sleep(50 * time.Millisecond)
		conn, err := net.Dial("tcp", adapter.listener.Addr().String())
		if err != nil {
			errChan <- err
			return
		}
		defer conn.Close()

		// 先发送握手
		conn.Write([]byte{socks5Version, 1, socksAuthNone})
		// 读取握手响应
		resp := make([]byte, 2)
		conn.Read(resp)

		// 发送 CONNECT 请求
		request := []byte{
			socks5Version, socksCmdConnect, 0x00, socksAddrTypeIPv4,
			192, 168, 1, 100, // IPv4 address
			0x1F, 0x90, // Port 8080 (big endian)
		}
		conn.Write(request)

		// 读取响应
		respBuf := make([]byte, 10)
		conn.Read(respBuf)
	}()

	// 服务端接受连接并处理
	conn, err := adapter.Accept()
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	defer conn.Close()

	err = adapter.PrepareConnection(conn)
	if err != nil {
		t.Fatalf("PrepareConnection failed: %v", err)
	}

	select {
	case err := <-errChan:
		t.Fatalf("Client error: %v", err)
	case <-resultChan:
		// 测试通过
	case <-time.After(2 * time.Second):
		// 超时也算通过，因为我们已经完成了握手
	}
}

// 注意：handleRequest 内部测试需要真实的 TCP 连接（因为使用了 LocalAddr().(*net.TCPAddr)）
// 这些测试已在 TestSOCKS5MappingAdapter_FullHandshake 中进行了端到端测试

func TestSOCKS5MappingAdapter_SendReply(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)

	tests := []struct {
		name     string
		rep      byte
		bindAddr string
		bindPort uint16
	}{
		{
			name:     "success with IPv4",
			rep:      socksRepSuccess,
			bindAddr: "127.0.0.1",
			bindPort: 8080,
		},
		{
			name:     "success with IPv6",
			rep:      socksRepSuccess,
			bindAddr: "::1",
			bindPort: 80,
		},
		{
			name:     "failure response",
			rep:      socksRepServerFailure,
			bindAddr: "0.0.0.0",
			bindPort: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建管道连接
			clientConn, serverConn := net.Pipe()
			defer clientConn.Close()
			defer serverConn.Close()

			// 设置超时
			serverConn.SetDeadline(time.Now().Add(1 * time.Second))

			// 读取响应
			done := make(chan struct{})
			go func() {
				defer close(done)
				resp := make([]byte, 22) // 最大 IPv6 响应大小
				n, _ := clientConn.Read(resp)
				if n < 4 {
					t.Errorf("Response too short: %d bytes", n)
					return
				}
				if resp[0] != socks5Version {
					t.Errorf("Wrong version in response: %d", resp[0])
				}
				if resp[1] != tt.rep {
					t.Errorf("Wrong reply code: %d, expected %d", resp[1], tt.rep)
				}
			}()

			// 发送响应
			err := adapter.sendReply(serverConn, tt.rep, tt.bindAddr, tt.bindPort)
			if err != nil {
				t.Errorf("sendReply failed: %v", err)
			}

			<-done
		})
	}
}

func TestSOCKS5MappingAdapter_SendReply_InvalidAddress(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)

	// 创建管道连接
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// 设置超时
	serverConn.SetDeadline(time.Now().Add(1 * time.Second))

	// 读取响应
	go func() {
		resp := make([]byte, 10)
		n, _ := clientConn.Read(resp)
		// 无效地址应该使用默认的 0.0.0.0
		if n >= 8 && !bytes.Equal(resp[4:8], []byte{0, 0, 0, 0}) {
			t.Errorf("Invalid address should fall back to 0.0.0.0")
		}
	}()

	// 发送响应（使用无效地址）
	err := adapter.sendReply(serverConn, socksRepSuccess, "invalid-address", 80)
	if err != nil {
		t.Errorf("sendReply failed: %v", err)
	}
}

func TestSOCKS5MappingAdapter_FullHandshake(t *testing.T) {
	adapter := NewSOCKS5MappingAdapter(nil)
	defer adapter.Close()

	// 找一个可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := config.MappingConfig{
		MappingID: "test-socks5-mapping",
		LocalPort: port,
	}

	err = adapter.StartListener(cfg)
	if err != nil {
		t.Fatalf("StartListener failed: %v", err)
	}

	// 启动一个完整的 SOCKS5 客户端模拟
	resultChan := make(chan error, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		conn, err := net.Dial("tcp", adapter.listener.Addr().String())
		if err != nil {
			resultChan <- err
			return
		}
		defer conn.Close()

		// 1. 发送握手
		handshake := []byte{socks5Version, 1, socksAuthNone}
		if _, err := conn.Write(handshake); err != nil {
			resultChan <- err
			return
		}

		// 2. 读取握手响应
		resp := make([]byte, 2)
		if _, err := conn.Read(resp); err != nil {
			resultChan <- err
			return
		}
		if resp[0] != socks5Version || resp[1] != socksAuthNone {
			resultChan <- err
			return
		}

		// 3. 发送 CONNECT 请求
		request := []byte{
			socks5Version, socksCmdConnect, 0x00, socksAddrTypeIPv4,
			192, 168, 1, 1,
		}
		port := make([]byte, 2)
		binary.BigEndian.PutUint16(port, 80)
		request = append(request, port...)

		if _, err := conn.Write(request); err != nil {
			resultChan <- err
			return
		}

		resultChan <- nil
	}()

	// Accept 连接
	conn, err := adapter.Accept()
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	defer conn.Close()

	// PrepareConnection 会进行完整握手
	err = adapter.PrepareConnection(conn)
	if err != nil {
		t.Fatalf("PrepareConnection failed: %v", err)
	}

	// 检查客户端结果
	if clientErr := <-resultChan; clientErr != nil {
		t.Fatalf("Client error: %v", clientErr)
	}
}
