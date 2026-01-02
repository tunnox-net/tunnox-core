package repos

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/repository/index"
	"tunnox-core/internal/core/repository/indexed"
	"tunnox-core/internal/core/store"
	"tunnox-core/internal/core/store/memory"
)

// =============================================================================
// 测试辅助：创建带内存存储的 ClientConfigRepositoryV2
// =============================================================================

// mockCachedStore 模拟 CachedPersistentStore（使用 MemoryStore）
type mockCachedStore[K comparable, V any] struct {
	store.TTLStore[K, V]
	batchStore store.BatchStore[K, V]
}

func newMockCachedStore[K comparable, V any](memStore *memory.MemoryStore[K, V]) *mockCachedStore[K, V] {
	return &mockCachedStore[K, V]{
		TTLStore:   memStore,
		batchStore: memStore,
	}
}

func (m *mockCachedStore[K, V]) GetFromCache(ctx context.Context, key K) (V, error) {
	return m.Get(ctx, key)
}

func (m *mockCachedStore[K, V]) GetFromPersistent(ctx context.Context, key K) (V, error) {
	return m.Get(ctx, key)
}

func (m *mockCachedStore[K, V]) SetToCache(ctx context.Context, key K, value V, ttl time.Duration) error {
	return m.SetWithTTL(ctx, key, value, ttl)
}

func (m *mockCachedStore[K, V]) SetToPersistent(ctx context.Context, key K, value V) error {
	return m.Set(ctx, key, value)
}

func (m *mockCachedStore[K, V]) DeleteFromCache(ctx context.Context, key K) error {
	return m.Delete(ctx, key)
}

func (m *mockCachedStore[K, V]) DeleteFromPersistent(ctx context.Context, key K) error {
	return m.Delete(ctx, key)
}

func (m *mockCachedStore[K, V]) GetCacheStats() store.CacheStats {
	return store.CacheStats{}
}

func (m *mockCachedStore[K, V]) InvalidateCache(ctx context.Context, key K) error {
	return m.Delete(ctx, key)
}

func (m *mockCachedStore[K, V]) RefreshCache(ctx context.Context, key K) error {
	return nil
}

func (m *mockCachedStore[K, V]) BatchGet(ctx context.Context, keys []K) (map[K]V, error) {
	return m.batchStore.BatchGet(ctx, keys)
}

func (m *mockCachedStore[K, V]) BatchSet(ctx context.Context, items map[K]V) error {
	return m.batchStore.BatchSet(ctx, items)
}

func (m *mockCachedStore[K, V]) BatchDelete(ctx context.Context, keys []K) error {
	return m.batchStore.BatchDelete(ctx, keys)
}

func (m *mockCachedStore[K, V]) Close() error {
	return nil
}

// newTestClientConfigRepoV2 创建测试用的 ClientConfigRepositoryV2
func newTestClientConfigRepoV2(ctx context.Context) *ClientConfigRepositoryV2 {
	// 使用内存存储
	memStore := memory.NewMemoryStore[string, *models.ClientConfig]()
	cachedStore := newMockCachedStore[string, *models.ClientConfig](memStore)
	indexStore := memory.NewMemorySetStore[string, string]()
	globalListStore := memory.NewMemorySetStore[string, string]()

	// 创建索引管理器
	indexManager := index.NewUserEntityIndexManager[*models.ClientConfig](
		indexStore,
		constants.KeyPrefixIndexUserClients,
		func(config *models.ClientConfig) string {
			return config.GetUserID()
		},
	)

	// 创建带索引的 Repository
	baseRepo := indexed.NewUserIndexedRepository[*models.ClientConfig](
		cachedStore,
		indexManager,
		constants.KeyPrefixPersistClientConfig,
		"ClientConfig",
	)

	return &ClientConfigRepositoryV2{
		baseRepo:        baseRepo,
		globalListStore: globalListStore,
		ctx:             ctx,
	}
}

// =============================================================================
// ClientConfigRepositoryV2 测试
// =============================================================================

func TestClientConfigRepositoryV2_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	// 创建配置
	config := &models.ClientConfig{
		ID:       1001,
		UserID:   "user-1",
		Name:     "test-client",
		AuthCode: "auth-001",
		Type:     models.ClientTypeRegistered,
	}

	err := repo.CreateConfig(config)
	if err != nil {
		t.Fatalf("CreateConfig failed: %v", err)
	}

	// 获取配置
	got, err := repo.GetConfig(1001)
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	if got.ID != config.ID {
		t.Errorf("expected ID %d, got %d", config.ID, got.ID)
	}
	if got.UserID != config.UserID {
		t.Errorf("expected UserID %s, got %s", config.UserID, got.UserID)
	}
	if got.Name != config.Name {
		t.Errorf("expected Name %s, got %s", config.Name, got.Name)
	}
}

func TestClientConfigRepositoryV2_GetNotFound(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	_, err := repo.GetConfig(9999)
	if err == nil {
		t.Error("expected error for non-existent config")
	}
}

func TestClientConfigRepositoryV2_UpdateConfig(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	// 创建配置
	config := &models.ClientConfig{
		ID:       1001,
		UserID:   "user-1",
		Name:     "original-name",
		AuthCode: "auth-001",
		Type:     models.ClientTypeRegistered,
	}
	_ = repo.CreateConfig(config)

	// 更新配置
	config.Name = "updated-name"
	err := repo.UpdateConfig(config)
	if err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	// 验证更新
	got, _ := repo.GetConfig(1001)
	if got.Name != "updated-name" {
		t.Errorf("expected name 'updated-name', got '%s'", got.Name)
	}
}

