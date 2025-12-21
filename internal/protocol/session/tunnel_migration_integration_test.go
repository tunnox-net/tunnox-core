package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Phase 2.5 集成测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestSaveActiveTunnelStates(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	// 创建TunnelStateManager
	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	// 创建SessionManager（简化版）
	sessionMgr := &SessionManager{
		tunnelConnMap:      make(map[string]*TunnelConnection),
		tunnelStateManager: stateManager,
		nodeID:             "test-node-123",
	}

	// 添加一些测试隧道
	tunnel1 := &TunnelConnection{
		TunnelID:      "tunnel-1",
		MappingID:     "mapping-abc",
		CreatedAt:     time.Now(),
		LastActiveAt:  time.Now(),
		sendBuffer:    NewTunnelSendBuffer(),
		receiveBuffer: NewTunnelReceiveBuffer(),
		enableSeqNum:  false, // 未启用序列号
	}

	tunnel2 := &TunnelConnection{
		TunnelID:      "tunnel-2",
		MappingID:     "mapping-xyz",
		CreatedAt:     time.Now(),
		LastActiveAt:  time.Now(),
		sendBuffer:    NewTunnelSendBuffer(),
		receiveBuffer: NewTunnelReceiveBuffer(),
		enableSeqNum:  true, // 启用序列号
	}

	// 为tunnel2发送一些数据（模拟活跃隧道）
	tunnel2.sendBuffer.Send([]byte("test-data-1"), nil)
	tunnel2.sendBuffer.Send([]byte("test-data-2"), nil)

	sessionMgr.tunnelConnMap["tunnel-1"] = tunnel1
	sessionMgr.tunnelConnMap["tunnel-2"] = tunnel2

	// 保存状态
	err := sessionMgr.SaveActiveTunnelStates()
	require.NoError(t, err)

	// 验证状态已保存
	state1, err := stateManager.LoadState("tunnel-1")
	require.NoError(t, err)
	assert.Equal(t, "tunnel-1", state1.TunnelID)
	assert.Equal(t, "mapping-abc", state1.MappingID)

	state2, err := stateManager.LoadState("tunnel-2")
	require.NoError(t, err)
	assert.Equal(t, "tunnel-2", state2.TunnelID)
	assert.Equal(t, "mapping-xyz", state2.MappingID)

	// tunnel2启用了序列号，应该有序列号状态
	assert.Greater(t, state2.LastSeqNum, uint64(0), "Should have sequence number state")
}

func TestGenerateTunnelResumeToken(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := &SessionManager{
		tunnelStateManager: stateManager,
	}

	// 先保存一个隧道状态
	tunnelState := &TunnelState{
		TunnelID:  "tunnel-resume-test",
		MappingID: "mapping-123",
	}
	err := stateManager.SaveState(tunnelState)
	require.NoError(t, err)

	// 生成恢复Token
	token, err := sessionMgr.GenerateTunnelResumeToken("tunnel-resume-test")
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// 验证Token是JSON格式
	var resumeToken TunnelResumeToken
	err = json.Unmarshal([]byte(token), &resumeToken)
	require.NoError(t, err)
	assert.Equal(t, "tunnel-resume-test", resumeToken.TunnelID)
	assert.NotEmpty(t, resumeToken.Signature)
}

func TestValidateTunnelResumeToken(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := &SessionManager{
		tunnelStateManager: stateManager,
	}

	// 1. 保存隧道状态
	tunnelState := &TunnelState{
		TunnelID:  "tunnel-validate-test",
		MappingID: "mapping-456",
	}
	err := stateManager.SaveState(tunnelState)
	require.NoError(t, err)

	// 2. 生成Token
	token, err := sessionMgr.GenerateTunnelResumeToken("tunnel-validate-test")
	require.NoError(t, err)

	// 3. 验证Token
	loadedState, err := sessionMgr.ValidateTunnelResumeToken(token)
	require.NoError(t, err)
	assert.Equal(t, "tunnel-validate-test", loadedState.TunnelID)
	assert.Equal(t, "mapping-456", loadedState.MappingID)
}

func TestValidateTunnelResumeToken_Invalid(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := &SessionManager{
		tunnelStateManager: stateManager,
	}

	// 无效的Token格式
	_, err := sessionMgr.ValidateTunnelResumeToken("invalid-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resume token format")
}

