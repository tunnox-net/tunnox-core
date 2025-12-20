package session

import (
	"context"
	"fmt"
	"testing"
	"time"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/packet"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// TunnelMigrationManager 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func createMigrationTestEnv() (*TunnelMigrationManager, *TunnelStateManager, context.Context) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	stateManager := NewTunnelStateManager(memStorage, "test-secret")

	// 创建一个简单的SessionManager（只需要NodeID）
	sessionMgr := &SessionManager{
		nodeID: "source-node-123",
	}

	migrationMgr := NewTunnelMigrationManager(stateManager, sessionMgr)

	return migrationMgr, stateManager, ctx
}

func TestInitiateMigration(t *testing.T) {
	migrationMgr, _, ctx := createMigrationTestEnv()

	tunnelState := &TunnelState{
		TunnelID:        "tunnel-migrate-1",
		MappingID:       "mapping-123",
		ListenClientID:  201,
		TargetClientID:  202,
		LastSeqNum:      100,
		LastAckNum:      50,
		NextExpectedSeq: 51,
	}

	err := migrationMgr.InitiateMigration(ctx, "tunnel-migrate-1", "target-node-456", tunnelState)
	require.NoError(t, err)

	// 验证迁移信息
	info, err := migrationMgr.GetMigrationInfo("tunnel-migrate-1")
	require.NoError(t, err)

	assert.Equal(t, "tunnel-migrate-1", info.TunnelID)
	assert.Equal(t, MigrationStatusInProgress, info.Status)
	assert.Equal(t, "source-node-123", info.SourceNodeID)
	assert.Equal(t, "target-node-456", info.TargetNodeID)
}

func TestInitiateMigration_AlreadyInProgress(t *testing.T) {
	migrationMgr, _, ctx := createMigrationTestEnv()

	tunnelState := &TunnelState{
		TunnelID:  "tunnel-migrate-2",
		MappingID: "mapping-123",
	}

	// 第一次发起
	err := migrationMgr.InitiateMigration(ctx, "tunnel-migrate-2", "target-node-456", tunnelState)
	require.NoError(t, err)

	// 第二次发起应该失败
	err = migrationMgr.InitiateMigration(ctx, "tunnel-migrate-2", "target-node-789", tunnelState)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already migrating")
}

func TestAcceptMigration(t *testing.T) {
	migrationMgr, stateManager, ctx := createMigrationTestEnv()

	// 先保存状态（模拟源节点操作）
	tunnelState := &TunnelState{
		TunnelID:        "tunnel-migrate-3",
		MappingID:       "mapping-123",
		ListenClientID:  301,
		TargetClientID:  302,
		LastSeqNum:      200,
		LastAckNum:      100,
		NextExpectedSeq: 101,
	}

	err := stateManager.SaveState(tunnelState)
	require.NoError(t, err)

	// 创建迁移命令
	migrateCmd := &packet.TunnelMigrateCommand{
		TunnelID:       "tunnel-migrate-3",
		MappingID:      "mapping-123",
		SourceNodeID:   "source-node-123",
		TargetNodeID:   "target-node-456",
		StateSignature: tunnelState.Signature,
	}

	// 接受迁移（模拟目标节点操作）
	loadedState, err := migrationMgr.AcceptMigration(ctx, migrateCmd)
	require.NoError(t, err)
	require.NotNil(t, loadedState)

	assert.Equal(t, tunnelState.TunnelID, loadedState.TunnelID)
	assert.Equal(t, tunnelState.LastSeqNum, loadedState.LastSeqNum)
	assert.Equal(t, tunnelState.LastAckNum, loadedState.LastAckNum)
}

