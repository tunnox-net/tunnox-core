package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"tunnox-core/internal/cloud"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTManager_GenerateTokenPair(t *testing.T) {
	config := &cloud.ControlConfig{
		JWTSecretKey:      "test-secret",
		JWTExpiration:     1 * time.Hour,
		RefreshExpiration: 24 * time.Hour,
	}

	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	manager := cloud.NewJWTManager(config, repo)
	require.NotNil(t, manager)

	ctx := context.Background()

	// 创建测试客户端
	client := &cloud.Client{
		ID:        1,
		UserID:    "test-user",
		Type:      cloud.ClientTypeRegistered,
		NodeID:    "test-node",
		Name:      "Test Client",
		Status:    cloud.ClientStatusOnline,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 测试生成令牌对
	tokenInfo, err := manager.GenerateTokenPair(ctx, client)
	require.NoError(t, err)
	require.NotNil(t, tokenInfo)

	assert.NotEmpty(t, tokenInfo.Token)
	assert.NotEmpty(t, tokenInfo.RefreshToken)
	assert.Equal(t, int64(1), tokenInfo.ClientId)
	assert.True(t, tokenInfo.ExpiresAt.After(time.Now()))
	assert.True(t, tokenInfo.ExpiresAt.Before(time.Now().Add(2*time.Hour)))

	// 测试验证访问令牌
	claims, err := manager.ValidateAccessToken(ctx, tokenInfo.Token)
	require.NoError(t, err)
	require.NotNil(t, claims)

	assert.Equal(t, int64(1), claims.ClientID)
	assert.Equal(t, "test-user", claims.UserID)
	assert.Equal(t, string(cloud.ClientTypeRegistered), claims.ClientType)
	assert.Equal(t, "test-node", claims.NodeID)
}

func TestJWTManager_RefreshToken(t *testing.T) {
	config := &cloud.ControlConfig{
		JWTSecretKey:      "test-secret",
		JWTExpiration:     1 * time.Hour,
		RefreshExpiration: 24 * time.Hour,
	}

	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	manager := cloud.NewJWTManager(config, repo)
	require.NotNil(t, manager)

	ctx := context.Background()

	// 创建测试客户端
	client := &cloud.Client{
		ID:        1,
		UserID:    "test-user",
		Type:      cloud.ClientTypeRegistered,
		NodeID:    "test-node",
		Name:      "Test Client",
		Status:    cloud.ClientStatusOnline,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 创建初始令牌对
	tokenInfo, err := manager.GenerateTokenPair(ctx, client)
	require.NoError(t, err)

	// 测试刷新访问令牌
	newTokenInfo, err := manager.RefreshAccessToken(ctx, tokenInfo.RefreshToken, client)
	require.NoError(t, err)
	require.NotNil(t, newTokenInfo)

	assert.NotEmpty(t, newTokenInfo.Token)
	assert.NotEmpty(t, newTokenInfo.RefreshToken)
	assert.Equal(t, int64(1), newTokenInfo.ClientId)
	assert.NotEqual(t, tokenInfo.Token, newTokenInfo.Token)
	assert.NotEqual(t, tokenInfo.RefreshToken, newTokenInfo.RefreshToken)

	// 验证新令牌
	claims, err := manager.ValidateAccessToken(ctx, newTokenInfo.Token)
	require.NoError(t, err)
	assert.Equal(t, int64(1), claims.ClientID)
}

func TestJWTManager_RevokeToken(t *testing.T) {
	config := &cloud.ControlConfig{
		JWTSecretKey:      "test-secret",
		JWTExpiration:     1 * time.Hour,
		RefreshExpiration: 24 * time.Hour,
	}

	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	manager := cloud.NewJWTManager(config, repo)
	require.NotNil(t, manager)

	ctx := context.Background()

	// 创建测试客户端
	client := &cloud.Client{
		ID:        1,
		UserID:    "test-user",
		Type:      cloud.ClientTypeRegistered,
		NodeID:    "test-node",
		Name:      "Test Client",
		Status:    cloud.ClientStatusOnline,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 创建令牌对
	tokenInfo, err := manager.GenerateTokenPair(ctx, client)
	require.NoError(t, err)

	// 测试撤销令牌
	err = manager.RevokeToken(ctx, tokenInfo.TokenID)
	require.NoError(t, err)

	// 验证令牌已被撤销
	_, err = manager.ValidateAccessToken(ctx, tokenInfo.Token)
	assert.Error(t, err)
}

func TestJWTManager_Concurrency(t *testing.T) {
	config := &cloud.ControlConfig{
		JWTSecretKey:      "test-secret",
		JWTExpiration:     1 * time.Hour,
		RefreshExpiration: 24 * time.Hour,
	}

	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	manager := cloud.NewJWTManager(config, repo)
	require.NotNil(t, manager)

	ctx := context.Background()

	// 并发创建令牌
	numGoroutines := 10
	done := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()

			client := &cloud.Client{
				ID:        int64(id),
				UserID:    fmt.Sprintf("user-%d", id),
				Type:      cloud.ClientTypeRegistered,
				NodeID:    fmt.Sprintf("node-%d", id),
				Name:      fmt.Sprintf("Client %d", id),
				Status:    cloud.ClientStatusOnline,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			tokenInfo, err := manager.GenerateTokenPair(ctx, client)
			assert.NoError(t, err)
			assert.NotNil(t, tokenInfo)
			assert.Equal(t, client.ID, tokenInfo.ClientId)
		}(i)
	}

	// 等待所有goroutine完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

func TestJWTManager_Dispose(t *testing.T) {
	config := &cloud.ControlConfig{
		JWTSecretKey:      "test-secret",
		JWTExpiration:     1 * time.Hour,
		RefreshExpiration: 24 * time.Hour,
		UseBuiltIn:        true,
	}
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	manager := cloud.NewJWTManager(config, repo)
	require.NotNil(t, manager)

	// 验证未关闭
	assert.False(t, manager.IsClosed())

	// 关闭管理器
	manager.Close()

	// 验证已关闭
	assert.True(t, manager.IsClosed())
}

func TestJWTManager_Dispose_Concurrent(t *testing.T) {
	config := &cloud.ControlConfig{
		JWTSecretKey:      "test-secret",
		JWTExpiration:     1 * time.Hour,
		RefreshExpiration: 24 * time.Hour,
		UseBuiltIn:        true,
	}
	repo := cloud.NewRepository(cloud.NewMemoryStorage(context.Background()))
	manager := cloud.NewJWTManager(config, repo)
	require.NotNil(t, manager)

	// 并发关闭
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			manager.Close()
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < 10; i++ {
			manager.Close()
		}
		done <- struct{}{}
	}()
	<-done
	<-done

	assert.True(t, manager.IsClosed())
}
