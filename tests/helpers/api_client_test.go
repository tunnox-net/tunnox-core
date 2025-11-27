package helpers

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
)

func TestNewAPIClient(t *testing.T) {
	ctx := context.Background()

	t.Run("创建API客户端", func(t *testing.T) {
		client := NewAPIClient(ctx, "http://localhost:8080/api/v1")
		if client == nil {
			t.Fatal("客户端实例为nil")
		}
		defer client.Close()

		if client.httpClient == nil {
			t.Error("HTTP客户端未初始化")
		}

		if client.baseURL != "http://localhost:8080/api/v1" {
			t.Errorf("baseURL不匹配: 期望 'http://localhost:8080/api/v1', 实际 '%s'", client.baseURL)
		}

		if client.authToken != "" {
			t.Error("初始认证令牌应该为空")
		}
	})

	t.Run("设置认证令牌", func(t *testing.T) {
		client := NewAPIClient(ctx, "http://localhost:8080/api/v1")
		defer client.Close()

		token := "test-token-123"
		client.SetAuthToken(token)

		if client.authToken != token {
			t.Errorf("认证令牌设置失败: 期望 '%s', 实际 '%s'", token, client.authToken)
		}
	})
}

func TestAPIClient_HealthCheck(t *testing.T) {
	ctx := context.Background()

	// 启动测试服务器
	server, err := NewTestAPIServer(ctx, nil)
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}
	defer server.Stop()

	if err := server.Start(); err != nil {
		t.Fatalf("启动测试服务器失败: %v", err)
	}

	// 创建客户端
	client := NewAPIClient(ctx, server.GetBaseURL())
	defer client.Close()

	t.Run("健康检查成功", func(t *testing.T) {
		ok, err := client.HealthCheck()
		if err != nil {
			t.Errorf("健康检查失败: %v", err)
		}

		if !ok {
			t.Error("健康检查应该返回成功")
		}
	})
}

func TestAPIClient_UserManagement(t *testing.T) {
	ctx := context.Background()

	// 启动测试服务器
	server, err := NewTestAPIServer(ctx, nil)
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}
	defer server.Stop()

	if err := server.Start(); err != nil {
		t.Fatalf("启动测试服务器失败: %v", err)
	}

	// 创建客户端
	client := NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()

	var createdUserID string

	t.Run("创建用户", func(t *testing.T) {
		user, err := client.CreateUser("testuser", "test@example.com")
		if err != nil {
			t.Fatalf("创建用户失败: %v", err)
		}

		if user == nil {
			t.Fatal("返回的用户为nil")
		}

		if user.Username != "testuser" {
			t.Errorf("用户名不匹配: 期望 'testuser', 实际 '%s'", user.Username)
		}

		if user.Email != "test@example.com" {
			t.Errorf("邮箱不匹配: 期望 'test@example.com', 实际 '%s'", user.Email)
		}

		if user.ID == "" {
			t.Error("用户ID为空")
		}

		createdUserID = user.ID
	})

	t.Run("获取用户", func(t *testing.T) {
		if createdUserID == "" {
			t.Skip("没有创建的用户ID")
		}

		user, err := client.GetUser(createdUserID)
		if err != nil {
			t.Fatalf("获取用户失败: %v", err)
		}

		if user.ID != createdUserID {
			t.Errorf("用户ID不匹配: 期望 '%s', 实际 '%s'", createdUserID, user.ID)
		}
	})

	t.Run("更新用户", func(t *testing.T) {
		if createdUserID == "" {
			t.Skip("没有创建的用户ID")
		}

		updates := map[string]interface{}{
			"email": "newemail@example.com",
		}

		user, err := client.UpdateUser(createdUserID, updates)
		if err != nil {
			t.Fatalf("更新用户失败: %v", err)
		}

		if user.Email != "newemail@example.com" {
			t.Errorf("邮箱未更新: 期望 'newemail@example.com', 实际 '%s'", user.Email)
		}
	})

	t.Run("列出用户", func(t *testing.T) {
		users, err := client.ListUsers()
		if err != nil {
			t.Fatalf("列出用户失败: %v", err)
		}

		if len(users) == 0 {
			t.Error("用户列表为空")
		}
	})

	t.Run("删除用户", func(t *testing.T) {
		if createdUserID == "" {
			t.Skip("没有创建的用户ID")
		}

		err := client.DeleteUser(createdUserID)
		if err != nil {
			t.Errorf("删除用户失败: %v", err)
		}

		// 验证用户已删除
		_, err = client.GetUser(createdUserID)
		if err == nil {
			t.Error("删除后仍能获取用户")
		}
	})
}

