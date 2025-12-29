package factories

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
)

func TestBuiltInCloudControl_JWTTokenManagement(t *testing.T) {
	config := managers.DefaultConfig()
	api := NewBuiltinCloudControlWithServices(context.Background(), config)

	t.Run("GenerateJWTToken", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient("test_user_1", "Test JWT Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		if tokenInfo.Token == "" {
			t.Error("Expected non-empty JWT token")
		}
		if tokenInfo.ClientId != client.ID {
			t.Errorf("Expected client ID %d, got %d", client.ID, tokenInfo.ClientId)
		}
		if tokenInfo.ExpiresAt.Before(time.Now()) {
			t.Error("Token should not be expired")
		}
	})

	t.Run("ValidateJWTToken", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient("test_user_1", "Test Validate Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		// 验证有效 token
		validTokenInfo, err := api.ValidateJWTToken(tokenInfo.Token)
		if err != nil {
			t.Fatalf("ValidateJWTToken failed: %v", err)
		}

		if validTokenInfo.ClientId != client.ID {
			t.Errorf("Expected client ID %d, got %d", client.ID, validTokenInfo.ClientId)
		}

		// 验证无效 token
		_, err = api.ValidateJWTToken("invalid_token")
		if err == nil {
			t.Error("Expected validation to fail with invalid token")
		}
	})

	t.Run("RevokeJWTToken", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient("test_user_1", "Test Revoke Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		// 验证 token 存在
		_, err = api.ValidateJWTToken(tokenInfo.Token)
		if err != nil {
			t.Fatalf("ValidateJWTToken failed: %v", err)
		}

		// 撤销 token
		err = api.RevokeJWTToken(tokenInfo.Token)
		if err != nil {
			t.Fatalf("RevokeJWTToken failed: %v", err)
		}

		// 验证 token 已被撤销
		_, err = api.ValidateJWTToken(tokenInfo.Token)
		if err == nil {
			t.Error("Expected token to be revoked")
		}
	})

	t.Run("RefreshJWTToken", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient("test_user_1", "Test Refresh Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		// 刷新 token
		newTokenInfo, err := api.RefreshJWTToken(tokenInfo.RefreshToken)
		if err != nil {
			t.Fatalf("RefreshJWTToken failed: %v", err)
		}

		if newTokenInfo.Token == tokenInfo.Token {
			t.Error("Expected new token to be different from original token")
		}
		if newTokenInfo.ClientId != client.ID {
			t.Errorf("Expected client ID %d, got %d", client.ID, newTokenInfo.ClientId)
		}

		// 新token也能通过校验
		validTokenInfo, err := api.ValidateJWTToken(newTokenInfo.Token)
		if err != nil {
			t.Fatalf("ValidateJWTToken for refreshed token failed: %v", err)
		}
		if validTokenInfo.ClientId != client.ID {
			t.Errorf("Expected client ID %d, got %d", client.ID, validTokenInfo.ClientId)
		}

		// 旧token仍然有效（当前实现不自动撤销旧token）
		_, err = api.ValidateJWTToken(tokenInfo.Token)
		if err != nil {
			t.Logf("Old token validation result: %v (this is expected in current implementation)", err)
		}
	})
}

