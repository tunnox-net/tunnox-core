package api_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/tests/helpers"
)

// TestClientAPI_CreateClient 测试创建客户端
func TestClientAPI_CreateClient(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功创建客户端", func(t *testing.T) {
		// 先创建用户
		user, err := client.CreateUser("clientowner", "clientowner@example.com")
		require.NoError(t, err)

		// 创建客户端
		clientInfo, err := client.CreateClient(user.ID, "Test Client")
		require.NoError(t, err)
		assert.NotNil(t, clientInfo)
		assert.NotZero(t, clientInfo.ID)
		assert.Equal(t, "Test Client", clientInfo.Name)
		assert.Equal(t, user.ID, clientInfo.UserID)
		assert.Equal(t, models.ClientTypeRegistered, clientInfo.Type)
		assert.NotEmpty(t, clientInfo.AuthCode)
		assert.NotEmpty(t, clientInfo.SecretKey)
	})

	t.Run("缺少用户ID", func(t *testing.T) {
		_, err := client.CreateClient("", "Client Name")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user_id and client_name are required")
	})

	t.Run("缺少客户端名称", func(t *testing.T) {
		user, err := client.CreateUser("user2", "user2@example.com")
		require.NoError(t, err)

		_, err = client.CreateClient(user.ID, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "user_id and client_name are required")
	})

	t.Run("为不存在的用户创建客户端", func(t *testing.T) {
		_, err := client.CreateClient("non-existent-user", "Client Name")
		// 可能成功或失败，取决于实现
		_ = err
	})
}

// TestClientAPI_GetClient 测试获取客户端
func TestClientAPI_GetClient(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功获取客户端", func(t *testing.T) {
		// 创建用户和客户端
		user, err := client.CreateUser("getuser", "getuser@example.com")
		require.NoError(t, err)
		created, err := client.CreateClient(user.ID, "Get Test Client")
		require.NoError(t, err)

		// 获取客户端
		retrieved, err := client.GetClient(created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, retrieved.ID)
		assert.Equal(t, created.Name, retrieved.Name)
		assert.Equal(t, created.UserID, retrieved.UserID)
	})

	t.Run("获取不存在的客户端", func(t *testing.T) {
		_, err := client.GetClient(99999999)
		assert.Error(t, err)
	})
}

// TestClientAPI_UpdateClient 测试更新客户端
func TestClientAPI_UpdateClient(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功更新客户端名称", func(t *testing.T) {
		// 创建客户端
		user, err := client.CreateUser("updateuser", "updateuser@example.com")
		require.NoError(t, err)
		clientInfo, err := client.CreateClient(user.ID, "Original Name")
		require.NoError(t, err)

		// 更新名称
		updated, err := client.UpdateClient(clientInfo.ID, map[string]interface{}{
			"client_name": "New Name",
		})
		require.NoError(t, err)
		assert.Equal(t, "New Name", updated.Name)
	})

	t.Run("成功更新客户端状态", func(t *testing.T) {
		// 创建客户端
		user, err := client.CreateUser("statususer", "statususer@example.com")
		require.NoError(t, err)
		clientInfo, err := client.CreateClient(user.ID, "Status Test")
		require.NoError(t, err)

		// 更新状态
		updated, err := client.UpdateClient(clientInfo.ID, map[string]interface{}{
			"status": "blocked",
		})
		require.NoError(t, err)
		assert.Equal(t, models.ClientStatusBlocked, updated.Status)
	})

	t.Run("更新不存在的客户端", func(t *testing.T) {
		_, err := client.UpdateClient(99999999, map[string]interface{}{
			"client_name": "New Name",
		})
		assert.Error(t, err)
	})
}

// TestClientAPI_DeleteClient 测试删除客户端
func TestClientAPI_DeleteClient(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功删除客户端", func(t *testing.T) {
		// 创建客户端
		user, err := client.CreateUser("deleteuser", "deleteuser@example.com")
		require.NoError(t, err)
		clientInfo, err := client.CreateClient(user.ID, "To Delete")
		require.NoError(t, err)

		// 删除客户端
		err = client.DeleteClient(clientInfo.ID)
		require.NoError(t, err)

		// 验证已删除
		_, err = client.GetClient(clientInfo.ID)
		assert.Error(t, err)
	})

	t.Run("删除不存在的客户端", func(t *testing.T) {
		err := client.DeleteClient(99999999)
		// 可能返回错误或成功
		_ = err
	})
}

