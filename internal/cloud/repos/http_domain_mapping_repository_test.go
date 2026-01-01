package repos

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage/types"
)

// =============================================================================
// Mock Storage Implementation
// =============================================================================

// mockStorage 实现所有存储接口用于测试
type mockStorage struct {
	mu       sync.RWMutex
	data     map[string]any
	lists    map[string][]any
	counters map[string]int64
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		data:     make(map[string]any),
		lists:    make(map[string][]any),
		counters: make(map[string]int64),
	}
}

// Storage interface methods
func (m *mockStorage) Set(key string, value any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockStorage) Get(key string) (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, types.ErrKeyNotFound
	}
	return v, nil
}

func (m *mockStorage) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockStorage) Exists(key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *mockStorage) SetExpiration(key string, ttl time.Duration) error {
	return nil
}

func (m *mockStorage) GetExpiration(key string) (time.Duration, error) {
	return 0, nil
}

func (m *mockStorage) CleanupExpired() error {
	return nil
}

func (m *mockStorage) Close() error {
	return nil
}

// ListStore interface methods
func (m *mockStorage) SetList(key string, values []any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lists[key] = values
	return nil
}

func (m *mockStorage) GetList(key string) ([]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list, ok := m.lists[key]
	if !ok {
		return nil, types.ErrKeyNotFound
	}
	return list, nil
}

func (m *mockStorage) AppendToList(key string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lists[key] = append(m.lists[key], value)
	return nil
}

func (m *mockStorage) RemoveFromList(key string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list, ok := m.lists[key]
	if !ok {
		return nil
	}
	newList := make([]any, 0, len(list))
	for _, v := range list {
		if v != value {
			newList = append(newList, v)
		}
	}
	m.lists[key] = newList
	return nil
}

// CounterStore interface methods
func (m *mockStorage) Incr(key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[key]++
	return m.counters[key], nil
}

func (m *mockStorage) IncrBy(key string, value int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[key] += value
	return m.counters[key], nil
}

// CASStore interface methods
func (m *mockStorage) SetNX(key string, value any, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; ok {
		return false, nil // Key already exists
	}
	m.data[key] = value
	return true, nil
}

func (m *mockStorage) CompareAndSwap(key string, oldValue, newValue any, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return false, nil
	}
	if v != oldValue {
		return false, nil
	}
	m.data[key] = newValue
	return true, nil
}

// HashStore interface methods (not used but included for completeness)
func (m *mockStorage) SetHash(key string, field string, value any) error {
	return nil
}

func (m *mockStorage) GetHash(key string, field string) (any, error) {
	return nil, types.ErrKeyNotFound
}

func (m *mockStorage) GetAllHash(key string) (map[string]any, error) {
	return nil, nil
}

func (m *mockStorage) DeleteHash(key string, field string) error {
	return nil
}

// WatchableStore interface methods (not used but included for completeness)
func (m *mockStorage) Watch(key string, callback func(any)) error {
	return nil
}

func (m *mockStorage) Unwatch(key string) error {
	return nil
}

// =============================================================================
// Helper Functions
// =============================================================================

func createTestRepository() (*HTTPDomainMappingRepository, *mockStorage) {
	mockStor := newMockStorage()
	baseRepo := &Repository{storage: mockStor}
	httpDomainRepo := NewHTTPDomainMappingRepository(baseRepo, []string{"tunnox.net", "test.local"})
	return httpDomainRepo, mockStor
}

// =============================================================================
// Test Cases
// =============================================================================

func TestNewHTTPDomainMappingRepository(t *testing.T) {
	t.Run("with custom base domains", func(t *testing.T) {
		mockStor := newMockStorage()
		baseRepo := &Repository{storage: mockStor}
		repo := NewHTTPDomainMappingRepository(baseRepo, []string{"example.com", "test.org"})

		domains := repo.GetBaseDomains()
		if len(domains) != 2 {
			t.Errorf("expected 2 domains, got %d", len(domains))
		}
		if domains[0] != "example.com" || domains[1] != "test.org" {
			t.Errorf("unexpected domains: %v", domains)
		}
	})

	t.Run("with empty base domains uses default", func(t *testing.T) {
		mockStor := newMockStorage()
		baseRepo := &Repository{storage: mockStor}
		repo := NewHTTPDomainMappingRepository(baseRepo, nil)

		domains := repo.GetBaseDomains()
		if len(domains) != 1 || domains[0] != "tunnox.net" {
			t.Errorf("expected default domain 'tunnox.net', got %v", domains)
		}
	})
}

