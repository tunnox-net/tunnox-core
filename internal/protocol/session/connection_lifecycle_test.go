package session

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
)

// TestConnectionCleanup_RemovesStaleConnections 验证超时清理功能
func TestConnectionCleanup_RemovesStaleConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建存储
	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	// 创建配置：短超时时间便于测试
	config := &SessionConfig{
		HeartbeatTimeout: 2 * time.Second,
		CleanupInterval:  1 * time.Second,
	}

	// 创建SessionManager
	sessionMgr := NewSessionManagerWithConfig(idManager, ctx, config)
	defer sessionMgr.Close()

	// 注册3个控制连接（stream可以为nil，清理时不需要发送数据）
	conn1 := NewControlConnection("conn1", nil, nil, "tcp")
	conn1.ClientID = 1001
	conn1.Authenticated = true
	sessionMgr.RegisterControlConnection(conn1)

	conn2 := NewControlConnection("conn2", nil, nil, "tcp")
	conn2.ClientID = 1002
	conn2.Authenticated = true
	sessionMgr.RegisterControlConnection(conn2)

	conn3 := NewControlConnection("conn3", nil, nil, "tcp")
	conn3.ClientID = 1003
	conn3.Authenticated = true
	sessionMgr.RegisterControlConnection(conn3)

	// 验证初始状态
	assert.Equal(t, 3, len(sessionMgr.controlConnMap), "Should have 3 connections")

	// 等待1.5秒
	time.Sleep(1500 * time.Millisecond)

	// 更新conn3的活跃时间（模拟接收心跳）
	sessionMgr.controlConnLock.Lock()
	conn3.UpdateActivity()
	sessionMgr.controlConnLock.Unlock()

	// 再等待1秒让conn1和conn2超时，并触发清理
	time.Sleep(1000 * time.Millisecond)

	// 验证：conn1和conn2应该被清理，conn3应该保留
	sessionMgr.controlConnLock.RLock()
	remainingCount := len(sessionMgr.controlConnMap)
	_, conn3Exists := sessionMgr.controlConnMap["conn3"]
	_, conn1Exists := sessionMgr.controlConnMap["conn1"]
	_, conn2Exists := sessionMgr.controlConnMap["conn2"]
	sessionMgr.controlConnLock.RUnlock()

	assert.Equal(t, 1, remainingCount, "Should have 1 connection after cleanup")
	assert.True(t, conn3Exists, "Conn3 should still exist (active)")
	assert.False(t, conn1Exists, "Conn1 should be removed (stale)")
	assert.False(t, conn2Exists, "Conn2 should be removed (stale)")
}

// TestConnectionCleanup_PreservesActiveConnections 验证活跃连接不被清理
func TestConnectionCleanup_PreservesActiveConnections(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	config := &SessionConfig{
		HeartbeatTimeout: 3 * time.Second,
		CleanupInterval:  1 * time.Second,
	}

	sessionMgr := NewSessionManagerWithConfig(idManager, ctx, config)
	defer sessionMgr.Close()

	// 注册连接
	conn := NewControlConnection("conn_active", nil, nil, "tcp")
	conn.ClientID = 2001
	conn.Authenticated = true
	sessionMgr.RegisterControlConnection(conn)

	// 持续更新活跃时间（模拟心跳）
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sessionMgr.controlConnLock.Lock()
				if c, exists := sessionMgr.controlConnMap["conn_active"]; exists {
					c.UpdateActivity()
				}
				sessionMgr.controlConnLock.Unlock()
			case <-done:
				return
			}
		}
	}()

	// 等待5秒（超过超时时间，但因为持续心跳，连接应该保留）
	time.Sleep(5 * time.Second)
	close(done)

	// 验证连接仍然存在
	sessionMgr.controlConnLock.RLock()
	_, exists := sessionMgr.controlConnMap["conn_active"]
	count := len(sessionMgr.controlConnMap)
	sessionMgr.controlConnLock.RUnlock()

	assert.True(t, exists, "Active connection should still exist")
	assert.Equal(t, 1, count, "Should have 1 connection")
}

