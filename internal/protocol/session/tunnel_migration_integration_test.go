package session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Phase 2.5 集成测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestSaveActiveTunnelStates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	// 创建TunnelStateManager
	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	// 创建SessionManager（使用构造函数）
	sessionMgr := NewSessionManager(idManager, ctx)
	defer sessionMgr.Close()

	// 设置TunnelStateManager
	sessionMgr.SetTunnelStateManager(stateManager)

	// 添加一些测试隧道 - 使用构造函数创建并注册
	conn1ID, _ := idManager.GenerateConnectionID()
	tunnel1 := NewTunnelConnection(conn1ID, nil, nil, "tcp")
	tunnel1.TunnelID = "tunnel-1"
	tunnel1.MappingID = "mapping-abc"
	// tunnel1 不启用序列号（默认）
	sessionMgr.RegisterTunnelConnection(tunnel1)

	conn2ID, _ := idManager.GenerateConnectionID()
	tunnel2 := NewTunnelConnection(conn2ID, nil, nil, "tcp")
	tunnel2.TunnelID = "tunnel-2"
	tunnel2.MappingID = "mapping-xyz"
	tunnel2.EnableSequenceNumbers() // 启用序列号
	// 为tunnel2发送一些数据（模拟活跃隧道）
	tunnel2.GetSendBuffer().Send([]byte("test-data-1"), nil)
	tunnel2.GetSendBuffer().Send([]byte("test-data-2"), nil)
	sessionMgr.RegisterTunnelConnection(tunnel2)

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := NewSessionManager(idManager, ctx)
	defer sessionMgr.Close()
	sessionMgr.SetTunnelStateManager(stateManager)

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := NewSessionManager(idManager, ctx)
	defer sessionMgr.Close()
	sessionMgr.SetTunnelStateManager(stateManager)

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := NewSessionManager(idManager, ctx)
	defer sessionMgr.Close()
	sessionMgr.SetTunnelStateManager(stateManager)

	// 无效的Token格式
	_, err := sessionMgr.ValidateTunnelResumeToken("invalid-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resume token format")
}

func TestValidateTunnelResumeToken_SignatureMismatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := NewSessionManager(idManager, ctx)
	defer sessionMgr.Close()
	sessionMgr.SetTunnelStateManager(stateManager)

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	sessionMgr := NewSessionManager(idManager, ctx)
	defer sessionMgr.Close()
	sessionMgr.SetTunnelStateManager(stateManager)

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	stateManager := NewTunnelStateManager(memStorage, "test-secret")
	migrationManager := NewTunnelMigrationManager(stateManager, nil)

	// 创建 SessionManager A (服务器A)
	sessionMgrA := NewSessionManager(idManager, ctx)
	defer sessionMgrA.Close()
	sessionMgrA.SetTunnelStateManager(stateManager)
	sessionMgrA.SetMigrationManager(migrationManager)

	// ========== 阶段1: 服务器A运行，有活跃隧道 ==========
	connID, _ := idManager.GenerateConnectionID()
	tunnel := NewTunnelConnection(connID, nil, nil, "tcp")
	tunnel.TunnelID = "tunnel-e2e"
	tunnel.MappingID = "mapping-e2e"
	tunnel.EnableSequenceNumbers()

	// 发送一些数据
	tunnel.GetSendBuffer().Send([]byte("data-before-shutdown"), nil)
	sessionMgrA.RegisterTunnelConnection(tunnel)

	// ========== 阶段2: 服务器A关闭，保存状态 ==========
	err := sessionMgrA.SaveActiveTunnelStates()
	require.NoError(t, err)

	// 生成恢复Token
	resumeToken, err := sessionMgrA.GenerateTunnelResumeToken("tunnel-e2e")
	require.NoError(t, err)
	assert.NotEmpty(t, resumeToken)

	// ========== 阶段3: 客户端重连到服务器B ==========
	// 服务器B使用相同的存储
	sessionMgrB := NewSessionManager(idManager, ctx)
	defer sessionMgrB.Close()
	sessionMgrB.SetTunnelStateManager(stateManager)

	// 客户端提供ResumeToken
	loadedState, err := sessionMgrB.ValidateTunnelResumeToken(resumeToken)
	require.NoError(t, err)

	// 验证恢复的状态
	assert.Equal(t, "tunnel-e2e", loadedState.TunnelID)
	assert.Equal(t, "mapping-e2e", loadedState.MappingID)
	assert.Greater(t, loadedState.LastSeqNum, uint64(0))

	// ========== 阶段4: 服务器B恢复隧道 ==========
	// 注意：缓冲区恢复功能已实现（见 session.RestoreToSendBuffer），
	// 如需测试缓冲区恢复，可在此处调用：session.RestoreToSendBuffer(newTunnel.GetSendBuffer(), loadedState.BufferedPackets)

	corelog.Infof("End-to-end tunnel resume test completed successfully")
}