func TestCheckSubdomainAvailable(t *testing.T) {
	repo, mockStor := createTestRepository()
	ctx := context.Background()

	t.Run("available subdomain", func(t *testing.T) {
		available, err := repo.CheckSubdomainAvailable(ctx, "newsubdomain", "tunnox.net")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !available {
			t.Error("expected subdomain to be available")
		}
	})

	t.Run("occupied subdomain", func(t *testing.T) {
		// Manually set an index key to simulate occupied domain
		indexKey := HTTPDomainIndexKey("occupied.tunnox.net")
		mockStor.data[indexKey] = "hdm_1"

		available, err := repo.CheckSubdomainAvailable(ctx, "occupied", "tunnox.net")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if available {
			t.Error("expected subdomain to be occupied")
		}
	})
}

func TestCreateMapping(t *testing.T) {
	ctx := context.Background()

	t.Run("successful creation", func(t *testing.T) {
		repo, _ := createTestRepository()

		mapping, err := repo.CreateMapping(ctx, 12345, "myapp", "tunnox.net", "localhost", 8080)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if mapping.ID == "" {
			t.Error("expected mapping ID to be generated")
		}
		if mapping.Subdomain != "myapp" {
			t.Errorf("expected subdomain 'myapp', got '%s'", mapping.Subdomain)
		}
		if mapping.BaseDomain != "tunnox.net" {
			t.Errorf("expected base domain 'tunnox.net', got '%s'", mapping.BaseDomain)
		}
		if mapping.FullDomain != "myapp.tunnox.net" {
			t.Errorf("expected full domain 'myapp.tunnox.net', got '%s'", mapping.FullDomain)
		}
		if mapping.ClientID != 12345 {
			t.Errorf("expected client ID 12345, got %d", mapping.ClientID)
		}
		if mapping.TargetHost != "localhost" {
			t.Errorf("expected target host 'localhost', got '%s'", mapping.TargetHost)
		}
		if mapping.TargetPort != 8080 {
			t.Errorf("expected target port 8080, got %d", mapping.TargetPort)
		}
		if mapping.Status != HTTPDomainMappingStatusActive {
			t.Errorf("expected status 'active', got '%s'", mapping.Status)
		}
		if mapping.CreatedAt == 0 {
			t.Error("expected created_at to be set")
		}
	})

	t.Run("duplicate domain fails", func(t *testing.T) {
		repo, _ := createTestRepository()

		// Create first mapping
		_, err := repo.CreateMapping(ctx, 12345, "duplicate", "tunnox.net", "localhost", 8080)
		if err != nil {
			t.Fatalf("first creation should succeed: %v", err)
		}

		// Try to create duplicate
		_, err = repo.CreateMapping(ctx, 67890, "duplicate", "tunnox.net", "localhost", 9090)
		if err == nil {
			t.Error("expected error for duplicate domain")
		}
		if !coreerrors.IsCode(err, coreerrors.CodeAlreadyExists) {
			t.Errorf("expected CodeAlreadyExists error, got: %v", err)
		}
	})

	t.Run("unsupported base domain fails", func(t *testing.T) {
		repo, _ := createTestRepository()

		_, err := repo.CreateMapping(ctx, 12345, "myapp", "unsupported.com", "localhost", 8080)
		if err == nil {
			t.Error("expected error for unsupported base domain")
		}
		if !coreerrors.IsCode(err, coreerrors.CodeInvalidParam) {
			t.Errorf("expected CodeInvalidParam error, got: %v", err)
		}
	})
}

