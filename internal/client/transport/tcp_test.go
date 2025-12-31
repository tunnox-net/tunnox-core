package transport

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDialTCP_Success(t *testing.T) {
	// 启动测试 TCP 服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	address := listener.Addr().String()

	// 接受连接的 goroutine
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			// 保持连接一段时间
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// 使用 DialTCP 连接
	ctx := context.Background()
	conn, err := DialTCP(ctx, address)
	if err != nil {
		t.Fatalf("DialTCP failed: %v", err)
	}
	defer conn.Close()

	if conn == nil {
		t.Error("conn should not be nil")
	}

	// 验证是 TCP 连接
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Error("conn should be *net.TCPConn")
	}

	// 验证连接地址
	if tcpConn.RemoteAddr().String() != address {
		t.Errorf("Remote address mismatch: got %s, want %s", tcpConn.RemoteAddr().String(), address)
	}
}

func TestDialTCP_ConnectionRefused(t *testing.T) {
	// 使用一个没有监听的端口
	ctx := context.Background()

	// 找一个未使用的端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	address := listener.Addr().String()
	listener.Close() // 立即关闭，确保端口未被监听

	_, err = DialTCP(ctx, address)
	if err == nil {
		t.Error("DialTCP should return error for connection refused")
	}
}

func TestDialTCP_InvalidAddress(t *testing.T) {
	ctx := context.Background()

	_, err := DialTCP(ctx, "invalid-address")
	if err == nil {
		t.Error("DialTCP should return error for invalid address")
	}
}

func TestDialTCP_ContextCanceled(t *testing.T) {
	// 创建一个已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 尝试连接一个不存在的地址（使用取消的 context）
	_, err := DialTCP(ctx, "192.0.2.1:1234") // 测试用地址
	if err == nil {
		t.Error("DialTCP should return error for canceled context")
	}
}

func TestDialTCP_ContextTimeout(t *testing.T) {
	// 创建一个立即超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// 等待超时
	time.Sleep(10 * time.Millisecond)

	// 尝试连接一个不存在的地址
	_, err := DialTCP(ctx, "192.0.2.1:1234") // 测试用地址
	if err == nil {
		t.Error("DialTCP should return error for timed out context")
	}
}

func TestDialTCP_ReadWrite(t *testing.T) {
	// 启动测试 TCP 服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	address := listener.Addr().String()

	// 服务器端处理
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// 读取数据
		buf := make([]byte, 100)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		// 回显数据
		conn.Write(buf[:n])
	}()

	// 客户端连接
	ctx := context.Background()
	conn, err := DialTCP(ctx, address)
	if err != nil {
		t.Fatalf("DialTCP failed: %v", err)
	}
	defer conn.Close()

	// 发送数据
	testData := []byte("hello")
	n, err := conn.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write returned wrong length: got %d, want %d", n, len(testData))
	}

	// 读取响应
	buf := make([]byte, 100)
	n, err = conn.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("Read returned wrong data: got %q, want %q", string(buf[:n]), "hello")
	}

	<-serverDone
}

func TestDialTCP_KeepAlive(t *testing.T) {
	// 启动测试 TCP 服务器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	address := listener.Addr().String()

	// 接受连接
	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			defer conn.Close()
			time.Sleep(100 * time.Millisecond)
		}
	}()

	// 连接
	ctx := context.Background()
	conn, err := DialTCP(ctx, address)
	if err != nil {
		t.Fatalf("DialTCP failed: %v", err)
	}
	defer conn.Close()

	// 验证是 TCP 连接，KeepAlive 应该已设置
	// 由于 Go 的 net.TCPConn 没有公开的方法来检查 KeepAlive 状态，
	// 我们只验证连接类型正确
	if _, ok := conn.(*net.TCPConn); !ok {
		t.Error("conn should be *net.TCPConn")
	}
}

func TestDialTCP_RegistrationPriority(t *testing.T) {
	// 验证 TCP 协议的优先级
	info, ok := GetProtocol("tcp")
	if !ok {
		t.Fatal("TCP protocol should be registered")
	}

	if info.Priority != 30 {
		t.Errorf("TCP priority should be 30, got %d", info.Priority)
	}
}
