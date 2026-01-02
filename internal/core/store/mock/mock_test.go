package mock

import (
	"context"
	"errors"
	"testing"
	"time"

	"tunnox-core/internal/core/store"
)

// =============================================================================
// MockStore 基础测试
// =============================================================================

func TestMockStore_SetGet(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

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

func TestMockStore_GetNotFound(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_, err := s.Get(ctx, "nonexistent")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestMockStore_Delete(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.Set(ctx, "key1", "value1")
	err := s.Delete(ctx, "key1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = s.Get(ctx, "key1")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMockStore_Exists(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	// Not exists
	exists, err := s.Exists(ctx, "key1")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("expected key1 not to exist")
	}

	// Set and check
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
// 预设错误测试
// =============================================================================

func TestMockStore_SetError(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	expectedErr := errors.New("test error")
	s.SetError("Get", expectedErr)

	_, err := s.Get(ctx, "key1")
	if err != expectedErr {
		t.Errorf("expected custom error, got %v", err)
	}

	// Clear error
	s.ClearError("Get")
	_ = s.Set(ctx, "key1", "value1")
	_, err = s.Get(ctx, "key1")
	if err != nil {
		t.Errorf("expected no error after clear, got %v", err)
	}
}

func TestMockStore_SetErrorForMultipleMethods(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	getErr := errors.New("get error")
	setErr := errors.New("set error")
	s.SetError("Get", getErr)
	s.SetError("Set", setErr)

	_, err := s.Get(ctx, "key1")
	if err != getErr {
		t.Errorf("expected get error, got %v", err)
	}

	err = s.Set(ctx, "key1", "value1")
	if err != setErr {
		t.Errorf("expected set error, got %v", err)
	}

	// Clear all
	s.ClearAllErrors()
	err = s.Set(ctx, "key1", "value1")
	if err != nil {
		t.Errorf("expected no error after clear all, got %v", err)
	}
}

// =============================================================================
// 调用记录测试
// =============================================================================

func TestMockStore_CallRecording(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	// Make some calls
	_ = s.Set(ctx, "key1", "value1")
	_, _ = s.Get(ctx, "key1")
	_, _ = s.Get(ctx, "key2")
	_ = s.Delete(ctx, "key1")

	// Check calls
	calls := s.GetCalls()
	if len(calls) != 4 {
		t.Errorf("expected 4 calls, got %d", len(calls))
	}

	getCalls := s.GetCallsForMethod("Get")
	if len(getCalls) != 2 {
		t.Errorf("expected 2 Get calls, got %d", len(getCalls))
	}

	if s.CallCount("Set") != 1 {
		t.Errorf("expected 1 Set call, got %d", s.CallCount("Set"))
	}
}

func TestMockStore_ClearCalls(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.Set(ctx, "key1", "value1")
	s.ClearCalls()

	if len(s.GetCalls()) != 0 {
		t.Error("expected no calls after clear")
	}
}

func TestMockStore_DisableCallRecording(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	s.SetRecordCalls(false)
	_ = s.Set(ctx, "key1", "value1")
	_, _ = s.Get(ctx, "key1")

	if len(s.GetCalls()) != 0 {
		t.Error("expected no calls when recording disabled")
	}
}

// =============================================================================
// TTL 测试
// =============================================================================

func TestMockStore_SetWithTTL(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

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

	_, err = s.Get(ctx, "key1")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound after TTL, got %v", err)
	}
}

func TestMockStore_GetTTL(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.SetWithTTL(ctx, "key1", "value1", 1*time.Second)

	ttl, err := s.GetTTL(ctx, "key1")
	if err != nil {
		t.Fatalf("GetTTL failed: %v", err)
	}
	if ttl <= 0 || ttl > 1*time.Second {
		t.Errorf("unexpected TTL: %v", ttl)
	}
}

func TestMockStore_Refresh(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.SetWithTTL(ctx, "key1", "value1", 100*time.Millisecond)
	err := s.Refresh(ctx, "key1", 1*time.Second)
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	// Should still exist after refresh
	_, err = s.Get(ctx, "key1")
	if err != nil {
		t.Errorf("expected key to exist after refresh, got %v", err)
	}
}

// =============================================================================
// 批量操作测试
// =============================================================================

func TestMockStore_BatchGet(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.Set(ctx, "key1", "value1")
	_ = s.Set(ctx, "key2", "value2")

	result, err := s.BatchGet(ctx, []string{"key1", "key2", "key3"})
	if err != nil {
		t.Fatalf("BatchGet failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestMockStore_BatchSet(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	items := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	err := s.BatchSet(ctx, items)
	if err != nil {
		t.Fatalf("BatchSet failed: %v", err)
	}

	for k, v := range items {
		got, _ := s.Get(ctx, k)
		if got != v {
			t.Errorf("expected '%s', got '%s'", v, got)
		}
	}
}

func TestMockStore_BatchDelete(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.Set(ctx, "key1", "value1")
	_ = s.Set(ctx, "key2", "value2")

	err := s.BatchDelete(ctx, []string{"key1"})
	if err != nil {
		t.Fatalf("BatchDelete failed: %v", err)
	}

	_, err = s.Get(ctx, "key1")
	if err != store.ErrNotFound {
		t.Error("expected key1 to be deleted")
	}

	_, err = s.Get(ctx, "key2")
	if err != nil {
		t.Error("expected key2 to still exist")
	}
}

// =============================================================================
// 原子操作测试
// =============================================================================

func TestMockStore_SetNX(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	// First should succeed
	ok, err := s.SetNX(ctx, "key1", "value1")
	if err != nil {
		t.Fatalf("SetNX failed: %v", err)
	}
	if !ok {
		t.Error("expected first SetNX to succeed")
	}

	// Second should fail
	ok, err = s.SetNX(ctx, "key1", "value2")
	if err != nil {
		t.Fatalf("SetNX failed: %v", err)
	}
	if ok {
		t.Error("expected second SetNX to fail")
	}
}

func TestMockStore_SetNXWithTTL(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	ok, err := s.SetNXWithTTL(ctx, "key1", "value1", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("SetNXWithTTL failed: %v", err)
	}
	if !ok {
		t.Error("expected SetNXWithTTL to succeed")
	}

	// Wait for TTL
	time.Sleep(150 * time.Millisecond)

	// Should succeed after TTL
	ok, _ = s.SetNX(ctx, "key1", "value2")
	if !ok {
		t.Error("expected SetNX to succeed after TTL")
	}
}

// =============================================================================
// Reset 测试
// =============================================================================

func TestMockStore_Reset(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.Set(ctx, "key1", "value1")
	s.SetError("Get", errors.New("error"))

	s.Reset()

	_, err := s.Get(ctx, "key1")
	if err != store.ErrNotFound {
		t.Errorf("expected ErrNotFound after reset, got %v", err)
	}

	if len(s.GetCalls()) != 1 { // only the Get call after reset
		t.Errorf("expected 1 call after reset, got %d", len(s.GetCalls()))
	}
}

// =============================================================================
// 数据访问测试
// =============================================================================

func TestMockStore_GetSetData(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	// Set data directly
	s.SetData(map[string]string{
		"key1": "value1",
		"key2": "value2",
	})

	// Get via normal method
	value, err := s.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if value != "value1" {
		t.Errorf("expected 'value1', got '%s'", value)
	}

	// Get data directly
	data := s.GetData()
	if len(data) != 2 {
		t.Errorf("expected 2 items, got %d", len(data))
	}
}

// =============================================================================
// 断言方法测试
// =============================================================================

func TestMockStore_AssertCalled(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.Set(ctx, "key1", "value1")

	err := s.AssertCalled("Set")
	if err != nil {
		t.Errorf("AssertCalled failed: %v", err)
	}

	err = s.AssertCalled("Get")
	if err == nil {
		t.Error("expected AssertCalled to fail for Get")
	}
}

func TestMockStore_AssertNotCalled(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.Set(ctx, "key1", "value1")

	err := s.AssertNotCalled("Get")
	if err != nil {
		t.Errorf("AssertNotCalled failed: %v", err)
	}

	err = s.AssertNotCalled("Set")
	if err == nil {
		t.Error("expected AssertNotCalled to fail for Set")
	}
}

func TestMockStore_AssertCallCount(t *testing.T) {
	ctx := context.Background()
	s := NewMockStore[string, string]()

	_ = s.Set(ctx, "key1", "value1")
	_ = s.Set(ctx, "key2", "value2")

	err := s.AssertCallCount("Set", 2)
	if err != nil {
		t.Errorf("AssertCallCount failed: %v", err)
	}

	err = s.AssertCallCount("Set", 1)
	if err == nil {
		t.Error("expected AssertCallCount to fail")
	}
}

// =============================================================================
// MockSetStore 测试
// =============================================================================

func TestMockSetStore_Add(t *testing.T) {
	ctx := context.Background()
	s := NewMockSetStore[string, string]()

	err := s.Add(ctx, "set1", "a")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	err = s.Add(ctx, "set1", "b")
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	size, _ := s.Size(ctx, "set1")
	if size != 2 {
		t.Errorf("expected size 2, got %d", size)
	}
}

func TestMockSetStore_Remove(t *testing.T) {
	ctx := context.Background()
	s := NewMockSetStore[string, string]()

	_ = s.Add(ctx, "set1", "a")
	_ = s.Add(ctx, "set1", "b")
	err := s.Remove(ctx, "set1", "a")
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	contains, _ := s.Contains(ctx, "set1", "a")
	if contains {
		t.Error("expected 'a' to be removed")
	}
}

func TestMockSetStore_Contains(t *testing.T) {
	ctx := context.Background()
	s := NewMockSetStore[string, string]()

	_ = s.Add(ctx, "set1", "a")

	contains, err := s.Contains(ctx, "set1", "a")
	if err != nil {
		t.Fatalf("Contains failed: %v", err)
	}
	if !contains {
		t.Error("expected 'a' to be in set")
	}

	contains, _ = s.Contains(ctx, "set1", "b")
	if contains {
		t.Error("expected 'b' not to be in set")
	}
}

func TestMockSetStore_Members(t *testing.T) {
	ctx := context.Background()
	s := NewMockSetStore[string, string]()

	_ = s.Add(ctx, "set1", "a")
	_ = s.Add(ctx, "set1", "b")
	_ = s.Add(ctx, "set1", "c")

	members, err := s.Members(ctx, "set1")
	if err != nil {
		t.Fatalf("Members failed: %v", err)
	}
	if len(members) != 3 {
		t.Errorf("expected 3 members, got %d", len(members))
	}
}

func TestMockSetStore_SetError(t *testing.T) {
	ctx := context.Background()
	s := NewMockSetStore[string, string]()

	expectedErr := errors.New("test error")
	s.SetError("Add", expectedErr)

	err := s.Add(ctx, "set1", "a")
	if err != expectedErr {
		t.Errorf("expected custom error, got %v", err)
	}
}

func TestMockSetStore_Pipeline(t *testing.T) {
	ctx := context.Background()
	s := NewMockSetStore[string, string]()

	pipeline := s.Pipeline()
	pipeline.SAdd(ctx, "set1", "a")
	pipeline.SAdd(ctx, "set1", "b")
	pipeline.SRem(ctx, "set1", "a")

	err := pipeline.Exec(ctx)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	// Should only have "b"
	contains, _ := s.Contains(ctx, "set1", "a")
	if contains {
		t.Error("expected 'a' to be removed")
	}
	contains, _ = s.Contains(ctx, "set1", "b")
	if !contains {
		t.Error("expected 'b' to be in set")
	}
}

func TestMockSetStore_SetData(t *testing.T) {
	ctx := context.Background()
	s := NewMockSetStore[string, string]()

	s.SetData(map[string][]string{
		"set1": {"a", "b", "c"},
		"set2": {"x", "y"},
	})

	size, _ := s.Size(ctx, "set1")
	if size != 3 {
		t.Errorf("expected size 3, got %d", size)
	}

	size, _ = s.Size(ctx, "set2")
	if size != 2 {
		t.Errorf("expected size 2, got %d", size)
	}
}

// =============================================================================
// 接口验证
// =============================================================================

func TestMockStore_Interfaces(t *testing.T) {
	s := NewMockStore[string, string]()

	var _ store.Store[string, string] = s
	var _ store.TTLStore[string, string] = s
	var _ store.BatchStore[string, string] = s
	var _ store.AtomicStore[string, string] = s
}

func TestMockSetStore_Interfaces(t *testing.T) {
	s := NewMockSetStore[string, string]()

	var _ store.SetStore[string, string] = s
	var _ store.PipelineSetStore[string, string] = s
}
