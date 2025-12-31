package adapter

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestQuicAdapterBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 测试基本功能
	adapter := NewQuicAdapter(ctx, nil)

	// 测试名称
	if adapter.Name() != "quic" {
		t.Errorf("Expected name 'quic', got '%s'", adapter.Name())
	}

	// 测试地址设置
	testAddr := "localhost:8080"
	adapter.SetAddr(testAddr)

	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}

	// 测试启动服务器（这里可能会失败，因为需要TLS证书）
	// 在实际环境中，应该使用有效的TLS证书
	err := adapter.ListenFrom(testAddr)
	if err != nil {
		t.Logf("QUIC server start failed (expected in test environment): %v", err)
	} else {
		// 等待服务器启动
		time.Sleep(100 * time.Millisecond)

		// 测试停止服务器
		adapter.Close()
	}

	// 测试关闭
	adapter.Close()
}

func TestQuicAdapterName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewQuicAdapter(ctx, nil)

	if adapter.Name() != "quic" {
		t.Errorf("Expected name 'quic', got '%s'", adapter.Name())
	}
}

func TestQuicAdapterAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewQuicAdapter(ctx, nil)
	testAddr := "localhost:8080"

	adapter.ListenFrom(testAddr)

	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}
}

// TestQuicAdapterConnectionType 测试连接类型
func TestQuicAdapterConnectionType(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewQuicAdapter(ctx, nil)
	defer adapter.Close()

	if adapter.getConnectionType() != "QUIC" {
		t.Errorf("Expected connection type 'QUIC', got '%s'", adapter.getConnectionType())
	}
}

// TestQuicAdapterListen 测试 QUIC 监听
func TestQuicAdapterListen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewQuicAdapter(ctx, nil)

	// 验证 TLS 配置已初始化
	if adapter.tlsConfig == nil {
		t.Fatal("Expected TLS config to be initialized")
	}

	// 监听随机端口
	err := adapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	// 验证 listener 已创建
	if adapter.listener == nil {
		t.Error("Expected listener to be created")
	}

	adapter.Close()
}

// TestQuicAdapterListenInvalidAddr 测试无效地址监听
func TestQuicAdapterListenInvalidAddr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewQuicAdapter(ctx, nil)

	// 使用无效地址
	err := adapter.Listen("invalid:address:format")
	if err == nil {
		t.Error("Expected error for invalid address")
	}

	adapter.Close()
}

// TestQuicAdapterClose 测试关闭逻辑
func TestQuicAdapterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewQuicAdapter(ctx, nil)

	// 启动监听
	err := adapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	// 关闭适配器
	err = adapter.Close()
	if err != nil {
		t.Errorf("Close error: %v", err)
	}

	// 验证已标记关闭
	if !adapter.closed {
		t.Error("Expected adapter to be marked as closed")
	}
}

// TestQuicAdapterMultipleClose 测试多次关闭
func TestQuicAdapterMultipleClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewQuicAdapter(ctx, nil)

	// 第一次关闭
	err := adapter.Close()
	if err != nil {
		t.Errorf("First close error: %v", err)
	}

	// 第二次关闭应该不报错（幂等）
	err = adapter.Close()
	if err != nil {
		t.Errorf("Second close error: %v", err)
	}
}

// TestQuicAdapterDialAndListen 测试完整的拨号和监听流程
func TestQuicAdapterDialAndListen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务端适配器
	serverAdapter := NewQuicAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Server listen failed: %v", err)
	}
	defer serverAdapter.Close()

	// 获取实际监听地址
	serverAddr := serverAdapter.listener.Addr().String()
	t.Logf("QUIC server listening on: %s", serverAddr)

	// 等待服务器准备好
	time.Sleep(200 * time.Millisecond)

	// 创建客户端适配器并拨号
	clientAdapter := NewQuicAdapter(ctx, nil)
	defer clientAdapter.Close()

	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Client dial failed: %v", err)
	}
	defer clientConn.Close()

	// 先写数据（这会触发服务端的 stream accept）
	testData := []byte("Hello QUIC")
	_, err = clientConn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 等待服务端 accept
	var serverConn io.ReadWriteCloser
	var acceptErr error
	acceptDone := make(chan struct{})
	go func() {
		serverConn, acceptErr = serverAdapter.Accept()
		close(acceptDone)
	}()

	select {
	case <-acceptDone:
		if acceptErr != nil {
			t.Fatalf("Server accept failed: %v", acceptErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Accept timed out")
	}
	defer serverConn.Close()

	buf := make([]byte, len(testData))
	serverConn.(*QuicStreamConn).SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = io.ReadFull(serverConn, buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(buf) != string(testData) {
		t.Errorf("Data mismatch: expected '%s', got '%s'", testData, buf)
	}
}

// TestQuicAdapterDialInvalidAddr 测试拨号无效地址
func TestQuicAdapterDialInvalidAddr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	adapter := NewQuicAdapter(ctx, nil)

	// 拨号一个不存在的地址（应该超时或失败）
	_, err := adapter.Dial("127.0.0.1:1")
	if err == nil {
		t.Error("Expected error for invalid dial address")
	}

	adapter.Close()
}

// TestQuicStreamConnMethods 测试 QuicStreamConn 方法
func TestQuicStreamConnMethods(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务器
	serverAdapter := NewQuicAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	serverAddr := serverAdapter.listener.Addr().String()

	time.Sleep(100 * time.Millisecond)

	// 客户端拨号
	clientAdapter := NewQuicAdapter(ctx, nil)
	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// 测试 QuicStreamConn 方法
	qConn := clientConn.(*QuicStreamConn)

	// 测试 LocalAddr
	if qConn.LocalAddr() == nil {
		t.Error("Expected non-nil LocalAddr")
	}

	// 测试 RemoteAddr
	if qConn.RemoteAddr() == nil {
		t.Error("Expected non-nil RemoteAddr")
	}

	// 测试 SetDeadline
	err = qConn.SetDeadline(time.Now().Add(time.Second))
	if err != nil {
		t.Errorf("SetDeadline error: %v", err)
	}

	// 测试 SetReadDeadline
	err = qConn.SetReadDeadline(time.Now().Add(time.Second))
	if err != nil {
		t.Errorf("SetReadDeadline error: %v", err)
	}

	// 测试 SetWriteDeadline
	err = qConn.SetWriteDeadline(time.Now().Add(time.Second))
	if err != nil {
		t.Errorf("SetWriteDeadline error: %v", err)
	}

	clientConn.Close()
	clientAdapter.Close()
	serverAdapter.Close()
}

// TestQuicStreamConnClosedRead 测试关闭后读取
func TestQuicStreamConnClosedRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务器和客户端连接
	serverAdapter := NewQuicAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	serverAddr := serverAdapter.listener.Addr().String()

	time.Sleep(100 * time.Millisecond)

	clientAdapter := NewQuicAdapter(ctx, nil)
	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// 关闭连接
	clientConn.Close()

	// 尝试读取应该返回 EOF
	buf := make([]byte, 10)
	_, err = clientConn.Read(buf)
	if err != io.EOF {
		t.Errorf("Expected EOF after close, got: %v", err)
	}

	clientAdapter.Close()
	serverAdapter.Close()
}

