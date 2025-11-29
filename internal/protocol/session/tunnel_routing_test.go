package session

import (
	"context"
	"testing"
	"time"
	"tunnox-core/internal/core/storage"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTunnelRoutingTable_RegisterAndLookup(t *testing.T) {
	// 创建内存存储
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	defer memStorage.Close()
	
	// 创建路由表
	routingTable := NewTunnelRoutingTable(memStorage, 10*time.Second)
	
	t.Run("注册并查找隧道", func(t *testing.T) {
		state := &TunnelWaitingState{
			TunnelID:       "tunnel-123",
			MappingID:      "mapping-456",
			SecretKey:      "secret-key",
			SourceNodeID:   "server-a",
			SourceClientID: 1001,
			TargetClientID: 2002,
			TargetHost:     "localhost",
			TargetPort:     8080,
		}
		
		// 注册
		err := routingTable.RegisterWaitingTunnel(ctx, state)
		require.NoError(t, err)
		
		// 查找
		found, err := routingTable.LookupWaitingTunnel(ctx, "tunnel-123")
		require.NoError(t, err)
		assert.Equal(t, "tunnel-123", found.TunnelID)
		assert.Equal(t, "server-a", found.SourceNodeID)
		assert.Equal(t, int64(1001), found.SourceClientID)
		assert.Equal(t, int64(2002), found.TargetClientID)
	})
	
	t.Run("查找不存在的隧道", func(t *testing.T) {
		_, err := routingTable.LookupWaitingTunnel(ctx, "non-existent")
		assert.ErrorIs(t, err, ErrTunnelNotFound)
	})
	
	t.Run("移除隧道", func(t *testing.T) {
		state := &TunnelWaitingState{
			TunnelID:       "tunnel-to-remove",
			MappingID:      "mapping-789",
			SecretKey:      "secret",
			SourceNodeID:   "server-b",
			SourceClientID: 3003,
			TargetClientID: 4004,
			TargetHost:     "localhost",
			TargetPort:     9090,
		}
		
		// 注册
		err := routingTable.RegisterWaitingTunnel(ctx, state)
		require.NoError(t, err)
		
		// 移除
		err = routingTable.RemoveWaitingTunnel(ctx, "tunnel-to-remove")
		require.NoError(t, err)
		
		// 确认已删除
		_, err = routingTable.LookupWaitingTunnel(ctx, "tunnel-to-remove")
		assert.ErrorIs(t, err, ErrTunnelNotFound)
	})
}

func TestTunnelRoutingTable_Expiry(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	defer memStorage.Close()
	
	// 创建路由表，TTL设为很短
	routingTable := NewTunnelRoutingTable(memStorage, 100*time.Millisecond)
	
	t.Run("过期的隧道应该返回错误", func(t *testing.T) {
		state := &TunnelWaitingState{
			TunnelID:       "tunnel-expire",
			MappingID:      "mapping-999",
			SecretKey:      "secret",
			SourceNodeID:   "server-c",
			SourceClientID: 5005,
			TargetClientID: 6006,
			TargetHost:     "localhost",
			TargetPort:     7070,
		}
		
		// 注册
		err := routingTable.RegisterWaitingTunnel(ctx, state)
		require.NoError(t, err)
		
		// 立即查找应该成功
		found, err := routingTable.LookupWaitingTunnel(ctx, "tunnel-expire")
		require.NoError(t, err)
		assert.Equal(t, "tunnel-expire", found.TunnelID)
		
		// 等待过期（TTL 是 100ms，等待 150ms 确保过期）
		time.Sleep(150 * time.Millisecond)
		
		// 再次查找
		// 注意：如果存储自动清理过期键，可能返回 ErrTunnelNotFound
		// 如果存储保留过期键，LookupWaitingTunnel 会检查 ExpiresAt 并返回 ErrTunnelExpired
		_, err = routingTable.LookupWaitingTunnel(ctx, "tunnel-expire")
		// 接受两种错误：过期错误或未找到错误（取决于存储实现）
		assert.True(t, err == ErrTunnelExpired || err == ErrTunnelNotFound, 
			"Expected ErrTunnelExpired or ErrTunnelNotFound, got: %v", err)
	})
}

func TestTunnelRoutingTable_InvalidInput(t *testing.T) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	defer memStorage.Close()
	
	routingTable := NewTunnelRoutingTable(memStorage, 10*time.Second)
	
	t.Run("空TunnelID注册应该失败", func(t *testing.T) {
		state := &TunnelWaitingState{
			TunnelID:       "",
			MappingID:      "mapping-123",
			SourceNodeID:   "server-a",
			SourceClientID: 1001,
			TargetClientID: 2002,
		}
		
		err := routingTable.RegisterWaitingTunnel(ctx, state)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tunnel_id is required")
	})
	
	t.Run("空TunnelID查找应该失败", func(t *testing.T) {
		_, err := routingTable.LookupWaitingTunnel(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tunnel_id is required")
	})
	
	t.Run("空TunnelID删除应该失败", func(t *testing.T) {
		err := routingTable.RemoveWaitingTunnel(ctx, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tunnel_id is required")
	})
}