func TestGetMapping(t *testing.T) {
	ctx := context.Background()
	repo, _ := createTestRepository()

	t.Run("existing mapping", func(t *testing.T) {
		// Create a mapping first
		created, err := repo.CreateMapping(ctx, 12345, "gettest", "tunnox.net", "localhost", 8080)
		if err != nil {
			t.Fatalf("failed to create mapping: %v", err)
		}

		// Get the mapping
		fetched, err := repo.GetMapping(ctx, created.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if fetched.ID != created.ID {
			t.Errorf("expected ID '%s', got '%s'", created.ID, fetched.ID)
		}
		if fetched.FullDomain != created.FullDomain {
			t.Errorf("expected domain '%s', got '%s'", created.FullDomain, fetched.FullDomain)
		}
	})

	t.Run("non-existing mapping", func(t *testing.T) {
		_, err := repo.GetMapping(ctx, "nonexistent")
		if err == nil {
			t.Error("expected error for non-existing mapping")
		}
		if !coreerrors.IsCode(err, coreerrors.CodeMappingNotFound) {
			t.Errorf("expected CodeMappingNotFound error, got: %v", err)
		}
	})
}

func TestGetMappingsByClientID(t *testing.T) {
	ctx := context.Background()
	repo, _ := createTestRepository()

	t.Run("multiple mappings for client", func(t *testing.T) {
		clientID := int64(99999)

		// Create multiple mappings for the same client
		_, err := repo.CreateMapping(ctx, clientID, "app1", "tunnox.net", "localhost", 8081)
		if err != nil {
			t.Fatalf("failed to create mapping 1: %v", err)
		}
		_, err = repo.CreateMapping(ctx, clientID, "app2", "tunnox.net", "localhost", 8082)
		if err != nil {
			t.Fatalf("failed to create mapping 2: %v", err)
		}
		_, err = repo.CreateMapping(ctx, clientID, "app3", "test.local", "localhost", 8083)
		if err != nil {
			t.Fatalf("failed to create mapping 3: %v", err)
		}

		// Get all mappings for the client
		mappings, err := repo.GetMappingsByClientID(ctx, clientID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mappings) != 3 {
			t.Errorf("expected 3 mappings, got %d", len(mappings))
		}
	})

	t.Run("no mappings for client", func(t *testing.T) {
		mappings, err := repo.GetMappingsByClientID(ctx, 11111)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mappings) != 0 {
			t.Errorf("expected 0 mappings, got %d", len(mappings))
		}
	})
}

func TestUpdateMapping(t *testing.T) {
	ctx := context.Background()
	repo, _ := createTestRepository()

	t.Run("successful update", func(t *testing.T) {
		// Create a mapping first
		created, err := repo.CreateMapping(ctx, 12345, "updatetest", "tunnox.net", "localhost", 8080)
		if err != nil {
			t.Fatalf("failed to create mapping: %v", err)
		}

		// Wait a bit to ensure different timestamp
		time.Sleep(time.Millisecond * 10)
		originalUpdatedAt := created.UpdatedAt

		// Update the mapping
		created.TargetHost = "192.168.1.100"
		created.TargetPort = 9090
		created.Status = HTTPDomainMappingStatusInactive

		err = repo.UpdateMapping(ctx, created)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify the update
		updated, err := repo.GetMapping(ctx, created.ID)
		if err != nil {
			t.Fatalf("failed to get updated mapping: %v", err)
		}

		if updated.TargetHost != "192.168.1.100" {
			t.Errorf("expected target host '192.168.1.100', got '%s'", updated.TargetHost)
		}
		if updated.TargetPort != 9090 {
			t.Errorf("expected target port 9090, got %d", updated.TargetPort)
		}
		if updated.Status != HTTPDomainMappingStatusInactive {
			t.Errorf("expected status 'inactive', got '%s'", updated.Status)
		}
		// Note: UpdatedAt uses Unix seconds, so within the same second it may not increase
		// Just verify it's at least equal (not less)
		if updated.UpdatedAt < originalUpdatedAt {
			t.Error("expected updated_at to be at least equal to original")
		}
	})

	t.Run("cannot modify immutable fields", func(t *testing.T) {
		// Create a mapping first
		created, err := repo.CreateMapping(ctx, 12345, "immutabletest", "tunnox.net", "localhost", 8080)
		if err != nil {
			t.Fatalf("failed to create mapping: %v", err)
		}

		// Try to modify subdomain
		created.Subdomain = "modified"
		err = repo.UpdateMapping(ctx, created)
		if err == nil {
			t.Error("expected error when modifying immutable field")
		}
		if !coreerrors.IsCode(err, coreerrors.CodeInvalidRequest) {
			t.Errorf("expected CodeInvalidRequest error, got: %v", err)
		}
	})
}

