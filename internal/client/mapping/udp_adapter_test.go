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
	if len(adapter.listeners) != 0 {
		t.Error("listeners should be empty before StartListener")
	}
	if adapter.connChan == nil {
		t.Error("connChan should not be nil")
	}
	// sync.Map 不需要检查 nil
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

	if len(adapter.listeners) == 0 {
		t.Error("listeners should not be empty after StartListener")
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

	// 关闭未初始化的适配器不应该报错
	err := adapter.Close()
	if err != nil {
		t.Errorf("Close should not error: %v", err)
	}
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
		readChan:   make(chan *udpPacket, udpReadChanSize),
		writeChan:  make(chan []byte, udpWriteChanSize),
		closeCh:    make(chan struct{}),
	}
	vc.lastActive.Store(time.Now().UnixNano())

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
		buf := getBuffer()
		data := []byte("world")
		copy(buf, data)
		vc.readChan <- &udpPacket{data: buf[:len(data)], buffer: buf}
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
		closeCh: make(chan struct{}),
	}
	// 设置 10 秒前的时间
	vc.lastActive.Store(time.Now().Add(-10 * time.Second).UnixNano())

	// 获取上次活跃时间
	lastActiveNanos := vc.lastActive.Load()
	lastActive := time.Unix(0, lastActiveNanos)
	if time.Since(lastActive) < 9*time.Second {
		t.Error("lastActive returned wrong time")
	}

	// 更新活跃时间
	vc.updateLastActive()
	lastActiveNanos = vc.lastActive.Load()
	lastActive = time.Unix(0, lastActiveNanos)
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
		readChan:   make(chan *udpPacket, udpReadChanSize),
		writeChan:  make(chan []byte, udpWriteChanSize),
		closeCh:    make(chan struct{}),
	}
	vc.lastActive.Store(time.Now().UnixNano())

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

func TestUDPVirtualConn_SetReadDeadline(t *testing.T) {
	vc := &UDPVirtualConn{
		readChan: make(chan *udpPacket, 1),
		closeCh:  make(chan struct{}),
	}

	// 设置 deadline
	deadline := time.Now().Add(100 * time.Millisecond)
	err := vc.SetReadDeadline(deadline)
	if err != nil {
		t.Errorf("SetReadDeadline failed: %v", err)
	}

	// 验证 deadline 已设置
	storedDeadline := vc.readDeadline.Load()
	if storedDeadline != deadline.UnixNano() {
		t.Error("deadline not stored correctly")
	}

	// 清除 deadline
	err = vc.SetReadDeadline(time.Time{})
	if err != nil {
		t.Errorf("SetReadDeadline (clear) failed: %v", err)
	}
	if vc.readDeadline.Load() != 0 {
		t.Error("deadline should be cleared")
	}
}

func TestUDPVirtualConn_ReadWithTimeout(t *testing.T) {
	vc := &UDPVirtualConn{
		readChan: make(chan *udpPacket, 1),
		closeCh:  make(chan struct{}),
	}

	// 设置很短的 deadline
	vc.SetReadDeadline(time.Now().Add(50 * time.Millisecond))

	buf := make([]byte, 10)
	start := time.Now()
	_, err := vc.Read(buf)
	elapsed := time.Since(start)

	// 应该超时
	if err == nil {
		t.Error("Read should timeout")
	}

	// 验证是超时错误
	if netErr, ok := err.(net.Error); ok {
		if !netErr.Timeout() {
			t.Error("Error should be timeout")
		}
	} else {
		t.Error("Error should implement net.Error")
	}

	// 超时时间应该在合理范围内
	if elapsed < 40*time.Millisecond || elapsed > 200*time.Millisecond {
		t.Errorf("Timeout elapsed time unexpected: %v", elapsed)
	}
}

func TestSupportsReusePort(t *testing.T) {
	// 测试平台检测函数
	supported := supportsReusePort()
	t.Logf("SO_REUSEPORT supported: %v", supported)
	// 这里不做断言，因为不同平台结果不同
}
