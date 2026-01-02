package memory

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/core/store"
)

// =============================================================================
// MemoryStore 基础测试
// =============================================================================

func TestMemoryStore_SetGet(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Set
	err := s.Set(ctx, "key1", "value1")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	value, err := s.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if value != "value1" {
		t.Errorf("expected 'value1', got '%s'", value)
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	_, err := s.Get(ctx, "nonexistent")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Set then delete
	_ = s.Set(ctx, "key1", "value1")
	err := s.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = s.Get(ctx, "key1")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMemoryStore_Exists(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Not exists
	exists, err := s.Exists(ctx, "key1")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("expected key1 not to exist")
	}

	// Set and check exists
	_ = s.Set(ctx, "key1", "value1")
	exists, err = s.Exists(ctx, "key1")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("expected key1 to exist")
	}
}

// =============================================================================
// TTL 测试
// =============================================================================

func TestMemoryStore_SetWithTTL(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Set with TTL
	err := s.SetWithTTL(ctx, "key1", "value1", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("SetWithTTL failed: %v", err)
	}

	// Should exist immediately
	value, err := s.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if value != "value1" {
		t.Errorf("expected 'value1', got '%s'", value)
	}

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Should not exist after TTL
	_, err = s.Get(ctx, "key1")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound after TTL, got %v", err)
	}
}

func TestMemoryStore_GetTTL(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Set with TTL
	_ = s.SetWithTTL(ctx, "key1", "value1", 1*time.Second)

	// Get TTL
	ttl, err := s.GetTTL(ctx, "key1")
	if err != nil {
		t.Fatalf("GetTTL failed: %v", err)
	}
	if ttl <= 0 || ttl > 1*time.Second {
		t.Errorf("unexpected TTL: %v", ttl)
	}

	// Set without TTL
	_ = s.Set(ctx, "key2", "value2")
	ttl, err = s.GetTTL(ctx, "key2")
	if err != nil {
		t.Fatalf("GetTTL failed: %v", err)
	}
	if ttl != -1 {
		t.Errorf("expected -1 for no TTL, got %v", ttl)
	}
}

func TestMemoryStore_Refresh(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Set with short TTL
	_ = s.SetWithTTL(ctx, "key1", "value1", 100*time.Millisecond)

	// Refresh to longer TTL
	err := s.Refresh(ctx, "key1", 1*time.Second)
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// Wait past original TTL
	time.Sleep(150 * time.Millisecond)

	// Should still exist
	_, err = s.Get(ctx, "key1")
	if err != nil {
		t.Errorf("expected key to exist after refresh, got %v", err)
	}
}

func TestMemoryStore_RefreshNotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	err := s.Refresh(ctx, "nonexistent", 1*time.Second)
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// =============================================================================
// 批量操作测试
// =============================================================================