func TestClientConfigRepositoryV2_DeleteConfig(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	// 创建并删除
	config := &models.ClientConfig{
		ID:       1001,
		UserID:   "user-1",
		Name:     "test-client",
		AuthCode: "auth-001",
		Type:     models.ClientTypeRegistered,
	}
	_ = repo.CreateConfig(config)

	err := repo.DeleteConfig(1001)
	if err != nil {
		t.Fatalf("DeleteConfig failed: %v", err)
	}

	// 验证删除
	_, err = repo.GetConfig(1001)
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestClientConfigRepositoryV2_ListUserConfigs(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	// 创建多个配置
	for i := 1; i <= 5; i++ {
		config := &models.ClientConfig{
			ID:       int64(1000 + i),
			UserID:   "user-1",
			Name:     "client-" + string(rune('0'+i)),
			AuthCode: "auth-" + string(rune('0'+i)),
			Type:     models.ClientTypeRegistered,
		}
		_ = repo.CreateConfig(config)
	}

	// 创建另一个用户的配置
	config := &models.ClientConfig{
		ID:       2001,
		UserID:   "user-2",
		Name:     "other-client",
		AuthCode: "auth-x",
		Type:     models.ClientTypeRegistered,
	}
	_ = repo.CreateConfig(config)

	// 列出 user-1 的配置
	configs, err := repo.ListUserConfigs("user-1")
	if err != nil {
		t.Fatalf("ListUserConfigs failed: %v", err)
	}

	if len(configs) != 5 {
		t.Errorf("expected 5 configs for user-1, got %d", len(configs))
	}

	// 验证都是 user-1 的
	for _, c := range configs {
		if c.UserID != "user-1" {
			t.Errorf("expected UserID 'user-1', got '%s'", c.UserID)
		}
	}
}

func TestClientConfigRepositoryV2_ExistsConfig(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	// 不存在
	exists, err := repo.ExistsConfig(1001)
	if err != nil {
		t.Fatalf("ExistsConfig failed: %v", err)
	}
	if exists {
		t.Error("expected config not to exist")
	}

	// 创建后存在
	config := &models.ClientConfig{
		ID:       1001,
		UserID:   "user-1",
		Name:     "test-client",
		AuthCode: "auth-001",
		Type:     models.ClientTypeRegistered,
	}
	_ = repo.CreateConfig(config)

	exists, err = repo.ExistsConfig(1001)
	if err != nil {
		t.Fatalf("ExistsConfig failed: %v", err)
	}
	if !exists {
		t.Error("expected config to exist")
	}
}

func TestClientConfigRepositoryV2_SaveConfig(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	// SaveConfig 应该自动处理创建和更新
	config := &models.ClientConfig{
		ID:       1001,
		UserID:   "user-1",
		Name:     "test-client",
		AuthCode: "auth-001",
		Type:     models.ClientTypeRegistered,
	}

	// 第一次保存（创建）
	err := repo.SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig (create) failed: %v", err)
	}

	// 第二次保存（更新）
	config.Name = "updated-name"
	err = repo.SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig (update) failed: %v", err)
	}

	got, _ := repo.GetConfig(1001)
	if got.Name != "updated-name" {
		t.Errorf("expected name 'updated-name', got '%s'", got.Name)
	}
}

func TestClientConfigRepositoryV2_BatchGetConfigs(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	// 创建配置
	for i := 1; i <= 5; i++ {
		config := &models.ClientConfig{
			ID:       int64(1000 + i),
			UserID:   "user-1",
			Name:     "client-" + string(rune('0'+i)),
			AuthCode: "auth-" + string(rune('0'+i)),
			Type:     models.ClientTypeRegistered,
		}
		_ = repo.CreateConfig(config)
	}

	// 批量获取
	configs, err := repo.BatchGetConfigs([]int64{1001, 1003, 1005, 9999})
	if err != nil {
		t.Fatalf("BatchGetConfigs failed: %v", err)
	}

	if len(configs) != 3 {
		t.Errorf("expected 3 configs, got %d", len(configs))
	}

	// 验证获取到的配置
	if _, ok := configs[1001]; !ok {
		t.Error("expected config 1001 in result")
	}
	if _, ok := configs[1003]; !ok {
		t.Error("expected config 1003 in result")
	}
	if _, ok := configs[1005]; !ok {
		t.Error("expected config 1005 in result")
	}
}

func TestClientConfigRepositoryV2_CountUserConfigs(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	// 创建配置
	for i := 1; i <= 3; i++ {
		config := &models.ClientConfig{
			ID:       int64(1000 + i),
			UserID:   "user-1",
			Name:     "client-" + string(rune('0'+i)),
			AuthCode: "auth-" + string(rune('0'+i)),
			Type:     models.ClientTypeRegistered,
		}
		_ = repo.CreateConfig(config)
	}

	count, err := repo.CountUserConfigs("user-1")
	if err != nil {
		t.Fatalf("CountUserConfigs failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestClientConfigRepositoryV2_ValidationError(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	// 无效配置（ID 为 0）
	config := &models.ClientConfig{
		ID:       0,
		UserID:   "user-1",
		Name:     "test-client",
		AuthCode: "auth-001",
		Type:     models.ClientTypeRegistered,
	}

	err := repo.CreateConfig(config)
	if err == nil {
		t.Error("expected validation error for ID=0")
	}
}

func TestClientConfigRepositoryV2_NilConfig(t *testing.T) {
	ctx := context.Background()
	repo := newTestClientConfigRepoV2(ctx)

	err := repo.CreateConfig(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}

	err = repo.UpdateConfig(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}

	err = repo.SaveConfig(nil)
	if err == nil {
		t.Error("expected error for nil config")
	}
}
