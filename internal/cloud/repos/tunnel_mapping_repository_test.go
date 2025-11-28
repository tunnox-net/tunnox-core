package repos

import (
	"context"
	"testing"
	"time"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/storage"
)

// setupMappingRepoTest 创建测试环境
func setupMappingRepoTest(t *testing.T) (*TunnelMappingRepository, storage.Storage) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	
	repo := NewRepository(memStorage)
	mappingRepo := NewTunnelMappingRepository(repo)
	
	return mappingRepo, memStorage
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 创建和获取
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestTunnelMappingRepository_Create(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	now := time.Now()
	mapping := &models.TunnelMapping{
		ID:               "mapping_test001",
		ConnectionCodeID: "conncode_test001",
		ListenClientID:   11111111,
		TargetClientID:   22222222,
		ListenAddress:    "0.0.0.0:9999",
		TargetAddress:    "192.168.1.100:8080/tcp",
		CreatedAt:        now,
		ExpiresAt:        now.Add(7 * 24 * time.Hour),
		Duration:         7 * 24 * time.Hour,
		CreatedBy:        "client-11111111",
		IsRevoked:        false,
		UsageCount:       0,
		BytesSent:        0,
		BytesReceived:    0,
		Description:      "Test mapping",
	}
	
	err := repo.Create(mapping)
	require.NoError(t, err, "Failed to create mapping")
	
	// 验证可以通过ID获取
	retrieved, err := repo.GetByID("mapping_test001")
	require.NoError(t, err, "Failed to get mapping by ID")
	assert.Equal(t, mapping.ID, retrieved.ID)
	assert.Equal(t, mapping.ListenClientID, retrieved.ListenClientID)
	assert.Equal(t, mapping.TargetClientID, retrieved.TargetClientID)
	assert.Equal(t, mapping.ListenAddress, retrieved.ListenAddress)
	assert.Equal(t, mapping.TargetAddress, retrieved.TargetAddress)
}

