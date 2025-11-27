package api_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/tests/helpers"
)

// TestAuthAPI_Authentication 测试认证流程
func TestAuthAPI_Authentication(t *testing.T) {
	ctx := context.Background()

	// 创建启用认证的测试服务器
	cfg := &helpers.TestAPIServerConfig{
		ListenAddr: helpers.DefaultTestAPIConfig().ListenAddr,
		AuthType:   "api_key",
		APISecret:  "test-api-secret-key",
		EnableCORS: false,
	}
	server, err := helpers.NewTestAPIServer(ctx, cfg)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	t.Run("无认证令牌访问受保护端点", func(t *testing.T) {
		// 不设置认证令牌
		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		// 尝试访问受保护的端点
		_, err := client.ListUsers()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Missing authorization header")
	})

	t.Run("使用错误的API Key", func(t *testing.T) {
		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		// 设置错误的 API Key
		client.SetAuthToken("wrong-api-key")

		// 尝试访问受保护的端点
		_, err := client.ListUsers()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid API key")
	})

	t.Run("使用正确的API Key", func(t *testing.T) {
		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		// 设置正确的 API Key
		client.SetAuthToken("test-api-secret-key")

		// 应该能成功访问
		users, err := client.ListUsers()
		require.NoError(t, err)
		assert.NotNil(t, users)
	})

	t.Run("健康检查端点无需认证", func(t *testing.T) {
		client := helpers.NewAPIClient(ctx, server.GetBaseURL())
		defer client.Close()

		// 不设置认证令牌
		// 健康检查应该可以访问
		ok, err := client.HealthCheck()
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

// TestAuthAPI_NoAuthMode 测试无认证模式
func TestAuthAPI_NoAuthMode(t *testing.T) {
	ctx := context.Background()

	// 创建无认证的测试服务器
	cfg := &helpers.TestAPIServerConfig{
		ListenAddr: helpers.DefaultTestAPIConfig().ListenAddr,
		AuthType:   "none",
		APISecret:  "",
		EnableCORS: false,
	}
	server, err := helpers.NewTestAPIServer(ctx, cfg)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	t.Run("无认证模式下可以直接访问", func(t *testing.T) {
		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		// 不设置认证令牌
		users, err := client.ListUsers()
		require.NoError(t, err)
		assert.NotNil(t, users)
	})

	t.Run("即使设置了令牌也能访问", func(t *testing.T) {
		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		// 设置任意令牌
		client.SetAuthToken("any-token")

		// 仍然可以访问
		users, err := client.ListUsers()
		require.NoError(t, err)
		assert.NotNil(t, users)
	})
}

// TestAuthAPI_AuthorizationHeader 测试授权头格式
func TestAuthAPI_AuthorizationHeader(t *testing.T) {
	ctx := context.Background()

	cfg := &helpers.TestAPIServerConfig{
		ListenAddr: helpers.DefaultTestAPIConfig().ListenAddr,
		AuthType:   "api_key",
		APISecret:  "test-secret",
		EnableCORS: false,
	}
	server, err := helpers.NewTestAPIServer(ctx, cfg)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	t.Run("Bearer令牌格式正确", func(t *testing.T) {
		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		client.SetAuthToken("test-secret")

		// 应该成功
		_, err := client.ListUsers()
		require.NoError(t, err)
	})
}

// TestAuthAPI_CORS 测试 CORS 配置
func TestAuthAPI_CORS(t *testing.T) {
	ctx := context.Background()

	t.Run("启用CORS", func(t *testing.T) {
		cfg := &helpers.TestAPIServerConfig{
			ListenAddr: helpers.DefaultTestAPIConfig().ListenAddr,
			AuthType:   "none",
			APISecret:  "",
			EnableCORS: true,
		}
		server, err := helpers.NewTestAPIServer(ctx, cfg)
		require.NoError(t, err)
		defer server.Stop()
		require.NoError(t, server.Start())

		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		// 应该能正常访问
		_, err = client.ListUsers()
		require.NoError(t, err)
	})

	t.Run("禁用CORS", func(t *testing.T) {
		cfg := &helpers.TestAPIServerConfig{
			ListenAddr: helpers.DefaultTestAPIConfig().ListenAddr,
			AuthType:   "none",
			APISecret:  "",
			EnableCORS: false,
		}
		server, err := helpers.NewTestAPIServer(ctx, cfg)
		require.NoError(t, err)
		defer server.Stop()
		require.NoError(t, server.Start())

		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		// 仍然能访问（CORS主要影响浏览器）
		_, err = client.ListUsers()
		require.NoError(t, err)
	})
}

// TestAuthAPI_TokenValidation 测试令牌验证
func TestAuthAPI_TokenValidation(t *testing.T) {
	ctx := context.Background()

	cfg := &helpers.TestAPIServerConfig{
		ListenAddr: helpers.DefaultTestAPIConfig().ListenAddr,
		AuthType:   "api_key",
		APISecret:  "valid-secret",
		EnableCORS: false,
	}
	server, err := helpers.NewTestAPIServer(ctx, cfg)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	t.Run("空令牌", func(t *testing.T) {
		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		client.SetAuthToken("")

		_, err := client.ListUsers()
		assert.Error(t, err)
	})

	t.Run("特殊字符令牌", func(t *testing.T) {
		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		specialTokens := []string{
			"token<>with<>brackets",
			"token with spaces",
			"token\nwith\nnewlines",
			"token\"with\"quotes",
		}

		for _, token := range specialTokens {
			client.SetAuthToken(token)
			_, err := client.ListUsers()
			// 应该被拒绝（除非恰好匹配）
			assert.Error(t, err)
		}
	})

	t.Run("超长令牌", func(t *testing.T) {
		client := helpers.NewAPIClient(ctx, server.GetAPIURL())
		defer client.Close()

		longToken := string(make([]byte, 10000))
		for i := range longToken {
			longToken = string(append([]byte(longToken[:i]), 'A'))
		}

		client.SetAuthToken(longToken[:1000])

		_, err := client.ListUsers()
		assert.Error(t, err)
	})
}

// TestAuthAPI_ConcurrentAuth 测试并发认证
func TestAuthAPI_ConcurrentAuth(t *testing.T) {
	ctx := context.Background()

	cfg := &helpers.TestAPIServerConfig{
		ListenAddr: helpers.DefaultTestAPIConfig().ListenAddr,
		AuthType:   "api_key",
		APISecret:  "concurrent-secret",
		EnableCORS: false,
	}
	server, err := helpers.NewTestAPIServer(ctx, cfg)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	t.Run("并发认证请求", func(t *testing.T) {
		const concurrency = 20
		done := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			go func() {
				client := helpers.NewAPIClient(ctx, server.GetAPIURL())
				defer client.Close()

				client.SetAuthToken("concurrent-secret")

				_, err := client.ListUsers()
				done <- err
			}()
		}

		// 验证所有请求都成功
		for i := 0; i < concurrency; i++ {
			err := <-done
			assert.NoError(t, err)
		}
	})

	t.Run("并发无效认证请求", func(t *testing.T) {
		const concurrency = 10
		done := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			go func() {
				client := helpers.NewAPIClient(ctx, server.GetAPIURL())
				defer client.Close()

				client.SetAuthToken("wrong-secret")

				_, err := client.ListUsers()
				done <- err
			}()
		}

		// 验证所有请求都失败
		for i := 0; i < concurrency; i++ {
			err := <-done
			assert.Error(t, err)
		}
	})
}

// TestAuthAPI_DifferentEndpoints 测试不同端点的认证
func TestAuthAPI_DifferentEndpoints(t *testing.T) {
	ctx := context.Background()

	cfg := &helpers.TestAPIServerConfig{
		ListenAddr: helpers.DefaultTestAPIConfig().ListenAddr,
		AuthType:   "api_key",
		APISecret:  "endpoint-secret",
		EnableCORS: false,
	}
	server, err := helpers.NewTestAPIServer(ctx, cfg)
	require.NoError(t, err)
	defer server.Stop()
	require.NoError(t, server.Start())

	client := helpers.NewAPIClient(ctx, server.GetAPIURL())
	defer client.Close()
	client.SetAuthToken("endpoint-secret")

	t.Run("所有端点都需要认证", func(t *testing.T) {
		// 创建用户
		user, err := client.CreateUser("authtest", "auth@example.com")
		require.NoError(t, err)

		// 获取用户
		_, err = client.GetUser(user.ID)
		require.NoError(t, err)

		// 列出用户
		_, err = client.ListUsers()
		require.NoError(t, err)

		// 搜索用户
		_, err = client.SearchUsers("auth")
		require.NoError(t, err)
	})
}