func TestMemoryStore_BatchGet(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Set some keys
	_ = s.Set(ctx, "key1", "value1")
	_ = s.Set(ctx, "key2", "value2")
	_ = s.Set(ctx, "key3", "value3")

	// Batch get
	result, err := s.BatchGet(ctx, []string{"key1", "key2", "key4"})
	if err != nil {
		t.Fatalf("BatchGet failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
	if result["key1"] != "value1" {
		t.Errorf("expected 'value1', got '%s'", result["key1"])
	}
	if result["key2"] != "value2" {
		t.Errorf("expected 'value2', got '%s'", result["key2"])
	}
}

func TestMemoryStore_BatchSet(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Batch set
	items := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	err := s.BatchSet(ctx, items)
	if err != nil {
		t.Fatalf("BatchSet failed: %v", err)
	}

	// Verify
	for k, v := range items {
		got, err := s.Get(ctx, k)
		if err != nil {
			t.Errorf("Get failed for %s: %v", k, err)
		}
		if got != v {
			t.Errorf("expected '%s', got '%s'", v, got)
		}
	}
}

func TestMemoryStore_BatchDelete(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Set some keys
	_ = s.Set(ctx, "key1", "value1")
	_ = s.Set(ctx, "key2", "value2")
	_ = s.Set(ctx, "key3", "value3")

	// Batch delete
	err := s.BatchDelete(ctx, []string{"key1", "key2"})
	if err != nil {
		t.Fatalf("BatchDelete failed: %v", err)
	}

	// Verify deleted
	_, err = s.Get(ctx, "key1")
	if err != store.ErrNotFound {
		t.Error("expected key1 to be deleted")
	}
	_, err = s.Get(ctx, "key2")
	if err != store.ErrNotFound {
		t.Error("expected key2 to be deleted")
	}

	// key3 should still exist
	_, err = s.Get(ctx, "key3")
	if err != nil {
		t.Errorf("expected key3 to exist, got %v", err)
	}
}

// =============================================================================
// 原子操作测试
// =============================================================================

func TestMemoryStore_SetNX(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// First SetNX should succeed
	ok, err := s.SetNX(ctx, "key1", "value1")
	if err != nil {
		t.Fatalf("SetNX failed: %v", err)
	}
	if !ok {
		t.Error("expected first SetNX to succeed")
	}

	// Second SetNX should fail
	ok, err = s.SetNX(ctx, "key1", "value2")
	if err != nil {
		t.Fatalf("SetNX failed: %v", err)
	}
	if ok {
		t.Error("expected second SetNX to fail")
	}

	// Value should be original
	value, _ := s.Get(ctx, "key1")
	if value != "value1" {
		t.Errorf("expected 'value1', got '%s'", value)
	}
}

func TestMemoryStore_SetNXWithTTL(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// SetNX with TTL
	ok, err := s.SetNXWithTTL(ctx, "key1", "value1", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("SetNXWithTTL failed: %v", err)
	}
	if !ok {
		t.Error("expected SetNXWithTTL to succeed")
	}

	// Second SetNX should fail
	ok, _ = s.SetNX(ctx, "key1", "value2")
	if ok {
		t.Error("expected second SetNX to fail")
	}

	// Wait for TTL
	time.Sleep(150 * time.Millisecond)

	// Now SetNX should succeed
	ok, _ = s.SetNX(ctx, "key1", "value3")
	if !ok {
		t.Error("expected SetNX to succeed after TTL")
	}
}

// =============================================================================
// 关闭后操作测试
// =============================================================================

func TestMemoryStore_OperationsAfterClose(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()

	_ = s.Set(ctx, "key1", "value1")
	_ = s.Close()

	_, err := s.Get(ctx, "key1")
	if err != store.ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}

	err = s.Set(ctx, "key2", "value2")
	if err != store.ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}

	err = s.Delete(ctx, "key1")
	if err != store.ErrClosed {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

// =============================================================================
// MemorySetStore 测试
// =============================================================================

func TestMemorySetStore_Add(t *testing.T) {
	ctx := context.Background()
	s := NewMemorySetStore[string, string]()
	defer s.Close()

	// Add elements
	err := s.Add(ctx, "set1", "a")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	err = s.Add(ctx, "set1", "b")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	err = s.Add(ctx, "set1", "a") // duplicate
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Check size (should be 2, not 3)
	size, err := s.Size(ctx, "set1")
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}
	if size != 2 {
		t.Errorf("expected size 2, got %d", size)
	}
}

func TestMemorySetStore_Remove(t *testing.T) {
	ctx := context.Background()
	s := NewMemorySetStore[string, string]()
	defer s.Close()

	// Add and remove
	_ = s.Add(ctx, "set1", "a")
	_ = s.Add(ctx, "set1", "b")
	err := s.Remove(ctx, "set1", "a")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Check contains
	contains, _ := s.Contains(ctx, "set1", "a")
	if contains {
		t.Error("expected 'a' to be removed")
	}
	contains, _ = s.Contains(ctx, "set1", "b")
	if !contains {
		t.Error("expected 'b' to still exist")
	}
}

func TestMemorySetStore_Members(t *testing.T) {
	ctx := context.Background()
	s := NewMemorySetStore[string, string]()
	defer s.Close()

	// Add elements
	_ = s.Add(ctx, "set1", "a")
	_ = s.Add(ctx, "set1", "b")
	_ = s.Add(ctx, "set1", "c")

	// Get members
	members, err := s.Members(ctx, "set1")
	if err != nil {
		t.Fatalf("Members failed: %v", err)
	}
	if len(members) != 3 {
		t.Errorf("expected 3 members, got %d", len(members))
	}

	// Verify all elements present
	memberSet := make(map[string]bool)
	for _, m := range members {
		memberSet[m] = true
	}
	if !memberSet["a"] || !memberSet["b"] || !memberSet["c"] {
		t.Error("missing expected members")
	}
}

func TestMemorySetStore_EmptySet(t *testing.T) {
	ctx := context.Background()
	s := NewMemorySetStore[string, string]()
	defer s.Close()

	// Empty set should return empty slice
	members, err := s.Members(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Members failed: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expected empty slice, got %d elements", len(members))
	}

	size, err := s.Size(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Size failed: %v", err)
	}
	if size != 0 {
		t.Errorf("expected size 0, got %d", size)
	}
}

// =============================================================================
// 清理测试
// =============================================================================

func TestMemoryStore_CleanExpired(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Set with short TTL
	_ = s.SetWithTTL(ctx, "key1", "value1", 50*time.Millisecond)
	_ = s.SetWithTTL(ctx, "key2", "value2", 50*time.Millisecond)
	_ = s.Set(ctx, "key3", "value3") // no TTL

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Clean
	count := s.CleanExpired()
	if count != 2 {
		t.Errorf("expected 2 cleaned, got %d", count)
	}

	// Verify key3 still exists
	_, err := s.Get(ctx, "key3")
	if err != nil {
		t.Errorf("expected key3 to exist, got %v", err)
	}
}

func TestMemoryStore_Clear(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Set some keys
	_ = s.Set(ctx, "key1", "value1")
	_ = s.Set(ctx, "key2", "value2")

	// Clear
	err := s.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify all cleared
	keys, _ := s.Keys(ctx, "*")
	if len(keys) != 0 {
		t.Errorf("expected no keys, got %d", len(keys))
	}
}

// =============================================================================
// 接口验证
// =============================================================================

func TestMemoryStore_Interfaces(t *testing.T) {
	s := NewMemoryStore[string, string]()
	defer s.Close()

	// Verify interface implementation
	var _ store.Store[string, string] = s
	var _ store.TTLStore[string, string] = s
	var _ store.BatchStore[string, string] = s
	var _ store.AtomicStore[string, string] = s
}

func TestMemorySetStore_Interfaces(t *testing.T) {
	s := NewMemorySetStore[string, string]()
	defer s.Close()

	var _ store.SetStore[string, string] = s
}
