// Package buffer 发送缓冲区测试
package buffer

import (
	"sync"
	"testing"
	"time"

	"tunnox-core/internal/packet"
)

// ============================================================================
// SendBuffer 基本功能测试
// ============================================================================

func TestNewSendBuffer(t *testing.T) {
	sb := NewSendBuffer()

	if sb == nil {
		t.Fatal("NewSendBuffer should not return nil")
	}

	if sb.nextSeq != 1 {
		t.Errorf("nextSeq should be 1, got %d", sb.nextSeq)
	}

	if sb.confirmedSeq != 0 {
		t.Errorf("confirmedSeq should be 0, got %d", sb.confirmedSeq)
	}

	if len(sb.Buffer) != 0 {
		t.Errorf("buffer should be empty, got %d", len(sb.Buffer))
	}

	if sb.maxBufferSize != DefaultMaxBufferSize {
		t.Errorf("maxBufferSize should be %d, got %d", DefaultMaxBufferSize, sb.maxBufferSize)
	}

	if sb.maxBufferedPackets != DefaultMaxBufferedPackets {
		t.Errorf("maxBufferedPackets should be %d, got %d", DefaultMaxBufferedPackets, sb.maxBufferedPackets)
	}
}

func TestNewSendBufferWithConfig(t *testing.T) {
	maxSize := 1024 * 1024 // 1MB
	maxPackets := 100
	timeout := 5 * time.Second

	sb := NewSendBufferWithConfig(maxSize, maxPackets, timeout)

	if sb.maxBufferSize != maxSize {
		t.Errorf("maxBufferSize should be %d, got %d", maxSize, sb.maxBufferSize)
	}

	if sb.maxBufferedPackets != maxPackets {
		t.Errorf("maxBufferedPackets should be %d, got %d", maxPackets, sb.maxBufferedPackets)
	}

	if sb.resendTimeout != timeout {
		t.Errorf("resendTimeout should be %v, got %v", timeout, sb.resendTimeout)
	}
}

// ============================================================================
// Send 测试
// ============================================================================

func TestSendBuffer_Send(t *testing.T) {
	sb := NewSendBuffer()

	data := []byte("test data")
	pkt := &packet.TransferPacket{Payload: data}

	seqNum, err := sb.Send(data, pkt)

	if err != nil {
		t.Errorf("Send should not return error: %v", err)
	}

	if seqNum != 1 {
		t.Errorf("first seqNum should be 1, got %d", seqNum)
	}

	if sb.GetNextSeq() != 2 {
		t.Errorf("nextSeq should be 2, got %d", sb.GetNextSeq())
	}

	if sb.GetBufferedCount() != 1 {
		t.Errorf("buffered count should be 1, got %d", sb.GetBufferedCount())
	}

	if sb.GetBufferSize() != len(data) {
		t.Errorf("buffer size should be %d, got %d", len(data), sb.GetBufferSize())
	}
}

func TestSendBuffer_Send_Multiple(t *testing.T) {
	sb := NewSendBuffer()

	for i := 1; i <= 5; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}

		seqNum, err := sb.Send(data, pkt)

		if err != nil {
			t.Errorf("Send %d should not return error: %v", i, err)
		}

		if seqNum != uint64(i) {
			t.Errorf("seqNum should be %d, got %d", i, seqNum)
		}
	}

	if sb.GetBufferedCount() != 5 {
		t.Errorf("buffered count should be 5, got %d", sb.GetBufferedCount())
	}
}

func TestSendBuffer_Send_MaxPackets(t *testing.T) {
	sb := NewSendBufferWithConfig(100*1024*1024, 3, time.Second) // 最多 3 个包

	// 发送 3 个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		_, err := sb.Send(data, pkt)
		if err != nil {
			t.Errorf("Send %d should not return error: %v", i, err)
		}
	}

	// 第 4 个包应该失败
	data := []byte("data")
	pkt := &packet.TransferPacket{Payload: data}
	_, err := sb.Send(data, pkt)

	if err == nil {
		t.Error("Send beyond maxBufferedPackets should return error")
	}
}

func TestSendBuffer_Send_MaxSize(t *testing.T) {
	sb := NewSendBufferWithConfig(10, 100, time.Second) // 最大 10 字节

	// 发送 5 字节
	data1 := []byte("hello")
	pkt1 := &packet.TransferPacket{Payload: data1}
	_, err := sb.Send(data1, pkt1)
	if err != nil {
		t.Errorf("First send should not return error: %v", err)
	}

	// 再发送 6 字节应该失败（5+6=11 > 10）
	data2 := []byte("world!")
	pkt2 := &packet.TransferPacket{Payload: data2}
	_, err = sb.Send(data2, pkt2)

	if err == nil {
		t.Error("Send beyond maxBufferSize should return error")
	}
}

// ============================================================================
// Confirm 测试
// ============================================================================