func TestAPIClient_ClientManagement(t *testing.T) {
	ctx := context.Background()

	// 启动测试服务器
	server, err := NewTestAPIServer(ctx, nil)
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}
	defer server.Stop()

	if err := server.Start(); err != nil {
		t.Fatalf("启动测试服务器失败: %v", err)
	}

	// 创建客户端
	apiClient := NewAPIClient(ctx, server.GetAPIURL())
	defer apiClient.Close()

	// 先创建一个用户
	user, err := apiClient.CreateUser("clienttest", "clienttest@example.com")
	if err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	defer apiClient.DeleteUser(user.ID)

	var createdClientID int64

	t.Run("创建客户端", func(t *testing.T) {
		client, err := apiClient.CreateClient(user.ID, "Test Client")
		if err != nil {
			t.Fatalf("创建客户端失败: %v", err)
		}

		if client == nil {
			t.Fatal("返回的客户端为nil")
		}

		if client.Name != "Test Client" {
			t.Errorf("客户端名称不匹配: 期望 'Test Client', 实际 '%s'", client.Name)
		}

		if client.UserID != user.ID {
			t.Errorf("用户ID不匹配: 期望 '%s', 实际 '%s'", user.ID, client.UserID)
		}

		if client.ID == 0 {
			t.Error("客户端ID为0")
		}

		createdClientID = client.ID
	})

	t.Run("获取客户端", func(t *testing.T) {
		if createdClientID == 0 {
			t.Skip("没有创建的客户端ID")
		}

		client, err := apiClient.GetClient(createdClientID)
		if err != nil {
			t.Fatalf("获取客户端失败: %v", err)
		}

		if client.ID != createdClientID {
			t.Errorf("客户端ID不匹配: 期望 '%d', 实际 '%d'", createdClientID, client.ID)
		}
	})

	t.Run("更新客户端", func(t *testing.T) {
		if createdClientID == 0 {
			t.Skip("没有创建的客户端ID")
		}

		updates := map[string]interface{}{
			"client_name": "Updated Client",
		}

		client, err := apiClient.UpdateClient(createdClientID, updates)
		if err != nil {
			t.Fatalf("更新客户端失败: %v", err)
		}

		if client.Name != "Updated Client" {
			t.Errorf("客户端名称未更新: 期望 'Updated Client', 实际 '%s'", client.Name)
		}
	})

	t.Run("列出客户端", func(t *testing.T) {
		clients, err := apiClient.ListClients()
		if err != nil {
			t.Fatalf("列出客户端失败: %v", err)
		}

		// 至少应该包含我们刚创建的客户端
		found := false
		for _, c := range clients {
			if c.ID == createdClientID {
				found = true
				break
			}
		}

		if !found && createdClientID != 0 {
			t.Errorf("列表中未找到创建的客户端 ID: %d, 总共 %d 个客户端", createdClientID, len(clients))
		}
	})

	t.Run("删除客户端", func(t *testing.T) {
		if createdClientID == 0 {
			t.Skip("没有创建的客户端ID")
		}

		err := apiClient.DeleteClient(createdClientID)
		if err != nil {
			t.Errorf("删除客户端失败: %v", err)
		}

		// 验证客户端已删除
		_, err = apiClient.GetClient(createdClientID)
		if err == nil {
			t.Error("删除后仍能获取客户端")
		}
	})
}