func TestDeleteMapping(t *testing.T) {
	ctx := context.Background()

	t.Run("successful deletion", func(t *testing.T) {
		repo, mockStor := createTestRepository()

		// Create a mapping first
		created, err := repo.CreateMapping(ctx, 12345, "deletetest", "tunnox.net", "localhost", 8080)
		if err != nil {
			t.Fatalf("failed to create mapping: %v", err)
		}

		// Verify it exists
		_, err = repo.GetMapping(ctx, created.ID)
		if err != nil {
			t.Fatalf("mapping should exist: %v", err)
		}

		// Delete the mapping
		err = repo.DeleteMapping(ctx, created.ID, 12345)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Verify it's deleted
		_, err = repo.GetMapping(ctx, created.ID)
		if err == nil {
			t.Error("expected error for deleted mapping")
		}

		// Verify domain index is also deleted
		indexKey := HTTPDomainIndexKey(created.FullDomain)
		_, ok := mockStor.data[indexKey]
		if ok {
			t.Error("expected domain index to be deleted")
		}
	})

	t.Run("cannot delete other client's mapping", func(t *testing.T) {
		repo, _ := createTestRepository()

		// Create a mapping for client 12345
		created, err := repo.CreateMapping(ctx, 12345, "ownertest", "tunnox.net", "localhost", 8080)
		if err != nil {
			t.Fatalf("failed to create mapping: %v", err)
		}

		// Try to delete with different client ID
		err = repo.DeleteMapping(ctx, created.ID, 99999)
		if err == nil {
			t.Error("expected error when deleting other client's mapping")
		}
		if !coreerrors.IsCode(err, coreerrors.CodeForbidden) {
			t.Errorf("expected CodeForbidden error, got: %v", err)
		}
	})

	t.Run("delete non-existing mapping succeeds", func(t *testing.T) {
		repo, _ := createTestRepository()

		// Delete non-existing mapping should not error (idempotent)
		err := repo.DeleteMapping(ctx, "nonexistent", 12345)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestLookupByDomain(t *testing.T) {
	ctx := context.Background()
	repo, _ := createTestRepository()

	t.Run("successful lookup", func(t *testing.T) {
		// Create a mapping first
		created, err := repo.CreateMapping(ctx, 12345, "lookuptest", "tunnox.net", "localhost", 8080)
		if err != nil {
			t.Fatalf("failed to create mapping: %v", err)
		}

		// Lookup by domain
		found, err := repo.LookupByDomain(ctx, "lookuptest.tunnox.net")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if found.ID != created.ID {
			t.Errorf("expected ID '%s', got '%s'", created.ID, found.ID)
		}
		if found.FullDomain != created.FullDomain {
			t.Errorf("expected domain '%s', got '%s'", created.FullDomain, found.FullDomain)
		}
	})

	t.Run("lookup non-existing domain", func(t *testing.T) {
		_, err := repo.LookupByDomain(ctx, "nonexistent.tunnox.net")
		if err == nil {
			t.Error("expected error for non-existing domain")
		}
		if !coreerrors.IsCode(err, coreerrors.CodeMappingNotFound) {
			t.Errorf("expected CodeMappingNotFound error, got: %v", err)
		}
	})
}

func TestListAllMappings(t *testing.T) {
	ctx := context.Background()
	repo, _ := createTestRepository()

	t.Run("list all mappings", func(t *testing.T) {
		// Create multiple mappings
		_, err := repo.CreateMapping(ctx, 11111, "list1", "tunnox.net", "localhost", 8081)
		if err != nil {
			t.Fatalf("failed to create mapping 1: %v", err)
		}
		_, err = repo.CreateMapping(ctx, 22222, "list2", "tunnox.net", "localhost", 8082)
		if err != nil {
			t.Fatalf("failed to create mapping 2: %v", err)
		}
		_, err = repo.CreateMapping(ctx, 33333, "list3", "test.local", "localhost", 8083)
		if err != nil {
			t.Fatalf("failed to create mapping 3: %v", err)
		}

		// List all
		mappings, err := repo.ListAllMappings(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mappings) != 3 {
			t.Errorf("expected 3 mappings, got %d", len(mappings))
		}
	})

	t.Run("empty list", func(t *testing.T) {
		emptyRepo, _ := createTestRepository()

		mappings, err := emptyRepo.ListAllMappings(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(mappings) != 0 {
			t.Errorf("expected 0 mappings, got %d", len(mappings))
		}
	})
}

func TestCleanupExpiredMappings(t *testing.T) {
	ctx := context.Background()
	repo, mockStor := createTestRepository()

	t.Run("cleanup expired mappings", func(t *testing.T) {
		// Create a mapping
		mapping, err := repo.CreateMapping(ctx, 12345, "expiredtest", "tunnox.net", "localhost", 8080)
		if err != nil {
			t.Fatalf("failed to create mapping: %v", err)
		}

		// Manually set expiration to past
		mapping.ExpiresAt = time.Now().Unix() - 3600 // 1 hour ago
		data, _ := json.Marshal(mapping)
		mappingKey := HTTPDomainMappingKey(mapping.ID)
		mockStor.data[mappingKey] = string(data)

		// Create a non-expired mapping
		_, err = repo.CreateMapping(ctx, 12345, "activetest", "tunnox.net", "localhost", 8081)
		if err != nil {
			t.Fatalf("failed to create active mapping: %v", err)
		}

		// Cleanup
		count, err := repo.CleanupExpiredMappings(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if count != 1 {
			t.Errorf("expected 1 cleaned up, got %d", count)
		}

		// Verify expired mapping is deleted
		_, err = repo.GetMapping(ctx, mapping.ID)
		if err == nil {
			t.Error("expected error for expired mapping")
		}
	})
}

func TestGetBaseDomains(t *testing.T) {
	repo, _ := createTestRepository()

	domains := repo.GetBaseDomains()
	if len(domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(domains))
	}

	// Verify the returned slice is a copy
	domains[0] = "modified.com"
	originalDomains := repo.GetBaseDomains()
	if originalDomains[0] == "modified.com" {
		t.Error("GetBaseDomains should return a copy, not the original slice")
	}
}

func TestHTTPDomainMappingMethods(t *testing.T) {
	t.Run("Target", func(t *testing.T) {
		m := &HTTPDomainMapping{TargetHost: "localhost", TargetPort: 8080}
		if m.Target() != "localhost:8080" {
			t.Errorf("expected 'localhost:8080', got '%s'", m.Target())
		}
	})

	t.Run("TargetURL", func(t *testing.T) {
		m := &HTTPDomainMapping{TargetHost: "localhost", TargetPort: 8080}
		if m.TargetURL() != "http://localhost:8080" {
			t.Errorf("expected 'http://localhost:8080', got '%s'", m.TargetURL())
		}
	})

	t.Run("IsExpired", func(t *testing.T) {
		// Not expired (no expiration)
		m1 := &HTTPDomainMapping{ExpiresAt: 0}
		if m1.IsExpired() {
			t.Error("mapping without expiration should not be expired")
		}

		// Not expired (future)
		m2 := &HTTPDomainMapping{ExpiresAt: time.Now().Unix() + 3600}
		if m2.IsExpired() {
			t.Error("mapping with future expiration should not be expired")
		}

		// Expired
		m3 := &HTTPDomainMapping{ExpiresAt: time.Now().Unix() - 3600}
		if !m3.IsExpired() {
			t.Error("mapping with past expiration should be expired")
		}
	})

	t.Run("IsActive", func(t *testing.T) {
		// Active and not expired
		m1 := &HTTPDomainMapping{Status: HTTPDomainMappingStatusActive, ExpiresAt: 0}
		if !m1.IsActive() {
			t.Error("active mapping without expiration should be active")
		}

		// Inactive
		m2 := &HTTPDomainMapping{Status: HTTPDomainMappingStatusInactive, ExpiresAt: 0}
		if m2.IsActive() {
			t.Error("inactive mapping should not be active")
		}

		// Active but expired
		m3 := &HTTPDomainMapping{Status: HTTPDomainMappingStatusActive, ExpiresAt: time.Now().Unix() - 3600}
		if m3.IsActive() {
			t.Error("expired mapping should not be active")
		}
	})

	t.Run("TimeRemaining", func(t *testing.T) {
		// No expiration
		m1 := &HTTPDomainMapping{ExpiresAt: 0}
		if m1.TimeRemaining() != 0 {
			t.Error("mapping without expiration should return 0 time remaining")
		}

		// Expired
		m2 := &HTTPDomainMapping{ExpiresAt: time.Now().Unix() - 3600}
		if m2.TimeRemaining() != 0 {
			t.Error("expired mapping should return 0 time remaining")
		}

		// Not expired
		futureTime := time.Now().Unix() + 3600
		m3 := &HTTPDomainMapping{ExpiresAt: futureTime}
		remaining := m3.TimeRemaining()
		if remaining < 3500*time.Second || remaining > 3600*time.Second {
			t.Errorf("expected ~3600 seconds remaining, got %v", remaining)
		}
	})

	t.Run("Validate", func(t *testing.T) {
		// Valid mapping
		valid := &HTTPDomainMapping{
			ID:         "hdm_1",
			Subdomain:  "test",
			BaseDomain: "tunnox.net",
			FullDomain: "test.tunnox.net",
			ClientID:   12345,
			TargetHost: "localhost",
			TargetPort: 8080,
		}
		if err := valid.Validate(); err != nil {
			t.Errorf("valid mapping should pass validation: %v", err)
		}

		// Missing ID
		noID := &HTTPDomainMapping{
			Subdomain:  "test",
			BaseDomain: "tunnox.net",
			FullDomain: "test.tunnox.net",
			ClientID:   12345,
			TargetHost: "localhost",
			TargetPort: 8080,
		}
		if err := noID.Validate(); err == nil {
			t.Error("mapping without ID should fail validation")
		}

		// Invalid port
		invalidPort := &HTTPDomainMapping{
			ID:         "hdm_1",
			Subdomain:  "test",
			BaseDomain: "tunnox.net",
			FullDomain: "test.tunnox.net",
			ClientID:   12345,
			TargetHost: "localhost",
			TargetPort: 99999, // Invalid port
		}
		if err := invalidPort.Validate(); err == nil {
			t.Error("mapping with invalid port should fail validation")
		}

		// Invalid client ID
		invalidClient := &HTTPDomainMapping{
			ID:         "hdm_1",
			Subdomain:  "test",
			BaseDomain: "tunnox.net",
			FullDomain: "test.tunnox.net",
			ClientID:   0, // Invalid
			TargetHost: "localhost",
			TargetPort: 8080,
		}
		if err := invalidClient.Validate(); err == nil {
			t.Error("mapping with zero client ID should fail validation")
		}
	})
}

func TestStorageKeyFunctions(t *testing.T) {
	t.Run("HTTPDomainMappingKey", func(t *testing.T) {
		key := HTTPDomainMappingKey("hdm_123")
		expected := "tunnox:http_domain:mapping:hdm_123"
		if key != expected {
			t.Errorf("expected '%s', got '%s'", expected, key)
		}
	})

	t.Run("HTTPDomainIndexKey", func(t *testing.T) {
		key := HTTPDomainIndexKey("myapp.tunnox.net")
		expected := "tunnox:http_domain:index:myapp.tunnox.net"
		if key != expected {
			t.Errorf("expected '%s', got '%s'", expected, key)
		}
	})

	t.Run("HTTPDomainClientKey", func(t *testing.T) {
		key := HTTPDomainClientKey(12345)
		expected := "tunnox:http_domain:client:12345"
		if key != expected {
			t.Errorf("expected '%s', got '%s'", expected, key)
		}
	})
}

// =============================================================================
// Concurrency Tests
// =============================================================================

func TestConcurrentCreateMapping(t *testing.T) {
	ctx := context.Background()
	repo, _ := createTestRepository()

	// Try to create the same domain from multiple goroutines
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			// Use idx+1 to ensure clientID is always > 0 (valid)
			_, err := repo.CreateMapping(ctx, int64(idx+1), "concurrent", "tunnox.net", "localhost", 8080+idx)
			results <- err
		}(i)
	}

	successCount := 0
	conflictCount := 0

	for i := 0; i < numGoroutines; i++ {
		err := <-results
		if err == nil {
			successCount++
		} else if coreerrors.IsCode(err, coreerrors.CodeAlreadyExists) {
			conflictCount++
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}

	// Only one should succeed
	if successCount != 1 {
		t.Errorf("expected exactly 1 success, got %d", successCount)
	}
	if conflictCount != numGoroutines-1 {
		t.Errorf("expected %d conflicts, got %d", numGoroutines-1, conflictCount)
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	ctx := context.Background()
	repo, _ := createTestRepository()

	// Create initial mapping
	mapping, err := repo.CreateMapping(ctx, 12345, "concurrent-rw", "tunnox.net", "localhost", 8080)
	if err != nil {
		t.Fatalf("failed to create mapping: %v", err)
	}

	const numOperations = 100
	done := make(chan bool, numOperations*3)

	// Concurrent reads
	for i := 0; i < numOperations; i++ {
		go func() {
			_, _ = repo.GetMapping(ctx, mapping.ID)
			done <- true
		}()
	}

	// Concurrent lookups
	for i := 0; i < numOperations; i++ {
		go func() {
			_, _ = repo.LookupByDomain(ctx, "concurrent-rw.tunnox.net")
			done <- true
		}()
	}

	// Concurrent list operations
	for i := 0; i < numOperations; i++ {
		go func() {
			_, _ = repo.ListAllMappings(ctx)
			done <- true
		}()
	}

	// Wait for all operations
	for i := 0; i < numOperations*3; i++ {
		<-done
	}
}
