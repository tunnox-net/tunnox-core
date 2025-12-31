package adapter

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// TestTcpAdapterBasic 测试 TCP 适配器基础功能
func TestTcpAdapterBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)

	// 测试名称
	if adapter.Name() != "tcp" {
		t.Errorf("Expected name 'tcp', got '%s'", adapter.Name())
	}

	// 测试连接类型
	if adapter.getConnectionType() != "TCP" {
		t.Errorf("Expected connection type 'TCP', got '%s'", adapter.getConnectionType())
	}

	adapter.Close()
}

// TestTcpAdapterAddress 测试地址设置
func TestTcpAdapterAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)

	testAddr := "localhost:8080"
	adapter.SetAddr(testAddr)

	if adapter.Addr() != testAddr {
		t.Errorf("Expected address '%s', got '%s'", testAddr, adapter.Addr())
	}

	if adapter.GetAddr() != testAddr {
		t.Errorf("Expected GetAddr '%s', got '%s'", testAddr, adapter.GetAddr())
	}

	adapter.Close()
}

// TestTcpAdapterListen 测试 TCP 监听
func TestTcpAdapterListen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)

	// 监听随机端口
	testAddr := "127.0.0.1:0"
	err := adapter.Listen(testAddr)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	// 验证 listener 已创建
	if adapter.listener == nil {
		t.Error("Expected listener to be created")
	}

	adapter.Close()
}

// TestTcpAdapterListenInvalidAddr 测试无效地址监听
func TestTcpAdapterListenInvalidAddr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)

	// 使用无效地址
	err := adapter.Listen("invalid:address:format")
	if err == nil {
		t.Error("Expected error for invalid address")
	}

	adapter.Close()
}

// TestTcpAdapterAcceptNoListener 测试无监听器时的 Accept
func TestTcpAdapterAcceptNoListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)

	// 不调用 Listen，直接 Accept
	_, err := adapter.Accept()
	if err == nil {
		t.Error("Expected error when accepting without listener")
	}

	adapter.Close()
}

// TestTcpAdapterDialAndListen 测试完整的拨号和监听流程
func TestTcpAdapterDialAndListen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务端适配器
	serverAdapter := NewTcpAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Server listen failed: %v", err)
	}

	// 获取实际监听地址
	serverAddr := serverAdapter.listener.Addr().String()

	// 启动 accept 协程
	var serverConn io.ReadWriteCloser
	var acceptErr error
	acceptDone := make(chan struct{})
	go func() {
		serverConn, acceptErr = serverAdapter.Accept()
		close(acceptDone)
	}()

	// 创建客户端适配器并拨号
	clientAdapter := NewTcpAdapter(ctx, nil)
	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Client dial failed: %v", err)
	}

	// 等待服务端 accept
	select {
	case <-acceptDone:
		if acceptErr != nil {
			t.Fatalf("Server accept failed: %v", acceptErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Accept timed out")
	}

	// 测试数据传输
	testData := []byte("Hello TCP")
	_, err = clientConn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	buf := make([]byte, len(testData))
	_, err = io.ReadFull(serverConn, buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(buf) != string(testData) {
		t.Errorf("Data mismatch: expected '%s', got '%s'", testData, buf)
	}

	// 清理
	clientConn.Close()
	serverConn.Close()
	clientAdapter.Close()
	serverAdapter.Close()
}

// TestTcpAdapterDialInvalidAddr 测试拨号无效地址
func TestTcpAdapterDialInvalidAddr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)

	// 拨号一个不存在的地址
	_, err := adapter.Dial("127.0.0.1:1")
	if err == nil {
		t.Error("Expected error for invalid dial address")
	}

	adapter.Close()
}

// TestTcpAdapterClose 测试关闭逻辑
func TestTcpAdapterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)

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

	// 验证 listener 已清理
	if adapter.listener != nil {
		t.Error("Expected listener to be nil after close")
	}
}

// TestTcpAdapterMultipleClose 测试多次关闭
func TestTcpAdapterMultipleClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)

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

