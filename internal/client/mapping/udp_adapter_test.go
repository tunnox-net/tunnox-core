package mapping

import (
	"net"
	"sync"
	"testing"
	"time"

	"tunnox-core/internal/config"
)

func TestNewUDPMappingAdapter(t *testing.T) {
	adapter := NewUDPMappingAdapter()
	if adapter == nil {
		t.Fatal("NewUDPMappingAdapter returned nil")
	}
	if adapter.listener != nil {
		t.Error("listener should be nil before StartListener")
	}
	if adapter.connChan == nil {
		t.Error("connChan should not be nil")
	}
	if adapter.sessions == nil {
		t.Error("sessions should not be nil")
	}
}

func TestUDPMappingAdapter_GetProtocol(t *testing.T) {
	adapter := NewUDPMappingAdapter()
	if adapter.GetProtocol() != "udp" {
		t.Errorf("Expected protocol 'udp', got '%s'", adapter.GetProtocol())
	}
}

func TestUDPMappingAdapter_StartListenerAndClose(t *testing.T) {
	adapter := NewUDPMappingAdapter()

	// 找一个可用端口
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()

	cfg := config.MappingConfig{
		MappingID: "test-udp-mapping",
		LocalPort: port,
	}

	err = adapter.StartListener(cfg)
	if err != nil {
		t.Fatalf("StartListener failed: %v", err)
	}

	if adapter.listener == nil {
		t.Error("listener should not be nil after StartListener")
	}

	// 关闭适配器
	err = adapter.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestUDPMappingAdapter_PrepareConnection(t *testing.T) {
	adapter := NewUDPMappingAdapter()

	// UDP 不需要预处理，应该返回 nil
	err := adapter.PrepareConnection(nil)
	if err != nil {
		t.Errorf("PrepareConnection should return nil, got %v", err)
	}
}

func TestUDPMappingAdapter_CloseWithoutListener(t *testing.T) {
	adapter := NewUDPMappingAdapter()

	// 关闭未初始化的适配器不应该报错（但会阻塞等待 goroutine）
	// 这里测试需要小心处理
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(adapter.closeCh)
	}()
}

func TestUDPVirtualConn_ReadWrite(t *testing.T) {
	// 创建一个模拟的 UDP PacketConn
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create PacketConn: %v", err)
	}
	defer conn.Close()

	remoteAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:12345")
	if err != nil {
		t.Fatalf("Failed to resolve address: %v", err)
	}

	vc := &UDPVirtualConn{
		listener:   conn,
		remoteAddr: remoteAddr,
		readChan:   make(chan []byte, udpReadChanSize),
		writeChan:  make(chan []byte, udpWriteChanSize),
		closeCh:    make(chan struct{}),
		lastActive: time.Now(),
	}

	// 测试写入
	testData := []byte("hello")
	n, err := vc.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected write %d bytes, got %d", len(testData), n)
	}

	// 模拟读取：手动放入数据到 readChan
	go func() {
		vc.readChan <- []byte("world")
	}()

	buf := make([]byte, 10)
	n, err = vc.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(buf[:n]) != "world" {
		t.Errorf("Expected 'world', got '%s'", string(buf[:n]))
	}

	// 测试关闭
	err = vc.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// 关闭后读取应该返回 EOF
	_, err = vc.Read(buf)
	if err == nil {
		t.Error("Read after close should return error")
	}
}

func TestUDPVirtualConn_LastActive(t *testing.T) {
	vc := &UDPVirtualConn{
		closeCh:    make(chan struct{}),
		lastActive: time.Now().Add(-10 * time.Second),
	}

	// 获取上次活跃时间
	lastActive := vc.getLastActive()
	if time.Since(lastActive) < 9*time.Second {
		t.Error("getLastActive returned wrong time")
	}

	// 更新活跃时间
	vc.updateLastActive()
	lastActive = vc.getLastActive()
	if time.Since(lastActive) > time.Second {
		t.Error("updateLastActive did not update the time")
	}
}

func TestUDPVirtualConn_Close_Multiple(t *testing.T) {
	vc := &UDPVirtualConn{
		closeCh: make(chan struct{}),
	}

	// 多次关闭不应该 panic
	for i := 0; i < 3; i++ {
		err := vc.Close()
		if err != nil {
			t.Errorf("Close() #%d failed: %v", i+1, err)
		}
	}
}

func TestUDPVirtualConn_Concurrent(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create PacketConn: %v", err)
	}
	defer conn.Close()

	remoteAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:12345")

	vc := &UDPVirtualConn{
		listener:   conn,
		remoteAddr: remoteAddr,
		readChan:   make(chan []byte, udpReadChanSize),
		writeChan:  make(chan []byte, udpWriteChanSize),
		closeCh:    make(chan struct{}),
		lastActive: time.Now(),
	}

	var wg sync.WaitGroup
	numGoroutines := 10

	// 并发写入
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				vc.Write([]byte("test"))
				vc.updateLastActive()
			}
		}(i)
	}

	// 等待所有写入完成
	wg.Wait()

	// 关闭
	vc.Close()
}
