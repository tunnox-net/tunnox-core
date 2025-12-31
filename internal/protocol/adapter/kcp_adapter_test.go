package adapter

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

// TestKcpAdapterBasic 测试 KCP 适配器基础功能
func TestKcpAdapterBasic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewKcpAdapter(ctx, nil)

	// 测试名称
	if adapter.Name() != "kcp" {
		t.Errorf("Expected name 'kcp', got '%s'", adapter.Name())
	}

	// 测试连接类型
	if adapter.getConnectionType() != "KCP" {
		t.Errorf("Expected connection type 'KCP', got '%s'", adapter.getConnectionType())
	}

	adapter.Close()
}

// TestKcpAdapterAddress 测试地址设置
func TestKcpAdapterAddress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewKcpAdapter(ctx, nil)

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

// TestKcpAdapterListen 测试 KCP 监听
func TestKcpAdapterListen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewKcpAdapter(ctx, nil)

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

// TestKcpAdapterListenInvalidAddr 测试无效地址监听
func TestKcpAdapterListenInvalidAddr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewKcpAdapter(ctx, nil)

	// 使用无效地址
	err := adapter.Listen("invalid:address:format")
	if err == nil {
		t.Error("Expected error for invalid address")
	}

	adapter.Close()
}

// TestKcpAdapterAcceptNoListener 测试无监听器时的 Accept
func TestKcpAdapterAcceptNoListener(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewKcpAdapter(ctx, nil)

	// 不调用 Listen，直接 Accept
	_, err := adapter.Accept()
	if err == nil {
		t.Error("Expected error when accepting without listener")
	}

	adapter.Close()
}

// TestKcpAdapterDialAndListen 测试完整的拨号和监听流程
func TestKcpAdapterDialAndListen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务端适配器
	serverAdapter := NewKcpAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Server listen failed: %v", err)
	}

	// 获取实际监听地址
	serverAddr := serverAdapter.listener.Addr().String()
	t.Logf("KCP server listening on: %s", serverAddr)

	// 启动 accept 协程
	var serverConn io.ReadWriteCloser
	var acceptErr error
	acceptDone := make(chan struct{})
	go func() {
		serverConn, acceptErr = serverAdapter.Accept()
		close(acceptDone)
	}()

	// 等待服务器准备好
	time.Sleep(100 * time.Millisecond)

	// 创建客户端适配器并拨号
	clientAdapter := NewKcpAdapter(ctx, nil)
	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Client dial failed: %v", err)
	}

	// KCP 需要发送数据来触发 Accept
	// 先发送一些数据以建立连接
	testData := []byte("Hello KCP")
	_, err = clientConn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 等待服务端 accept
	select {
	case <-acceptDone:
		if acceptErr != nil {
			t.Fatalf("Server accept failed: %v", acceptErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Accept timed out")
	}

	buf := make([]byte, len(testData))
	// KCP 需要一些时间来传输数据
	serverConn.(*kcpConn).conn.SetReadDeadline(time.Now().Add(5 * time.Second))
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

// TestKcpAdapterDialInvalidAddr 测试拨号无效地址
func TestKcpAdapterDialInvalidAddr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewKcpAdapter(ctx, nil)

	// KCP 拨号到无效地址（由于 UDP 特性，可能不会立即失败）
	// 但格式错误的地址应该会失败
	_, err := adapter.Dial("invalid:address:format")
	if err == nil {
		t.Error("Expected error for invalid dial address")
	}

	adapter.Close()
}

// TestKcpAdapterClose 测试关闭逻辑
func TestKcpAdapterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewKcpAdapter(ctx, nil)

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

// TestKcpAdapterMultipleClose 测试多次关闭
func TestKcpAdapterMultipleClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewKcpAdapter(ctx, nil)

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

// TestKcpConnWrapper 测试 kcpConn 包装器方法
func TestKcpConnWrapper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务器
	serverAdapter := NewKcpAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	serverAddr := serverAdapter.listener.Addr().String()

	// 启动 accept 协程
	acceptDone := make(chan io.ReadWriteCloser, 1)
	go func() {
		conn, _ := serverAdapter.Accept()
		acceptDone <- conn
	}()

	time.Sleep(50 * time.Millisecond)

	// 客户端拨号
	clientAdapter := NewKcpAdapter(ctx, nil)
	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// KCP 需要发送数据来触发 Accept
	_, err = clientConn.Write([]byte("trigger"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 等待 accept
	select {
	case serverConn := <-acceptDone:
		// 测试 kcpConn 方法
		kConn := clientConn.(*kcpConn)

		// 测试 LocalAddr
		if kConn.LocalAddr() == nil {
			t.Error("Expected non-nil LocalAddr")
		}

		// 测试 RemoteAddr
		if kConn.RemoteAddr() == nil {
			t.Error("Expected non-nil RemoteAddr")
		}

		// 测试 SetDeadline
		err = kConn.SetDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Errorf("SetDeadline error: %v", err)
		}

		// 测试 SetReadDeadline
		err = kConn.SetReadDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Errorf("SetReadDeadline error: %v", err)
		}

		// 测试 SetWriteDeadline
		err = kConn.SetWriteDeadline(time.Now().Add(time.Second))
		if err != nil {
			t.Errorf("SetWriteDeadline error: %v", err)
		}

		clientConn.Close()
		serverConn.Close()
	case <-time.After(10 * time.Second):
		t.Fatal("Accept timed out")
	}

	clientAdapter.Close()
	serverAdapter.Close()
}

