package session

import (
	"testing"
	"time"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TunnelSendBuffer 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestNewTunnelSendBuffer(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	assert.NotNil(t, buffer)
	assert.Equal(t, uint64(1), buffer.GetNextSeq(), "NextSeq should start at 1")
	assert.Equal(t, uint64(0), buffer.GetConfirmedSeq(), "ConfirmedSeq should start at 0")
	assert.Equal(t, 0, buffer.GetBufferedCount())
	assert.Equal(t, 0, buffer.GetBufferSize())
}

func TestTunnelSendBuffer_Send(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	data := []byte("hello world")
	pkt := &packet.TransferPacket{
		PacketType: packet.TunnelData,
		TunnelID:   "tunnel-123",
		Payload:    data,
	}
	
	seqNum, err := buffer.Send(data, pkt)
	require.NoError(t, err)
	
	assert.Equal(t, uint64(1), seqNum, "First packet should have seqNum=1")
	assert.Equal(t, uint64(2), buffer.GetNextSeq(), "NextSeq should increment to 2")
	assert.Equal(t, 1, buffer.GetBufferedCount())
	assert.Equal(t, len(data), buffer.GetBufferSize())
}

func TestTunnelSendBuffer_SendMultiple(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	// 发送3个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{
			PacketType: packet.TunnelData,
			Payload:    data,
		}
		
		seqNum, err := buffer.Send(data, pkt)
		require.NoError(t, err)
		assert.Equal(t, uint64(i+1), seqNum)
	}
	
	assert.Equal(t, uint64(4), buffer.GetNextSeq())
	assert.Equal(t, 3, buffer.GetBufferedCount())
	assert.Equal(t, 12, buffer.GetBufferSize()) // 3 * 4 bytes
}

func TestTunnelSendBuffer_SendBufferFull(t *testing.T) {
	// 创建小缓冲区
	buffer := NewTunnelSendBufferWithConfig(100, 5, 3*time.Second)
	
	// 填满缓冲区（5个包）
	for i := 0; i < 5; i++ {
		data := []byte("test")
		pkt := &packet.TransferPacket{Payload: data}
		_, err := buffer.Send(data, pkt)
		require.NoError(t, err)
	}
	
	// 第6个包应该失败
	data := []byte("overflow")
	pkt := &packet.TransferPacket{Payload: data}
	_, err := buffer.Send(data, pkt)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "buffer full")
}

func TestTunnelSendBuffer_SendSizeLimitExceeded(t *testing.T) {
	// 创建100字节的缓冲区
	buffer := NewTunnelSendBufferWithConfig(100, 1000, 3*time.Second)
	
	// 发送90字节
	data1 := make([]byte, 90)
	pkt1 := &packet.TransferPacket{Payload: data1}
	_, err := buffer.Send(data1, pkt1)
	require.NoError(t, err)
	
	// 再发送20字节应该失败（总共110字节 > 100）
	data2 := make([]byte, 20)
	pkt2 := &packet.TransferPacket{Payload: data2}
	_, err = buffer.Send(data2, pkt2)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "size limit exceeded")
}

func TestTunnelSendBuffer_ConfirmUpTo(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	// 发送5个包
	for i := 0; i < 5; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		_, err := buffer.Send(data, pkt)
		require.NoError(t, err)
	}
	
	assert.Equal(t, 5, buffer.GetBufferedCount())
	
	// 确认前3个包（seqNum 1, 2, 3）
	buffer.ConfirmUpTo(4) // ackNum=4 表示期望接收4，已确认1,2,3
	
	assert.Equal(t, uint64(3), buffer.GetConfirmedSeq())
	assert.Equal(t, 2, buffer.GetBufferedCount()) // 还剩4,5两个包
	assert.Equal(t, 8, buffer.GetBufferSize())    // 2 * 4 bytes
}

func TestTunnelSendBuffer_ConfirmPacket(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	// 发送3个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		_, err := buffer.Send(data, pkt)
		require.NoError(t, err)
	}
	
	// 确认第1个包
	buffer.ConfirmPacket(1)
	assert.Equal(t, uint64(1), buffer.GetConfirmedSeq())
	assert.Equal(t, 2, buffer.GetBufferedCount())
	
	// 确认第2个包
	buffer.ConfirmPacket(2)
	assert.Equal(t, uint64(2), buffer.GetConfirmedSeq())
	assert.Equal(t, 1, buffer.GetBufferedCount())
}

