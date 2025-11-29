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

// setupConnCodeRepoTest 创建测试环境
func setupConnCodeRepoTest(t *testing.T) (*ConnectionCodeRepository, storage.Storage) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)
	
	repo := NewRepository(memStorage)
	connCodeRepo := NewConnectionCodeRepository(repo)
	
	return connCodeRepo, memStorage
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 创建和获取
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeRepository_Create(t *testing.T) {
	repo, _ := setupConnCodeRepoTest(t)
	
	now := time.Now()
	connCode := &models.TunnelConnectionCode{
		ID:                  "conncode_test001",
		Code:                "abc-def-123",
		TargetClientID:      77777777,
		TargetAddress:       "192.168.1.100:8080/tcp",
		ActivationTTL:       10 * time.Minute,
		MappingDuration:     7 * 24 * time.Hour,
		CreatedAt:           now,
		ActivationExpiresAt: now.Add(10 * time.Minute),
		IsActivated:         false,
		CreatedBy:           "test-user",
		IsRevoked:           false,
		Description:         "Test connection code",
	}
	
	err := repo.Create(connCode)
	require.NoError(t, err, "Failed to create connection code")
	
	// 验证可以通过Code获取
	retrieved, err := repo.GetByCode("abc-def-123")
	require.NoError(t, err, "Failed to get connection code by code")
	assert.Equal(t, connCode.ID, retrieved.ID)
	assert.Equal(t, connCode.Code, retrieved.Code)
	assert.Equal(t, connCode.TargetClientID, retrieved.TargetClientID)
	assert.Equal(t, connCode.TargetAddress, retrieved.TargetAddress)
	
	// 验证可以通过ID获取
	retrieved2, err := repo.GetByID("conncode_test001")
	require.NoError(t, err, "Failed to get connection code by ID")
	assert.Equal(t, connCode.Code, retrieved2.Code)
}

func TestConnectionCodeRepository_GetByCode_NotFound(t *testing.T) {
	repo, _ := setupConnCodeRepoTest(t)
	
	_, err := repo.GetByCode("nonexistent-code")
	assert.ErrorIs(t, err, ErrNotFound, "Expected ErrNotFound for nonexistent code")
}

