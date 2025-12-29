package mapping

import (
	"net"
	"testing"
	"time"

	"tunnox-core/internal/config"
)

func TestNewTCPMappingAdapter(t *testing.T) {
	adapter := NewTCPMappingAdapter()
	if adapter == nil {
		t.Fatal("NewTCPMappingAdapter returned nil")
	}
	if adapter.listener != nil {
		t.Error("listener should be nil before StartListener")
	}
}

func TestTCPMappingAdapter_GetProtocol(t *testing.T) {
	adapter := NewTCPMappingAdapter()
	if adapter.GetProtocol() != "tcp" {
		t.Errorf("Expected protocol 'tcp', got '%s'", adapter.GetProtocol())
	}
}

func TestTCPMappingAdapter_StartListener(t *testing.T) {
	adapter := NewTCPMappingAdapter()
	defer adapter.Close()

	cfg := config.MappingConfig{
		MappingID: "test-mapping",
		LocalPort: 0, // 使用随机端口
	}

	// 找一个可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg.LocalPort = port

	err = adapter.StartListener(cfg)
	if err != nil {
		t.Fatalf("StartListener failed: %v", err)
	}

	if adapter.listener == nil {
		t.Error("listener should not be nil after StartListener")
	}
}

func TestTCPMappingAdapter_Accept(t *testing.T) {
	adapter := NewTCPMappingAdapter()
	defer adapter.Close()

	// 找一个可用端口
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	cfg := config.MappingConfig{
		MappingID: "test-mapping",
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
		conn.Write([]byte("hello"))
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

	// 读取数据
	buf := make([]byte, 5)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(buf[:n]))
	}
}

func TestTCPMappingAdapter_AcceptWithoutListener(t *testing.T) {
	adapter := NewTCPMappingAdapter()

	_, err := adapter.Accept()
	if err == nil {
		t.Error("Accept should return error when listener is nil")
	}
}

func TestTCPMappingAdapter_PrepareConnection(t *testing.T) {
	adapter := NewTCPMappingAdapter()

	// TCP 不需要预处理，应该返回 nil
	err := adapter.PrepareConnection(nil)
	if err != nil {
		t.Errorf("PrepareConnection should return nil, got %v", err)
	}
}

func TestTCPMappingAdapter_Close(t *testing.T) {
	adapter := NewTCPMappingAdapter()

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
		MappingID: "test-mapping",
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