func TestSendBuffer_ConfirmUpTo(t *testing.T) {
	sb := NewSendBuffer()

	// 发送 5 个包
	for i := 0; i < 5; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sb.Send(data, pkt)
	}

	// 确认到 3（即确认 1, 2）
	sb.ConfirmUpTo(3)

	if sb.GetConfirmedSeq() != 2 {
		t.Errorf("confirmedSeq should be 2, got %d", sb.GetConfirmedSeq())
	}

	if sb.GetBufferedCount() != 3 {
		t.Errorf("buffered count should be 3, got %d", sb.GetBufferedCount())
	}

	// 确认到 6（即确认所有）
	sb.ConfirmUpTo(6)

	if sb.GetConfirmedSeq() != 5 {
		t.Errorf("confirmedSeq should be 5, got %d", sb.GetConfirmedSeq())
	}

	if sb.GetBufferedCount() != 0 {
		t.Errorf("buffered count should be 0, got %d", sb.GetBufferedCount())
	}
}

func TestSendBuffer_ConfirmPacket(t *testing.T) {
	sb := NewSendBuffer()

	// 发送 5 个包
	for i := 0; i < 5; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sb.Send(data, pkt)
	}

	// 单独确认包 1
	sb.ConfirmPacket(1)

	if sb.GetConfirmedSeq() != 1 {
		t.Errorf("confirmedSeq should be 1, got %d", sb.GetConfirmedSeq())
	}

	if sb.GetBufferedCount() != 4 {
		t.Errorf("buffered count should be 4, got %d", sb.GetBufferedCount())
	}

	// 单独确认包 3（跳过 2）
	sb.ConfirmPacket(3)

	// confirmedSeq 仍然是 1（因为 2 没确认）
	if sb.GetConfirmedSeq() != 1 {
		t.Errorf("confirmedSeq should still be 1, got %d", sb.GetConfirmedSeq())
	}

	if sb.GetBufferedCount() != 3 {
		t.Errorf("buffered count should be 3, got %d", sb.GetBufferedCount())
	}

	// 确认包 2（应该触发 confirmedSeq 推进到 3）
	sb.ConfirmPacket(2)

	if sb.GetConfirmedSeq() != 3 {
		t.Errorf("confirmedSeq should be 3 after confirming 2, got %d", sb.GetConfirmedSeq())
	}
}

func TestSendBuffer_ConfirmPacket_NonExistent(t *testing.T) {
	sb := NewSendBuffer()

	// 发送 1 个包
	data := []byte("data")
	pkt := &packet.TransferPacket{Payload: data}
	sb.Send(data, pkt)

	// 确认不存在的包（不应该 panic）
	sb.ConfirmPacket(100)

	if sb.GetBufferedCount() != 1 {
		t.Errorf("buffered count should still be 1, got %d", sb.GetBufferedCount())
	}
}

// ============================================================================
// 重传测试
// ============================================================================

func TestSendBuffer_GetUnconfirmedPackets(t *testing.T) {
	sb := NewSendBufferWithConfig(10*1024*1024, 1000, 100*time.Millisecond)

	// 发送 3 个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sb.Send(data, pkt)
	}

	// 立即获取，应该没有超时的包
	unconfirmed := sb.GetUnconfirmedPackets()
	if len(unconfirmed) != 0 {
		t.Errorf("should have no unconfirmed packets immediately, got %d", len(unconfirmed))
	}

	// 等待超时
	time.Sleep(150 * time.Millisecond)

	unconfirmed = sb.GetUnconfirmedPackets()
	if len(unconfirmed) != 3 {
		t.Errorf("should have 3 unconfirmed packets after timeout, got %d", len(unconfirmed))
	}
}

func TestSendBuffer_MarkResent(t *testing.T) {
	sb := NewSendBufferWithConfig(10*1024*1024, 1000, 100*time.Millisecond)

	// 发送 1 个包
	data := []byte("data")
	pkt := &packet.TransferPacket{Payload: data}
	sb.Send(data, pkt)

	// 等待超时
	time.Sleep(150 * time.Millisecond)

	// 应该有 1 个需要重传的包
	unconfirmed := sb.GetUnconfirmedPackets()
	if len(unconfirmed) != 1 {
		t.Errorf("should have 1 unconfirmed packet, got %d", len(unconfirmed))
	}

	// 标记已重传
	sb.MarkResent(1)

	// 立即获取，应该没有超时的包（因为刚刚重置了发送时间）
	unconfirmed = sb.GetUnconfirmedPackets()
	if len(unconfirmed) != 0 {
		t.Errorf("should have no unconfirmed packets after MarkResent, got %d", len(unconfirmed))
	}

	// 验证重试计数
	stats := sb.GetStats()
	if stats["total_resent"] != 1 {
		t.Errorf("total_resent should be 1, got %d", stats["total_resent"])
	}
}

func TestSendBuffer_MarkResent_NonExistent(t *testing.T) {
	sb := NewSendBuffer()

	// 发送 1 个包
	data := []byte("data")
	pkt := &packet.TransferPacket{Payload: data}
	sb.Send(data, pkt)

	// 标记不存在的包（不应该 panic）
	sb.MarkResent(100)

	stats := sb.GetStats()
	if stats["total_resent"] != 0 {
		t.Errorf("total_resent should be 0, got %d", stats["total_resent"])
	}
}

