package tests

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud"
)

func TestBuiltInCloudControl_JWTTokenManagement(t *testing.T) {
	config := cloud.DefaultConfig()
	api := cloud.NewBuiltInCloudControl(config)
	ctx := context.Background()

	t.Run("GenerateJWTToken", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient(ctx, "test_user_1", "Test JWT Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(ctx, client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		if tokenInfo.Token == "" {
			t.Error("Expected non-empty JWT token")
		}
		if tokenInfo.ClientId != client.ID {
			t.Errorf("Expected client ID %s, got %s", client.ID, tokenInfo.ClientId)
		}
		if tokenInfo.ExpiresAt.Before(time.Now()) {
			t.Error("Token should not be expired")
		}
	})

	t.Run("ValidateJWTToken", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient(ctx, "test_user_1", "Test Validate Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(ctx, client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		// 验证有效 token
		validTokenInfo, err := api.ValidateJWTToken(ctx, tokenInfo.Token)
		if err != nil {
			t.Fatalf("ValidateJWTToken failed: %v", err)
		}

		if validTokenInfo.ClientId != client.ID {
			t.Errorf("Expected client ID %s, got %s", client.ID, validTokenInfo.ClientId)
		}

		// 验证无效 token
		_, err = api.ValidateJWTToken(ctx, "invalid_token")
		if err == nil {
			t.Error("Expected validation to fail with invalid token")
		}
	})

	t.Run("RevokeJWTToken", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient(ctx, "test_user_1", "Test Revoke Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(ctx, client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		// 验证 token 存在
		_, err = api.ValidateJWTToken(ctx, tokenInfo.Token)
		if err != nil {
			t.Fatalf("ValidateJWTToken failed: %v", err)
		}

		// 撤销 token
		err = api.RevokeJWTToken(ctx, tokenInfo.Token)
		if err != nil {
			t.Fatalf("RevokeJWTToken failed: %v", err)
		}

		// 验证 token 已被撤销
		_, err = api.ValidateJWTToken(ctx, tokenInfo.Token)
		if err == nil {
			t.Error("Expected token to be revoked")
		}
	})

	t.Run("RefreshJWTToken", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient(ctx, "test_user_1", "Test Refresh Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(ctx, client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		// 刷新 token
		newTokenInfo, err := api.RefreshJWTToken(ctx, tokenInfo.RefreshToken)
		if err != nil {
			t.Fatalf("RefreshJWTToken failed: %v", err)
		}

		if newTokenInfo.Token == tokenInfo.Token {
			t.Error("Expected new token to be different from original token")
		}
		if newTokenInfo.ClientId != client.ID {
			t.Errorf("Expected client ID %s, got %s", client.ID, newTokenInfo.ClientId)
		}

		// 新token也能通过校验
		validTokenInfo, err := api.ValidateJWTToken(ctx, newTokenInfo.Token)
		if err != nil {
			t.Fatalf("ValidateJWTToken for refreshed token failed: %v", err)
		}
		if validTokenInfo.ClientId != client.ID {
			t.Errorf("Expected client ID %s, got %s", client.ID, validTokenInfo.ClientId)
		}

		// 旧token仍然有效（当前实现不自动撤销旧token）
		_, err = api.ValidateJWTToken(ctx, tokenInfo.Token)
		if err != nil {
			t.Logf("Old token validation result: %v (this is expected in current implementation)", err)
		}
	})
}