func TestBuiltInCloudControl_ConnectionManagement(t *testing.T) {
	config := managers.DefaultConfig()
	api := NewBuiltinCloudControlWithServices(context.Background(), config)

	// 注意：RegisterConnection 会生成新的连接 ID，而不是使用传入的 ConnID
	// 因此需要在同一个测试中完成整个生命周期

	t.Run("RegisterConnection_and_GetConnections", func(t *testing.T) {
		// 先创建两个客户端
		client1, err := api.CreateClient("test_user_1", "Test Client 1")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}
		client2, err := api.CreateClient("test_user_1", "Test Client 2")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 先创建一个端口映射
		mapping := &models.PortMapping{
			ID:             "test_mapping_1",
			UserID:         "test_user_1",
			ListenClientID: client1.ID,
			TargetClientID: client2.ID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     8080,
			TargetPort:     80,
			Status:         models.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		createdMapping, err := api.CreatePortMapping(mapping)
		if err != nil {
			t.Fatalf("CreatePortMapping failed: %v", err)
		}

		// 创建连接信息（注意：ConnID 会被 RegisterConnection 重新生成）
		connInfo := &models.ConnectionInfo{
			MappingID:     createdMapping.ID,
			ClientID:      client1.ID,
			SourceIP:      "127.0.0.1",
			EstablishedAt: time.Now(),
			LastActivity:  time.Now(),
			BytesSent:     1024,
			BytesReceived: 2048,
			Status:        "active",
		}

		// 注册连接
		err = api.RegisterConnection(createdMapping.ID, connInfo)
		if err != nil {
			t.Fatalf("RegisterConnection failed: %v", err)
		}

		// 获取连接列表
		connections, err := api.GetConnections(createdMapping.ID)
		if err != nil {
			t.Fatalf("GetConnections failed: %v", err)
		}

		if connections == nil {
			t.Error("Expected connections list, got nil")
		}

		// 验证连接被注册（connInfo.ConnID 应该已被更新）
		if connInfo.ConnID == "" {
			t.Error("Expected ConnID to be set after registration")
		}
	})

	t.Run("GetClientConnections", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient("test_user_1", "Test Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 获取客户端连接列表
		connections, err := api.GetClientConnections(client.ID)
		if err != nil {
			t.Fatalf("GetClientConnections failed: %v", err)
		}

		// 新创建的客户端没有连接，应该返回空列表
		if connections == nil {
			t.Error("Expected connections list, got nil")
		}
	})

	t.Run("FullConnectionLifecycle", func(t *testing.T) {
		// 创建客户端
		client1, err := api.CreateClient("test_user_lifecycle", "Lifecycle Client 1")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}
		client2, err := api.CreateClient("test_user_lifecycle", "Lifecycle Client 2")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 创建端口映射
		mapping := &models.PortMapping{
			UserID:         "test_user_lifecycle",
			ListenClientID: client1.ID,
			TargetClientID: client2.ID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     9090,
			TargetPort:     90,
			Status:         models.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		createdMapping, err := api.CreatePortMapping(mapping)
		if err != nil {
			t.Fatalf("CreatePortMapping failed: %v", err)
		}

		// 创建连接信息
		connInfo := &models.ConnectionInfo{
			MappingID:     createdMapping.ID,
			ClientID:      client1.ID,
			SourceIP:      "192.168.1.1",
			EstablishedAt: time.Now(),
			LastActivity:  time.Now(),
			BytesSent:     100,
			BytesReceived: 200,
			Status:        "active",
		}

		// 1. 注册连接
		err = api.RegisterConnection(createdMapping.ID, connInfo)
		if err != nil {
			t.Fatalf("RegisterConnection failed: %v", err)
		}

		// 获取生成的连接 ID
		generatedConnID := connInfo.ConnID
		if generatedConnID == "" {
			t.Fatal("Expected ConnID to be generated")
		}

		// 2. 更新连接统计（使用生成的连接 ID）
		err = api.UpdateConnectionStats(generatedConnID, 1500, 2500)
		if err != nil {
			t.Fatalf("UpdateConnectionStats failed: %v", err)
		}

		// 3. 注销连接
		err = api.UnregisterConnection(generatedConnID)
		if err != nil {
			t.Fatalf("UnregisterConnection failed: %v", err)
		}
	})
}

func TestBuiltInCloudControl_AuthenticationWithJWT(t *testing.T) {
	config := managers.DefaultConfig()
	api := NewBuiltinCloudControlWithServices(context.Background(), config)

	t.Run("Authenticate with JWT token", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient("test_user_1", "Test Auth Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		_, err = api.GenerateJWTToken(client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		authReq := &models.AuthRequest{
			ClientID:  client.ID,
			AuthCode:  client.AuthCode,
			SecretKey: client.SecretKey,
			NodeID:    "test_node_1",
			IPAddress: "127.0.0.1",
			Version:   "1.0.0",
		}

		authResp, err := api.Authenticate(authReq)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}

		if !authResp.Success {
			t.Errorf("Expected authentication success, got: %s", authResp.Message)
		}

		if authResp.Client.ID != client.ID {
			t.Errorf("Expected client ID %d, got %d", client.ID, authResp.Client.ID)
		}
	})

	t.Run("ValidateToken with JWT", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient("test_user_1", "Test Validate JWT Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		// 注意：ValidateToken 不需要客户端在线状态，它只验证 JWT 令牌

		// 验证有效 token
		authResp, err := api.ValidateToken(tokenInfo.Token)
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}

		if !authResp.Success {
			t.Errorf("Expected token validation success, got: %s", authResp.Message)
		}

		if authResp.Client != nil && authResp.Client.ID != client.ID {
			t.Errorf("Expected client ID %d, got %d", client.ID, authResp.Client.ID)
		}

		// 验证无效 token
		authResp, err = api.ValidateToken("invalid_token")
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}

		if authResp.Success {
			t.Error("Expected token validation to fail with invalid token")
		}
	})
}