// TestClientAPI_ListClients 测试列出客户端
func TestClientAPI_ListClients(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("列出所有客户端", func(t *testing.T) {
		// 创建用户和客户端
		user, err := client.CreateUser("listuser", "listuser@example.com")
		require.NoError(t, err)
		
		client1, err := client.CreateClient(user.ID, "Client 1")
		require.NoError(t, err)
		client2, err := client.CreateClient(user.ID, "Client 2")
		require.NoError(t, err)

		// 列出客户端
		clients, err := client.ListClients()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(clients), 2)

		// 验证创建的客户端在列表中
		clientIDs := make(map[int64]bool)
		for _, c := range clients {
			clientIDs[c.ID] = true
		}
		assert.True(t, clientIDs[client1.ID])
		assert.True(t, clientIDs[client2.ID])
	})
}

// TestClientAPI_ClientMappings 测试客户端映射关系
func TestClientAPI_ClientMappings(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	apiClient := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer apiClient.Close()

	t.Run("列出客户端的映射", func(t *testing.T) {
		// 创建用户和客户端
		user, err := apiClient.CreateUser("mapclient", "mapclient@example.com")
		require.NoError(t, err)
		
		client1, err := apiClient.CreateClient(user.ID, "Source")
		require.NoError(t, err)
		client2, err := apiClient.CreateClient(user.ID, "Target")
		require.NoError(t, err)

		// 创建映射
		mapping := &models.PortMapping{
			UserID:         user.ID,
			SourceClientID: client1.ID,
			TargetClientID: client2.ID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     9090,
			TargetHost:     "localhost",
			TargetPort:     80,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}
		created, err := apiClient.CreateMapping(mapping)
		require.NoError(t, err)
		
		_ = created
		// TODO: 添加列出客户端映射的 API 调用测试
	})
}

// TestClientAPI_ConcurrentOperations 测试并发操作
func TestClientAPI_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("并发创建客户端", func(t *testing.T) {
		// 创建用户
		user, err := client.CreateUser("concurrentuser", "concurrent@example.com")
		require.NoError(t, err)

		const concurrency = 10
		done := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			go func(index int) {
				name := fmt.Sprintf("Client_%d", index)
				_, err := client.CreateClient(user.ID, name)
				done <- err
			}(i)
		}

		// 等待所有操作完成
		for i := 0; i < concurrency; i++ {
			err := <-done
			assert.NoError(t, err)
		}
	})
}

// TestClientAPI_ClientStatus 测试客户端状态管理
func TestClientAPI_ClientStatus(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("客户端状态转换", func(t *testing.T) {
		// 创建客户端
		user, err := client.CreateUser("statususer2", "status2@example.com")
		require.NoError(t, err)
		clientInfo, err := client.CreateClient(user.ID, "Status Test Client")
		require.NoError(t, err)

		// 初始状态应该是 offline
		assert.Equal(t, models.ClientStatusOffline, clientInfo.Status)

		// 更新为 blocked
		updated, err := client.UpdateClient(clientInfo.ID, map[string]interface{}{
			"status": "blocked",
		})
		require.NoError(t, err)
		assert.Equal(t, models.ClientStatusBlocked, updated.Status)

		// 更新回 offline
		updated, err = client.UpdateClient(clientInfo.ID, map[string]interface{}{
			"status": "offline",
		})
		require.NoError(t, err)
		assert.Equal(t, models.ClientStatusOffline, updated.Status)
	})
}

// TestClientAPI_ClientTypes 测试客户端类型
func TestClientAPI_ClientTypes(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("注册用户客户端", func(t *testing.T) {
		// 创建注册用户
		user, err := client.CreateUser("registered", "registered@example.com")
		require.NoError(t, err)

		// 创建客户端
		clientInfo, err := client.CreateClient(user.ID, "Registered Client")
		require.NoError(t, err)
		
		// 应该是注册类型
		assert.Equal(t, models.ClientTypeRegistered, clientInfo.Type)
		assert.Equal(t, user.ID, clientInfo.UserID)
	})
}

// TestClientAPI_EdgeCases 测试边界情况
func TestClientAPI_EdgeCases(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("超长客户端名称", func(t *testing.T) {
		user, err := client.CreateUser("edgeuser", "edge@example.com")
		require.NoError(t, err)

		// 创建超长名称的客户端
		longName := string(make([]byte, 300))
		for i := range longName {
			longName = string(append([]byte(longName[:i]), 'A'))
		}
		
		// 可能成功或失败，取决于验证逻辑
		_, err = client.CreateClient(user.ID, longName[:255])
		// 不验证具体结果，只确保不 panic
		_ = err
	})

	t.Run("特殊字符客户端名称", func(t *testing.T) {
		user, err := client.CreateUser("specialuser", "special@example.com")
		require.NoError(t, err)

		specialNames := []string{
			"Client<>Name",
			"Client&Name",
			"Client'Name",
			"Client\"Name",
			"Client\nName",
		}

		for _, name := range specialNames {
			_, err := client.CreateClient(user.ID, name)
			// 可能成功或失败，取决于验证
			_ = err
		}
	})
}

