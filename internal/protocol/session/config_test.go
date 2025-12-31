package session

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/core/idgen"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
)

// TestSessionManagerConfigValidate 测试配置验证
func TestSessionManagerConfigValidate(t *testing.T) {
	// 测试空 IDManager
	config := &SessionManagerConfig{}
	err := config.Validate()
	if err == nil {
		t.Error("Should return error when IDManager is nil")
	}

	// 测试有效配置
	ctx := context.Background()
	storageFactory := storage.NewStorageFactory(ctx)
	memStorage, _ := storageFactory.CreateStorage(&storage.MemoryStorageConfig{})
	idManager := idgen.NewIDManager(memStorage, ctx)

	config = &SessionManagerConfig{
		IDManager: idManager,
	}
	err = config.Validate()
	if err != nil {
		t.Errorf("Should not return error for valid config: %v", err)
	}
}

// TestSessionManagerConfigApplyDefaults 测试默认值应用
func TestSessionManagerConfigApplyDefaults(t *testing.T) {
	config := &SessionManagerConfig{}
	config.ApplyDefaults()

	if config.Logger == nil {
		t.Error("Logger should have default value")
	}

	if config.HeartbeatTimeout != 60*time.Second {
		t.Errorf("HeartbeatTimeout should be 60s, got %v", config.HeartbeatTimeout)
	}

	if config.CleanupInterval != 15*time.Second {
		t.Errorf("CleanupInterval should be 15s, got %v", config.CleanupInterval)
	}

	if config.MaxConnections != 10000 {
		t.Errorf("MaxConnections should be 10000, got %d", config.MaxConnections)
	}

	if config.MaxControlConnections != 5000 {
		t.Errorf("MaxControlConnections should be 5000, got %d", config.MaxControlConnections)
	}
}

// TestNewSessionManagerV2 测试新版构造函数
func TestNewSessionManagerV2(t *testing.T) {
	ctx := context.Background()
	storageFactory := storage.NewStorageFactory(ctx)
	memStorage, _ := storageFactory.CreateStorage(&storage.MemoryStorageConfig{})
	idManager := idgen.NewIDManager(memStorage, ctx)

	// 测试空配置
	_, err := NewSessionManagerV2(ctx, nil)
	if err == nil {
		t.Error("Should return error when config is nil")
	}

	// 测试缺少 IDManager
	_, err = NewSessionManagerV2(ctx, &SessionManagerConfig{})
	if err == nil {
		t.Error("Should return error when IDManager is nil")
	}

	// 测试有效配置
	sm, err := NewSessionManagerV2(ctx, &SessionManagerConfig{
		IDManager: idManager,
	})
	if err != nil {
		t.Fatalf("Should not return error: %v", err)
	}
	defer sm.Close()

	if sm == nil {
		t.Fatal("SessionManager should not be nil")
	}

	if sm.logger == nil {
		t.Error("Logger should be set")
	}
}

// TestNewSessionManagerV2WithOptions 测试带选项的构造函数
func TestNewSessionManagerV2WithOptions(t *testing.T) {
	ctx := context.Background()
	storageFactory := storage.NewStorageFactory(ctx)
	memStorage, _ := storageFactory.CreateStorage(&storage.MemoryStorageConfig{})
	idManager := idgen.NewIDManager(memStorage, ctx)

	// 创建自定义 Logger
	customLogger := corelog.NewNopLogger()

	sm, err := NewSessionManagerV2(ctx, &SessionManagerConfig{
		IDManager: idManager,
		Logger:    customLogger,
	},
		WithNodeID("test-node-1"),
	)
	if err != nil {
		t.Fatalf("Should not return error: %v", err)
	}
	defer sm.Close()

	// 验证 NodeID 被设置
	if sm.GetNodeID() != "test-node-1" {
		t.Errorf("NodeID should be 'test-node-1', got '%s'", sm.GetNodeID())
	}

	// 验证自定义 Logger 被使用
	if sm.logger != customLogger {
		t.Error("Custom logger should be used")
	}
}

// TestSessionManagerOptions 测试各种 Option 函数
func TestSessionManagerOptions(t *testing.T) {
	ctx := context.Background()
	storageFactory := storage.NewStorageFactory(ctx)
	memStorage, _ := storageFactory.CreateStorage(&storage.MemoryStorageConfig{})
	idManager := idgen.NewIDManager(memStorage, ctx)

	// 创建基础 SessionManager
	sm, err := NewSessionManagerV2(ctx, &SessionManagerConfig{
		IDManager: idManager,
	})
	if err != nil {
		t.Fatalf("Should not return error: %v", err)
	}
	defer sm.Close()

	// 测试 WithNodeID
	WithNodeID("node-123")(sm)
	if sm.nodeID != "node-123" {
		t.Errorf("NodeID should be 'node-123', got '%s'", sm.nodeID)
	}

	// 测试 WithTunnelRoutingTable
	rt := NewTunnelRoutingTable(memStorage, 30*time.Second)
	WithTunnelRoutingTable(rt)(sm)
	if sm.tunnelRouting != rt {
		t.Error("TunnelRoutingTable should be set")
	}

	// 测试 WithTunnelStateManager
	tsm := NewTunnelStateManager(memStorage, "")
	WithTunnelStateManager(tsm)(sm)
	if sm.tunnelStateManager != tsm {
		t.Error("TunnelStateManager should be set")
	}
}
