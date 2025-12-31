package session

import (
	"context"
	"testing"

	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
)

// createTestIDManagerForManager 创建测试用的ID管理器
func createTestIDManagerForManager() *idgen.IDManager {
	stor := storage.NewMemoryStorage(context.Background())
	return idgen.NewIDManager(stor, context.Background())
}

func TestSessionManager_GetStreamFactory(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	factory := sm.GetStreamFactory()
	assert.NotNil(t, factory)
}

func TestSessionManager_GetStreamManager(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	streamMgr := sm.GetStreamManager()
	assert.NotNil(t, streamMgr)
}

func TestSessionManager_GetActiveConnections(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 初始状态应该为0
	count := sm.GetActiveConnections()
	assert.Equal(t, 0, count)
}

func TestSessionManager_Close(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())

	// 关闭
	result := sm.Close()
	assert.False(t, result.HasErrors())
}

func TestSessionManager_CloseMultipleTimes(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())

	// 第一次关闭
	result1 := sm.Close()
	assert.False(t, result1.HasErrors())

	// 第二次关闭（应该是幂等的）
	result2 := sm.Close()
	assert.False(t, result2.HasErrors())
}

func TestWithNodeID(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithNodeID 选项
	opt := WithNodeID("test-node-123")
	opt(sm)

	assert.Equal(t, "test-node-123", sm.nodeID)
}

func TestWithAuthHandler(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithAuthHandler 选项（nil）
	opt := WithAuthHandler(nil)
	opt(sm)

	assert.Nil(t, sm.authHandler)
}

func TestWithTunnelHandler(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithTunnelHandler 选项（nil）
	opt := WithTunnelHandler(nil)
	opt(sm)

	assert.Nil(t, sm.tunnelHandler)
}

func TestWithCloudControl(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithCloudControl 选项（nil）
	opt := WithCloudControl(nil)
	opt(sm)

	assert.Nil(t, sm.cloudControl)
}

func TestWithCommandRegistry(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithCommandRegistry 选项（nil）
	opt := WithCommandRegistry(nil)
	opt(sm)

	assert.Nil(t, sm.commandRegistry)
}

func TestWithCommandExecutor(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithCommandExecutor 选项（nil）
	opt := WithCommandExecutor(nil)
	opt(sm)

	assert.Nil(t, sm.commandExecutor)
}

func TestWithReconnectTokenManager(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithReconnectTokenManager 选项（nil）
	opt := WithReconnectTokenManager(nil)
	opt(sm)

	assert.Nil(t, sm.reconnectTokenManager)
}

func TestWithSessionTokenManager(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithSessionTokenManager 选项（nil）
	opt := WithSessionTokenManager(nil)
	opt(sm)

	assert.Nil(t, sm.sessionTokenManager)
}

func TestWithTunnelStateManager(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithTunnelStateManager 选项（nil）
	opt := WithTunnelStateManager(nil)
	opt(sm)

	assert.Nil(t, sm.tunnelStateManager)
}

func TestWithMigrationManager(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithMigrationManager 选项（nil）
	opt := WithMigrationManager(nil)
	opt(sm)

	assert.Nil(t, sm.migrationManager)
}

func TestWithEventBus(t *testing.T) {
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, context.Background())
	defer sm.Close()

	// 应用 WithEventBus 选项（nil）
	opt := WithEventBus(nil)
	opt(sm)

	assert.Nil(t, sm.eventBus)
}

func TestWithTunnelRoutingTable(t *testing.T) {
	ctx := context.Background()
	idManager := createTestIDManagerForManager()
	defer idManager.Close()

	sm := NewSessionManager(idManager, ctx)
	defer sm.Close()

	// 使用内存存储创建路由表
	stor := storage.NewMemoryStorage(ctx)
	rt := NewTunnelRoutingTable(stor, 0)

	// 应用 WithTunnelRoutingTable 选项
	opt := WithTunnelRoutingTable(rt)
	opt(sm)

	assert.NotNil(t, sm.tunnelRouting)
	assert.Equal(t, rt, sm.tunnelRouting)
}