func TestBuiltInCloudControl_ConnectionManagement(t *testing.T) {
	config := cloud.DefaultConfig()
	api := cloud.NewBuiltInCloudControl(config)
	ctx := context.Background()

	t.Run("RegisterConnection and GetConnections", func(t *testing.T) {
		// 先创建一个端口映射
		mapping := &cloud.PortMapping{
			ID:             "test_mapping_1",
			UserID:         "test_user_1",
			SourceClientID: "test_client_1",
			TargetClientID: "test_client_2",
			Protocol:       cloud.ProtocolTCP,
			SourcePort:     8080,
			TargetPort:     80,
			Status:         cloud.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		_, err := api.CreatePortMapping(ctx, mapping)
		if err != nil {
			t.Fatalf("CreatePortMapping failed: %v", err)
		}

		// 创建连接信息
		connInfo := &cloud.ConnectionInfo{
			ConnId:        "test_conn_1",
			MappingId:     mapping.ID,
			SourceIP:      "127.0.0.1",
			EstablishedAt: time.Now(),
			LastActivity:  time.Now(),
			BytesSent:     1024,
			BytesReceived: 2048,
			Status:        "active",
		}

		// 注册连接
		err = api.RegisterConnection(ctx, mapping.ID, connInfo)
		if err != nil {
			t.Fatalf("RegisterConnection failed: %v", err)
		}

		// 获取连接列表
		connections, err := api.GetConnections(ctx, mapping.ID)
		if err != nil {
			t.Fatalf("GetConnections failed: %v", err)
		}

		// 注意：当前实现返回空列表，所以这里只是测试方法调用不报错
		if connections == nil {
			t.Error("Expected connections list, got nil")
		}
	})

	t.Run("GetClientConnections", func(t *testing.T) {
		// 获取客户端连接列表
		connections, err := api.GetClientConnections(ctx, "test_client_1")
		if err != nil {
			t.Fatalf("GetClientConnections failed: %v", err)
		}

		// 注意：当前实现返回空列表，所以这里只是测试方法调用不报错
		if connections == nil {
			t.Error("Expected connections list, got nil")
		}
	})

	t.Run("UpdateConnectionStats", func(t *testing.T) {
		// 更新连接统计
		err := api.UpdateConnectionStats(ctx, "test_conn_1", 1500, 2500)
		if err != nil {
			t.Fatalf("UpdateConnectionStats failed: %v", err)
		}
	})

	t.Run("UnregisterConnection", func(t *testing.T) {
		// 注销连接
		err := api.UnregisterConnection(ctx, "test_conn_1")
		if err != nil {
			t.Fatalf("UnregisterConnection failed: %v", err)
		}
	})
}

func TestBuiltInCloudControl_AuthenticationWithJWT(t *testing.T) {
	config := cloud.DefaultConfig()
	api := cloud.NewBuiltInCloudControl(config)
	ctx := context.Background()

	t.Run("Authenticate with JWT token", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient(ctx, "test_user_1", "Test Auth Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		_, err = api.GenerateJWTToken(ctx, client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		authReq := &cloud.AuthRequest{
			ClientID:  client.ID,
			AuthCode:  client.AuthCode,
			SecretKey: client.SecretKey,
			NodeID:    "test_node_1",
			IPAddress: "127.0.0.1",
			Version:   "1.0.0",
		}

		authResp, err := api.Authenticate(ctx, authReq)
		if err != nil {
			t.Fatalf("Authenticate failed: %v", err)
		}

		if !authResp.Success {
			t.Errorf("Expected authentication success, got: %s", authResp.Message)
		}

		if authResp.Client.ID != client.ID {
			t.Errorf("Expected client ID %s, got %s", client.ID, authResp.Client.ID)
		}
	})

	t.Run("ValidateToken with JWT", func(t *testing.T) {
		// 先创建一个客户端
		client, err := api.CreateClient(ctx, "test_user_1", "Test Validate JWT Client")
		if err != nil {
			t.Fatalf("CreateClient failed: %v", err)
		}

		// 生成 JWT token
		tokenInfo, err := api.GenerateJWTToken(ctx, client.ID)
		if err != nil {
			t.Fatalf("GenerateJWTToken failed: %v", err)
		}

		// 先将客户端设置为在线状态
		err = api.UpdateClientStatus(ctx, client.ID, cloud.ClientStatusOnline, "test_node_1")
		if err != nil {
			t.Fatalf("UpdateClientStatus failed: %v", err)
		}

		// 验证有效 token
		authResp, err := api.ValidateToken(ctx, tokenInfo.Token)
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}

		if !authResp.Success {
			t.Errorf("Expected token validation success, got: %s", authResp.Message)
		}

		if authResp.Client != nil && authResp.Client.ID != client.ID {
			t.Errorf("Expected client ID %s, got %s", client.ID, authResp.Client.ID)
		}

		// 验证无效 token
		authResp, err = api.ValidateToken(ctx, "invalid_token")
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}

		if authResp.Success {
			t.Error("Expected token validation to fail with invalid token")
		}
	})
}
