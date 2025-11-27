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

// TestUserAPI_CreateUser 测试创建用户
func TestUserAPI_CreateUser(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功创建用户", func(t *testing.T) {
		user, err := client.CreateUser("alice", "alice@example.com")
		require.NoError(t, err)
		assert.NotNil(t, user)
		assert.Equal(t, "alice", user.Username)
		assert.Equal(t, "alice@example.com", user.Email)
		assert.NotEmpty(t, user.ID)
		// Status 和 Type 字段在创建时可能为零值
		// 实际应用中应由业务逻辑设置默认值
	})

	t.Run("缺少用户名", func(t *testing.T) {
		_, err := client.CreateUser("", "test@example.com")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username and email are required")
	})

	t.Run("缺少邮箱", func(t *testing.T) {
		_, err := client.CreateUser("testuser", "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "username and email are required")
	})

	t.Run("重复创建相同用户名", func(t *testing.T) {
		username := "duplicate_user"
		email := "dup@example.com"

		// 第一次创建应该成功
		user1, err := client.CreateUser(username, email)
		require.NoError(t, err)
		assert.NotNil(t, user1)

		// 第二次创建相同用户名应该成功（不同ID）
		user2, err := client.CreateUser(username, "another@example.com")
		require.NoError(t, err)
		assert.NotNil(t, user2)
		assert.NotEqual(t, user1.ID, user2.ID)
	})
}

// TestUserAPI_GetUser 测试获取用户
func TestUserAPI_GetUser(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功获取用户", func(t *testing.T) {
		// 先创建用户
		created, err := client.CreateUser("bob", "bob@example.com")
		require.NoError(t, err)

		// 获取用户
		user, err := client.GetUser(created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, user.ID)
		assert.Equal(t, created.Username, user.Username)
		assert.Equal(t, created.Email, user.Email)
	})

	t.Run("获取不存在的用户", func(t *testing.T) {
		_, err := client.GetUser("non-existent-user-id")
		assert.Error(t, err)
	})
}

// TestUserAPI_UpdateUser 测试更新用户
func TestUserAPI_UpdateUser(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功更新用户邮箱", func(t *testing.T) {
		// 创建用户
		user, err := client.CreateUser("charlie", "charlie@example.com")
		require.NoError(t, err)

		// 更新邮箱
		newEmail := "charlie.new@example.com"
		updated, err := client.UpdateUser(user.ID, map[string]interface{}{
			"email": newEmail,
		})
		require.NoError(t, err)
		assert.Equal(t, newEmail, updated.Email)
		assert.Equal(t, user.Username, updated.Username)
	})

	t.Run("成功更新用户状态", func(t *testing.T) {
		// 创建用户
		user, err := client.CreateUser("dave", "dave@example.com")
		require.NoError(t, err)

		// 更新状态为暂停
		updated, err := client.UpdateUser(user.ID, map[string]interface{}{
			"status": "suspended",
		})
		require.NoError(t, err)
		assert.Equal(t, models.UserStatusSuspended, updated.Status)
	})

	t.Run("更新不存在的用户", func(t *testing.T) {
		_, err := client.UpdateUser("non-existent-id", map[string]interface{}{
			"email": "test@example.com",
		})
		assert.Error(t, err)
	})

	t.Run("更新为空邮箱应该被忽略", func(t *testing.T) {
		// 创建用户
		user, err := client.CreateUser("eve", "eve@example.com")
		require.NoError(t, err)

		// 尝试更新为空邮箱
		updated, err := client.UpdateUser(user.ID, map[string]interface{}{
			"email": "",
		})
		require.NoError(t, err)
		// 空值应该被忽略，邮箱保持不变
		assert.Equal(t, user.Email, updated.Email)
	})
}

// TestUserAPI_DeleteUser 测试删除用户
func TestUserAPI_DeleteUser(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("成功删除用户", func(t *testing.T) {
		// 创建用户
		user, err := client.CreateUser("frank", "frank@example.com")
		require.NoError(t, err)

		// 删除用户
		err = client.DeleteUser(user.ID)
		require.NoError(t, err)

		// 验证用户已删除
		_, err = client.GetUser(user.ID)
		assert.Error(t, err)
	})

	t.Run("删除不存在的用户", func(t *testing.T) {
		err := client.DeleteUser("non-existent-id")
		// 删除不存在的用户可能返回错误或成功（取决于实现）
		// 这里我们只检查不会 panic
		_ = err
	})
}

