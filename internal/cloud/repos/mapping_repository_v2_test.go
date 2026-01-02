package repos

import (
	"context"
	"testing"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/repository/index"
	"tunnox-core/internal/core/repository/indexed"
	"tunnox-core/internal/core/store/memory"
)

// =============================================================================
// 测试辅助：创建带内存存储的 PortMappingRepositoryV2
// =============================================================================

func newTestPortMappingRepoV2(ctx context.Context) *PortMappingRepositoryV2 {
	// 使用内存存储
	memStore := memory.NewMemoryStore[string, *models.PortMapping]()
	cachedStore := newMockCachedStore[string, *models.PortMapping](memStore)
	userIndexStore := memory.NewMemorySetStore[string, string]()
	clientIndexStore := memory.NewMemorySetStore[string, string]()
	domainIndexStore := memory.NewMemoryStore[string, string]()
	globalListStore := memory.NewMemorySetStore[string, string]()

	// 创建用户索引管理器
	userIndexManager := index.NewUserEntityIndexManager[*models.PortMapping](
		userIndexStore,
		constants.KeyPrefixIndexUserClients,
		func(mapping *models.PortMapping) string {
			return mapping.GetUserID()
		},
	)

	// 创建带索引的 Repository
	baseRepo := indexed.NewUserIndexedRepository[*models.PortMapping](
		cachedStore,
		userIndexManager,
		constants.KeyPrefixPortMapping,
		"PortMapping",
	)

	return &PortMappingRepositoryV2{
		baseRepo:         baseRepo,
		clientIndexStore: clientIndexStore,
		domainIndexStore: domainIndexStore,
		globalListStore:  globalListStore,
		ctx:              ctx,
	}
}

// =============================================================================
// PortMappingRepositoryV2 测试
// =============================================================================

func TestPortMappingRepositoryV2_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建映射
	mapping := &models.PortMapping{
		ID:             "mapping-1",
		UserID:         "user-1",
		ListenClientID: 1001,
		TargetClientID: 1002,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8080,
		TargetAddress:  "127.0.0.1:80",
		Status:         models.MappingStatusActive,
	}

	err := repo.CreatePortMapping(mapping)
	if err != nil {
		t.Fatalf("CreatePortMapping failed: %v", err)
	}

	// 获取映射
	got, err := repo.GetPortMapping("mapping-1")
	if err != nil {
		t.Fatalf("GetPortMapping failed: %v", err)
	}

	if got.ID != mapping.ID {
		t.Errorf("expected ID %s, got %s", mapping.ID, got.ID)
	}
	if got.UserID != mapping.UserID {
		t.Errorf("expected UserID %s, got %s", mapping.UserID, got.UserID)
	}
	if got.ListenClientID != mapping.ListenClientID {
		t.Errorf("expected ListenClientID %d, got %d", mapping.ListenClientID, got.ListenClientID)
	}
}

func TestPortMappingRepositoryV2_GetNotFound(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	_, err := repo.GetPortMapping("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent mapping")
	}
}

func TestPortMappingRepositoryV2_UpdatePortMapping(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建映射
	mapping := &models.PortMapping{
		ID:             "mapping-1",
		UserID:         "user-1",
		ListenClientID: 1001,
		TargetClientID: 1002,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8080,
		TargetAddress:  "127.0.0.1:80",
		Status:         models.MappingStatusActive,
	}
	_ = repo.CreatePortMapping(mapping)

	// 更新
	mapping.SourcePort = 9090
	err := repo.UpdatePortMapping(mapping)
	if err != nil {
		t.Fatalf("UpdatePortMapping failed: %v", err)
	}

	// 验证
	got, _ := repo.GetPortMapping("mapping-1")
	if got.SourcePort != 9090 {
		t.Errorf("expected SourcePort 9090, got %d", got.SourcePort)
	}
}

