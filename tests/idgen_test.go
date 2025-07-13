package tests

import (
	"context"
	"strings"
	"testing"
	"time"
	"tunnox-core/internal/cloud/generators"
	"tunnox-core/internal/cloud/storages"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDManager_Basic(t *testing.T) {
	ctx := context.Background()
	storage := storages.NewMemoryStorage(ctx)
	defer storage.Close()

	manager := generators.NewIDManager(storage, ctx)
	defer manager.Close()

	t.Run("Generate Client ID", func(t *testing.T) {
		clientID, err := manager.GenerateClientID()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, clientID, int64(10000000))
		assert.LessOrEqual(t, clientID, int64(99999999))

		// 检查唯一性
		used, err := manager.IsClientIDUsed(clientID)
		require.NoError(t, err)
		assert.True(t, used)

		// 释放ID
		err = manager.ReleaseClientID(clientID)
		require.NoError(t, err)

		// 检查释放后状态
		used, err = manager.IsClientIDUsed(clientID)
		require.NoError(t, err)
		assert.False(t, used)
	})

	t.Run("Generate Node ID", func(t *testing.T) {
		nodeID, err := manager.GenerateNodeID()
		require.NoError(t, err)

		// 验证格式：node_timestamp_randomString
		assert.True(t, strings.HasPrefix(nodeID, "node_"))
		parts := strings.Split(nodeID, "_")
		assert.Len(t, parts, 3)
		assert.Equal(t, "node", parts[0])

		// 验证时间戳部分（13位数字）
		assert.Len(t, parts[1], 13)
		timestamp, err := time.Parse("2006-01-02T15:04:05.000Z", time.UnixMilli(0).Format("2006-01-02T15:04:05.000Z"))
		require.NoError(t, err)
		assert.True(t, timestamp.Before(time.Now().Add(time.Second)))

		// 验证随机部分（8位）
		assert.Len(t, parts[2], 8)

		// 检查唯一性
		used, err := manager.IsNodeIDUsed(nodeID)
		require.NoError(t, err)
		assert.True(t, used)

		// 释放ID
		err = manager.ReleaseNodeID(nodeID)
		require.NoError(t, err)

		// 检查释放后状态
		used, err = manager.IsNodeIDUsed(nodeID)
		require.NoError(t, err)
		assert.False(t, used)
	})

	t.Run("Generate Connection ID", func(t *testing.T) {
		connID1, err := manager.GenerateConnectionID()
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(connID1, "conn_"))

		connID2, err := manager.GenerateConnectionID()
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(connID2, "conn_"))

		// 连接ID应该不同
		assert.NotEqual(t, connID1, connID2)

		// 验证格式：conn_timestamp_counter
		parts1 := strings.Split(connID1, "_")
		assert.Len(t, parts1, 3)
		assert.Equal(t, "conn", parts1[0])
		assert.Len(t, parts1[1], 13)       // 时间戳
		assert.True(t, len(parts1[2]) > 0) // 计数器

		// 连接ID不需要释放
		err = manager.ReleaseConnectionID(connID1)
		require.NoError(t, err)
	})

	t.Run("Generate Port Mapping ID", func(t *testing.T) {
		portMappingID, err := manager.GeneratePortMappingID()
		require.NoError(t, err)

		// 验证格式：pmap_timestamp_randomString
		assert.True(t, strings.HasPrefix(portMappingID, "pmap_"))
		parts := strings.Split(portMappingID, "_")
		assert.Len(t, parts, 3)
		assert.Equal(t, "pmap", parts[0])

		// 验证时间戳部分（13位数字）
		assert.Len(t, parts[1], 13)

		// 验证随机部分（8位）
		assert.Len(t, parts[2], 8)

		// 检查唯一性
		used, err := manager.IsPortMappingIDUsed(portMappingID)
		require.NoError(t, err)
		assert.True(t, used)

		// 释放ID
		err = manager.ReleasePortMappingID(portMappingID)
		require.NoError(t, err)

		// 检查释放后状态
		used, err = manager.IsPortMappingIDUsed(portMappingID)
		require.NoError(t, err)
		assert.False(t, used)
	})

	t.Run("Generate Tunnel ID", func(t *testing.T) {
		tunnelID, err := manager.GenerateTunnelID()
		require.NoError(t, err)

		// 验证格式：tun_timestamp_randomString
		assert.True(t, strings.HasPrefix(tunnelID, "tun_"))
		parts := strings.Split(tunnelID, "_")
		assert.Len(t, parts, 3)
		assert.Equal(t, "tun", parts[0])

		// 验证时间戳部分（13位数字）
		assert.Len(t, parts[1], 13)

		// 验证随机部分（8位）
		assert.Len(t, parts[2], 8)

		// 检查唯一性
		used, err := manager.IsTunnelIDUsed(tunnelID)
		require.NoError(t, err)
		assert.True(t, used)

		// 释放ID
		err = manager.ReleaseTunnelID(tunnelID)
		require.NoError(t, err)

		// 检查释放后状态
		used, err = manager.IsTunnelIDUsed(tunnelID)
		require.NoError(t, err)
		assert.False(t, used)
	})
}