func TestTunnelSendBuffer_ConfirmOutOfOrder(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	// 发送5个包
	for i := 0; i < 5; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		_, err := buffer.Send(data, pkt)
		require.NoError(t, err)
	}
	
	// 乱序确认：3, 1, 2
	buffer.ConfirmPacket(3)
	assert.Equal(t, uint64(0), buffer.GetConfirmedSeq(), "ConfirmedSeq should stay at 0")
	assert.Equal(t, 4, buffer.GetBufferedCount())
	
	buffer.ConfirmPacket(1)
	assert.Equal(t, uint64(1), buffer.GetConfirmedSeq())
	assert.Equal(t, 3, buffer.GetBufferedCount())
	
	buffer.ConfirmPacket(2)
	// 确认2后，应该连续推进到3（因为3已经确认过了）
	assert.Equal(t, uint64(3), buffer.GetConfirmedSeq())
	assert.Equal(t, 2, buffer.GetBufferedCount())
}

func TestTunnelSendBuffer_GetUnconfirmedPackets(t *testing.T) {
	// 使用短超时时间
	buffer := NewTunnelSendBufferWithConfig(1024*1024, 100, 100*time.Millisecond)
	
	// 发送3个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		_, err := buffer.Send(data, pkt)
		require.NoError(t, err)
	}
	
	// 立即检查，应该没有需要重传的
	unconfirmed := buffer.GetUnconfirmedPackets()
	assert.Equal(t, 0, len(unconfirmed))
	
	// 等待超时
	time.Sleep(150 * time.Millisecond)
	
	// 现在应该有3个包需要重传
	unconfirmed = buffer.GetUnconfirmedPackets()
	assert.Equal(t, 3, len(unconfirmed))
}

func TestTunnelSendBuffer_MarkResent(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	data := []byte("data")
	pkt := &packet.TransferPacket{Payload: data}
	seqNum, err := buffer.Send(data, pkt)
	require.NoError(t, err)
	
	// 获取初始发送时间
	buffer.mu.RLock()
	originalSentAt := buffer.buffer[seqNum].SentAt
	retryCount := buffer.buffer[seqNum].RetryCount
	buffer.mu.RUnlock()
	
	assert.Equal(t, 0, retryCount)
	
	// 等待一小段时间
	time.Sleep(10 * time.Millisecond)
	
	// 标记为重传
	buffer.MarkResent(seqNum)
	
	buffer.mu.RLock()
	newSentAt := buffer.buffer[seqNum].SentAt
	newRetryCount := buffer.buffer[seqNum].RetryCount
	buffer.mu.RUnlock()
	
	assert.True(t, newSentAt.After(originalSentAt), "SentAt should be updated")
	assert.Equal(t, 1, newRetryCount, "RetryCount should increment")
}

func TestTunnelSendBuffer_GetStats(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	// 发送3个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		_, err := buffer.Send(data, pkt)
		require.NoError(t, err)
	}
	
	// 确认1个包
	buffer.ConfirmPacket(1)
	
	// 重传1个包
	buffer.MarkResent(2)
	
	stats := buffer.GetStats()
	assert.Equal(t, uint64(3), stats["total_sent"])
	assert.Equal(t, uint64(1), stats["total_confirmed"])
	assert.Equal(t, uint64(1), stats["total_resent"])
	assert.Equal(t, uint64(2), stats["buffered_count"])
	assert.Equal(t, uint64(8), stats["buffer_size"])
	assert.Equal(t, uint64(4), stats["next_seq"])
	assert.Equal(t, uint64(1), stats["confirmed_seq"])
}

func TestTunnelSendBuffer_Reset(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	// 发送几个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		_, err := buffer.Send(data, pkt)
		require.NoError(t, err)
	}
	
	nextSeq := buffer.GetNextSeq()
	confirmedSeq := buffer.GetConfirmedSeq()
	
	// Reset
	buffer.Reset()
	
	assert.Equal(t, 0, buffer.GetBufferedCount())
	assert.Equal(t, 0, buffer.GetBufferSize())
	assert.Equal(t, nextSeq, buffer.GetNextSeq(), "Reset should preserve sequence numbers")
	assert.Equal(t, confirmedSeq, buffer.GetConfirmedSeq())
}

func TestTunnelSendBuffer_Clear(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	// 发送几个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		_, err := buffer.Send(data, pkt)
		require.NoError(t, err)
	}
	
	// Clear
	buffer.Clear()
	
	assert.Equal(t, 0, buffer.GetBufferedCount())
	assert.Equal(t, 0, buffer.GetBufferSize())
	assert.Equal(t, uint64(1), buffer.GetNextSeq(), "Clear should reset sequence numbers")
	assert.Equal(t, uint64(0), buffer.GetConfirmedSeq())
	
	stats := buffer.GetStats()
	assert.Equal(t, uint64(0), stats["total_sent"])
	assert.Equal(t, uint64(0), stats["total_confirmed"])
	assert.Equal(t, uint64(0), stats["total_resent"])
}