func TestConnectionCodeRepository_GetByID_NotFound(t *testing.T) {
	repo, _ := setupConnCodeRepoTest(t)
	
	_, err := repo.GetByID("conncode_nonexistent")
	assert.ErrorIs(t, err, ErrNotFound, "Expected ErrNotFound for nonexistent ID")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 更新和撤销
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeRepository_Update(t *testing.T) {
	repo, _ := setupConnCodeRepoTest(t)
	
	now := time.Now()
	connCode := &models.TunnelConnectionCode{
		ID:                  "conncode_update001",
		Code:                "upd-ate-001",
		TargetClientID:      88888888,
		TargetAddress:       "192.168.1.200:9090/tcp",
		ActivationTTL:       10 * time.Minute,
		MappingDuration:     7 * 24 * time.Hour,
		CreatedAt:           now,
		ActivationExpiresAt: now.Add(10 * time.Minute),
		IsActivated:         false,
		CreatedBy:           "test-user",
		IsRevoked:           false,
	}
	
	err := repo.Create(connCode)
	require.NoError(t, err, "Failed to create connection code")
	
	// 激活连接码
	mappingID := "mapping_test001"
	activatedBy := int64(99999999)
	connCode.IsActivated = true
	connCode.ActivatedAt = &now
	connCode.ActivatedBy = &activatedBy
	connCode.MappingID = &mappingID
	
	err = repo.Update(connCode)
	require.NoError(t, err, "Failed to update connection code")
	
	// 验证更新成功
	retrieved, err := repo.GetByCode("upd-ate-001")
	require.NoError(t, err, "Failed to get updated connection code")
	assert.True(t, retrieved.IsActivated)
	assert.NotNil(t, retrieved.ActivatedAt)
	assert.NotNil(t, retrieved.ActivatedBy, "ActivatedBy should not be nil")
	assert.Equal(t, int64(99999999), *retrieved.ActivatedBy)
	assert.NotNil(t, retrieved.MappingID)
	assert.Equal(t, "mapping_test001", *retrieved.MappingID)
}

func TestConnectionCodeRepository_UpdateRevoke(t *testing.T) {
	repo, _ := setupConnCodeRepoTest(t)
	
	now := time.Now()
	connCode := &models.TunnelConnectionCode{
		ID:                  "conncode_revoke001",
		Code:                "rev-oke-001",
		TargetClientID:      77777777,
		TargetAddress:       "192.168.1.100:8080/tcp",
		ActivationTTL:       10 * time.Minute,
		MappingDuration:     7 * 24 * time.Hour,
		CreatedAt:           now,
		ActivationExpiresAt: now.Add(10 * time.Minute),
		IsActivated:         false,
		CreatedBy:           "test-user",
		IsRevoked:           false,
	}
	
	err := repo.Create(connCode)
	require.NoError(t, err, "Failed to create connection code")
	
	// 撤销连接码（通过Update）
	connCode.IsRevoked = true
	connCode.RevokedAt = &now
	connCode.RevokedBy = "admin"
	
	err = repo.Update(connCode)
	require.NoError(t, err, "Failed to update connection code for revocation")
	
	// 验证撤销成功
	retrieved, err := repo.GetByID(connCode.ID)
	require.NoError(t, err, "Failed to get revoked connection code")
	assert.True(t, retrieved.IsRevoked)
	assert.NotNil(t, retrieved.RevokedAt)
	assert.Equal(t, "admin", retrieved.RevokedBy)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 列表和索引
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeRepository_ListByTargetClient(t *testing.T) {
	repo, _ := setupConnCodeRepoTest(t)
	
	now := time.Now()
	targetClientID := int64(12345678)
	
	// 创建多个连接码
	codes := []*models.TunnelConnectionCode{
		{
			ID:                  "conncode_list001",
			Code:                "lst-001-aaa",
			TargetClientID:      targetClientID,
			TargetAddress:       "192.168.1.1:8080/tcp",
			ActivationTTL:       10 * time.Minute,
			MappingDuration:     7 * 24 * time.Hour,
			CreatedAt:           now,
			ActivationExpiresAt: now.Add(10 * time.Minute),
			IsActivated:         false,
			CreatedBy:           "test-user",
			IsRevoked:           false,
		},
		{
			ID:                  "conncode_list002",
			Code:                "lst-002-bbb",
			TargetClientID:      targetClientID,
			TargetAddress:       "192.168.1.2:9090/tcp",
			ActivationTTL:       10 * time.Minute,
			MappingDuration:     7 * 24 * time.Hour,
			CreatedAt:           now,
			ActivationExpiresAt: now.Add(10 * time.Minute),
			IsActivated:         false,
			CreatedBy:           "test-user",
			IsRevoked:           false,
		},
		{
			ID:                  "conncode_list003",
			Code:                "lst-003-ccc",
			TargetClientID:      99999999, // 不同的client
			TargetAddress:       "192.168.1.3:7070/tcp",
			ActivationTTL:       10 * time.Minute,
			MappingDuration:     7 * 24 * time.Hour,
			CreatedAt:           now,
			ActivationExpiresAt: now.Add(10 * time.Minute),
			IsActivated:         false,
			CreatedBy:           "test-user",
			IsRevoked:           false,
		},
	}
	
	for _, code := range codes {
		err := repo.Create(code)
		require.NoError(t, err, "Failed to create connection code")
	}
	
	// 列出targetClientID的连接码
	list, err := repo.ListByTargetClient(targetClientID)
	require.NoError(t, err, "Failed to list connection codes")
	assert.Len(t, list, 2, "Expected 2 connection codes for target client")
	
	// 验证结果
	codeSet := make(map[string]bool)
	for _, code := range list {
		codeSet[code.Code] = true
		assert.Equal(t, targetClientID, code.TargetClientID)
	}
	assert.True(t, codeSet["lst-001-aaa"])
	assert.True(t, codeSet["lst-002-bbb"])
	assert.False(t, codeSet["lst-003-ccc"])
}

func TestConnectionCodeRepository_CountActiveByTargetClient(t *testing.T) {
	repo, _ := setupConnCodeRepoTest(t)
	
	now := time.Now()
	targetClientID := int64(11111111)
	
	// 创建3个连接码：2个活跃，1个已撤销
	codes := []*models.TunnelConnectionCode{
		{
			ID:                  "conncode_count001",
			Code:                "cnt-001-aaa",
			TargetClientID:      targetClientID,
			TargetAddress:       "192.168.1.1:8080/tcp",
			ActivationTTL:       10 * time.Minute,
			MappingDuration:     7 * 24 * time.Hour,
			CreatedAt:           now,
			ActivationExpiresAt: now.Add(10 * time.Minute),
			IsActivated:         false,
			CreatedBy:           "test-user",
			IsRevoked:           false, // 活跃
		},
		{
			ID:                  "conncode_count002",
			Code:                "cnt-002-bbb",
			TargetClientID:      targetClientID,
			TargetAddress:       "192.168.1.2:9090/tcp",
			ActivationTTL:       10 * time.Minute,
			MappingDuration:     7 * 24 * time.Hour,
			CreatedAt:           now,
			ActivationExpiresAt: now.Add(10 * time.Minute),
			IsActivated:         false,
			CreatedBy:           "test-user",
			IsRevoked:           false, // 活跃
		},
		{
			ID:                  "conncode_count003",
			Code:                "cnt-003-ccc",
			TargetClientID:      targetClientID,
			TargetAddress:       "192.168.1.3:7070/tcp",
			ActivationTTL:       10 * time.Minute,
			MappingDuration:     7 * 24 * time.Hour,
			CreatedAt:           now,
			ActivationExpiresAt: now.Add(10 * time.Minute),
			IsActivated:         false,
			CreatedBy:           "test-user",
			IsRevoked:           true, // 已撤销
			RevokedAt:           &now,
			RevokedBy:           "admin",
		},
	}
	
	for _, code := range codes {
		err := repo.Create(code)
		require.NoError(t, err, "Failed to create connection code")
	}
	
	// 统计活跃连接码（未撤销且未过期）
	count, err := repo.CountActiveByTargetClient(targetClientID)
	require.NoError(t, err, "Failed to count active connection codes")
	assert.Equal(t, 2, count, "Expected 2 active connection codes")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 删除
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeRepository_Delete(t *testing.T) {
	repo, _ := setupConnCodeRepoTest(t)
	
	now := time.Now()
	connCode := &models.TunnelConnectionCode{
		ID:                  "conncode_delete001",
		Code:                "del-ete-001",
		TargetClientID:      77777777,
		TargetAddress:       "192.168.1.100:8080/tcp",
		ActivationTTL:       10 * time.Minute,
		MappingDuration:     7 * 24 * time.Hour,
		CreatedAt:           now,
		ActivationExpiresAt: now.Add(10 * time.Minute),
		IsActivated:         false,
		CreatedBy:           "test-user",
		IsRevoked:           false,
	}
	
	err := repo.Create(connCode)
	require.NoError(t, err, "Failed to create connection code")
	
	// 删除连接码
	err = repo.Delete(connCode.ID)
	require.NoError(t, err, "Failed to delete connection code")
	
	// 验证删除成功
	_, err = repo.GetByID(connCode.ID)
	assert.ErrorIs(t, err, ErrNotFound, "Expected ErrNotFound after deletion")
	
	_, err = repo.GetByCode(connCode.Code)
	assert.ErrorIs(t, err, ErrNotFound, "Expected ErrNotFound after deletion")
}

