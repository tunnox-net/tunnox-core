// Package buffer 接收缓冲区测试
package buffer

import (
	"sync"
	"testing"

	"tunnox-core/internal/packet"
)

// ============================================================================
// ReceiveBuffer 基本功能测试
// ============================================================================

func TestNewReceiveBuffer(t *testing.T) {
	rb := NewReceiveBuffer()

	if rb == nil {
		t.Fatal("NewReceiveBuffer should not return nil")
	}

	if rb.nextExpected != 1 {
		t.Errorf("nextExpected should be 1, got %d", rb.nextExpected)
	}

	if len(rb.buffer) != 0 {
		t.Errorf("buffer should be empty, got %d", len(rb.buffer))
	}

	if rb.maxOutOfOrder != DefaultMaxOutOfOrder {
		t.Errorf("maxOutOfOrder should be %d, got %d", DefaultMaxOutOfOrder, rb.maxOutOfOrder)
	}
}

func TestNewReceiveBufferWithConfig(t *testing.T) {
	tests := []struct {
		name          string
		maxOutOfOrder int
	}{
		{"small", 10},
		{"medium", 50},
		{"large", 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := NewReceiveBufferWithConfig(tt.maxOutOfOrder)

			if rb.maxOutOfOrder != tt.maxOutOfOrder {
				t.Errorf("maxOutOfOrder should be %d, got %d", tt.maxOutOfOrder, rb.maxOutOfOrder)
			}
		})
	}
}

// ============================================================================
// Receive 测试
// ============================================================================

func TestReceiveBuffer_Receive_NilPacket(t *testing.T) {
	rb := NewReceiveBuffer()

	result, err := rb.Receive(nil)

	if err == nil {
		t.Error("Receive with nil packet should return error")
	}

	if result != nil {
		t.Error("Receive with nil packet should return nil result")
	}
}

func TestReceiveBuffer_Receive_InOrder(t *testing.T) {
	rb := NewReceiveBuffer()

	tests := []struct {
		seqNum   uint64
		payload  []byte
		expected [][]byte
	}{
		{1, []byte("data1"), [][]byte{[]byte("data1")}},
		{2, []byte("data2"), [][]byte{[]byte("data2")}},
		{3, []byte("data3"), [][]byte{[]byte("data3")}},
	}

	for _, tt := range tests {
		pkt := &packet.TransferPacket{
			SeqNum:  tt.seqNum,
			Payload: tt.payload,
		}

		result, err := rb.Receive(pkt)

		if err != nil {
			t.Errorf("Receive seqNum=%d should not return error: %v", tt.seqNum, err)
		}

		if len(result) != len(tt.expected) {
			t.Errorf("Receive seqNum=%d result length should be %d, got %d",
				tt.seqNum, len(tt.expected), len(result))
		}

		for i, data := range result {
			if string(data) != string(tt.expected[i]) {
				t.Errorf("Receive seqNum=%d result[%d] should be %s, got %s",
					tt.seqNum, i, string(tt.expected[i]), string(data))
			}
		}
	}

	// 验证 nextExpected 已正确更新
	if rb.GetNextExpected() != 4 {
		t.Errorf("nextExpected should be 4, got %d", rb.GetNextExpected())
	}
}

func TestReceiveBuffer_Receive_OutOfOrder(t *testing.T) {
	rb := NewReceiveBuffer()

	// 发送乱序包: 3, 2, 1
	pkt3 := &packet.TransferPacket{SeqNum: 3, Payload: []byte("data3")}
	pkt2 := &packet.TransferPacket{SeqNum: 2, Payload: []byte("data2")}
	pkt1 := &packet.TransferPacket{SeqNum: 1, Payload: []byte("data1")}

	// 接收包 3（应该被缓冲）
	result, err := rb.Receive(pkt3)
	if err != nil {
		t.Errorf("Receive pkt3 should not return error: %v", err)
	}
	if result != nil {
		t.Error("Receive out-of-order pkt3 should return nil result")
	}
	if rb.GetBufferedCount() != 1 {
		t.Errorf("buffered count should be 1, got %d", rb.GetBufferedCount())
	}

	// 接收包 2（应该被缓冲）
	result, err = rb.Receive(pkt2)
	if err != nil {
		t.Errorf("Receive pkt2 should not return error: %v", err)
	}
	if result != nil {
		t.Error("Receive out-of-order pkt2 should return nil result")
	}
	if rb.GetBufferedCount() != 2 {
		t.Errorf("buffered count should be 2, got %d", rb.GetBufferedCount())
	}

	// 接收包 1（应该触发连续数据返回）
	result, err = rb.Receive(pkt1)
	if err != nil {
		t.Errorf("Receive pkt1 should not return error: %v", err)
	}

	// 应该返回 data1, data2, data3
	if len(result) != 3 {
		t.Errorf("result length should be 3, got %d", len(result))
	}

	expected := []string{"data1", "data2", "data3"}
	for i, exp := range expected {
		if string(result[i]) != exp {
			t.Errorf("result[%d] should be %s, got %s", i, exp, string(result[i]))
		}
	}

	// 缓冲区应该被清空
	if rb.GetBufferedCount() != 0 {
		t.Errorf("buffered count should be 0, got %d", rb.GetBufferedCount())
	}

	if rb.GetNextExpected() != 4 {
		t.Errorf("nextExpected should be 4, got %d", rb.GetNextExpected())
	}
}

