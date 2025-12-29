package session

import (
	"context"
	"testing"
	"time"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TunnelStateManager 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestSaveAndLoadState(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewTunnelStateManager(memStorage, "test-secret")

	// 创建状态
	state := &TunnelState{
		TunnelID:        "tunnel-123",
		MappingID:       "mapping-abc",
		ListenClientID:  101,
		TargetClientID:  102,
		LastSeqNum:      100,
		LastAckNum:      50,
		NextExpectedSeq: 51,
		BufferedPackets: []BufferedState{
			{SeqNum: 101, Data: []byte("data1"), SentAt: time.Now().Unix()},
			{SeqNum: 102, Data: []byte("data2"), SentAt: time.Now().Unix()},
		},
	}

	// 保存状态
	err := manager.SaveState(state)
	require.NoError(t, err)
	assert.NotEmpty(t, state.Signature, "Signature should be generated")

	// 加载状态
	loadedState, err := manager.LoadState("tunnel-123")
	require.NoError(t, err)
	require.NotNil(t, loadedState)

	// 验证字段
	assert.Equal(t, state.TunnelID, loadedState.TunnelID)
	assert.Equal(t, state.MappingID, loadedState.MappingID)
	assert.Equal(t, state.ListenClientID, loadedState.ListenClientID)
	assert.Equal(t, state.TargetClientID, loadedState.TargetClientID)
	assert.Equal(t, state.LastSeqNum, loadedState.LastSeqNum)
	assert.Equal(t, state.LastAckNum, loadedState.LastAckNum)
	assert.Equal(t, state.NextExpectedSeq, loadedState.NextExpectedSeq)
	assert.Equal(t, len(state.BufferedPackets), len(loadedState.BufferedPackets))
	assert.Equal(t, state.Signature, loadedState.Signature)
}

func TestLoadState_NotFound(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewTunnelStateManager(memStorage, "test-secret")

	// 加载不存在的状态
	_, err := manager.LoadState("non-existent")
	assert.Error(t, err)
}

func TestLoadState_SignatureMismatch(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	// 使用不同的密钥
	manager1 := NewTunnelStateManager(memStorage, "secret-1")
	manager2 := NewTunnelStateManager(memStorage, "secret-2")

	// manager1保存状态
	state := &TunnelState{
		TunnelID:       "tunnel-123",
		MappingID:      "mapping-abc",
		ListenClientID: 101,
		TargetClientID: 102,
		LastSeqNum:     100,
	}
	err := manager1.SaveState(state)
	require.NoError(t, err)

	// manager2加载状态（密钥不同，签名验证失败）
	_, err = manager2.LoadState("tunnel-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature mismatch")
}

func TestDeleteState(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	manager := NewTunnelStateManager(memStorage, "test-secret")

	// 保存状态
	state := &TunnelState{
		TunnelID:  "tunnel-123",
		MappingID: "mapping-abc",
	}
	err := manager.SaveState(state)
	require.NoError(t, err)

	// 删除状态
	err = manager.DeleteState("tunnel-123")
	assert.NoError(t, err)

	// 加载应该失败
	_, err = manager.LoadState("tunnel-123")
	assert.Error(t, err)
}

func TestCaptureSendBufferState(t *testing.T) {
	sendBuffer := NewTunnelSendBuffer()

	// 发送几个包
	for i := 0; i < 3; i++ {
		data := []byte("data")
		pkt := &packet.TransferPacket{Payload: data}
		sendBuffer.Send(data, pkt)
	}

	// 捕获状态
	bufferedStates := CaptureSendBufferState(sendBuffer)
	assert.Equal(t, 3, len(bufferedStates))

	// 验证序列号
	for i, state := range bufferedStates {
		assert.Contains(t, []uint64{1, 2, 3}, state.SeqNum)
		assert.Equal(t, []byte("data"), state.Data)
		assert.Greater(t, state.SentAt, int64(0))
		_ = i // 避免未使用警告
	}
}

func TestRestoreToSendBuffer(t *testing.T) {
	sendBuffer := NewTunnelSendBuffer()

	// 准备状态数据
	bufferedStates := []BufferedState{
		{SeqNum: 5, Data: []byte("data5"), SentAt: time.Now().Unix()},
		{SeqNum: 6, Data: []byte("data6"), SentAt: time.Now().Unix()},
	}

	// 恢复到缓冲区
	RestoreToSendBuffer(sendBuffer, bufferedStates)

	assert.Equal(t, 2, sendBuffer.GetBufferedCount())
	assert.Equal(t, 10, sendBuffer.GetBufferSize()) // 2 * 5 bytes

	// 验证包是否存在（通过公共 API）
	sendBuffer.RLock()
	assert.NotNil(t, sendBuffer.Buffer[5])
	assert.NotNil(t, sendBuffer.Buffer[6])
	assert.Equal(t, []byte("data5"), sendBuffer.Buffer[5].Data)
	assert.Equal(t, []byte("data6"), sendBuffer.Buffer[6].Data)
	sendBuffer.RUnlock()
}

func TestTunnelState_FullCycle(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	// 创建发送缓冲区
	sendBuffer := NewTunnelSendBuffer()

	// 发送数据
	for i := 0; i < 5; i++ {
		data := []byte("test-data")
		pkt := &packet.TransferPacket{Payload: data}
		sendBuffer.Send(data, pkt)
	}

	// 确认部分数据
	sendBuffer.ConfirmUpTo(3) // 确认1, 2

	// 捕获状态
	bufferedStates := CaptureSendBufferState(sendBuffer)

	// 创建隧道状态
	state := &TunnelState{
		TunnelID:        "tunnel-full-cycle",
		MappingID:       "mapping-123",
		ListenClientID:  201,
		TargetClientID:  202,
		LastSeqNum:      sendBuffer.GetNextSeq() - 1,
		LastAckNum:      sendBuffer.GetConfirmedSeq(),
		NextExpectedSeq: 1,
		BufferedPackets: bufferedStates,
	}

	// 保存状态
	err := stateManager.SaveState(state)
	require.NoError(t, err)

	// 加载状态
	loadedState, err := stateManager.LoadState("tunnel-full-cycle")
	require.NoError(t, err)

	// 验证状态
	assert.Equal(t, state.TunnelID, loadedState.TunnelID)
	assert.Equal(t, state.LastSeqNum, loadedState.LastSeqNum)
	assert.Equal(t, state.LastAckNum, loadedState.LastAckNum)
	assert.Equal(t, len(bufferedStates), len(loadedState.BufferedPackets))

	// 恢复到新的缓冲区
	newSendBuffer := NewTunnelSendBuffer()
	RestoreToSendBuffer(newSendBuffer, loadedState.BufferedPackets)

	assert.Equal(t, sendBuffer.GetBufferedCount(), newSendBuffer.GetBufferedCount())
}