// ============================================================================
// 状态查询测试
// ============================================================================

func TestSendBuffer_GetStats(t *testing.T) {
	sb := NewSendBuffer()

	// 发送 3 个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sb.Send(data, pkt)
	}

	// 确认 2 个包
	sb.ConfirmUpTo(3)

	stats := sb.GetStats()

	if stats["total_sent"] != 3 {
		t.Errorf("total_sent should be 3, got %d", stats["total_sent"])
	}

	if stats["total_confirmed"] != 2 {
		t.Errorf("total_confirmed should be 2, got %d", stats["total_confirmed"])
	}

	if stats["buffered_count"] != 1 {
		t.Errorf("buffered_count should be 1, got %d", stats["buffered_count"])
	}

	if stats["next_seq"] != 4 {
		t.Errorf("next_seq should be 4, got %d", stats["next_seq"])
	}

	if stats["confirmed_seq"] != 2 {
		t.Errorf("confirmed_seq should be 2, got %d", stats["confirmed_seq"])
	}
}

// ============================================================================
// Reset 和 Clear 测试
// ============================================================================

func TestSendBuffer_Reset(t *testing.T) {
	sb := NewSendBuffer()

	// 发送 3 个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sb.Send(data, pkt)
	}

	// Reset 不重置序列号
	sb.Reset()

	if sb.GetBufferedCount() != 0 {
		t.Error("buffer should be empty after Reset")
	}

	if sb.GetBufferSize() != 0 {
		t.Error("buffer size should be 0 after Reset")
	}

	// nextSeq 保持不变
	if sb.GetNextSeq() != 4 {
		t.Errorf("nextSeq should still be 4 after Reset, got %d", sb.GetNextSeq())
	}
}

func TestSendBuffer_Clear(t *testing.T) {
	sb := NewSendBuffer()

	// 发送 3 个包并确认 1 个
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sb.Send(data, pkt)
	}
	sb.ConfirmPacket(1)

	// Clear 重置所有状态
	sb.Clear()

	if sb.GetNextSeq() != 1 {
		t.Errorf("nextSeq should be reset to 1 after Clear, got %d", sb.GetNextSeq())
	}

	if sb.GetConfirmedSeq() != 0 {
		t.Errorf("confirmedSeq should be reset to 0 after Clear, got %d", sb.GetConfirmedSeq())
	}

	if sb.GetBufferedCount() != 0 {
		t.Error("buffer should be empty after Clear")
	}

	stats := sb.GetStats()
	if stats["total_sent"] != 0 {
		t.Error("total_sent should be reset after Clear")
	}
}

// ============================================================================
// 锁测试
// ============================================================================

func TestSendBuffer_Locks(t *testing.T) {
	sb := NewSendBuffer()

	// 测试 Lock/Unlock
	sb.Lock()
	// 直接访问内部状态（通常不推荐，但用于测试）
	sb.nextSeq = 100
	sb.Unlock()

	if sb.GetNextSeq() != 100 {
		t.Errorf("nextSeq should be 100, got %d", sb.GetNextSeq())
	}

	// 测试 RLock/RUnlock
	sb.RLock()
	nextSeq := sb.nextSeq
	sb.RUnlock()

	if nextSeq != 100 {
		t.Errorf("nextSeq should be 100, got %d", nextSeq)
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestSendBuffer_ConcurrentSend(t *testing.T) {
	sb := NewSendBuffer()

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			data := []byte("data")
			pkt := &packet.TransferPacket{Payload: data}
			sb.Send(data, pkt)
		}()
	}

	wg.Wait()

	if sb.GetBufferedCount() != numGoroutines {
		t.Errorf("buffered count should be %d, got %d", numGoroutines, sb.GetBufferedCount())
	}
}

func TestSendBuffer_ConcurrentConfirm(t *testing.T) {
	sb := NewSendBuffer()

	// 先发送一些包
	for i := 0; i < 100; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sb.Send(data, pkt)
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(seqNum uint64) {
			defer wg.Done()
			sb.ConfirmPacket(seqNum)
		}(uint64(i + 1))
	}

	wg.Wait()

	// 确认了 50 个包
	if sb.GetBufferedCount() != 50 {
		t.Errorf("buffered count should be 50, got %d", sb.GetBufferedCount())
	}
}

func TestSendBuffer_ConcurrentReads(t *testing.T) {
	sb := NewSendBuffer()

	// 先发送一些包
	for i := 0; i < 10; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sb.Send(data, pkt)
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines * 5)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			sb.GetNextSeq()
		}()

		go func() {
			defer wg.Done()
			sb.GetConfirmedSeq()
		}()

		go func() {
			defer wg.Done()
			sb.GetBufferedCount()
		}()

		go func() {
			defer wg.Done()
			sb.GetBufferSize()
		}()

		go func() {
			defer wg.Done()
			sb.GetStats()
		}()
	}

	wg.Wait()
	// 测试不应该 panic
}