func TestReceiveBuffer_Receive_Duplicate(t *testing.T) {
	rb := NewReceiveBuffer()

	pkt1 := &packet.TransferPacket{SeqNum: 1, Payload: []byte("data1")}

	// 第一次接收
	result, err := rb.Receive(pkt1)
	if err != nil {
		t.Errorf("First receive should not return error: %v", err)
	}
	if len(result) != 1 {
		t.Error("First receive should return data")
	}

	// 重复接收（应该丢弃）
	result, err = rb.Receive(pkt1)
	if err != nil {
		t.Errorf("Duplicate receive should not return error: %v", err)
	}
	if result != nil {
		t.Error("Duplicate receive should return nil")
	}
}

func TestReceiveBuffer_Receive_DuplicateBuffered(t *testing.T) {
	rb := NewReceiveBuffer()

	// 先缓冲包 3
	pkt3 := &packet.TransferPacket{SeqNum: 3, Payload: []byte("data3")}
	_, err := rb.Receive(pkt3)
	if err != nil {
		t.Errorf("First receive should not return error: %v", err)
	}

	// 重复发送包 3（应该丢弃）
	result, err := rb.Receive(pkt3)
	if err != nil {
		t.Errorf("Duplicate buffered receive should not return error: %v", err)
	}
	if result != nil {
		t.Error("Duplicate buffered receive should return nil")
	}

	// 确认只缓冲了一个包
	if rb.GetBufferedCount() != 1 {
		t.Errorf("buffered count should be 1, got %d", rb.GetBufferedCount())
	}
}

func TestReceiveBuffer_Receive_MaxOutOfOrder(t *testing.T) {
	rb := NewReceiveBufferWithConfig(3) // 最多缓冲 3 个乱序包

	// 缓冲 3 个乱序包
	for seqNum := uint64(2); seqNum <= 4; seqNum++ {
		pkt := &packet.TransferPacket{SeqNum: seqNum, Payload: []byte("data")}
		_, err := rb.Receive(pkt)
		if err != nil {
			t.Errorf("Receive seqNum=%d should not return error: %v", seqNum, err)
		}
	}

	// 第 4 个乱序包应该返回错误
	pkt5 := &packet.TransferPacket{SeqNum: 5, Payload: []byte("data5")}
	_, err := rb.Receive(pkt5)
	if err == nil {
		t.Error("Receive beyond maxOutOfOrder should return error")
	}
}

// ============================================================================
// 状态查询测试
// ============================================================================

func TestReceiveBuffer_GetStats(t *testing.T) {
	rb := NewReceiveBuffer()

	// 发送一些包
	pkt1 := &packet.TransferPacket{SeqNum: 1, Payload: []byte("data1")}
	pkt3 := &packet.TransferPacket{SeqNum: 3, Payload: []byte("data3")}

	rb.Receive(pkt1)
	rb.Receive(pkt3)

	stats := rb.GetStats()

	if stats["total_received"] != 2 {
		t.Errorf("total_received should be 2, got %d", stats["total_received"])
	}

	if stats["total_out_of_order"] != 1 {
		t.Errorf("total_out_of_order should be 1, got %d", stats["total_out_of_order"])
	}

	if stats["buffered_count"] != 1 {
		t.Errorf("buffered_count should be 1, got %d", stats["buffered_count"])
	}

	if stats["next_expected"] != 2 {
		t.Errorf("next_expected should be 2, got %d", stats["next_expected"])
	}
}

