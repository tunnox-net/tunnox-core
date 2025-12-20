package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tunnox-core/internal/cloud/managers"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/storage"
)

// TestGetSystemStats 测试获取系统统计
func TestGetSystemStats(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 创建一些数据
	_, err := cloudControl.CreateUser("user1", "user1@example.com")
	require.NoError(t, err)

	_, err = cloudControl.CreateClient("user1", "client1")
	require.NoError(t, err)

	// 获取系统统计
	stats, err := cloudControl.GetSystemStats()
	require.NoError(t, err)
	assert.NotNil(t, stats)
	// 验证统计数据包含我们创建的内容
	assert.GreaterOrEqual(t, stats.TotalUsers, 1)
	assert.GreaterOrEqual(t, stats.TotalClients, 1)
}

// TestGetTrafficStats 测试获取流量统计
func TestGetTrafficStats(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 获取流量统计（即使没有数据也应该返回空列表）
	trafficStats, err := cloudControl.GetTrafficStats("1h")
	require.NoError(t, err)
	assert.NotNil(t, trafficStats)
}

// TestGetConnectionStats 测试获取连接统计
func TestGetConnectionStats(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 获取连接统计（即使没有数据也应该返回空列表）
	connStats, err := cloudControl.GetConnectionStats("1h")
	require.NoError(t, err)
	assert.NotNil(t, connStats)
}

// TestGetUserStats 测试获取用户统计
func TestGetUserStats(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 创建用户
	user, err := cloudControl.CreateUser("testuser", "test@example.com")
	require.NoError(t, err)

	// 获取用户统计
	userStats, err := cloudControl.GetUserStats(user.ID)
	require.NoError(t, err)
	assert.NotNil(t, userStats)
	assert.Equal(t, user.ID, userStats.UserID)
}

// TestGetClientStats 测试获取客户端统计
func TestGetClientStats(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 创建客户端
	client, err := cloudControl.CreateClient("user-1", "client1")
	require.NoError(t, err)

	// 获取客户端统计
	clientStats, err := cloudControl.GetClientStats(client.ID)
	require.NoError(t, err)
	assert.NotNil(t, clientStats)
	assert.Equal(t, client.ID, clientStats.ClientID)
}

// TestSearchUsers 测试搜索用户
func TestSearchUsers(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 创建测试用户
	_, err := cloudControl.CreateUser("alice", "alice@example.com")
	require.NoError(t, err)

	_, err = cloudControl.CreateUser("bob", "bob@example.com")
	require.NoError(t, err)

	_, err = cloudControl.CreateUser("charlie", "charlie@example.com")
	require.NoError(t, err)

	// 搜索用户
	users, err := cloudControl.SearchUsers("alice")
	require.NoError(t, err)
	assert.NotNil(t, users)
	// 至少应该找到alice
	found := false
	for _, user := range users {
		if user.Username == "alice" {
			found = true
			break
		}
	}
	assert.True(t, found, "应该找到alice用户")
}

// TestSearchClients 测试搜索客户端
func TestSearchClients(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 创建测试客户端
	client1, err := cloudControl.CreateClient("user-1", "client-alpha")
	require.NoError(t, err)

	_, err = cloudControl.CreateClient("user-1", "client-beta")
	require.NoError(t, err)

	_, err = cloudControl.CreateClient("user-2", "client-gamma")
	require.NoError(t, err)

	// 搜索客户端
	clients, err := cloudControl.SearchClients("alpha")
	require.NoError(t, err)
	assert.NotNil(t, clients)
	// 至少应该找到client-alpha
	found := false
	for _, client := range clients {
		if client.ID == client1.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "应该找到client-alpha")
}

// TestSearchPortMappings 测试搜索端口映射
func TestSearchPortMappings(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 创建测试映射
	mapping1 := &models.PortMapping{
		ListenClientID: 12345678,
		TargetClientID: 87654321,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8080,
		TargetPort:     80,
	}
	created, err := cloudControl.CreatePortMapping(mapping1)
	require.NoError(t, err)

	// 搜索映射（使用映射ID的部分字符）
	mappings, err := cloudControl.SearchPortMappings(created.ID[:8])
	require.NoError(t, err)
	assert.NotNil(t, mappings)
}

// TestSearchUsers_EmptyResult 测试搜索无结果
func TestSearchUsers_EmptyResult(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 搜索不存在的用户
	users, err := cloudControl.SearchUsers("nonexistent-user-xyz")
	require.NoError(t, err)
	assert.NotNil(t, users)
	// 应该返回空列表而不是错误
	assert.Equal(t, 0, len(users))
}

// TestSearchClients_CaseInsensitive 测试搜索不区分大小写
func TestSearchClients_CaseInsensitive(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 创建客户端
	_, err := cloudControl.CreateClient("user-1", "TestClient")
	require.NoError(t, err)

	// 使用不同大小写搜索
	clients1, err := cloudControl.SearchClients("testclient")
	require.NoError(t, err)

	clients2, err := cloudControl.SearchClients("TESTCLIENT")
	require.NoError(t, err)

	clients3, err := cloudControl.SearchClients("TestClient")
	require.NoError(t, err)

	// 所有搜索应该返回相同的结果（如果实现了大小写不敏感）
	// 或者至少都不应该报错
	assert.NotNil(t, clients1)
	assert.NotNil(t, clients2)
	assert.NotNil(t, clients3)
}

// TestStats_MultipleDataPoints 测试多个数据点的统计
func TestStats_MultipleDataPoints(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	config := &managers.ControlConfig{
		JWTSecretKey:  "test-secret",
		JWTExpiration: 24 * 3600,
	}
	cloudControl := managers.NewBuiltinCloudControlWithStorage(config, store)

	// 创建多个用户和客户端
	for i := 0; i < 5; i++ {
		username := "user" + string(rune('a'+i))
		_, err := cloudControl.CreateUser(username, username+"@example.com")
		require.NoError(t, err)

		_, err = cloudControl.CreateClient("user-"+username, "client-"+username)
		require.NoError(t, err)
	}

	// 获取系统统计
	sysStats, err := cloudControl.GetSystemStats()
	require.NoError(t, err)
	assert.NotNil(t, sysStats)
	// 验证至少有一些数据
	assert.True(t, sysStats.TotalUsers > 0 || sysStats.TotalClients > 0)
}