// TestQuicStreamConnClosedWrite 测试关闭后写入
func TestQuicStreamConnClosedWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务器和客户端连接
	serverAdapter := NewQuicAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	serverAddr := serverAdapter.listener.Addr().String()

	time.Sleep(100 * time.Millisecond)

	clientAdapter := NewQuicAdapter(ctx, nil)
	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// 关闭连接
	clientConn.Close()

	// 尝试写入应该返回错误
	_, err = clientConn.Write([]byte("test"))
	if err != io.ErrClosedPipe {
		t.Errorf("Expected ErrClosedPipe after close, got: %v", err)
	}

	clientAdapter.Close()
	serverAdapter.Close()
}

// TestQuicAdapterAcceptClosed 测试关闭后 Accept
func TestQuicAdapterAcceptClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewQuicAdapter(ctx, nil)

	// 先关闭
	adapter.Close()

	// Accept 应该返回错误
	_, err := adapter.Accept()
	if err == nil {
		t.Error("Expected error when accepting on closed adapter")
	}
}

// TestGenerateTLSConfig 测试 TLS 配置生成
func TestGenerateTLSConfig(t *testing.T) {
	config := generateTLSConfig()

	if config == nil {
		t.Fatal("Expected non-nil TLS config")
	}

	if len(config.Certificates) == 0 {
		t.Error("Expected at least one certificate")
	}

	if len(config.NextProtos) == 0 || config.NextProtos[0] != "tunnox-quic" {
		t.Error("Expected 'tunnox-quic' in NextProtos")
	}
}

// TestQuicAdapterBidirectionalData 测试双向数据传输
func TestQuicAdapterBidirectionalData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务器
	serverAdapter := NewQuicAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer serverAdapter.Close()

	serverAddr := serverAdapter.listener.Addr().String()

	time.Sleep(200 * time.Millisecond)

	// 客户端拨号
	clientAdapter := NewQuicAdapter(ctx, nil)
	defer clientAdapter.Close()

	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	// 先发送数据以触发 stream accept
	clientData := []byte("Client to Server")
	_, err = clientConn.Write(clientData)
	if err != nil {
		t.Fatalf("Client write failed: %v", err)
	}

	// Accept 服务端连接
	acceptDone := make(chan io.ReadWriteCloser, 1)
	go func() {
		conn, _ := serverAdapter.Accept()
		acceptDone <- conn
	}()

	var serverConn io.ReadWriteCloser
	select {
	case serverConn = <-acceptDone:
	case <-time.After(10 * time.Second):
		t.Fatal("Accept timed out")
	}
	defer serverConn.Close()

	// 设置读超时
	clientConn.(*QuicStreamConn).SetReadDeadline(time.Now().Add(5 * time.Second))
	serverConn.(*QuicStreamConn).SetReadDeadline(time.Now().Add(5 * time.Second))

	buf := make([]byte, len(clientData))
	_, err = io.ReadFull(serverConn, buf)
	if err != nil {
		t.Fatalf("Server read failed: %v", err)
	}
	if string(buf) != string(clientData) {
		t.Errorf("Client->Server data mismatch")
	}

	// 测试服务器到客户端
	serverData := []byte("Server to Client")
	_, err = serverConn.Write(serverData)
	if err != nil {
		t.Fatalf("Server write failed: %v", err)
	}

	buf = make([]byte, len(serverData))
	_, err = io.ReadFull(clientConn, buf)
	if err != nil {
		t.Fatalf("Client read failed: %v", err)
	}
	if string(buf) != string(serverData) {
		t.Errorf("Server->Client data mismatch")
	}
}
