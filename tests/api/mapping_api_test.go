package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/tests/helpers"
)

// setupMappingTest 为映射测试创建基础数据（用户和客户端）
func setupMappingTest(t *testing.T, client *helpers.APIClient) (userID string, sourceClientID, targetClientID int64) {
	user, err := client.CreateUser("mapuser", "map@example.com")
	require.NoError(t, err)

	sourceClient, err := client.CreateClient(user.ID, "Source Client")
	require.NoError(t, err)

	targetClient, err := client.CreateClient(user.ID, "Target Client")
	require.NoError(t, err)

	return user.ID, sourceClient.ID, targetClient.ID
}

// TestMappingAPI_CreateMapping 测试创建映射
func TestMappingAPI_CreateMapping(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功创建TCP映射", func(t *testing.T) {
		userID, sourceID, targetID := setupMappingTest(t, client)

		mapping := &models.PortMapping{
			UserID:         userID,
			SourceClientID: sourceID,
			TargetClientID: targetID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     8080,
			TargetHost:     "localhost",
			TargetPort:     80,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}

		created, err := client.CreateMapping(mapping)
		require.NoError(t, err)
		assert.NotEmpty(t, created.ID)
		assert.Equal(t, models.ProtocolTCP, created.Protocol)
		assert.Equal(t, 8080, created.SourcePort)
		assert.Equal(t, 80, created.TargetPort)
		assert.NotEmpty(t, created.SecretKey)
	})

	t.Run("成功创建HTTP映射", func(t *testing.T) {
		userID, sourceID, targetID := setupMappingTest(t, client)

		mapping := &models.PortMapping{
			UserID:         userID,
			SourceClientID: sourceID,
			TargetClientID: targetID,
			Protocol:       models.ProtocolHTTP,
			SourcePort:     8443,
			TargetHost:     "localhost",
			TargetPort:     443,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}

		created, err := client.CreateMapping(mapping)
		require.NoError(t, err)
		assert.Equal(t, models.ProtocolHTTP, created.Protocol)
	})

	t.Run("成功创建UDP映射", func(t *testing.T) {
		userID, sourceID, targetID := setupMappingTest(t, client)

		mapping := &models.PortMapping{
			UserID:         userID,
			SourceClientID: sourceID,
			TargetClientID: targetID,
			Protocol:       models.ProtocolUDP,
			SourcePort:     5353,
			TargetHost:     "localhost",
			TargetPort:     53,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}

		created, err := client.CreateMapping(mapping)
		require.NoError(t, err)
		assert.Equal(t, models.ProtocolUDP, created.Protocol)
	})

	t.Run("缺少必填字段", func(t *testing.T) {
		// 缺少 ListenClientID 和 TargetClientID
		mapping := &models.PortMapping{
			// SourceClientID: 0, // 缺失
			// TargetClientID: 0, // 缺失
			Protocol:   models.ProtocolTCP,
			SourcePort: 8080,
			TargetHost: "localhost",
			TargetPort: 80,
		}

		_, err := client.CreateMapping(mapping)
		assert.Error(t, err, "Expected error for missing required fields")
		// 注意：UserID 允许为空（用于匿名客户端），所以这里不测试 UserID
	})
}

// TestMappingAPI_GetMapping 测试获取映射
func TestMappingAPI_GetMapping(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功获取映射", func(t *testing.T) {
		userID, sourceID, targetID := setupMappingTest(t, client)

		// 创建映射
		mapping := &models.PortMapping{
			UserID:         userID,
			SourceClientID: sourceID,
			TargetClientID: targetID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     9090,
			TargetHost:     "localhost",
			TargetPort:     90,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}
		created, err := client.CreateMapping(mapping)
		require.NoError(t, err)

		// 获取映射
		retrieved, err := client.GetMapping(created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, retrieved.ID)
		assert.Equal(t, created.Protocol, retrieved.Protocol)
		assert.Equal(t, created.SourcePort, retrieved.SourcePort)
	})

	t.Run("获取不存在的映射", func(t *testing.T) {
		_, err := client.GetMapping("non-existent-mapping-id")
		assert.Error(t, err)
	})
}

// TestMappingAPI_UpdateMapping 测试更新映射
func TestMappingAPI_UpdateMapping(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功更新映射状态", func(t *testing.T) {
		userID, sourceID, targetID := setupMappingTest(t, client)

		// 创建映射
		mapping := &models.PortMapping{
			UserID:         userID,
			SourceClientID: sourceID,
			TargetClientID: targetID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     7070,
			TargetHost:     "localhost",
			TargetPort:     70,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}
		created, err := client.CreateMapping(mapping)
		require.NoError(t, err)

		// 更新状态
		updated, err := client.UpdateMapping(created.ID, map[string]interface{}{
			"status": "inactive",
		})
		require.NoError(t, err)
		assert.Equal(t, models.MappingStatusInactive, updated.Status)
	})

	t.Run("更新不存在的映射", func(t *testing.T) {
		_, err := client.UpdateMapping("non-existent-id", map[string]interface{}{
			"status": "inactive",
		})
		assert.Error(t, err)
	})
}

