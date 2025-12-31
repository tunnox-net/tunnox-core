package session

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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

	// 验证初始状态 - 使用 clientRegistry.Count()
	assert.Equal(t, 3, sessionMgr.clientRegistry.Count(), "Should have 3 connections")

	// 等待1.5秒
	time.Sleep(1500 * time.Millisecond)

	// 更新conn3的活跃时间（模拟接收心跳）
	conn3.UpdateActivity()

	// 再等待1秒让conn1和conn2超时，并触发清理
	time.Sleep(1000 * time.Millisecond)

	// 验证：conn1和conn2应该被清理，conn3应该保留
	remainingCount := sessionMgr.clientRegistry.Count()
	conn3Result := sessionMgr.GetControlConnection("conn3")
	conn1Result := sessionMgr.GetControlConnection("conn1")
	conn2Result := sessionMgr.GetControlConnection("conn2")

	assert.Equal(t, 1, remainingCount, "Should have 1 connection after cleanup")
	assert.NotNil(t, conn3Result, "Conn3 should still exist (active)")
	assert.Nil(t, conn1Result, "Conn1 should be removed (stale)")
	assert.Nil(t, conn2Result, "Conn2 should be removed (stale)")
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
				if c := sessionMgr.GetControlConnection("conn_active"); c != nil {
					c.UpdateActivity()
				}
			case <-done:
				return
			}
		}
	}()

	// 等待5秒（超过超时时间，但因为持续心跳，连接应该保留）
	time.Sleep(5 * time.Second)
	close(done)

	// 验证连接仍然存在
	result := sessionMgr.GetControlConnection("conn_active")
	count := sessionMgr.clientRegistry.Count()

	assert.NotNil(t, result, "Active connection should still exist")
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
	conn := NewControlConnection("test_conn", nil, nil, "tcp")

	// 刚创建，不应该过期
	assert.False(t, conn.IsStale(100*time.Millisecond), "Newly created connection should not be stale")

	// 等待，让连接过期
	time.Sleep(150 * time.Millisecond)
	assert.True(t, conn.IsStale(100*time.Millisecond), "Connection should be stale after timeout")

	// 更新活跃时间
	conn.UpdateActivity()
	assert.False(t, conn.IsStale(100*time.Millisecond), "Connection should not be stale after update")
}