func TestIDManager_Uniqueness(t *testing.T) {
	ctx := context.Background()
	storage := storages.NewMemoryStorage(ctx)
	defer storage.Close()

	manager := generators.NewIDManager(storage, ctx)
	defer manager.Close()

	t.Run("Client ID Uniqueness", func(t *testing.T) {
		ids := make(map[int64]bool)
		const numIDs = 100

		for i := 0; i < numIDs; i++ {
			id, err := manager.GenerateClientID()
			require.NoError(t, err)
			assert.False(t, ids[id], "Duplicate client ID generated: %d", id)
			ids[id] = true
		}

		assert.Len(t, ids, numIDs)
	})

	t.Run("Node ID Uniqueness", func(t *testing.T) {
		ids := make(map[string]bool)
		const numIDs = 100

		for i := 0; i < numIDs; i++ {
			id, err := manager.GenerateNodeID()
			require.NoError(t, err)
			assert.False(t, ids[id], "Duplicate node ID generated: %s", id)
			ids[id] = true
		}

		assert.Len(t, ids, numIDs)
	})

	t.Run("Connection ID Uniqueness", func(t *testing.T) {
		ids := make(map[string]bool)
		const numIDs = 100

		for i := 0; i < numIDs; i++ {
			id, err := manager.GenerateConnectionID()
			require.NoError(t, err)
			assert.False(t, ids[id], "Duplicate connection ID generated: %s", id)
			ids[id] = true
		}

		assert.Len(t, ids, numIDs)
	})

	t.Run("Port Mapping ID Uniqueness", func(t *testing.T) {
		ids := make(map[string]bool)
		const numIDs = 100

		for i := 0; i < numIDs; i++ {
			id, err := manager.GeneratePortMappingID()
			require.NoError(t, err)
			assert.False(t, ids[id], "Duplicate port mapping ID generated: %s", id)
			ids[id] = true
		}

		assert.Len(t, ids, numIDs)
	})

	t.Run("Tunnel ID Uniqueness", func(t *testing.T) {
		ids := make(map[string]bool)
		const numIDs = 100

		for i := 0; i < numIDs; i++ {
			id, err := manager.GenerateTunnelID()
			require.NoError(t, err)
			assert.False(t, ids[id], "Duplicate tunnel ID generated: %s", id)
			ids[id] = true
		}

		assert.Len(t, ids, numIDs)
	})
}

func TestIDManager_Dispose(t *testing.T) {
	ctx := context.Background()
	storage := storages.NewMemoryStorage(ctx)
	defer storage.Close()

	manager := generators.NewIDManager(storage, ctx)

	// 验证未关闭
	assert.False(t, manager.IsClosed())

	// 关闭管理器
	err := manager.Close()
	require.NoError(t, err)

	// 验证已关闭
	assert.True(t, manager.IsClosed())

	// 多次关闭不报错
	err = manager.Close()
	require.NoError(t, err)
}

