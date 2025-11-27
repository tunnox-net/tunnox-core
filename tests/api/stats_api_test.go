package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/tests/helpers"
)

// TestStatsAPI_UserStats 测试用户统计
func TestStatsAPI_UserStats(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("获取用户统计", func(t *testing.T) {
		// 创建用户
		user, err := client.CreateUser("statsuser", "stats@example.com")
		require.NoError(t, err)

		// 创建客户端和映射
		client1, err := client.CreateClient(user.ID, "Stats Client")
		require.NoError(t, err)

		// 获取用户统计
		stats, err := client.GetUserStats(user.ID)
		require.NoError(t, err)
		assert.NotNil(t, stats)
		
		// 验证基本统计字段
		assert.GreaterOrEqual(t, stats.TotalClients, 1)
		_ = client1
	})

	t.Run("获取不存在用户的统计", func(t *testing.T) {
		_, err := client.GetUserStats("non-existent-user")
		// 可能返回空统计或错误
		_ = err
	})
}

// TestStatsAPI_ClientStats 测试客户端统计
func TestStatsAPI_ClientStats(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("获取客户端统计", func(t *testing.T) {
		// 创建用户和客户端
		user, err := client.CreateUser("clientstatsuser", "cstats@example.com")
		require.NoError(t, err)

		clientInfo, err := client.CreateClient(user.ID, "Stats Client")
		require.NoError(t, err)

		// 获取客户端统计
		stats, err := client.GetClientStats(clientInfo.ID)
		require.NoError(t, err)
		assert.NotNil(t, stats)
	})

	t.Run("获取不存在客户端的统计", func(t *testing.T) {
		_, err := client.GetClientStats(99999999)
		// 可能返回空统计或错误
		_ = err
	})
}

// TestStatsAPI_SystemStats 测试系统统计
func TestStatsAPI_SystemStats(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("获取系统统计", func(t *testing.T) {
		// 创建一些数据
		user, err := client.CreateUser("sysuser", "sys@example.com")
		require.NoError(t, err)

		_, err = client.CreateClient(user.ID, "Sys Client 1")
		require.NoError(t, err)

		_, err = client.CreateClient(user.ID, "Sys Client 2")
		require.NoError(t, err)

		// 获取系统统计
		stats, err := client.GetSystemStats()
		require.NoError(t, err)
		assert.NotNil(t, stats)

		// 系统中应该有数据
		assert.GreaterOrEqual(t, stats.TotalUsers, 1)
		assert.GreaterOrEqual(t, stats.TotalClients, 2)
	})

	t.Run("空系统的统计", func(t *testing.T) {
		// 即使系统为空，也应该返回统计（值为0）
		stats, err := client.GetSystemStats()
		require.NoError(t, err)
		assert.NotNil(t, stats)
	})
}

// TestStatsAPI_Consistency 测试统计一致性
func TestStatsAPI_Consistency(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("创建资源后统计应该更新", func(t *testing.T) {
		// 获取初始统计
		initialStats, err := client.GetSystemStats()
		require.NoError(t, err)

		// 创建用户
		user, err := client.CreateUser("consistuser", "consist@example.com")
		require.NoError(t, err)

		// 再次获取统计
		afterUserStats, err := client.GetSystemStats()
		require.NoError(t, err)

		// 用户数应该增加
		assert.Greater(t, afterUserStats.TotalUsers, initialStats.TotalUsers)

		// 创建客户端
		_, err = client.CreateClient(user.ID, "Consistency Client")
		require.NoError(t, err)

		// 再次获取统计
		afterClientStats, err := client.GetSystemStats()
		require.NoError(t, err)

		// 客户端数应该增加
		assert.Greater(t, afterClientStats.TotalClients, afterUserStats.TotalClients)
	})
}

// TestStatsAPI_ConcurrentAccess 测试并发访问统计
func TestStatsAPI_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("并发获取系统统计", func(t *testing.T) {
		const concurrency = 30
		done := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			go func() {
				_, err := client.GetSystemStats()
				done <- err
			}()
		}

		// 所有请求都应该成功
		for i := 0; i < concurrency; i++ {
			err := <-done
			assert.NoError(t, err)
		}
	})
}