func TestAPIClient_MappingManagement(t *testing.T) {
	ctx := context.Background()

	// 启动测试服务器
	server, err := NewTestAPIServer(ctx, nil)
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}
	defer server.Stop()

	if err := server.Start(); err != nil {
		t.Fatalf("启动测试服务器失败: %v", err)
	}

	// 创建客户端
	apiClient := NewAPIClient(ctx, server.GetAPIURL())
	defer apiClient.Close()

	// 创建测试用户和客户端
	user, err := apiClient.CreateUser("mappingtest", "mappingtest@example.com")
	if err != nil {
		t.Fatalf("创建用户失败: %v", err)
	}
	defer apiClient.DeleteUser(user.ID)

	client1, err := apiClient.CreateClient(user.ID, "Source Client")
	if err != nil {
		t.Fatalf("创建源客户端失败: %v", err)
	}
	defer apiClient.DeleteClient(client1.ID)

	client2, err := apiClient.CreateClient(user.ID, "Target Client")
	if err != nil {
		t.Fatalf("创建目标客户端失败: %v", err)
	}
	defer apiClient.DeleteClient(client2.ID)

	var createdMappingID string

	t.Run("创建映射", func(t *testing.T) {
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

		result, err := apiClient.CreateMapping(mapping)
		if err != nil {
			t.Fatalf("创建映射失败: %v", err)
		}

		if result == nil {
			t.Fatal("返回的映射为nil")
		}

		if result.ID == "" {
			t.Error("映射ID为空")
		}

		if result.SourcePort != 8080 {
			t.Errorf("源端口不匹配: 期望 8080, 实际 %d", result.SourcePort)
		}

		createdMappingID = result.ID
	})

	t.Run("获取映射", func(t *testing.T) {
		if createdMappingID == "" {
			t.Skip("没有创建的映射ID")
		}

		mapping, err := apiClient.GetMapping(createdMappingID)
		if err != nil {
			t.Fatalf("获取映射失败: %v", err)
		}

		if mapping.ID != createdMappingID {
			t.Errorf("映射ID不匹配: 期望 '%s', 实际 '%s'", createdMappingID, mapping.ID)
		}
	})

	t.Run("更新映射", func(t *testing.T) {
		if createdMappingID == "" {
			t.Skip("没有创建的映射ID")
		}

		// 注意：UpdateMapping API 只支持更新 status 字段
		updates := map[string]interface{}{
			"status": "inactive",
		}

		mapping, err := apiClient.UpdateMapping(createdMappingID, updates)
		if err != nil {
			t.Fatalf("更新映射失败: %v", err)
		}

		if mapping.Status != models.MappingStatusInactive {
			t.Errorf("状态未更新: 期望 'inactive', 实际 '%s'", mapping.Status)
		}
	})

	t.Run("列出映射", func(t *testing.T) {
		mappings, err := apiClient.ListMappings()
		if err != nil {
			t.Fatalf("列出映射失败: %v", err)
		}

		// 至少应该包含我们刚创建的映射
		found := false
		for _, m := range mappings {
			if m.ID == createdMappingID {
				found = true
				break
			}
		}

		if !found && createdMappingID != "" {
			t.Errorf("列表中未找到创建的映射 ID: %s, 总共 %d 个映射", createdMappingID, len(mappings))
		}
	})

	t.Run("删除映射", func(t *testing.T) {
		if createdMappingID == "" {
			t.Skip("没有创建的映射ID")
		}

		err := apiClient.DeleteMapping(createdMappingID)
		if err != nil {
			t.Errorf("删除映射失败: %v", err)
		}

		// 验证映射已删除
		_, err = apiClient.GetMapping(createdMappingID)
		if err == nil {
			t.Error("删除后仍能获取映射")
		}
	})
}

func TestAPIClient_SearchOperations(t *testing.T) {
	ctx := context.Background()

	// 启动测试服务器
	server, err := NewTestAPIServer(ctx, nil)
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}
	defer server.Stop()

	if err := server.Start(); err != nil {
		t.Fatalf("启动测试服务器失败: %v", err)
	}

	// 创建客户端
	apiClient := NewAPIClient(ctx, server.GetAPIURL())
	defer apiClient.Close()

	// 创建一些测试数据
	user, _ := apiClient.CreateUser("searchtest", "searchtest@example.com")
	if user != nil {
		defer apiClient.DeleteUser(user.ID)

		client, _ := apiClient.CreateClient(user.ID, "Searchable Client")
		if client != nil {
			defer apiClient.DeleteClient(client.ID)
		}
	}

	t.Run("搜索用户", func(t *testing.T) {
		users, err := apiClient.SearchUsers("search")
		if err != nil {
			t.Errorf("搜索用户失败: %v", err)
		}

		// 不一定能找到结果，但不应该报错
		_ = users
	})

	t.Run("搜索客户端", func(t *testing.T) {
		clients, err := apiClient.SearchClients("search")
		if err != nil {
			t.Errorf("搜索客户端失败: %v", err)
		}

		_ = clients
	})

	t.Run("搜索映射", func(t *testing.T) {
		mappings, err := apiClient.SearchMappings("test")
		if err != nil {
			t.Errorf("搜索映射失败: %v", err)
		}

		_ = mappings
	})
}

func TestAPIClient_DisposablePattern(t *testing.T) {
	ctx := context.Background()

	t.Run("验证dispose模式", func(t *testing.T) {
		client := NewAPIClient(ctx, "http://localhost:8080/api/v1")

		// 验证ResourceBase初始化
		if client.ResourceBase == nil {
			t.Fatal("ResourceBase未初始化")
		}

		// 第一次关闭
		if err := client.Close(); err != nil {
			t.Errorf("第一次关闭失败: %v", err)
		}

		// 验证IsClosed状态
		if !client.IsClosed() {
			t.Error("客户端应该已关闭")
		}

		// 第二次关闭应该是安全的
		if err := client.Close(); err != nil {
			t.Errorf("第二次关闭失败: %v", err)
		}
	})

	t.Run("上下文取消时自动清理", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		client := NewAPIClient(ctx, "http://localhost:8080/api/v1")

		// 取消上下文
		cancel()

		// 等待一小段时间让清理完成
		time.Sleep(100 * time.Millisecond)

		// 验证客户端已关闭
		if !client.IsClosed() {
			t.Error("客户端应该在上下文取消时自动关闭")
		}
	})
}