func TestTunnelSendBuffer_Concurrent(t *testing.T) {
	buffer := NewTunnelSendBuffer()
	
	// 并发发送
	done := make(chan bool, 2)
	
	go func() {
		for i := 0; i < 50; i++ {
			data := []byte("data1")
			pkt := &packet.TransferPacket{Payload: data}
			buffer.Send(data, pkt)
		}
		done <- true
	}()
	
	go func() {
		for i := 0; i < 50; i++ {
			data := []byte("data2")
			pkt := &packet.TransferPacket{Payload: data}
			buffer.Send(data, pkt)
		}
		done <- true
	}()
	
	<-done
	<-done
	
	// 验证没有数据竞争
	assert.Equal(t, 100, buffer.GetBufferedCount())
	assert.Equal(t, uint64(101), buffer.GetNextSeq())
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TunnelReceiveBuffer 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestNewTunnelReceiveBuffer(t *testing.T) {
	buffer := NewTunnelReceiveBuffer()
	
	assert.NotNil(t, buffer)
	assert.Equal(t, uint64(1), buffer.GetNextExpected())
	assert.Equal(t, 0, buffer.GetBufferedCount())
	assert.Equal(t, 0, buffer.GetBufferSize())
}

func TestTunnelReceiveBuffer_ReceiveInOrder(t *testing.T) {
	buffer := NewTunnelReceiveBuffer()
	
	// 按序接收3个包
	for i := 1; i <= 3; i++ {
		pkt := &packet.TransferPacket{
			SeqNum:  uint64(i),
			Payload: []byte("data"),
		}
		
		result, err := buffer.Receive(pkt)
		require.NoError(t, err)
		
		assert.Equal(t, 1, len(result), "Should return 1 data block")
		assert.Equal(t, []byte("data"), result[0])
	}
	
	assert.Equal(t, uint64(4), buffer.GetNextExpected())
	assert.Equal(t, 0, buffer.GetBufferedCount(), "No buffered packets for in-order delivery")
}

func TestTunnelReceiveBuffer_ReceiveOutOfOrder(t *testing.T) {
	buffer := NewTunnelReceiveBuffer()
	
	// 先接收 seqNum=3（乱序）
	pkt3 := &packet.TransferPacket{
		SeqNum:  3,
		Payload: []byte("data3"),
	}
	result, err := buffer.Receive(pkt3)
	require.NoError(t, err)
	assert.Nil(t, result, "Out-of-order packet should be buffered, not returned")
	assert.Equal(t, uint64(1), buffer.GetNextExpected(), "NextExpected should not change")
	assert.Equal(t, 1, buffer.GetBufferedCount())
	
	// 接收 seqNum=2（乱序）
	pkt2 := &packet.TransferPacket{
		SeqNum:  2,
		Payload: []byte("data2"),
	}
	result, err = buffer.Receive(pkt2)
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 2, buffer.GetBufferedCount())
	
	// 接收 seqNum=1（期望的包）
	pkt1 := &packet.TransferPacket{
		SeqNum:  1,
		Payload: []byte("data1"),
	}
	result, err = buffer.Receive(pkt1)
	require.NoError(t, err)
	
	// 应该返回3个连续的包
	assert.Equal(t, 3, len(result))
	assert.Equal(t, []byte("data1"), result[0])
	assert.Equal(t, []byte("data2"), result[1])
	assert.Equal(t, []byte("data3"), result[2])
	assert.Equal(t, uint64(4), buffer.GetNextExpected())
	assert.Equal(t, 0, buffer.GetBufferedCount(), "All buffered packets should be consumed")
}

func TestTunnelReceiveBuffer_ReceiveDuplicate(t *testing.T) {
	buffer := NewTunnelReceiveBuffer()
	
	// 接收 seqNum=1
	pkt1 := &packet.TransferPacket{
		SeqNum:  1,
		Payload: []byte("data1"),
	}
	result, err := buffer.Receive(pkt1)
	require.NoError(t, err)
	assert.Equal(t, 1, len(result))
	
	// 再次接收 seqNum=1（重复）
	result, err = buffer.Receive(pkt1)
	require.NoError(t, err)
	assert.Nil(t, result, "Duplicate packet should be discarded")
	assert.Equal(t, uint64(2), buffer.GetNextExpected())
}

func TestTunnelReceiveBuffer_MaxOutOfOrder(t *testing.T) {
	// 创建只允许2个乱序包的缓冲区
	buffer := NewTunnelReceiveBufferWithConfig(2)
	
	// 发送 seqNum=2, 3（乱序）
	pkt2 := &packet.TransferPacket{SeqNum: 2, Payload: []byte("data2")}
	_, err := buffer.Receive(pkt2)
	require.NoError(t, err)
	
	pkt3 := &packet.TransferPacket{SeqNum: 3, Payload: []byte("data3")}
	_, err = buffer.Receive(pkt3)
	require.NoError(t, err)
	
	// 发送 seqNum=4（超过限制）
	pkt4 := &packet.TransferPacket{SeqNum: 4, Payload: []byte("data4")}
	_, err = buffer.Receive(pkt4)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many out-of-order packets")
}

