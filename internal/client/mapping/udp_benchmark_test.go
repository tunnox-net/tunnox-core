package mapping

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tunnox-core/internal/config"
)

// BenchmarkUDPAdapter_HighPPS 测试高 PPS 场景下的性能
func BenchmarkUDPAdapter_HighPPS(b *testing.B) {
	// 创建适配器
	adapter := NewUDPMappingAdapter()

	// 找一个可用端口
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("Failed to find port: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()

	cfg := config.MappingConfig{
		MappingID: "benchmark",
		LocalPort: port,
	}

	if err := adapter.StartListener(cfg); err != nil {
		b.Fatalf("StartListener failed: %v", err)
	}
	defer adapter.Close()

	// 启动消费者
	go func() {
		for {
			conn, err := adapter.Accept()
			if err != nil {
				return
			}
			go func(c interface{ Close() error }) {
				defer c.Close()
				buf := make([]byte, 2048)
				for {
					_, err := c.(interface{ Read([]byte) (int, error) }).Read(buf)
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	// 发送端
	clientConn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		b.Fatalf("Dial failed: %v", err)
	}
	defer clientConn.Close()

	data := make([]byte, 1400)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		clientConn.Write(data)
	}
}

// BenchmarkUDPAdapter_ConcurrentSessions 测试并发会话性能
func BenchmarkUDPAdapter_ConcurrentSessions(b *testing.B) {
	adapter := NewUDPMappingAdapter()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("Failed to find port: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()

	cfg := config.MappingConfig{
		MappingID: "benchmark-concurrent",
		LocalPort: port,
	}

	if err := adapter.StartListener(cfg); err != nil {
		b.Fatalf("StartListener failed: %v", err)
	}
	defer adapter.Close()

	// 启动消费者
	go func() {
		for {
			conn, err := adapter.Accept()
			if err != nil {
				return
			}
			go func(c interface{ Close() error }) {
				defer c.Close()
				buf := make([]byte, 2048)
				for {
					_, err := c.(interface{ Read([]byte) (int, error) }).Read(buf)
					if err != nil {
						return
					}
				}
			}(conn)
		}
	}()

	// 并发发送
	numClients := 100
	clients := make([]*net.UDPConn, numClients)
	for i := 0; i < numClients; i++ {
		c, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			b.Fatalf("Dial failed: %v", err)
		}
		clients[i] = c.(*net.UDPConn)
	}
	defer func() {
		for _, c := range clients {
			c.Close()
		}
	}()

	data := make([]byte, 1400)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			clients[i%numClients].Write(data)
			i++
		}
	})
}