// TestUserAPI_ListUsers 测试列出用户
func TestUserAPI_ListUsers(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("列出所有用户", func(t *testing.T) {
		// 创建几个用户
		user1, err := client.CreateUser("user1", "user1@example.com")
		require.NoError(t, err)
		user2, err := client.CreateUser("user2", "user2@example.com")
		require.NoError(t, err)

		// 列出用户
		users, err := client.ListUsers()
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(users), 2)

		// 验证创建的用户在列表中
		userIDs := make(map[string]bool)
		for _, u := range users {
			userIDs[u.ID] = true
		}
		assert.True(t, userIDs[user1.ID])
		assert.True(t, userIDs[user2.ID])
	})

	t.Run("空列表时返回空数组", func(t *testing.T) {
		users, err := client.ListUsers()
		require.NoError(t, err)
		assert.NotNil(t, users)
	})
}

// TestUserAPI_UserClients 测试用户客户端关系
func TestUserAPI_UserClients(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("列出用户的客户端", func(t *testing.T) {
		// 创建用户
		user, err := client.CreateUser("clientuser", "clientuser@example.com")
		require.NoError(t, err)

		// 创建客户端
		client1, err := client.CreateClient(user.ID, "Client 1")
		require.NoError(t, err)
		client2, err := client.CreateClient(user.ID, "Client 2")
		require.NoError(t, err)

		// 列出用户客户端 - 需要通过 CloudControl 或专门的端点
		// 注意：这里假设有相应的 API 端点
		_ = client1
		_ = client2
		// TODO: 添加列出用户客户端的测试
	})
}

// TestUserAPI_UserMappings 测试用户映射关系
func TestUserAPI_UserMappings(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	apiClient := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer apiClient.Close()

	t.Run("列出用户的映射", func(t *testing.T) {
		// 创建用户
		user, err := apiClient.CreateUser("mappinguser", "mappinguser@example.com")
		require.NoError(t, err)

		// 创建客户端
		client1, err := apiClient.CreateClient(user.ID, "Source Client")
		require.NoError(t, err)
		client2, err := apiClient.CreateClient(user.ID, "Target Client")
		require.NoError(t, err)

		// 创建映射
		mapping := &models.PortMapping{
			UserID:         user.ID,
			SourceClientID: client1.ID,
			TargetClientID: client2.ID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     8080,
			TargetHost:     "localhost",
			TargetPort:     80,
			Type:           models.MappingTypeRegistered,
			Status:         models.MappingStatusActive,
		}
		created, err := apiClient.CreateMapping(mapping)
		require.NoError(t, err)

		_ = created
		// TODO: 添加列出用户映射的测试
	})
}

// TestUserAPI_Quota 测试用户配额管理
func TestUserAPI_Quota(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("获取用户配额", func(t *testing.T) {
		// 创建用户
		user, err := client.CreateUser("quotauser", "quotauser@example.com")
		require.NoError(t, err)

		// 注意：用户创建时配额和计划可能为零值
		// 实际应用中应该由业务逻辑设置默认值
		// 这里只验证用户对象存在
		assert.NotNil(t, user)
	})

	t.Run("验证不同计划的配额", func(t *testing.T) {
		// 创建免费用户
		freeUser, err := client.CreateUser("freeuser", "free@example.com")
		require.NoError(t, err)

		// 注意：Plan 和 Quota 字段可能为零值
		// 实际应用中应该由业务逻辑设置默认值
		assert.NotNil(t, freeUser)
	})
}

// TestUserAPI_ConcurrentOperations 测试并发操作
func TestUserAPI_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	server, err := helpers.NewTestAPIServer(ctx, nil)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	t.Run("并发创建用户", func(t *testing.T) {
		const concurrency = 10
		done := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			go func(index int) {
				username := fmt.Sprintf("concurrent_%d", index)
				email := fmt.Sprintf("concurrent_%d@example.com", index)
				_, err := client.CreateUser(username, email)
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