func TestTunnelReceiveBuffer_PartialReordering(t *testing.T) {
	buffer := NewTunnelReceiveBuffer()
	
	// 接收乱序包：1, 3, 5
	pkts := []*packet.TransferPacket{
		{SeqNum: 1, Payload: []byte("data1")},
		{SeqNum: 3, Payload: []byte("data3")},
		{SeqNum: 5, Payload: []byte("data5")},
	}
	
	for _, pkt := range pkts {
		result, err := buffer.Receive(pkt)
		require.NoError(t, err)
		
		if pkt.SeqNum == 1 {
			assert.Equal(t, 1, len(result))
		} else {
			assert.Nil(t, result)
		}
	}
	
	assert.Equal(t, uint64(2), buffer.GetNextExpected())
	assert.Equal(t, 2, buffer.GetBufferedCount()) // 3和5在缓冲区
	
	// 接收 seqNum=2
	pkt2 := &packet.TransferPacket{SeqNum: 2, Payload: []byte("data2")}
	result, err := buffer.Receive(pkt2)
	require.NoError(t, err)
	
	// 应该返回 2 和 3（连续）
	assert.Equal(t, 2, len(result))
	assert.Equal(t, []byte("data2"), result[0])
	assert.Equal(t, []byte("data3"), result[1])
	assert.Equal(t, uint64(4), buffer.GetNextExpected())
	assert.Equal(t, 1, buffer.GetBufferedCount()) // 只剩5在缓冲区
}

func TestTunnelReceiveBuffer_GetStats(t *testing.T) {
	buffer := NewTunnelReceiveBuffer()
	
	// 接收顺序包
	pkt1 := &packet.TransferPacket{SeqNum: 1, Payload: []byte("data")}
	buffer.Receive(pkt1)
	
	// 接收乱序包
	pkt3 := &packet.TransferPacket{SeqNum: 3, Payload: []byte("data")}
	buffer.Receive(pkt3)
	
	// 接收期望包（触发重组）
	pkt2 := &packet.TransferPacket{SeqNum: 2, Payload: []byte("data")}
	buffer.Receive(pkt2)
	
	stats := buffer.GetStats()
	assert.Equal(t, uint64(3), stats["total_received"])
	assert.Equal(t, uint64(1), stats["total_out_of_order"])
	assert.Equal(t, uint64(1), stats["total_reordered"])
	assert.Equal(t, uint64(0), stats["buffered_count"])
	assert.Equal(t, uint64(4), stats["next_expected"])
}

func TestTunnelReceiveBuffer_ResetAndClear(t *testing.T) {
	buffer := NewTunnelReceiveBuffer()
	
	// 接收一些包
	pkt1 := &packet.TransferPacket{SeqNum: 1, Payload: []byte("data")}
	buffer.Receive(pkt1)
	
	pkt3 := &packet.TransferPacket{SeqNum: 3, Payload: []byte("data")}
	buffer.Receive(pkt3)
	
	nextExpected := buffer.GetNextExpected()
	
	// Reset
	buffer.Reset()
	assert.Equal(t, 0, buffer.GetBufferedCount())
	assert.Equal(t, nextExpected, buffer.GetNextExpected(), "Reset should preserve sequence")
	
	// Clear
	buffer.Clear()
	assert.Equal(t, uint64(1), buffer.GetNextExpected(), "Clear should reset sequence")
	
	stats := buffer.GetStats()
	assert.Equal(t, uint64(0), stats["total_received"])
}

func TestTunnelReceiveBuffer_Concurrent(t *testing.T) {
	buffer := NewTunnelReceiveBuffer()
	
	done := make(chan bool, 2)
	
	// 并发接收奇数包
	go func() {
		for i := 1; i <= 50; i += 2 {
			pkt := &packet.TransferPacket{
				SeqNum:  uint64(i),
				Payload: []byte("odd"),
			}
			buffer.Receive(pkt)
		}
		done <- true
	}()
	
	// 并发接收偶数包
	go func() {
		for i := 2; i <= 50; i += 2 {
			pkt := &packet.TransferPacket{
				SeqNum:  uint64(i),
				Payload: []byte("even"),
			}
			buffer.Receive(pkt)
		}
		done <- true
	}()
	
	<-done
	<-done
	
	// 所有包应该都接收了
	assert.Equal(t, uint64(51), buffer.GetNextExpected())
	assert.Equal(t, 0, buffer.GetBufferedCount())
}