// TestUDPAdapter_DropRate 测试丢包率
func TestUDPAdapter_DropRate(t *testing.T) {
	adapter := NewUDPMappingAdapter()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find port: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()

	cfg := config.MappingConfig{
		MappingID: "droprate-test",
		LocalPort: port,
	}

	if err := adapter.StartListener(cfg); err != nil {
		t.Fatalf("StartListener failed: %v", err)
	}
	defer adapter.Close()

	// 统计接收
	var received uint64
	var wg sync.WaitGroup

	// 启动消费者
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := adapter.Accept()
			if err != nil {
				return
			}
			go func(c interface{ Close() error }) {
				defer c.Close()
				buf := make([]byte, 2048)
				for {
					_, err := c.(interface{ Read([]byte) (int, error) }).Read(buf)
					if err != nil {
						return
					}
					atomic.AddUint64(&received, 1)
				}
			}(conn)
		}
	}()

	// 发送端
	clientConn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}

	data := make([]byte, 1400)
	totalSent := 100000
	targetPPS := 50000 // 50k pps

	interval := time.Second / time.Duration(targetPPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	startTime := time.Now()
	var sent uint64

	// 发送进度
	go func() {
		for {
			time.Sleep(time.Second)
			s := atomic.LoadUint64(&sent)
			r := atomic.LoadUint64(&received)
			if s >= uint64(totalSent) {
				return
			}
			t.Logf("Progress: sent=%d, received=%d, drop=%.2f%%",
				s, r, 100.0*(1.0-float64(r)/float64(s)))
		}
	}()

	for i := 0; i < totalSent; i++ {
		<-ticker.C
		clientConn.Write(data)
		atomic.AddUint64(&sent, 1)
	}

	// 等待接收完成
	time.Sleep(500 * time.Millisecond)
	clientConn.Close()

	elapsed := time.Since(startTime)
	finalSent := atomic.LoadUint64(&sent)
	finalRecv := atomic.LoadUint64(&received)
	dropRate := 100.0 * (1.0 - float64(finalRecv)/float64(finalSent))
	pps := float64(finalSent) / elapsed.Seconds()
	throughput := pps * 1400 * 8 / 1000000 // Mbps

	t.Logf("\n========== UDP Performance Test Results ==========")
	t.Logf("Duration:    %.2f seconds", elapsed.Seconds())
	t.Logf("Sent:        %d packets", finalSent)
	t.Logf("Received:    %d packets", finalRecv)
	t.Logf("Drop Rate:   %.2f%%", dropRate)
	t.Logf("PPS:         %.0f", pps)
	t.Logf("Throughput:  %.2f Mbps", throughput)
	t.Logf("Listeners:   %d (SO_REUSEPORT: %v)", len(adapter.listeners), supportsReusePort())
	t.Logf("=================================================")

	// 丢包率应该低于 5%
	if dropRate > 5.0 {
		t.Errorf("Drop rate %.2f%% exceeds 5%% threshold", dropRate)
	}
}

// TestUDPAdapter_HighLoadDropRate 高负载丢包率测试
func TestUDPAdapter_HighLoadDropRate(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high load test in short mode")
	}

	adapter := NewUDPMappingAdapter()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to find port: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()

	cfg := config.MappingConfig{
		MappingID: "highload-test",
		LocalPort: port,
	}

	if err := adapter.StartListener(cfg); err != nil {
		t.Fatalf("StartListener failed: %v", err)
	}
	defer adapter.Close()

	var received uint64

	// 启动消费者
	go func() {
		for {
			conn, err := adapter.Accept()
			if err != nil {
				return
			}
			go func(c interface{ Close() error }) {
				defer c.Close()
				buf := make([]byte, 2048)
				for {
					_, err := c.(interface{ Read([]byte) (int, error) }).Read(buf)
					if err != nil {
						return
					}
					atomic.AddUint64(&received, 1)
				}
			}(conn)
		}
	}()

	// 多客户端并发发送
	numClients := 10
	packetsPerClient := 10000
	data := make([]byte, 1400)

	var wg sync.WaitGroup
	var totalSent uint64

	startTime := time.Now()

	for c := 0; c < numClients; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clientConn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				return
			}
			defer clientConn.Close()

			for i := 0; i < packetsPerClient; i++ {
				clientConn.Write(data)
				atomic.AddUint64(&totalSent, 1)
			}
		}()
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond)

	elapsed := time.Since(startTime)
	finalSent := atomic.LoadUint64(&totalSent)
	finalRecv := atomic.LoadUint64(&received)
	dropRate := 100.0 * (1.0 - float64(finalRecv)/float64(finalSent))
	pps := float64(finalSent) / elapsed.Seconds()
	throughput := pps * 1400 * 8 / 1000000

	t.Logf("\n========== High Load UDP Test Results ==========")
	t.Logf("Clients:     %d concurrent", numClients)
	t.Logf("Duration:    %.2f seconds", elapsed.Seconds())
	t.Logf("Sent:        %d packets", finalSent)
	t.Logf("Received:    %d packets", finalRecv)
	t.Logf("Drop Rate:   %.2f%%", dropRate)
	t.Logf("PPS:         %.0f", pps)
	t.Logf("Throughput:  %.2f Mbps", throughput)
	t.Logf("Listeners:   %d", len(adapter.listeners))
	t.Logf("================================================")
}
