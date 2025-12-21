package services

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tunnox-core/internal/cloud/repos"
	"tunnox-core/internal/core/idgen"
	"tunnox-core/internal/core/storage"
)

// setupConnCodeServiceTest 创建测试环境
func setupConnCodeServiceTest(t *testing.T) (*ConnectionCodeService, *repos.ConnectionCodeRepository, *repos.PortMappingRepo, storage.Storage) {
	ctx := context.Background()
	memStorage := storage.NewMemoryStorage(ctx)

	repo := repos.NewRepository(memStorage)
	connCodeRepo := repos.NewConnectionCodeRepository(repo)

	// ✅ 创建 PortMappingService
	portMappingRepo := repos.NewPortMappingRepo(repo)
	idManager := idgen.NewIDManager(memStorage, ctx)
	portMappingService := NewPortMappingService(portMappingRepo, idManager, nil, ctx)

	service := NewConnectionCodeService(connCodeRepo, portMappingService, portMappingRepo, nil, ctx)

	return service, connCodeRepo, portMappingRepo, memStorage
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 创建连接码
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeService_CreateConnectionCode(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	req := &CreateConnectionCodeRequest{
		TargetClientID:  77777777,
		TargetAddress:   "tcp://192.168.1.100:8080",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
		Description:     "Test connection code",
		CreatedBy:       "test-user",
	}

	connCode, err := service.CreateConnectionCode(req)
	require.NoError(t, err, "Failed to create connection code")
	assert.NotEmpty(t, connCode.ID)
	assert.NotEmpty(t, connCode.Code)
	assert.Equal(t, req.TargetClientID, connCode.TargetClientID)
	assert.Equal(t, req.TargetAddress, connCode.TargetAddress)
	assert.False(t, connCode.IsActivated)
	assert.False(t, connCode.IsRevoked)
}

func TestConnectionCodeService_CreateConnectionCode_MissingTargetClientID(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	req := &CreateConnectionCodeRequest{
		TargetClientID:  0, // 缺失
		TargetAddress:   "tcp://192.168.1.100:8080",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	_, err := service.CreateConnectionCode(req)
	assert.Error(t, err, "Expected error for missing target client ID")
	assert.Contains(t, err.Error(), "target client ID is required")
}

func TestConnectionCodeService_CreateConnectionCode_MissingTargetAddress(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	req := &CreateConnectionCodeRequest{
		TargetClientID:  77777777,
		TargetAddress:   "", // 缺失
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	_, err := service.CreateConnectionCode(req)
	assert.Error(t, err, "Expected error for missing target address")
	assert.Contains(t, err.Error(), "target address is required")
}

func TestConnectionCodeService_CreateConnectionCode_QuotaExceeded(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	// 创建连接码直到达到配额限制（默认10个）
	for i := 0; i < 10; i++ {
		req := &CreateConnectionCodeRequest{
			TargetClientID:  77777777,
			TargetAddress:   "tcp://192.168.1.100:8080",
			ActivationTTL:   10 * time.Minute,
			MappingDuration: 7 * 24 * time.Hour,
		}

		_, err := service.CreateConnectionCode(req)
		require.NoError(t, err, "Failed to create connection code %d", i)
	}

	// 尝试创建第11个，应该失败
	req := &CreateConnectionCodeRequest{
		TargetClientID:  77777777,
		TargetAddress:   "tcp://192.168.1.100:8080",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	_, err := service.CreateConnectionCode(req)
	assert.Error(t, err, "Expected error for quota exceeded")
	assert.Contains(t, err.Error(), "quota exceeded")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 激活连接码
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeService_ActivateConnectionCode(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	// 1. 创建连接码
	createReq := &CreateConnectionCodeRequest{
		TargetClientID:  88888888,
		TargetAddress:   "tcp://192.168.1.200:9090",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
		CreatedBy:       "client-88888888",
	}

	connCode, err := service.CreateConnectionCode(createReq)
	require.NoError(t, err, "Failed to create connection code")

	// 2. 激活连接码
	activateReq := &ActivateConnectionCodeRequest{
		Code:           connCode.Code,
		ListenClientID: 99999999,
		ListenAddress:  "0.0.0.0:7777",
	}

	mapping, err := service.ActivateConnectionCode(activateReq)
	require.NoError(t, err, "Failed to activate connection code")
	assert.NotEmpty(t, mapping.ID)
	assert.Equal(t, int64(99999999), mapping.ListenClientID)
	assert.Equal(t, int64(88888888), mapping.TargetClientID)
	assert.Equal(t, "0.0.0.0:7777", mapping.ListenAddress)
	assert.Equal(t, "tcp://192.168.1.200:9090", mapping.TargetAddress)

	// 3. 验证连接码已激活
	retrieved, err := service.GetConnectionCode(connCode.Code)
	require.NoError(t, err, "Failed to get connection code")
	assert.True(t, retrieved.IsActivated)
	assert.NotNil(t, retrieved.MappingID)
	assert.Equal(t, mapping.ID, *retrieved.MappingID)
}

func TestConnectionCodeService_ActivateConnectionCode_NotFound(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	activateReq := &ActivateConnectionCodeRequest{
		Code:           "nonexistent-code",
		ListenClientID: 99999999,
		ListenAddress:  "0.0.0.0:7777",
	}

	_, err := service.ActivateConnectionCode(activateReq)
	assert.Error(t, err, "Expected error for nonexistent code")
	assert.Contains(t, err.Error(), "not found")
}

func TestConnectionCodeService_ActivateConnectionCode_AlreadyUsed(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	// 1. 创建连接码
	createReq := &CreateConnectionCodeRequest{
		TargetClientID:  88888888,
		TargetAddress:   "tcp://192.168.1.200:9090",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
		CreatedBy:       "client-88888888",
	}

	connCode, err := service.CreateConnectionCode(createReq)
	require.NoError(t, err, "Failed to create connection code")

	// 2. 第一次激活
	activateReq := &ActivateConnectionCodeRequest{
		Code:           connCode.Code,
		ListenClientID: 99999999,
		ListenAddress:  "0.0.0.0:7777",
	}

	_, err = service.ActivateConnectionCode(activateReq)
	require.NoError(t, err, "Failed to activate connection code")

	// 3. 第二次激活，应该失败
	activateReq2 := &ActivateConnectionCodeRequest{
		Code:           connCode.Code,
		ListenClientID: 11111111,
		ListenAddress:  "0.0.0.0:8888",
	}

	_, err = service.ActivateConnectionCode(activateReq2)
	assert.Error(t, err, "Expected error for already used code")
	assert.Contains(t, err.Error(), "already been used")
}

// TestConnectionCodeService_ActivateConnectionCode_MappingQuotaExceeded 测试映射配额超限
func TestConnectionCodeService_ActivateConnectionCode_MappingQuotaExceeded(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	listenClientID := int64(99999999)

	// 创建并激活映射直到达到配额限制（默认50个）
	for i := 0; i < 50; i++ {
		createReq := &CreateConnectionCodeRequest{
			TargetClientID:  int64(10000000 + i),
			TargetAddress:   "tcp://192.168.1.1:8080",
			ActivationTTL:   10 * time.Minute,
			MappingDuration: 7 * 24 * time.Hour,
		}

		connCode, err := service.CreateConnectionCode(createReq)
		require.NoError(t, err, "Failed to create connection code %d", i)

		activateReq := &ActivateConnectionCodeRequest{
			Code:           connCode.Code,
			ListenClientID: listenClientID,
			ListenAddress:  "0.0.0.0:9000",
		}

		_, err = service.ActivateConnectionCode(activateReq)
		require.NoError(t, err, "Failed to activate connection code %d", i)
	}

	// 尝试激活第51个，应该失败
	createReq := &CreateConnectionCodeRequest{
		TargetClientID:  88888888,
		TargetAddress:   "tcp://192.168.1.200:9090",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	connCode, err := service.CreateConnectionCode(createReq)
	require.NoError(t, err, "Failed to create connection code")

	activateReq := &ActivateConnectionCodeRequest{
		Code:           connCode.Code,
		ListenClientID: listenClientID,
		ListenAddress:  "0.0.0.0:8888",
	}

	_, err = service.ActivateConnectionCode(activateReq)
	assert.Error(t, err, "Expected error for quota exceeded")
	assert.Contains(t, err.Error(), "quota exceeded")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 撤销
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeService_RevokeConnectionCode(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	// 1. 创建连接码
	createReq := &CreateConnectionCodeRequest{
		TargetClientID:  77777777,
		TargetAddress:   "tcp://192.168.1.100:8080",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	connCode, err := service.CreateConnectionCode(createReq)
	require.NoError(t, err, "Failed to create connection code")

	// 2. 撤销连接码
	err = service.RevokeConnectionCode(connCode.Code, "admin")
	require.NoError(t, err, "Failed to revoke connection code")

	// 3. 验证撤销成功
	retrieved, err := service.GetConnectionCode(connCode.Code)
	require.NoError(t, err, "Failed to get connection code")
	assert.True(t, retrieved.IsRevoked)
	assert.NotNil(t, retrieved.RevokedAt)
	assert.Equal(t, "admin", retrieved.RevokedBy)
}

func TestConnectionCodeService_RevokeMapping(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	// 1. 创建并激活连接码
	createReq := &CreateConnectionCodeRequest{
		TargetClientID:  88888888,
		TargetAddress:   "tcp://192.168.1.200:9090",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	connCode, err := service.CreateConnectionCode(createReq)
	require.NoError(t, err, "Failed to create connection code")

	activateReq := &ActivateConnectionCodeRequest{
		Code:           connCode.Code,
		ListenClientID: 99999999,
		ListenAddress:  "0.0.0.0:7777",
	}

	mapping, err := service.ActivateConnectionCode(activateReq)
	require.NoError(t, err, "Failed to activate connection code")

	// 2. 撤销映射
	err = service.RevokeMapping(mapping.ID, 99999999, "client-99999999")
	require.NoError(t, err, "Failed to revoke mapping")

	// 3. 验证撤销成功
	retrieved, err := service.GetMapping(mapping.ID)
	require.NoError(t, err, "Failed to get mapping")
	assert.True(t, retrieved.IsRevoked)
	assert.NotNil(t, retrieved.RevokedAt)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 验证映射
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeService_ValidateMapping(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	// 1. 创建并激活连接码
	createReq := &CreateConnectionCodeRequest{
		TargetClientID:  88888888,
		TargetAddress:   "tcp://192.168.1.200:9090",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	connCode, err := service.CreateConnectionCode(createReq)
	require.NoError(t, err, "Failed to create connection code")

	activateReq := &ActivateConnectionCodeRequest{
		Code:           connCode.Code,
		ListenClientID: 99999999,
		ListenAddress:  "0.0.0.0:7777",
	}

	mapping, err := service.ActivateConnectionCode(activateReq)
	require.NoError(t, err, "Failed to activate connection code")

	// 2. 验证映射（正确的客户端）
	validated, err := service.ValidateMapping(mapping.ID, 99999999)
	require.NoError(t, err, "Failed to validate mapping")
	assert.Equal(t, mapping.ID, validated.ID)

	// 3. 验证映射（错误的客户端）
	_, err = service.ValidateMapping(mapping.ID, 11111111)
	assert.Error(t, err, "Expected error for wrong client")
	assert.Contains(t, err.Error(), "not authorized")
}

func TestConnectionCodeService_ValidateMapping_Revoked(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	// 1. 创建并激活连接码
	createReq := &CreateConnectionCodeRequest{
		TargetClientID:  88888888,
		TargetAddress:   "tcp://192.168.1.200:9090",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	connCode, err := service.CreateConnectionCode(createReq)
	require.NoError(t, err, "Failed to create connection code")

	activateReq := &ActivateConnectionCodeRequest{
		Code:           connCode.Code,
		ListenClientID: 99999999,
		ListenAddress:  "0.0.0.0:7777",
	}

	mapping, err := service.ActivateConnectionCode(activateReq)
	require.NoError(t, err, "Failed to activate connection code")

	// 2. 撤销映射
	err = service.RevokeMapping(mapping.ID, 99999999, "admin")
	require.NoError(t, err, "Failed to revoke mapping")

	// 3. 验证映射，应该失败
	_, err = service.ValidateMapping(mapping.ID, 99999999)
	assert.Error(t, err, "Expected error for revoked mapping")
	assert.Contains(t, err.Error(), "revoked")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 查询
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeService_ListConnectionCodesByTargetClient(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	targetClientID := int64(12345678)

	// 创建多个连接码
	for i := 0; i < 3; i++ {
		createReq := &CreateConnectionCodeRequest{
			TargetClientID:  targetClientID,
			TargetAddress:   "tcp://192.168.1.1:8080",
			ActivationTTL:   10 * time.Minute,
			MappingDuration: 7 * 24 * time.Hour,
		}

		_, err := service.CreateConnectionCode(createReq)
		require.NoError(t, err, "Failed to create connection code %d", i)
	}

	// 列出连接码
	list, err := service.ListConnectionCodesByTargetClient(targetClientID)
	require.NoError(t, err, "Failed to list connection codes")
	assert.Len(t, list, 3, "Expected 3 connection codes")

	for _, code := range list {
		assert.Equal(t, targetClientID, code.TargetClientID)
	}
}

func TestConnectionCodeService_ListOutboundMappings(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	listenClientID := int64(99999999)

	// 创建并激活多个连接码
	for i := 0; i < 3; i++ {
		createReq := &CreateConnectionCodeRequest{
			TargetClientID:  int64(10000000 + i),
			TargetAddress:   "tcp://192.168.1.1:8080",
			ActivationTTL:   10 * time.Minute,
			MappingDuration: 7 * 24 * time.Hour,
		}

		connCode, err := service.CreateConnectionCode(createReq)
		require.NoError(t, err, "Failed to create connection code %d", i)

		activateReq := &ActivateConnectionCodeRequest{
			Code:           connCode.Code,
			ListenClientID: listenClientID,
			ListenAddress:  "0.0.0.0:9000",
		}

		_, err = service.ActivateConnectionCode(activateReq)
		require.NoError(t, err, "Failed to activate connection code %d", i)
	}

	// 列出出站映射
	list, err := service.ListOutboundMappings(listenClientID)
	require.NoError(t, err, "Failed to list outbound mappings")
	assert.Len(t, list, 3, "Expected 3 outbound mappings")

	for _, mapping := range list {
		assert.Equal(t, listenClientID, mapping.ListenClientID)
	}
}

func TestConnectionCodeService_ListInboundMappings(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	targetClientID := int64(88888888)

	// 创建并激活多个连接码
	for i := 0; i < 3; i++ {
		createReq := &CreateConnectionCodeRequest{
			TargetClientID:  targetClientID,
			TargetAddress:   "tcp://192.168.1.1:8080",
			ActivationTTL:   10 * time.Minute,
			MappingDuration: 7 * 24 * time.Hour,
		}

		connCode, err := service.CreateConnectionCode(createReq)
		require.NoError(t, err, "Failed to create connection code %d", i)

		activateReq := &ActivateConnectionCodeRequest{
			Code:           connCode.Code,
			ListenClientID: int64(10000000 + i),
			ListenAddress:  "0.0.0.0:9000",
		}

		_, err = service.ActivateConnectionCode(activateReq)
		require.NoError(t, err, "Failed to activate connection code %d", i)
	}

	// 列出入站映射
	list, err := service.ListInboundMappings(targetClientID)
	require.NoError(t, err, "Failed to list inbound mappings")
	assert.Len(t, list, 3, "Expected 3 inbound mappings")

	for _, mapping := range list {
		assert.Equal(t, targetClientID, mapping.TargetClientID)
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 使用统计
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConnectionCodeService_RecordMappingUsage(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	// 1. 创建并激活连接码
	createReq := &CreateConnectionCodeRequest{
		TargetClientID:  88888888,
		TargetAddress:   "tcp://192.168.1.200:9090",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	connCode, err := service.CreateConnectionCode(createReq)
	require.NoError(t, err, "Failed to create connection code")

	activateReq := &ActivateConnectionCodeRequest{
		Code:           connCode.Code,
		ListenClientID: 99999999,
		ListenAddress:  "0.0.0.0:7777",
	}

	mapping, err := service.ActivateConnectionCode(activateReq)
	require.NoError(t, err, "Failed to activate connection code")

	// 2. 记录使用
	err = service.RecordMappingUsage(mapping.ID)
	require.NoError(t, err, "Failed to record mapping usage")

	// 3. 验证使用次数（PortMapping 使用 LastActive 来记录使用）
	retrieved, err := service.GetMapping(mapping.ID)
	require.NoError(t, err, "Failed to get mapping")
	// ✅ PortMapping 使用 LastActive 来记录最后使用时间，不再使用 UsageCount
	assert.NotNil(t, retrieved.LastActive)
}

func TestConnectionCodeService_RecordMappingTraffic(t *testing.T) {
	service, _, _, _ := setupConnCodeServiceTest(t)

	// 1. 创建并激活连接码
	createReq := &CreateConnectionCodeRequest{
		TargetClientID:  88888888,
		TargetAddress:   "tcp://192.168.1.200:9090",
		ActivationTTL:   10 * time.Minute,
		MappingDuration: 7 * 24 * time.Hour,
	}

	connCode, err := service.CreateConnectionCode(createReq)
	require.NoError(t, err, "Failed to create connection code")

	activateReq := &ActivateConnectionCodeRequest{
		Code:           connCode.Code,
		ListenClientID: 99999999,
		ListenAddress:  "0.0.0.0:7777",
	}

	mapping, err := service.ActivateConnectionCode(activateReq)
	require.NoError(t, err, "Failed to activate connection code")

	// 2. 记录流量
	err = service.RecordMappingTraffic(mapping.ID, 1024, 2048)
	require.NoError(t, err, "Failed to record mapping traffic")

	// 3. 验证流量（PortMapping 使用 TrafficStats）
	retrieved, err := service.GetMapping(mapping.ID)
	require.NoError(t, err, "Failed to get mapping")
	// ✅ PortMapping 使用 TrafficStats 来存储流量统计
	assert.Equal(t, int64(1024), retrieved.TrafficStats.BytesSent)
	assert.Equal(t, int64(2048), retrieved.TrafficStats.BytesReceived)
}
