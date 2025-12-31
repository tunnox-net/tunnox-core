package storage

import (
	"context"
	"testing"
	"time"
)

// TestTypedFullStorageAdapter_BasicOps 测试基础 KV 操作
func TestTypedFullStorageAdapter_BasicOps(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	type User struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	fs, ok := AsFullStorage(storage)
	if !ok {
		t.Fatal("Expected storage to implement FullStorage")
	}

	adapter := NewTypedFullStorageAdapter[User](fs)

	// 测试 Set 和 Get
	key := "test:user:1"
	user := User{
		ID:    1,
		Name:  "Alice",
		Email: "alice@example.com",
	}

	err := adapter.Set(key, user, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, err := adapter.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.ID != user.ID {
		t.Errorf("Expected ID %d, got %d", user.ID, retrieved.ID)
	}
	if retrieved.Name != user.Name {
		t.Errorf("Expected Name %s, got %s", user.Name, retrieved.Name)
	}
	if retrieved.Email != user.Email {
		t.Errorf("Expected Email %s, got %s", user.Email, retrieved.Email)
	}

	// 测试 Exists
	exists, err := adapter.Exists(key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected key to exist")
	}

	// 测试 Delete
	err = adapter.Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	exists, _ = adapter.Exists(key)
	if exists {
		t.Error("Expected key to not exist after delete")
	}
}

// TestTypedFullStorageAdapter_ListOps 测试列表操作
func TestTypedFullStorageAdapter_ListOps(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	fs, ok := AsFullStorage(storage)
	if !ok {
		t.Fatal("Expected storage to implement FullStorage")
	}

	adapter := NewTypedFullStorageAdapter[string](fs)

	key := "test:list"
	values := []string{"item1", "item2", "item3"}

	// 测试 SetList
	err := adapter.SetList(key, values, 0)
	if err != nil {
		t.Fatalf("SetList failed: %v", err)
	}

	// 测试 GetList
	retrieved, err := adapter.GetList(key)
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}

	if len(retrieved) != len(values) {
		t.Errorf("Expected list length %d, got %d", len(values), len(retrieved))
	}

	for i, v := range retrieved {
		if v != values[i] {
			t.Errorf("Expected list[%d]=%s, got %s", i, values[i], v)
		}
	}

	// 测试 AppendToList
	err = adapter.AppendToList(key, "item4")
	if err != nil {
		t.Fatalf("AppendToList failed: %v", err)
	}

	retrieved, _ = adapter.GetList(key)
	if len(retrieved) != 4 {
		t.Errorf("Expected list length 4 after append, got %d", len(retrieved))
	}
	if retrieved[3] != "item4" {
		t.Errorf("Expected last item to be 'item4', got %s", retrieved[3])
	}
}

// TestTypedFullStorageAdapter_HashOps 测试哈希操作
func TestTypedFullStorageAdapter_HashOps(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	type Config struct {
		Value string `json:"value"`
		Count int    `json:"count"`
	}

	fs, ok := AsFullStorage(storage)
	if !ok {
		t.Fatal("Expected storage to implement FullStorage")
	}

	adapter := NewTypedFullStorageAdapter[Config](fs)

	key := "test:hash"
	field1 := "config1"
	config1 := Config{Value: "value1", Count: 10}
	field2 := "config2"
	config2 := Config{Value: "value2", Count: 20}

	// 测试 SetHash
	err := adapter.SetHash(key, field1, config1)
	if err != nil {
		t.Fatalf("SetHash failed: %v", err)
	}

	err = adapter.SetHash(key, field2, config2)
	if err != nil {
		t.Fatalf("SetHash failed: %v", err)
	}

	// 测试 GetHash
	retrieved, err := adapter.GetHash(key, field1)
	if err != nil {
		t.Fatalf("GetHash failed: %v", err)
	}

	if retrieved.Value != config1.Value || retrieved.Count != config1.Count {
		t.Errorf("Expected %+v, got %+v", config1, retrieved)
	}

	// 测试 GetAllHash
	allValues, err := adapter.GetAllHash(key)
	if err != nil {
		t.Fatalf("GetAllHash failed: %v", err)
	}

	if len(allValues) != 2 {
		t.Errorf("Expected hash size 2, got %d", len(allValues))
	}

	// 测试 DeleteHash
	err = adapter.DeleteHash(key, field1)
	if err != nil {
		t.Fatalf("DeleteHash failed: %v", err)
	}

	allValues, _ = adapter.GetAllHash(key)
	if len(allValues) != 1 {
		t.Errorf("Expected hash size 1 after delete, got %d", len(allValues))
	}
}