// TestTcpConnWrapper 测试 TcpConn 包装器
func TestTcpConnWrapper(t *testing.T) {
	// 创建一个临时监听器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	// 启动 accept 协程
	var serverConn net.Conn
	acceptDone := make(chan struct{})
	go func() {
		serverConn, _ = listener.Accept()
		close(acceptDone)
	}()

	// 拨号连接
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// 包装连接
	tcpConn := &TcpConn{Conn: conn}

	// 等待 accept
	<-acceptDone
	defer serverConn.Close()

	// 测试 Close
	err = tcpConn.Close()
	if err != nil {
		t.Errorf("TcpConn.Close() error: %v", err)
	}
}

// TestTcpAdapterConcurrentAccept 测试并发 Accept
func TestTcpAdapterConcurrentAccept(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)
	err := adapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	serverAddr := adapter.listener.Addr().String()

	// 启动多个 accept 协程
	const numConns = 5
	var wg sync.WaitGroup
	acceptedConns := make(chan io.ReadWriteCloser, numConns)

	for i := 0; i < numConns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := adapter.Accept()
			if err == nil {
				acceptedConns <- conn
			}
		}()
	}

	// 创建客户端连接
	time.Sleep(50 * time.Millisecond) // 等待 accept 协程启动

	var clientConns []net.Conn
	for i := 0; i < numConns; i++ {
		conn, err := net.Dial("tcp", serverAddr)
		if err != nil {
			t.Logf("Client dial %d failed: %v", i, err)
			continue
		}
		clientConns = append(clientConns, conn)
	}

	// 等待所有连接处理完成
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Log("Accept goroutines did not finish in time")
	}

	// 关闭所有连接
	close(acceptedConns)
	for conn := range acceptedConns {
		conn.Close()
	}
	for _, conn := range clientConns {
		conn.Close()
	}

	adapter.Close()
}

// TestTcpAdapterListenFrom 测试 ListenFrom（通过 BaseAdapter）
func TestTcpAdapterListenFrom(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewTcpAdapter(ctx, nil)

	// 使用 ListenFrom（BaseAdapter 方法）
	err := adapter.ListenFrom("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenFrom failed: %v", err)
	}

	// 验证地址已设置
	if adapter.Addr() == "" {
		t.Error("Expected address to be set")
	}

	// 等待 accept 循环启动
	time.Sleep(50 * time.Millisecond)

	adapter.Close()
}

// TestTcpAdapterConnectTo 测试 ConnectTo（通过 BaseAdapter）
func TestTcpAdapterConnectTo(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 先创建一个服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	serverAddr := listener.Addr().String()

	// 启动 accept 协程
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	// 使用 ConnectTo（BaseAdapter 方法）
	adapter := NewTcpAdapter(ctx, nil)
	err = adapter.ConnectTo(serverAddr)
	if err != nil {
		t.Fatalf("ConnectTo failed: %v", err)
	}

	// 验证 reader/writer 可用
	if adapter.GetReader() == nil {
		t.Error("Expected non-nil reader after ConnectTo")
	}
	if adapter.GetWriter() == nil {
		t.Error("Expected non-nil writer after ConnectTo")
	}

	adapter.Close()
}

// TestTcpAdapterTCPOptions 测试 TCP 选项设置
func TestTcpAdapterTCPOptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务端
	serverAdapter := NewTcpAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	serverAddr := serverAdapter.listener.Addr().String()

	// 启动 accept 协程
	acceptDone := make(chan io.ReadWriteCloser)
	go func() {
		conn, _ := serverAdapter.Accept()
		acceptDone <- conn
	}()

	// 客户端拨号
	clientAdapter := NewTcpAdapter(ctx, nil)
	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// 等待 accept
	serverConn := <-acceptDone

	// TCP 选项已在 Dial/Accept 中设置
	// 这里主要验证连接可以正常使用
	testData := []byte("TCP options test")
	_, err = clientConn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	buf := make([]byte, len(testData))
	_, err = io.ReadFull(serverConn, buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(buf) != string(testData) {
		t.Errorf("Data mismatch")
	}

	clientConn.Close()
	serverConn.Close()
	clientAdapter.Close()
	serverAdapter.Close()
}