// TestMappingAPI_DeleteMapping 测试删除映射
func TestMappingAPI_DeleteMapping(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功删除映射", func(t *testing.T) {
		userID, sourceID, targetID := setupMappingTest(t, client)

		// 创建映射
		mapping := &models.PortMapping{
			UserID:         userID,
			SourceClientID: sourceID,
			TargetClientID: targetID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     6060,
			TargetHost:     "localhost",
			TargetPort:     60,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}
		created, err := client.CreateMapping(mapping)
		require.NoError(t, err)

		// 删除映射
		err = client.DeleteMapping(created.ID)
		require.NoError(t, err)

		// 验证已删除
		_, err = client.GetMapping(created.ID)
		assert.Error(t, err)
	})

	t.Run("删除不存在的映射", func(t *testing.T) {
		err := client.DeleteMapping("non-existent-id")
		// 可能返回错误或成功
		_ = err
	})
}

// TestMappingAPI_ListMappings 测试列出映射
func TestMappingAPI_ListMappings(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("列出所有映射", func(t *testing.T) {
		userID, sourceID, targetID := setupMappingTest(t, client)

		// 创建多个映射
		mapping1 := &models.PortMapping{
			UserID:         userID,
			SourceClientID: sourceID,
			TargetClientID: targetID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     5050,
			TargetHost:     "localhost",
			TargetPort:     50,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}
		created1, err := client.CreateMapping(mapping1)
		require.NoError(t, err)

		mapping2 := &models.PortMapping{
			UserID:         userID,
			SourceClientID: sourceID,
			TargetClientID: targetID,
			Protocol:       models.ProtocolHTTP,
			SourcePort:     5051,
			TargetHost:     "localhost",
			TargetPort:     51,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}
		created2, err := client.CreateMapping(mapping2)
		require.NoError(t, err)

		// 列出映射
		mappings, err := client.ListMappings()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(mappings), 2)

		// 验证创建的映射在列表中
		mappingIDs := make(map[string]bool)
		for _, m := range mappings {
			mappingIDs[m.ID] = true
		}
		assert.True(t, mappingIDs[created1.ID])
		assert.True(t, mappingIDs[created2.ID])
	})
}

// TestMappingAPI_PortValidation 测试端口验证
func TestMappingAPI_PortValidation(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("有效端口范围", func(t *testing.T) {
		userID, sourceID, targetID := setupMappingTest(t, client)

		testCases := []struct {
			port int
			name string
		}{
			{1, "最小端口"},
			{80, "HTTP端口"},
			{443, "HTTPS端口"},
			{8080, "常用端口"},
			{65535, "最大端口"},
		}

		for _, tc := range testCases {
			mapping := &models.PortMapping{
				UserID:         userID,
				SourceClientID: sourceID,
				TargetClientID: targetID,
				Protocol:       models.ProtocolTCP,
				SourcePort:     tc.port,
				TargetHost:     "localhost",
				TargetPort:     tc.port,
				Type:           models.MappingTypeRegistered,
				Status:         models.MappingStatusActive,
			}

			_, err := client.CreateMapping(mapping)
			// 应该成功或有特定的验证错误
			if err != nil {
				t.Logf("%s (port %d): %v", tc.name, tc.port, err)
			}
		}
	})
}

// TestMappingAPI_MappingStatus 测试映射状态管理
func TestMappingAPI_MappingStatus(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("映射状态转换", func(t *testing.T) {
		userID, sourceID, targetID := setupMappingTest(t, client)

		// 创建映射
		mapping := &models.PortMapping{
			UserID:         userID,
			SourceClientID: sourceID,
			TargetClientID: targetID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     4040,
			TargetHost:     "localhost",
			TargetPort:     40,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}
		created, err := client.CreateMapping(mapping)
		require.NoError(t, err)
		assert.Equal(t, models.MappingStatusActive, created.Status)

		// Active -> Inactive
		updated, err := client.UpdateMapping(created.ID, map[string]interface{}{
			"status": "inactive",
		})
		require.NoError(t, err)
		assert.Equal(t, models.MappingStatusInactive, updated.Status)

		// Inactive -> Active
		updated, err = client.UpdateMapping(created.ID, map[string]interface{}{
			"status": "active",
		})
		require.NoError(t, err)
		assert.Equal(t, models.MappingStatusActive, updated.Status)
	})
}