func TestPortMappingRepositoryV2_DeletePortMapping(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建并删除
	mapping := &models.PortMapping{
		ID:             "mapping-1",
		UserID:         "user-1",
		ListenClientID: 1001,
		TargetClientID: 1002,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8080,
		TargetAddress:  "127.0.0.1:80",
		Status:         models.MappingStatusActive,
	}
	_ = repo.CreatePortMapping(mapping)

	err := repo.DeletePortMapping("mapping-1")
	if err != nil {
		t.Fatalf("DeletePortMapping failed: %v", err)
	}

	// 验证删除
	_, err = repo.GetPortMapping("mapping-1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestPortMappingRepositoryV2_GetUserPortMappings(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建多个映射
	for i := 1; i <= 5; i++ {
		mapping := &models.PortMapping{
			ID:             "mapping-" + string(rune('0'+i)),
			UserID:         "user-1",
			ListenClientID: int64(1000 + i),
			TargetClientID: int64(2000 + i),
			Protocol:       models.ProtocolTCP,
			SourcePort:     8080 + i,
			TargetAddress:  "127.0.0.1:80",
			Status:         models.MappingStatusActive,
		}
		_ = repo.CreatePortMapping(mapping)
	}

	// 另一个用户的映射
	mapping := &models.PortMapping{
		ID:             "mapping-other",
		UserID:         "user-2",
		ListenClientID: 3001,
		TargetClientID: 3002,
		Protocol:       models.ProtocolTCP,
		SourcePort:     9090,
		TargetAddress:  "127.0.0.1:80",
		Status:         models.MappingStatusActive,
	}
	_ = repo.CreatePortMapping(mapping)

	// 列出 user-1 的映射
	mappings, err := repo.GetUserPortMappings("user-1")
	if err != nil {
		t.Fatalf("GetUserPortMappings failed: %v", err)
	}

	if len(mappings) != 5 {
		t.Errorf("expected 5 mappings for user-1, got %d", len(mappings))
	}
}

func TestPortMappingRepositoryV2_GetClientPortMappings(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建映射（1001 作为 ListenClient）
	mapping1 := &models.PortMapping{
		ID:             "mapping-1",
		UserID:         "user-1",
		ListenClientID: 1001,
		TargetClientID: 2001,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8080,
		TargetAddress:  "127.0.0.1:80",
		Status:         models.MappingStatusActive,
	}
	_ = repo.CreatePortMapping(mapping1)

	// 创建映射（1001 作为 TargetClient）
	mapping2 := &models.PortMapping{
		ID:             "mapping-2",
		UserID:         "user-1",
		ListenClientID: 3001,
		TargetClientID: 1001,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8081,
		TargetAddress:  "127.0.0.1:81",
		Status:         models.MappingStatusActive,
	}
	_ = repo.CreatePortMapping(mapping2)

	// 列出 1001 的映射（应该有 2 个）
	mappings, err := repo.GetClientPortMappings("1001")
	if err != nil {
		t.Fatalf("GetClientPortMappings failed: %v", err)
	}

	if len(mappings) != 2 {
		t.Errorf("expected 2 mappings for client 1001, got %d", len(mappings))
	}
}

func TestPortMappingRepositoryV2_GetPortMappingByDomain(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建 HTTP 映射
	mapping := &models.PortMapping{
		ID:             "mapping-http",
		UserID:         "user-1",
		ListenClientID: 1001,
		TargetClientID: 1002,
		Protocol:       models.ProtocolHTTP,
		SourcePort:     80,
		TargetAddress:  "127.0.0.1:8080",
		HTTPSubdomain:  "test",
		HTTPBaseDomain: "example.com",
		Status:         models.MappingStatusActive,
	}
	_ = repo.CreatePortMapping(mapping)

	// 通过域名查找
	got, err := repo.GetPortMappingByDomain("test.example.com")
	if err != nil {
		t.Fatalf("GetPortMappingByDomain failed: %v", err)
	}

	if got.ID != "mapping-http" {
		t.Errorf("expected ID 'mapping-http', got '%s'", got.ID)
	}
}

func TestPortMappingRepositoryV2_ListAllMappings(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建多个映射
	for i := 1; i <= 10; i++ {
		mapping := &models.PortMapping{
			ID:             "mapping-" + string(rune('0'+i)),
			UserID:         "user-1",
			ListenClientID: int64(1000 + i),
			TargetClientID: int64(2000 + i),
			Protocol:       models.ProtocolTCP,
			SourcePort:     8080 + i,
			TargetAddress:  "127.0.0.1:80",
			Status:         models.MappingStatusActive,
		}
		_ = repo.CreatePortMapping(mapping)
	}

	// 列出所有
	mappings, err := repo.ListAllMappings()
	if err != nil {
		t.Fatalf("ListAllMappings failed: %v", err)
	}

	if len(mappings) != 10 {
		t.Errorf("expected 10 mappings, got %d", len(mappings))
	}
}

func TestPortMappingRepositoryV2_SavePortMapping(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// SavePortMapping 应该自动处理创建和更新
	mapping := &models.PortMapping{
		ID:             "mapping-1",
		UserID:         "user-1",
		ListenClientID: 1001,
		TargetClientID: 1002,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8080,
		TargetAddress:  "127.0.0.1:80",
		Status:         models.MappingStatusActive,
	}

	// 第一次保存（创建）
	err := repo.SavePortMapping(mapping)
	if err != nil {
		t.Fatalf("SavePortMapping (create) failed: %v", err)
	}

	// 第二次保存（更新）
	mapping.SourcePort = 9090
	err = repo.SavePortMapping(mapping)
	if err != nil {
		t.Fatalf("SavePortMapping (update) failed: %v", err)
	}

	got, _ := repo.GetPortMapping("mapping-1")
	if got.SourcePort != 9090 {
		t.Errorf("expected SourcePort 9090, got %d", got.SourcePort)
	}
}

func TestPortMappingRepositoryV2_UpdatePortMappingStatus(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建映射
	mapping := &models.PortMapping{
		ID:             "mapping-1",
		UserID:         "user-1",
		ListenClientID: 1001,
		TargetClientID: 1002,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8080,
		TargetAddress:  "127.0.0.1:80",
		Status:         models.MappingStatusActive,
	}
	_ = repo.CreatePortMapping(mapping)

	// 更新状态
	err := repo.UpdatePortMappingStatus("mapping-1", models.MappingStatusInactive)
	if err != nil {
		t.Fatalf("UpdatePortMappingStatus failed: %v", err)
	}

	got, _ := repo.GetPortMapping("mapping-1")
	if got.Status != models.MappingStatusInactive {
		t.Errorf("expected status inactive, got %s", got.Status)
	}
}

func TestPortMappingRepositoryV2_BatchGetMappings(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建映射
	for i := 1; i <= 5; i++ {
		mapping := &models.PortMapping{
			ID:             "mapping-" + string(rune('0'+i)),
			UserID:         "user-1",
			ListenClientID: int64(1000 + i),
			TargetClientID: int64(2000 + i),
			Protocol:       models.ProtocolTCP,
			SourcePort:     8080 + i,
			TargetAddress:  "127.0.0.1:80",
			Status:         models.MappingStatusActive,
		}
		_ = repo.CreatePortMapping(mapping)
	}

	// 批量获取
	mappings, err := repo.BatchGetMappings([]string{"mapping-1", "mapping-3", "mapping-5", "nonexistent"})
	if err != nil {
		t.Fatalf("BatchGetMappings failed: %v", err)
	}

	if len(mappings) != 3 {
		t.Errorf("expected 3 mappings, got %d", len(mappings))
	}
}

func TestPortMappingRepositoryV2_CountUserMappings(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建映射
	for i := 1; i <= 7; i++ {
		mapping := &models.PortMapping{
			ID:             "mapping-" + string(rune('0'+i)),
			UserID:         "user-1",
			ListenClientID: int64(1000 + i),
			TargetClientID: int64(2000 + i),
			Protocol:       models.ProtocolTCP,
			SourcePort:     8080 + i,
			TargetAddress:  "127.0.0.1:80",
			Status:         models.MappingStatusActive,
		}
		_ = repo.CreatePortMapping(mapping)
	}

	count, err := repo.CountUserMappings("user-1")
	if err != nil {
		t.Fatalf("CountUserMappings failed: %v", err)
	}

	if count != 7 {
		t.Errorf("expected count 7, got %d", count)
	}
}

func TestPortMappingRepositoryV2_NilMapping(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	err := repo.CreatePortMapping(nil)
	if err == nil {
		t.Error("expected error for nil mapping")
	}

	err = repo.UpdatePortMapping(nil)
	if err == nil {
		t.Error("expected error for nil mapping")
	}

	err = repo.SavePortMapping(nil)
	if err == nil {
		t.Error("expected error for nil mapping")
	}
}

func TestPortMappingRepositoryV2_ClientIndexRemovalOnDelete(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建映射
	mapping := &models.PortMapping{
		ID:             "mapping-1",
		UserID:         "user-1",
		ListenClientID: 1001,
		TargetClientID: 1002,
		Protocol:       models.ProtocolTCP,
		SourcePort:     8080,
		TargetAddress:  "127.0.0.1:80",
		Status:         models.MappingStatusActive,
	}
	_ = repo.CreatePortMapping(mapping)

	// 验证客户端索引存在
	mappings1, _ := repo.GetClientPortMappings("1001")
	if len(mappings1) != 1 {
		t.Errorf("expected 1 mapping for client 1001 before delete, got %d", len(mappings1))
	}

	// 删除映射
	_ = repo.DeletePortMapping("mapping-1")

	// 验证客户端索引被清理
	mappings2, _ := repo.GetClientPortMappings("1001")
	if len(mappings2) != 0 {
		t.Errorf("expected 0 mappings for client 1001 after delete, got %d", len(mappings2))
	}
}

func TestPortMappingRepositoryV2_DomainIndexRemovalOnDelete(t *testing.T) {
	ctx := context.Background()
	repo := newTestPortMappingRepoV2(ctx)

	// 创建 HTTP 映射
	mapping := &models.PortMapping{
		ID:             "mapping-http",
		UserID:         "user-1",
		ListenClientID: 1001,
		TargetClientID: 1002,
		Protocol:       models.ProtocolHTTP,
		SourcePort:     80,
		TargetAddress:  "127.0.0.1:8080",
		HTTPSubdomain:  "test",
		HTTPBaseDomain: "example.com",
		Status:         models.MappingStatusActive,
	}
	_ = repo.CreatePortMapping(mapping)

	// 验证域名索引存在
	got, err := repo.GetPortMappingByDomain("test.example.com")
	if err != nil {
		t.Fatalf("expected mapping for domain before delete, got error: %v", err)
	}
	if got.ID != "mapping-http" {
		t.Errorf("expected mapping-http, got %s", got.ID)
	}

	// 删除映射
	_ = repo.DeletePortMapping("mapping-http")

	// 验证域名索引被清理
	_, err = repo.GetPortMappingByDomain("test.example.com")
	if err == nil {
		t.Error("expected error for domain after delete")
	}
}