// TestHeartbeat_UpdatesLastActiveAt 验证心跳更新活跃时间
func TestHeartbeat_UpdatesLastActiveAt(t *testing.T) {
	conn := NewControlConnection("test_conn", nil, nil, "tcp")

	initialTime := conn.LastActiveAt
	time.Sleep(100 * time.Millisecond)

	// 更新活跃时间
	conn.UpdateActivity()

	updatedTime := conn.LastActiveAt

	assert.True(t, updatedTime.After(initialTime), "LastActiveAt should be updated")
	assert.False(t, conn.IsStale(1*time.Second), "Connection should not be stale after update")
}

// TestControlConnection_IsStale 测试IsStale方法
func TestControlConnection_IsStale(t *testing.T) {
	tests := []struct {
		name     string
		conn     *ControlConnection
		timeout  time.Duration
		expected bool
	}{
		{
			name:     "nil connection is stale",
			conn:     nil,
			timeout:  time.Second,
			expected: true,
		},
		{
			name: "fresh connection is not stale",
			conn: &ControlConnection{
				LastActiveAt: time.Now(),
			},
			timeout:  10 * time.Second,
			expected: false,
		},
		{
			name: "old connection is stale",
			conn: &ControlConnection{
				LastActiveAt: time.Now().Add(-20 * time.Second),
			},
			timeout:  10 * time.Second,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.conn.IsStale(tt.timeout)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCleanupStaleConnections_ReturnCount 验证清理方法返回正确的计数
func TestCleanupStaleConnections_ReturnCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	config := &SessionConfig{
		HeartbeatTimeout: 1 * time.Second,
		CleanupInterval:  10 * time.Second, // 长间隔，手动触发
	}

	sessionMgr := NewSessionManagerWithConfig(idManager, ctx, config)
	defer sessionMgr.Close()

	// 注册5个连接，其中3个将超时
	for i := 1; i <= 5; i++ {
		connID := fmt.Sprintf("conn_%d", i)
		conn := NewControlConnection(
			connID,
			nil,
			nil,
			"tcp",
		)
		conn.ClientID = int64(3000 + i)
		conn.Authenticated = true
		sessionMgr.RegisterControlConnection(conn)
	}

	// 等待超时
	time.Sleep(1200 * time.Millisecond)

	// 更新conn_4和conn_5（保持活跃）
	sessionMgr.controlConnLock.Lock()
	sessionMgr.controlConnMap["conn_4"].UpdateActivity()
	sessionMgr.controlConnMap["conn_5"].UpdateActivity()
	sessionMgr.controlConnLock.Unlock()

	// 手动触发清理
	cleaned := sessionMgr.cleanupStaleConnections()

	// 验证清理数量
	assert.Equal(t, 3, cleaned, "Should clean 3 stale connections")

	// 验证剩余连接
	sessionMgr.controlConnLock.RLock()
	remainingCount := len(sessionMgr.controlConnMap)
	sessionMgr.controlConnLock.RUnlock()

	assert.Equal(t, 2, remainingCount, "Should have 2 active connections remaining")
}

// TestConnectionCleanup_ConfigNil 验证配置为nil时清理被禁用
func TestConnectionCleanup_ConfigNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	memStorage := storage.NewMemoryStorage(ctx)
	idManager := idgen.NewIDManager(memStorage, ctx)

	// 创建SessionManager但config为nil
	sessionMgr := NewSessionManagerWithConfig(idManager, ctx, nil)
	defer sessionMgr.Close()

	// config应该被设为默认值
	require.NotNil(t, sessionMgr.config, "Config should be set to default if nil")
	assert.Equal(t, 60*time.Second, sessionMgr.config.HeartbeatTimeout) // 默认值已更新为60秒
}