func TestReceiveBuffer_GetBufferSize(t *testing.T) {
	rb := NewReceiveBuffer()

	// 缓冲一些乱序包
	pkt2 := &packet.TransferPacket{SeqNum: 2, Payload: []byte("hello")} // 5 bytes
	pkt3 := &packet.TransferPacket{SeqNum: 3, Payload: []byte("world")} // 5 bytes

	rb.Receive(pkt2)
	rb.Receive(pkt3)

	if rb.GetBufferSize() != 10 {
		t.Errorf("buffer size should be 10, got %d", rb.GetBufferSize())
	}

	// 接收包 1 触发清空缓冲
	pkt1 := &packet.TransferPacket{SeqNum: 1, Payload: []byte("!")}
	rb.Receive(pkt1)

	if rb.GetBufferSize() != 0 {
		t.Errorf("buffer size should be 0 after flush, got %d", rb.GetBufferSize())
	}
}

// ============================================================================
// Reset 和 Clear 测试
// ============================================================================

func TestReceiveBuffer_Reset(t *testing.T) {
	rb := NewReceiveBuffer()

	// 缓冲一些数据
	pkt2 := &packet.TransferPacket{SeqNum: 2, Payload: []byte("data")}
	rb.Receive(pkt2)

	// 接收包 1
	pkt1 := &packet.TransferPacket{SeqNum: 1, Payload: []byte("data1")}
	rb.Receive(pkt1)

	// 现在 nextExpected 是 3
	if rb.GetNextExpected() != 3 {
		t.Errorf("nextExpected should be 3, got %d", rb.GetNextExpected())
	}

	// Reset 不重置 nextExpected
	rb.Reset()

	if rb.GetBufferedCount() != 0 {
		t.Error("buffer should be empty after Reset")
	}

	// nextExpected 保持不变
	if rb.GetNextExpected() != 3 {
		t.Errorf("nextExpected should still be 3 after Reset, got %d", rb.GetNextExpected())
	}
}

func TestReceiveBuffer_Clear(t *testing.T) {
	rb := NewReceiveBuffer()

	// 发送一些数据
	pkt1 := &packet.TransferPacket{SeqNum: 1, Payload: []byte("data1")}
	pkt3 := &packet.TransferPacket{SeqNum: 3, Payload: []byte("data3")}
	rb.Receive(pkt1)
	rb.Receive(pkt3)

	// Clear 重置所有状态
	rb.Clear()

	if rb.GetNextExpected() != 1 {
		t.Errorf("nextExpected should be reset to 1 after Clear, got %d", rb.GetNextExpected())
	}

	if rb.GetBufferedCount() != 0 {
		t.Error("buffer should be empty after Clear")
	}

	stats := rb.GetStats()
	if stats["total_received"] != 0 {
		t.Error("total_received should be reset after Clear")
	}
}

// ============================================================================
// 并发安全测试
// ============================================================================

func TestReceiveBuffer_ConcurrentReceive(t *testing.T) {
	rb := NewReceiveBuffer()

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(seqNum uint64) {
			defer wg.Done()
			pkt := &packet.TransferPacket{SeqNum: seqNum, Payload: []byte("data")}
			rb.Receive(pkt)
		}(uint64(i + 1))
	}

	wg.Wait()

	// 由于并发，不能保证顺序，但不应该 panic
	stats := rb.GetStats()
	if stats["total_received"] != uint64(numGoroutines) {
		t.Errorf("total_received should be %d, got %d", numGoroutines, stats["total_received"])
	}
}

func TestReceiveBuffer_ConcurrentReads(t *testing.T) {
	rb := NewReceiveBuffer()

	// 先添加一些数据
	pkt1 := &packet.TransferPacket{SeqNum: 1, Payload: []byte("data1")}
	rb.Receive(pkt1)

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines * 4)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			rb.GetNextExpected()
		}()

		go func() {
			defer wg.Done()
			rb.GetBufferedCount()
		}()

		go func() {
			defer wg.Done()
			rb.GetBufferSize()
		}()

		go func() {
			defer wg.Done()
			rb.GetStats()
		}()
	}

	wg.Wait()
	// 测试不应该 panic
}