func TestValidateTunnelResumeToken_SignatureMismatch(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := &SessionManager{
		tunnelStateManager: stateManager,
	}

	// 1. 保存隧道状态
	tunnelState := &TunnelState{
		TunnelID:  "tunnel-sig-test",
		MappingID: "mapping-789",
	}
	err := stateManager.SaveState(tunnelState)
	require.NoError(t, err)

	// 2. 创建一个签名不匹配的Token
	fakeToken := TunnelResumeToken{
		TunnelID:  "tunnel-sig-test",
		Signature: "fake-signature",
		IssuedAt:  time.Now().Unix(),
	}
	tokenJSON, _ := json.Marshal(fakeToken)

	// 3. 验证应该失败
	_, err = sessionMgr.ValidateTunnelResumeToken(string(tokenJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature mismatch")
}

func TestValidateTunnelResumeToken_Expired(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := &SessionManager{
		tunnelStateManager: stateManager,
	}

	// 1. 保存隧道状态
	tunnelState := &TunnelState{
		TunnelID:  "tunnel-expire-test",
		MappingID: "mapping-999",
	}
	err := stateManager.SaveState(tunnelState)
	require.NoError(t, err)

	// 2. 创建一个过期的Token（6分钟前）
	expiredToken := TunnelResumeToken{
		TunnelID:  "tunnel-expire-test",
		Signature: tunnelState.Signature,
		IssuedAt:  time.Now().Add(-6 * time.Minute).Unix(),
	}
	tokenJSON, _ := json.Marshal(expiredToken)

	// 3. 验证应该失败
	_, err = sessionMgr.ValidateTunnelResumeToken(string(tokenJSON))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestTunnelResumeFlow_EndToEnd(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")
	migrationManager := NewTunnelMigrationManager(stateManager, nil)

	sessionMgr := &SessionManager{
		tunnelConnMap:      make(map[string]*TunnelConnection),
		tunnelStateManager: stateManager,
		migrationManager:   migrationManager,
		nodeID:             "node-A",
	}

	// ========== 阶段1: 服务器A运行，有活跃隧道 ==========
	tunnel := &TunnelConnection{
		TunnelID:      "tunnel-e2e",
		MappingID:     "mapping-e2e",
		CreatedAt:     time.Now(),
		LastActiveAt:  time.Now(),
		sendBuffer:    NewTunnelSendBuffer(),
		receiveBuffer: NewTunnelReceiveBuffer(),
		enableSeqNum:  true,
	}

	// 发送一些数据
	tunnel.sendBuffer.Send([]byte("data-before-shutdown"), nil)
	sessionMgr.tunnelConnMap["tunnel-e2e"] = tunnel

	// ========== 阶段2: 服务器A关闭，保存状态 ==========
	err := sessionMgr.SaveActiveTunnelStates()
	require.NoError(t, err)

	// 生成恢复Token
	resumeToken, err := sessionMgr.GenerateTunnelResumeToken("tunnel-e2e")
	require.NoError(t, err)
	assert.NotEmpty(t, resumeToken)

	// ========== 阶段3: 客户端重连到服务器B ==========
	// 服务器B使用相同的存储
	sessionMgrB := &SessionManager{
		tunnelStateManager: stateManager,
		nodeID:             "node-B",
	}

	// 客户端提供ResumeToken
	loadedState, err := sessionMgrB.ValidateTunnelResumeToken(resumeToken)
	require.NoError(t, err)

	// 验证恢复的状态
	assert.Equal(t, "tunnel-e2e", loadedState.TunnelID)
	assert.Equal(t, "mapping-e2e", loadedState.MappingID)
	assert.Greater(t, loadedState.LastSeqNum, uint64(0))

	// ========== 阶段4: 服务器B恢复隧道 ==========
	// 注意：缓冲区恢复功能已实现（见 session.RestoreToSendBuffer），
	// 如需测试缓冲区恢复，可在此处调用：session.RestoreToSendBuffer(newTunnel.sendBuffer, loadedState.BufferedPackets)

	corelog.Infof("End-to-end tunnel resume test completed successfully")
}