func TestAcceptMigration_SignatureMismatch(t *testing.T) {
	migrationMgr, stateManager, ctx := createMigrationTestEnv()

	// 保存状态
	tunnelState := &TunnelState{
		TunnelID:  "tunnel-migrate-4",
		MappingID: "mapping-123",
	}
	err := stateManager.SaveState(tunnelState)
	require.NoError(t, err)

	// 创建迁移命令，但签名不匹配
	migrateCmd := &packet.TunnelMigrateCommand{
		TunnelID:       "tunnel-migrate-4",
		SourceNodeID:   "source-node-123",
		TargetNodeID:   "target-node-456",
		StateSignature: "wrong-signature",
	}

	// 应该失败
	_, err = migrationMgr.AcceptMigration(ctx, migrateCmd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signature mismatch")
}

func TestCompleteMigration(t *testing.T) {
	migrationMgr, _, ctx := createMigrationTestEnv()

	// 先发起迁移
	tunnelState := &TunnelState{
		TunnelID:  "tunnel-migrate-5",
		MappingID: "mapping-123",
	}
	err := migrationMgr.InitiateMigration(ctx, "tunnel-migrate-5", "target-node-456", tunnelState)
	require.NoError(t, err)

	// 完成迁移
	err = migrationMgr.CompleteMigration("tunnel-migrate-5")
	require.NoError(t, err)

	// 验证状态
	info, err := migrationMgr.GetMigrationInfo("tunnel-migrate-5")
	require.NoError(t, err)

	assert.Equal(t, MigrationStatusCompleted, info.Status)
	assert.NotNil(t, info.CompletedAt)
}

func TestFailMigration(t *testing.T) {
	migrationMgr, _, ctx := createMigrationTestEnv()

	// 先发起迁移
	tunnelState := &TunnelState{
		TunnelID:  "tunnel-migrate-6",
		MappingID: "mapping-123",
	}
	err := migrationMgr.InitiateMigration(ctx, "tunnel-migrate-6", "target-node-456", tunnelState)
	require.NoError(t, err)

	// 标记失败
	migrationMgr.FailMigration("tunnel-migrate-6", assert.AnError)

	// 验证状态
	info, err := migrationMgr.GetMigrationInfo("tunnel-migrate-6")
	require.NoError(t, err)

	assert.Equal(t, MigrationStatusFailed, info.Status)
	assert.NotEmpty(t, info.Error)
}

func TestIsMigrating(t *testing.T) {
	migrationMgr, _, ctx := createMigrationTestEnv()

	// 初始没有迁移
	assert.False(t, migrationMgr.IsMigrating("tunnel-migrate-7"))

	// 发起迁移
	tunnelState := &TunnelState{
		TunnelID:  "tunnel-migrate-7",
		MappingID: "mapping-123",
	}
	err := migrationMgr.InitiateMigration(ctx, "tunnel-migrate-7", "target-node-456", tunnelState)
	require.NoError(t, err)

	// 现在应该在迁移中
	assert.True(t, migrationMgr.IsMigrating("tunnel-migrate-7"))

	// 完成后不再迁移中
	migrationMgr.CompleteMigration("tunnel-migrate-7")
	assert.False(t, migrationMgr.IsMigrating("tunnel-migrate-7"))
}

func TestGetActiveMigrations(t *testing.T) {
	migrationMgr, _, ctx := createMigrationTestEnv()

	// 发起多个迁移
	for i := 1; i <= 3; i++ {
		tunnelState := &TunnelState{
			TunnelID:  fmt.Sprintf("tunnel-migrate-%d", i),
			MappingID: "mapping-123",
		}
		migrationMgr.InitiateMigration(ctx, tunnelState.TunnelID, "target-node-456", tunnelState)
	}

	// 完成第一个
	migrationMgr.CompleteMigration("tunnel-migrate-1")

	// 获取活跃迁移
	active := migrationMgr.GetActiveMigrations()
	assert.Equal(t, 2, len(active), "Should have 2 active migrations")
}

func TestCleanupOldMigrations(t *testing.T) {
	migrationMgr, _, _ := createMigrationTestEnv()

	// 手动添加一个旧的已完成迁移
	oldTime := time.Now().Add(-2 * time.Hour)
	completedTime := oldTime
	migrationMgr.migrations["old-tunnel"] = &TunnelMigrationInfo{
		TunnelID:    "old-tunnel",
		Status:      MigrationStatusCompleted,
		InitiatedAt: oldTime,
		CompletedAt: &completedTime,
	}

	// 添加一个新的迁移
	recentTime := time.Now()
	migrationMgr.migrations["recent-tunnel"] = &TunnelMigrationInfo{
		TunnelID:    "recent-tunnel",
		Status:      MigrationStatusInProgress,
		InitiatedAt: recentTime,
	}

	// 清理
	migrationMgr.CleanupOldMigrations()

	// 验证旧的被删除，新的保留
	_, err := migrationMgr.GetMigrationInfo("old-tunnel")
	assert.Error(t, err, "Old migration should be cleaned up")

	_, err = migrationMgr.GetMigrationInfo("recent-tunnel")
	assert.NoError(t, err, "Recent migration should be kept")
}