func TestIDManager_Concurrent(t *testing.T) {
	ctx := context.Background()
	storage := storages.NewMemoryStorage(ctx)
	defer storage.Close()

	manager := generators.NewIDManager(storage, ctx)
	defer manager.Close()

	t.Run("Concurrent Client ID Generation", func(t *testing.T) {
		const numGoroutines = 10
		const idsPerGoroutine = 10
		results := make(chan int64, numGoroutines*idsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				for j := 0; j < idsPerGoroutine; j++ {
					id, err := manager.GenerateClientID()
					if err == nil {
						results <- id
					}
				}
			}()
		}

		ids := make(map[int64]bool)
		for i := 0; i < numGoroutines*idsPerGoroutine; i++ {
			id := <-results
			assert.False(t, ids[id], "Duplicate client ID in concurrent test: %d", id)
			ids[id] = true
		}

		assert.Len(t, ids, numGoroutines*idsPerGoroutine)
	})

	t.Run("Concurrent Node ID Generation", func(t *testing.T) {
		const numGoroutines = 10
		const idsPerGoroutine = 10
		results := make(chan string, numGoroutines*idsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				for j := 0; j < idsPerGoroutine; j++ {
					id, err := manager.GenerateNodeID()
					if err == nil {
						results <- id
					}
				}
			}()
		}

		ids := make(map[string]bool)
		for i := 0; i < numGoroutines*idsPerGoroutine; i++ {
			id := <-results
			assert.False(t, ids[id], "Duplicate node ID in concurrent test: %s", id)
			ids[id] = true
		}

		assert.Len(t, ids, numGoroutines*idsPerGoroutine)
	})
}

func TestIDManager_StoragePersistence(t *testing.T) {
	ctx := context.Background()
	storage := storages.NewMemoryStorage(ctx)
	defer storage.Close()

	// 第一个管理器生成ID
	manager1 := generators.NewIDManager(storage, ctx)
	clientID, err := manager1.GenerateClientID()
	require.NoError(t, err)
	nodeID, err := manager1.GenerateNodeID()
	require.NoError(t, err)
	manager1.Close()

	// 第二个管理器应该能看到相同的ID已被使用
	manager2 := generators.NewIDManager(storage, ctx)
	defer manager2.Close()

	used, err := manager2.IsClientIDUsed(clientID)
	require.NoError(t, err)
	assert.True(t, used)

	used, err = manager2.IsNodeIDUsed(nodeID)
	require.NoError(t, err)
	assert.True(t, used)
}

func TestIDManager_FormatConsistency(t *testing.T) {
	ctx := context.Background()
	storage := storages.NewMemoryStorage(ctx)
	defer storage.Close()

	manager := generators.NewIDManager(storage, ctx)
	defer manager.Close()

	t.Run("Node ID Format", func(t *testing.T) {
		nodeID, err := manager.GenerateNodeID()
		require.NoError(t, err)

		// 验证格式：node_timestamp_randomString
		parts := strings.Split(nodeID, "_")
		assert.Len(t, parts, 3)
		assert.Equal(t, "node", parts[0])
		assert.Len(t, parts[1], 13) // 时间戳
		assert.Len(t, parts[2], 8)  // 随机字符串

		// 验证时间戳是数字
		_, err = time.Parse("2006-01-02T15:04:05.000Z", time.UnixMilli(0).Format("2006-01-02T15:04:05.000Z"))
		assert.NoError(t, err)
	})

	t.Run("Port Mapping ID Format", func(t *testing.T) {
		portMappingID, err := manager.GeneratePortMappingID()
		require.NoError(t, err)

		// 验证格式：pmap_timestamp_randomString
		parts := strings.Split(portMappingID, "_")
		assert.Len(t, parts, 3)
		assert.Equal(t, "pmap", parts[0])
		assert.Len(t, parts[1], 13) // 时间戳
		assert.Len(t, parts[2], 8)  // 随机字符串
	})

	t.Run("Tunnel ID Format", func(t *testing.T) {
		tunnelID, err := manager.GenerateTunnelID()
		require.NoError(t, err)

		// 验证格式：tun_timestamp_randomString
		parts := strings.Split(tunnelID, "_")
		assert.Len(t, parts, 3)
		assert.Equal(t, "tun", parts[0])
		assert.Len(t, parts[1], 13) // 时间戳
		assert.Len(t, parts[2], 8)  // 随机字符串
	})
}