func TestTunnelMappingRepository_GetByID_NotFound(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	_, err := repo.GetByID("mapping_nonexistent")
	assert.ErrorIs(t, err, ErrNotFound, "Expected ErrNotFound for nonexistent ID")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 更新
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestTunnelMappingRepository_Update(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	now := time.Now()
	mapping := &models.TunnelMapping{
		ID:               "mapping_update001",
		ConnectionCodeID: "conncode_update001",
		ListenClientID:   33333333,
		TargetClientID:   44444444,
		ListenAddress:    "0.0.0.0:8888",
		TargetAddress:    "192.168.1.200:9090/tcp",
		CreatedAt:        now,
		ExpiresAt:        now.Add(7 * 24 * time.Hour),
		Duration:         7 * 24 * time.Hour,
		CreatedBy:        "client-33333333",
		IsRevoked:        false,
		UsageCount:       0,
		BytesSent:        0,
		BytesReceived:    0,
	}
	
	err := repo.Create(mapping)
	require.NoError(t, err, "Failed to create mapping")
	
	// 更新映射
	mapping.UsageCount = 5
	mapping.BytesSent = 1024
	mapping.BytesReceived = 2048
	
	err = repo.Update(mapping)
	require.NoError(t, err, "Failed to update mapping")
	
	// 验证更新成功
	retrieved, err := repo.GetByID("mapping_update001")
	require.NoError(t, err, "Failed to get updated mapping")
	assert.Equal(t, int64(5), retrieved.UsageCount)
	assert.Equal(t, int64(1024), retrieved.BytesSent)
	assert.Equal(t, int64(2048), retrieved.BytesReceived)
}

func TestTunnelMappingRepository_UpdateUsage(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	now := time.Now()
	mapping := &models.TunnelMapping{
		ID:               "mapping_usage001",
		ConnectionCodeID: "conncode_usage001",
		ListenClientID:   55555555,
		TargetClientID:   66666666,
		ListenAddress:    "0.0.0.0:7777",
		TargetAddress:    "192.168.1.50:3306/tcp",
		CreatedAt:        now,
		ExpiresAt:        now.Add(7 * 24 * time.Hour),
		Duration:         7 * 24 * time.Hour,
		CreatedBy:        "client-55555555",
		IsRevoked:        false,
		UsageCount:       0,
		BytesSent:        0,
		BytesReceived:    0,
	}
	
	err := repo.Create(mapping)
	require.NoError(t, err, "Failed to create mapping")
	
	// 记录使用
	err = repo.UpdateUsage("mapping_usage001")
	require.NoError(t, err, "Failed to update usage")
	
	err = repo.UpdateUsage("mapping_usage001")
	require.NoError(t, err, "Failed to update usage second time")
	
	// 验证使用次数增加
	retrieved, err := repo.GetByID("mapping_usage001")
	require.NoError(t, err, "Failed to get mapping after usage update")
	assert.Equal(t, int64(2), retrieved.UsageCount)
	assert.NotNil(t, retrieved.LastUsedAt)
}

func TestTunnelMappingRepository_UpdateTraffic(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	now := time.Now()
	mapping := &models.TunnelMapping{
		ID:               "mapping_traffic001",
		ConnectionCodeID: "conncode_traffic001",
		ListenClientID:   77777777,
		TargetClientID:   88888888,
		ListenAddress:    "0.0.0.0:6666",
		TargetAddress:    "192.168.1.30:22/tcp",
		CreatedAt:        now,
		ExpiresAt:        now.Add(7 * 24 * time.Hour),
		Duration:         7 * 24 * time.Hour,
		CreatedBy:        "client-77777777",
		IsRevoked:        false,
		UsageCount:       0,
		BytesSent:        0,
		BytesReceived:    0,
	}
	
	err := repo.Create(mapping)
	require.NoError(t, err, "Failed to create mapping")
	
	// 记录流量
	err = repo.UpdateTraffic("mapping_traffic001", 1024, 2048)
	require.NoError(t, err, "Failed to update traffic")
	
	err = repo.UpdateTraffic("mapping_traffic001", 512, 1024)
	require.NoError(t, err, "Failed to update traffic second time")
	
	// 验证流量累加
	retrieved, err := repo.GetByID("mapping_traffic001")
	require.NoError(t, err, "Failed to get mapping after traffic update")
	assert.Equal(t, int64(1536), retrieved.BytesSent)    // 1024 + 512
	assert.Equal(t, int64(3072), retrieved.BytesReceived) // 2048 + 1024
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 列表和统计
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestTunnelMappingRepository_ListByListenClient(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	now := time.Now()
	listenClientID := int64(12345678)
	
	// 创建多个映射
	mappings := []*models.TunnelMapping{
		{
			ID:               "mapping_list001",
			ConnectionCodeID: "conncode_list001",
			ListenClientID:   listenClientID,
			TargetClientID:   22222222,
			ListenAddress:    "0.0.0.0:9001",
			TargetAddress:    "192.168.1.1:8080/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-12345678",
			IsRevoked:        false,
		},
		{
			ID:               "mapping_list002",
			ConnectionCodeID: "conncode_list002",
			ListenClientID:   listenClientID,
			TargetClientID:   33333333,
			ListenAddress:    "0.0.0.0:9002",
			TargetAddress:    "192.168.1.2:9090/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-12345678",
			IsRevoked:        false,
		},
		{
			ID:               "mapping_list003",
			ConnectionCodeID: "conncode_list003",
			ListenClientID:   99999999, // 不同的client
			TargetClientID:   44444444,
			ListenAddress:    "0.0.0.0:9003",
			TargetAddress:    "192.168.1.3:7070/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-99999999",
			IsRevoked:        false,
		},
	}
	
	for _, mapping := range mappings {
		err := repo.Create(mapping)
		require.NoError(t, err, "Failed to create mapping")
	}
	
	// 列出listenClientID的映射
	list, err := repo.ListByListenClient(listenClientID)
	require.NoError(t, err, "Failed to list mappings")
	assert.Len(t, list, 2, "Expected 2 mappings for listen client")
	
	// 验证结果
	ids := make(map[string]bool)
	for _, mapping := range list {
		ids[mapping.ID] = true
		assert.Equal(t, listenClientID, mapping.ListenClientID)
	}
	assert.True(t, ids["mapping_list001"])
	assert.True(t, ids["mapping_list002"])
	assert.False(t, ids["mapping_list003"])
}

func TestTunnelMappingRepository_ListByTargetClient(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	now := time.Now()
	targetClientID := int64(87654321)
	
	// 创建多个映射
	mappings := []*models.TunnelMapping{
		{
			ID:               "mapping_target001",
			ConnectionCodeID: "conncode_target001",
			ListenClientID:   11111111,
			TargetClientID:   targetClientID,
			ListenAddress:    "0.0.0.0:8001",
			TargetAddress:    "192.168.1.1:8080/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-11111111",
			IsRevoked:        false,
		},
		{
			ID:               "mapping_target002",
			ConnectionCodeID: "conncode_target002",
			ListenClientID:   22222222,
			TargetClientID:   targetClientID,
			ListenAddress:    "0.0.0.0:8002",
			TargetAddress:    "192.168.1.2:9090/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-22222222",
			IsRevoked:        false,
		},
		{
			ID:               "mapping_target003",
			ConnectionCodeID: "conncode_target003",
			ListenClientID:   33333333,
			TargetClientID:   99999999, // 不同的target
			ListenAddress:    "0.0.0.0:8003",
			TargetAddress:    "192.168.1.3:7070/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-33333333",
			IsRevoked:        false,
		},
	}
	
	for _, mapping := range mappings {
		err := repo.Create(mapping)
		require.NoError(t, err, "Failed to create mapping")
	}
	
	// 列出targetClientID的映射
	list, err := repo.ListByTargetClient(targetClientID)
	require.NoError(t, err, "Failed to list mappings")
	assert.Len(t, list, 2, "Expected 2 mappings for target client")
	
	// 验证结果
	ids := make(map[string]bool)
	for _, mapping := range list {
		ids[mapping.ID] = true
		assert.Equal(t, targetClientID, mapping.TargetClientID)
	}
	assert.True(t, ids["mapping_target001"])
	assert.True(t, ids["mapping_target002"])
	assert.False(t, ids["mapping_target003"])
}

func TestTunnelMappingRepository_CountActiveByListenClient(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	now := time.Now()
	listenClientID := int64(11223344)
	
	// 创建3个映射：2个活跃，1个已撤销
	mappings := []*models.TunnelMapping{
		{
			ID:               "mapping_count_listen001",
			ConnectionCodeID: "conncode_count001",
			ListenClientID:   listenClientID,
			TargetClientID:   55555555,
			ListenAddress:    "0.0.0.0:7001",
			TargetAddress:    "192.168.1.1:8080/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-11223344",
			IsRevoked:        false, // 活跃
		},
		{
			ID:               "mapping_count_listen002",
			ConnectionCodeID: "conncode_count002",
			ListenClientID:   listenClientID,
			TargetClientID:   66666666,
			ListenAddress:    "0.0.0.0:7002",
			TargetAddress:    "192.168.1.2:9090/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-11223344",
			IsRevoked:        false, // 活跃
		},
		{
			ID:               "mapping_count_listen003",
			ConnectionCodeID: "conncode_count003",
			ListenClientID:   listenClientID,
			TargetClientID:   77777777,
			ListenAddress:    "0.0.0.0:7003",
			TargetAddress:    "192.168.1.3:7070/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour),
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-11223344",
			IsRevoked:        true, // 已撤销
			RevokedAt:        &now,
			RevokedBy:        "admin",
		},
	}
	
	for _, mapping := range mappings {
		err := repo.Create(mapping)
		require.NoError(t, err, "Failed to create mapping")
	}
	
	// 统计活跃映射
	count, err := repo.CountActiveByListenClient(listenClientID)
	require.NoError(t, err, "Failed to count active mappings")
	assert.Equal(t, 2, count, "Expected 2 active mappings")
}

func TestTunnelMappingRepository_CountActiveByTargetClient(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	now := time.Now()
	targetClientID := int64(44332211)
	
	// 创建3个映射：2个活跃，1个已过期
	mappings := []*models.TunnelMapping{
		{
			ID:               "mapping_count_target001",
			ConnectionCodeID: "conncode_count_t001",
			ListenClientID:   11111111,
			TargetClientID:   targetClientID,
			ListenAddress:    "0.0.0.0:6001",
			TargetAddress:    "192.168.1.1:8080/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour), // 未来
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-11111111",
			IsRevoked:        false, // 活跃
		},
		{
			ID:               "mapping_count_target002",
			ConnectionCodeID: "conncode_count_t002",
			ListenClientID:   22222222,
			TargetClientID:   targetClientID,
			ListenAddress:    "0.0.0.0:6002",
			TargetAddress:    "192.168.1.2:9090/tcp",
			CreatedAt:        now.Add(-8 * 24 * time.Hour),
			ExpiresAt:        now.Add(-1 * time.Hour), // 过去（已过期）
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-22222222",
			IsRevoked:        false,
		},
		{
			ID:               "mapping_count_target003",
			ConnectionCodeID: "conncode_count_t003",
			ListenClientID:   33333333,
			TargetClientID:   targetClientID,
			ListenAddress:    "0.0.0.0:6003",
			TargetAddress:    "192.168.1.3:7070/tcp",
			CreatedAt:        now,
			ExpiresAt:        now.Add(7 * 24 * time.Hour), // 未来
			Duration:         7 * 24 * time.Hour,
			CreatedBy:        "client-33333333",
			IsRevoked:        false, // 活跃
		},
	}
	
	for _, mapping := range mappings {
		err := repo.Create(mapping)
		require.NoError(t, err, "Failed to create mapping")
	}
	
	// 统计活跃映射（未撤销且未过期）
	count, err := repo.CountActiveByTargetClient(targetClientID)
	require.NoError(t, err, "Failed to count active mappings")
	assert.Equal(t, 2, count, "Expected 2 active mappings")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 删除
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestTunnelMappingRepository_Delete(t *testing.T) {
	repo, _ := setupMappingRepoTest(t)
	
	now := time.Now()
	mapping := &models.TunnelMapping{
		ID:               "mapping_delete001",
		ConnectionCodeID: "conncode_delete001",
		ListenClientID:   11111111,
		TargetClientID:   22222222,
		ListenAddress:    "0.0.0.0:5555",
		TargetAddress:    "192.168.1.100:8080/tcp",
		CreatedAt:        now,
		ExpiresAt:        now.Add(7 * 24 * time.Hour),
		Duration:         7 * 24 * time.Hour,
		CreatedBy:        "client-11111111",
		IsRevoked:        false,
	}
	
	err := repo.Create(mapping)
	require.NoError(t, err, "Failed to create mapping")
	
	// 删除映射
	err = repo.Delete(mapping.ID)
	require.NoError(t, err, "Failed to delete mapping")
	
	// 验证删除成功
	_, err = repo.GetByID(mapping.ID)
	assert.ErrorIs(t, err, ErrNotFound, "Expected ErrNotFound after deletion")
}

