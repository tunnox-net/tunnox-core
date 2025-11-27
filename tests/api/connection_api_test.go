package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/tests/helpers"
)

// setupConnectionTest 为连接测试创建基础数据
func setupConnectionTest(t *testing.T, client *helpers.APIClient) (userID string, mappingID string) {
	// 创建用户
	user, err := client.CreateUser("connuser", "conn@example.com")
	require.NoError(t, err)

	// 创建客户端
	sourceClient, err := client.CreateClient(user.ID, "Conn Source")
	require.NoError(t, err)

	targetClient, err := client.CreateClient(user.ID, "Conn Target")
	require.NoError(t, err)

	// 创建映射
	mapping := &models.PortMapping{
		UserID:         user.ID,
		SourceClientID: sourceClient.ID,
		TargetClientID: targetClient.ID,
		Protocol:       models.ProtocolTCP,
		SourcePort:     2020,
		TargetHost:     "localhost",
		TargetPort:     20,
		Type:           models.MappingTypeRegistered,
		Status:         models.MappingStatusActive,
	}
	created, err := client.CreateMapping(mapping)
	require.NoError(t, err)

	return user.ID, created.ID
}

// TestConnectionAPI_ListConnections 测试列出连接
func TestConnectionAPI_ListConnections(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("列出所有连接", func(t *testing.T) {
		// 设置测试数据
		_, _ = setupConnectionTest(t, client)

		// 注意：实际的连接需要客户端建立才会有
		// 这里只测试 API 端点的可访问性
		// TODO: 需要扩展 APIClient 添加 ListConnections 方法
		
		// 暂时跳过，因为需要真实的连接
		t.Skip("需要真实连接数据")
	})
}

// TestConnectionAPI_ConnectionLifecycle 测试连接生命周期
func TestConnectionAPI_ConnectionLifecycle(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("连接创建和关闭", func(t *testing.T) {
		// 设置测试数据
		_, mappingID := setupConnectionTest(t, client)
		_ = mappingID

		// 注意：实际的连接管理需要真实的网络连接
		// 这里只验证 API 的基本可访问性
		t.Skip("需要真实连接数据")
	})
}

// TestConnectionAPI_EdgeCases 测试边界情况
func TestConnectionAPI_EdgeCases(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("空系统的连接列表", func(t *testing.T) {
		// 获取系统统计应该包含连接信息
		stats, err := client.GetSystemStats()
		require.NoError(t, err)
		assert.NotNil(t, stats)
		// 初始连接数应该是0
		assert.Equal(t, int64(0), stats.TotalConnections)
	})
}

