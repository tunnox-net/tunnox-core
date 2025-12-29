package mapping

import (
	"sync"
	"testing"
	"time"
)

func TestTrafficStats_Reset(t *testing.T) {
	stats := &TrafficStats{}

	// 设置一些值
	stats.BytesSent.Store(1000)
	stats.BytesReceived.Store(2000)
	stats.ConnectionCount.Store(5)

	// 重置
	stats.Reset()

	// 验证重置后的值
	if stats.BytesSent.Load() != 0 {
		t.Errorf("BytesSent should be 0 after reset, got %d", stats.BytesSent.Load())
	}
	if stats.BytesReceived.Load() != 0 {
		t.Errorf("BytesReceived should be 0 after reset, got %d", stats.BytesReceived.Load())
	}

	// LastReportTime 应该被更新
	stats.mu.RLock()
	timeDiff := time.Since(stats.LastReportTime)
	stats.mu.RUnlock()

	if timeDiff > time.Second {
		t.Error("LastReportTime should be recently updated")
	}
}

func TestTrafficStats_GetStats(t *testing.T) {
	stats := &TrafficStats{}

	// 设置值
	stats.BytesSent.Store(1000)
	stats.BytesReceived.Store(2000)
	stats.ConnectionCount.Store(5)

	// 获取统计
	sent, received, count := stats.GetStats()

	if sent != 1000 {
		t.Errorf("Expected sent=1000, got %d", sent)
	}
	if received != 2000 {
		t.Errorf("Expected received=2000, got %d", received)
	}
	if count != 5 {
		t.Errorf("Expected count=5, got %d", count)
	}
}

func TestTrafficStats_ConcurrentAccess(t *testing.T) {
	stats := &TrafficStats{}
	var wg sync.WaitGroup
	numGoroutines := 100
	incrementPerGoroutine := 100

	// 并发增加计数
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incrementPerGoroutine; j++ {
				stats.BytesSent.Add(1)
				stats.BytesReceived.Add(2)
				stats.ConnectionCount.Add(1)
			}
		}()
	}

	wg.Wait()

	expectedSent := int64(numGoroutines * incrementPerGoroutine)
	expectedReceived := int64(numGoroutines * incrementPerGoroutine * 2)
	expectedCount := int64(numGoroutines * incrementPerGoroutine)

	sent, received, count := stats.GetStats()

	if sent != expectedSent {
		t.Errorf("Expected sent=%d, got %d", expectedSent, sent)
	}
	if received != expectedReceived {
		t.Errorf("Expected received=%d, got %d", expectedReceived, received)
	}
	if count != expectedCount {
		t.Errorf("Expected count=%d, got %d", expectedCount, count)
	}
}