// TestKcpAdapterListenFrom 测试 ListenFrom（通过 BaseAdapter）
func TestKcpAdapterListenFrom(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := NewKcpAdapter(ctx, nil)

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

// TestKcpAdapterConcurrentConnections 测试并发连接
func TestKcpAdapterConcurrentConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务器
	serverAdapter := NewKcpAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	serverAddr := serverAdapter.listener.Addr().String()

	const numConns = 3
	var wg sync.WaitGroup

	// 启动 accept 协程
	serverConns := make(chan io.ReadWriteCloser, numConns)
	go func() {
		for i := 0; i < numConns; i++ {
			conn, err := serverAdapter.Accept()
			if err == nil {
				serverConns <- conn
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)

	// 创建多个客户端连接
	clientConns := make([]io.ReadWriteCloser, 0, numConns)
	var connMu sync.Mutex

	for i := 0; i < numConns; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			clientAdapter := NewKcpAdapter(ctx, nil)
			conn, err := clientAdapter.Dial(serverAddr)
			if err != nil {
				t.Logf("Client %d dial failed: %v", idx, err)
				return
			}
			connMu.Lock()
			clientConns = append(clientConns, conn)
			connMu.Unlock()
		}(i)
	}

	wg.Wait()

	// 等待服务器接受连接
	time.Sleep(500 * time.Millisecond)

	// 清理
	close(serverConns)
	for conn := range serverConns {
		conn.Close()
	}

	connMu.Lock()
	for _, conn := range clientConns {
		conn.Close()
	}
	connMu.Unlock()

	serverAdapter.Close()
}

// TestKcpAdapterConstants 测试 KCP 配置常量
func TestKcpAdapterConstants(t *testing.T) {
	// 验证常量值符合预期
	if KcpDataShards != 0 {
		t.Errorf("Expected KcpDataShards=0, got %d", KcpDataShards)
	}
	if KcpParityShards != 0 {
		t.Errorf("Expected KcpParityShards=0, got %d", KcpParityShards)
	}
	if KcpSndWnd != 1024 {
		t.Errorf("Expected KcpSndWnd=1024, got %d", KcpSndWnd)
	}
	if KcpRcvWnd != 1024 {
		t.Errorf("Expected KcpRcvWnd=1024, got %d", KcpRcvWnd)
	}
	if KcpNoDelay != 1 {
		t.Errorf("Expected KcpNoDelay=1, got %d", KcpNoDelay)
	}
	if KcpInterval != 10 {
		t.Errorf("Expected KcpInterval=10, got %d", KcpInterval)
	}
	if KcpResend != 2 {
		t.Errorf("Expected KcpResend=2, got %d", KcpResend)
	}
	if KcpNC != 1 {
		t.Errorf("Expected KcpNC=1, got %d", KcpNC)
	}
	if KcpMTU != 1400 {
		t.Errorf("Expected KcpMTU=1400, got %d", KcpMTU)
	}
	if KcpStreamBufferSize != 4*1024*1024 {
		t.Errorf("Expected KcpStreamBufferSize=4MB, got %d", KcpStreamBufferSize)
	}
}

// TestKcpAdapterBidirectionalData 测试双向数据传输
func TestKcpAdapterBidirectionalData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建服务器
	serverAdapter := NewKcpAdapter(ctx, nil)
	err := serverAdapter.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	serverAddr := serverAdapter.listener.Addr().String()

	// 启动 accept 协程
	acceptDone := make(chan io.ReadWriteCloser, 1)
	go func() {
		conn, _ := serverAdapter.Accept()
		acceptDone <- conn
	}()

	time.Sleep(50 * time.Millisecond)

	// 客户端拨号
	clientAdapter := NewKcpAdapter(ctx, nil)
	clientConn, err := clientAdapter.Dial(serverAddr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	// 测试客户端到服务器（KCP 需要发送数据来触发 Accept）
	clientData := []byte("Client to Server")
	_, err = clientConn.Write(clientData)
	if err != nil {
		t.Fatalf("Client write failed: %v", err)
	}

	// 等待 accept
	select {
	case serverConn := <-acceptDone:
		// 设置读超时
		clientConn.(*kcpConn).conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		serverConn.(*kcpConn).conn.SetReadDeadline(time.Now().Add(5 * time.Second))

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

		// 清理
		clientConn.Close()
		serverConn.Close()
	case <-time.After(10 * time.Second):
		t.Fatal("Accept timed out")
	}

	clientAdapter.Close()
	serverAdapter.Close()
}
