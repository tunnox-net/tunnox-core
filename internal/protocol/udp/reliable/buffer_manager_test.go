package reliable

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBufferManager_SendBuffer(t *testing.T) {
	bm := NewBufferManager()

	// 添加到发送缓冲区
	packet := &Packet{
		Header: &PacketHeader{
			SequenceNum: 1,
		},
	}

	err := bm.AddSendBuffer(1, packet)
	require.NoError(t, err)

	// 获取
	entry := bm.GetSendBuffer(1)
	assert.NotNil(t, entry)
	assert.Equal(t, packet, entry.Packet)
	assert.False(t, entry.Acked)

	// 标记为已确认
	bm.MarkAcked(1)
	entry = bm.GetSendBuffer(1)
	assert.True(t, entry.Acked)

	// 移除
	bm.RemoveSendBuffer(1)
	entry = bm.GetSendBuffer(1)
	assert.Nil(t, entry)
}

func TestBufferManager_RecvBuffer(t *testing.T) {
	bm := NewBufferManager()

	// 添加乱序的包
	for i := uint32(5); i > 0; i-- {
		packet := &Packet{
			Header: &PacketHeader{
				SequenceNum: i,
			},
		}
		err := bm.AddRecvBuffer(i, packet)
		require.NoError(t, err)
	}

	// 获取有序的包
	packets := bm.GetOrderedRecvPackets(1)
	assert.Equal(t, 5, len(packets))
	for i, pkt := range packets {
		assert.Equal(t, uint32(i+1), pkt.Header.SequenceNum)
	}
}

func TestBufferManager_BufferFull(t *testing.T) {
	bm := NewBufferManager()
	bm.maxSendSize = 10

	// 填满缓冲区
	for i := uint32(0); i < 10; i++ {
		packet := &Packet{
			Header: &PacketHeader{
				SequenceNum: i,
			},
		}
		err := bm.AddSendBuffer(i, packet)
		require.NoError(t, err)
	}

	// 再添加应该失败
	packet := &Packet{
		Header: &PacketHeader{
			SequenceNum: 10,
		},
	}
	err := bm.AddSendBuffer(10, packet)
	assert.Error(t, err)
	assert.Equal(t, ErrBufferFull, err)
}

func TestBufferManager_Cleanup(t *testing.T) {
	bm := NewBufferManager()

	// 添加一些包
	for i := uint32(0); i < 5; i++ {
		packet := &Packet{
			Header: &PacketHeader{
				SequenceNum: i,
			},
		}
		err := bm.AddSendBuffer(i, packet)
		require.NoError(t, err)
	}

	// 标记一些为已确认
	bm.MarkAcked(0)
	bm.MarkAcked(1)

	// 等待一段时间
	time.Sleep(100 * time.Millisecond)

	// 清理
	cleaned := bm.Cleanup(50 * time.Millisecond)
	assert.Equal(t, 2, cleaned)

	// 未确认的包应该还在
	sendSize, _ := bm.GetStats()
	assert.Equal(t, 3, sendSize)
}

func TestBufferManager_GetUnackedPackets(t *testing.T) {
	bm := NewBufferManager()

	// 添加包
	for i := uint32(0); i < 5; i++ {
		packet := &Packet{
			Header: &PacketHeader{
				SequenceNum: i,
			},
		}
		err := bm.AddSendBuffer(i, packet)
		require.NoError(t, err)
	}

	// 标记一些为已确认
	bm.MarkAcked(0)
	bm.MarkAcked(2)

	// 获取未确认的包
	unacked := bm.GetUnackedPackets()
	assert.Equal(t, 3, len(unacked))
}

func TestBufferManager_Stats(t *testing.T) {
	bm := NewBufferManager()

	// 添加发送缓冲区
	for i := uint32(0); i < 3; i++ {
		packet := &Packet{
			Header: &PacketHeader{
				SequenceNum: i,
			},
		}
		err := bm.AddSendBuffer(i, packet)
		require.NoError(t, err)
	}

	// 添加接收缓冲区
	for i := uint32(0); i < 5; i++ {
		packet := &Packet{
			Header: &PacketHeader{
				SequenceNum: i,
			},
		}
		err := bm.AddRecvBuffer(i, packet)
		require.NoError(t, err)
	}

	sendSize, recvSize := bm.GetStats()
	assert.Equal(t, 3, sendSize)
	assert.Equal(t, 5, recvSize)
}