// TestTypedFullStorageAdapter_CASops 测试原子操作
func TestTypedFullStorageAdapter_CASops(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	fs, ok := AsFullStorage(storage)
	if !ok {
		t.Fatal("Expected storage to implement FullStorage")
	}

	adapter := NewTypedFullStorageAdapter[string](fs)

	key := "test:cas"
	value1 := "first"
	value2 := "second"

	// 测试 SetNX - 第一次应该成功
	ok, err := adapter.SetNX(key, value1, 0)
	if err != nil {
		t.Fatalf("SetNX failed: %v", err)
	}
	if !ok {
		t.Error("Expected SetNX to return true for new key")
	}

	// 测试 SetNX - 第二次应该失败
	ok, err = adapter.SetNX(key, value2, 0)
	if err != nil {
		t.Fatalf("Second SetNX failed: %v", err)
	}
	if ok {
		t.Error("Expected SetNX to return false for existing key")
	}

	// 验证值没有被覆盖
	retrieved, _ := adapter.Get(key)
	if retrieved != value1 {
		t.Errorf("Expected value %s, got %s", value1, retrieved)
	}
}

// TestTypedFullStorageAdapter_TTL 测试过期时间
func TestTypedFullStorageAdapter_TTL(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	fs, ok := AsFullStorage(storage)
	if !ok {
		t.Fatal("Expected storage to implement FullStorage")
	}

	adapter := NewTypedFullStorageAdapter[string](fs)

	key := "test:ttl"
	value := "expires soon"
	ttl := 100 * time.Millisecond

	// 设置带 TTL 的值
	err := adapter.Set(key, value, ttl)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// 立即读取应该成功
	_, err = adapter.Get(key)
	if err != nil {
		t.Errorf("Get should succeed immediately: %v", err)
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 过期后读取应该失败
	_, err = adapter.Get(key)
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound after expiration, got %v", err)
	}
}

// TestTypedFullStorageAdapter_Counter 测试计数器操作
func TestTypedFullStorageAdapter_Counter(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	fs, ok := AsFullStorage(storage)
	if !ok {
		t.Fatal("Expected storage to implement FullStorage")
	}

	adapter := NewTypedFullStorageAdapter[int64](fs)

	key := "test:counter"

	// 测试 Incr
	val, err := adapter.Incr(key)
	if err != nil {
		t.Fatalf("Incr failed: %v", err)
	}
	if val != 1 {
		t.Errorf("Expected 1, got %d", val)
	}

	// 再次 Incr
	val, _ = adapter.Incr(key)
	if val != 2 {
		t.Errorf("Expected 2, got %d", val)
	}

	// 测试 IncrBy
	val, err = adapter.IncrBy(key, 10)
	if err != nil {
		t.Fatalf("IncrBy failed: %v", err)
	}
	if val != 12 {
		t.Errorf("Expected 12, got %d", val)
	}
}

// TestTypedStorageAdapter_Basic 测试基础适配器
func TestTypedStorageAdapter_Basic(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	type Item struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	adapter := NewTypedStorageAdapter[Item](storage)

	key := "test:item"
	item := Item{ID: 1, Name: "Test"}

	err := adapter.Set(key, item, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, err := adapter.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.ID != item.ID || retrieved.Name != item.Name {
		t.Errorf("Expected %+v, got %+v", item, retrieved)
	}
}

// TestTypedCacheAdapter_Basic 测试缓存适配器
func TestTypedCacheAdapter_Basic(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	type Session struct {
		UserID string `json:"user_id"`
		Token  string `json:"token"`
	}

	adapter := NewTypedCacheAdapter[Session](storage)

	key := "session:123"
	session := Session{UserID: "user1", Token: "token123"}

	err := adapter.Set(key, session, time.Hour)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, err := adapter.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.UserID != session.UserID || retrieved.Token != session.Token {
		t.Errorf("Expected %+v, got %+v", session, retrieved)
	}
}

// BenchmarkTypedFullStorageAdapter_Set 基准测试 Set 操作
func BenchmarkTypedFullStorageAdapter_Set(b *testing.B) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	type User struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	fs, _ := AsFullStorage(storage)
	adapter := NewTypedFullStorageAdapter[User](fs)
	user := User{ID: 1, Name: "Test", Email: "test@example.com"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adapter.Set("bench:user", user, 0)
	}
}

// BenchmarkTypedFullStorageAdapter_Get 基准测试 Get 操作
func BenchmarkTypedFullStorageAdapter_Get(b *testing.B) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	type User struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	fs, _ := AsFullStorage(storage)
	adapter := NewTypedFullStorageAdapter[User](fs)
	user := User{ID: 1, Name: "Test", Email: "test@example.com"}
	adapter.Set("bench:user", user, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adapter.Get("bench:user")
	}
}
