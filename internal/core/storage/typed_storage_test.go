package storage

import (
	"context"
	"testing"
	"time"
)

// TestTypedStorage_String 测试字符串类型存储
func TestTypedStorage_String(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	stringStorage, err := NewStringStorage(storage)
	if err != nil {
		t.Fatalf("NewStringStorage failed: %v", err)
	}

	// 测试 Set 和 Get
	key := "test:string"
	value := "hello world"

	err = stringStorage.Set(key, value, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, err := stringStorage.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved != value {
		t.Errorf("Expected %s, got %s", value, retrieved)
	}
}

// TestTypedStorage_Int64 测试 Int64 类型存储
func TestTypedStorage_Int64(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	int64Storage, err := NewInt64Storage(storage)
	if err != nil {
		t.Fatalf("NewInt64Storage failed: %v", err)
	}

	key := "test:int64"
	value := int64(123456789)

	err = int64Storage.Set(key, value, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, err := int64Storage.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved != value {
		t.Errorf("Expected %d, got %d", value, retrieved)
	}
}

// TestTypedStorage_Bool 测试布尔类型存储
func TestTypedStorage_Bool(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	boolStorage, err := NewBoolStorage(storage)
	if err != nil {
		t.Fatalf("NewBoolStorage failed: %v", err)
	}

	key := "test:bool"
	value := true

	err = boolStorage.Set(key, value, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, err := boolStorage.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved != value {
		t.Errorf("Expected %v, got %v", value, retrieved)
	}
}

// TestTypedStorage_List 测试列表操作
func TestTypedStorage_List(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	stringStorage, err := NewStringStorage(storage)
	if err != nil {
		t.Fatalf("NewStringStorage failed: %v", err)
	}

	key := "test:list"
	values := []string{"item1", "item2", "item3"}

	// 测试 SetList
	err = stringStorage.SetList(key, values, 0)
	if err != nil {
		t.Fatalf("SetList failed: %v", err)
	}

	// 测试 GetList
	retrieved, err := stringStorage.GetList(key)
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
	newItem := "item4"
	err = stringStorage.AppendToList(key, newItem)
	if err != nil {
		t.Fatalf("AppendToList failed: %v", err)
	}

	retrieved, _ = stringStorage.GetList(key)
	if len(retrieved) != 4 {
		t.Errorf("Expected list length 4 after append, got %d", len(retrieved))
	}
}

// TestTypedStorage_Hash 测试哈希操作
func TestTypedStorage_Hash(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	int64Storage, err := NewInt64Storage(storage)
	if err != nil {
		t.Fatalf("NewInt64Storage failed: %v", err)
	}

	key := "test:hash"
	field1 := "score1"
	value1 := int64(100)
	field2 := "score2"
	value2 := int64(200)

	// 测试 SetHash
	err = int64Storage.SetHash(key, field1, value1)
	if err != nil {
		t.Fatalf("SetHash failed: %v", err)
	}

	err = int64Storage.SetHash(key, field2, value2)
	if err != nil {
		t.Fatalf("SetHash failed: %v", err)
	}

	// 测试 GetHash
	retrieved, err := int64Storage.GetHash(key, field1)
	if err != nil {
		t.Fatalf("GetHash failed: %v", err)
	}

	if retrieved != value1 {
		t.Errorf("Expected %d, got %d", value1, retrieved)
	}

	// 测试 GetAllHash
	allValues, err := int64Storage.GetAllHash(key)
	if err != nil {
		t.Fatalf("GetAllHash failed: %v", err)
	}

	if len(allValues) != 2 {
		t.Errorf("Expected hash size 2, got %d", len(allValues))
	}

	if allValues[field1] != value1 {
		t.Errorf("Expected hash[%s]=%d, got %d", field1, value1, allValues[field1])
	}

	if allValues[field2] != value2 {
		t.Errorf("Expected hash[%s]=%d, got %d", field2, value2, allValues[field2])
	}
}

// TestTypedStorage_SetNX 测试原子设置
func TestTypedStorage_SetNX(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	stringStorage, err := NewStringStorage(storage)
	if err != nil {
		t.Fatalf("NewStringStorage failed: %v", err)
	}

	key := "test:setnx"
	value1 := "first"
	value2 := "second"

	// 第一次设置应该成功（不带 TTL，避免过期）
	ok, err := stringStorage.SetNX(key, value1, 0)
	if err != nil {
		t.Fatalf("SetNX failed: %v", err)
	}
	if !ok {
		t.Error("Expected SetNX to return true for new key")
	}

	// 立即验证值已设置
	retrieved, err := stringStorage.Get(key)
	if err != nil {
		t.Fatalf("Get after SetNX failed: %v", err)
	}
	if retrieved != value1 {
		t.Errorf("Expected value %s immediately after SetNX, got %s", value1, retrieved)
	}

	// 第二次设置应该失败（键已存在）
	ok, err = stringStorage.SetNX(key, value2, 0)
	if err != nil {
		t.Fatalf("Second SetNX failed: %v", err)
	}
	if ok {
		t.Error("Expected SetNX to return false for existing key")
	}

	// 验证值没有被覆盖
	retrieved, err = stringStorage.Get(key)
	if err != nil {
		t.Fatalf("Final Get failed: %v", err)
	}
	if retrieved != value1 {
		t.Errorf("Expected value %s, got %s", value1, retrieved)
	}
}

// TestTypedStorage_TTL 测试过期时间
func TestTypedStorage_TTL(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	stringStorage, err := NewStringStorage(storage)
	if err != nil {
		t.Fatalf("NewStringStorage failed: %v", err)
	}

	key := "test:ttl"
	value := "expires soon"
	ttl := 100 * time.Millisecond

	// 设置带 TTL 的值
	err = stringStorage.Set(key, value, ttl)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// 立即读取应该成功
	_, err = stringStorage.Get(key)
	if err != nil {
		t.Errorf("Get should succeed immediately: %v", err)
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 过期后读取应该失败
	_, err = stringStorage.Get(key)
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound after expiration, got %v", err)
	}
}

// TestTypedStorage_TypeSafety 测试类型安全
func TestTypedStorage_TypeSafety(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	key := "test:type-safety"

	// 使用底层存储存储一个 int64
	storage.Set(key, int64(123), 0)

	// 尝试用字符串存储读取应该失败
	stringStorage, err := NewStringStorage(storage)
	if err != nil {
		t.Fatalf("NewStringStorage failed: %v", err)
	}
	_, err = stringStorage.Get(key)
	if err == nil {
		t.Error("Expected type mismatch error")
	}
	if err != nil && err != ErrInvalidType {
		// 检查错误消息是否包含类型信息
		t.Logf("Got expected error: %v", err)
	}
}

// ============================================================================
// JSON 存储测试
// ============================================================================

// TestUser 测试用户结构
type TestUser struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	IsActive bool   `json:"is_active"`
}

// TestJSONStorage_Struct 测试 JSON 存储结构体
func TestJSONStorage_Struct(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	userStorage := NewTypedJSONStorage[TestUser](storage)

	key := "test:user:1"
	user := TestUser{
		ID:       1,
		Name:     "Alice",
		Email:    "alice@example.com",
		IsActive: true,
	}

	// 测试 Set
	err := userStorage.Set(key, user, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// 测试 Get
	retrieved, err := userStorage.Get(key)
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
	if retrieved.IsActive != user.IsActive {
		t.Errorf("Expected IsActive %v, got %v", user.IsActive, retrieved.IsActive)
	}
}

// TestJSONStorage_Pointer 测试 JSON 存储指针类型
func TestJSONStorage_Pointer(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	userStorage := NewTypedJSONStorage[*TestUser](storage)

	key := "test:user:2"
	user := &TestUser{
		ID:    2,
		Name:  "Bob",
		Email: "bob@example.com",
	}

	err := userStorage.Set(key, user, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, err := userStorage.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected non-nil pointer")
	}

	if retrieved.ID != user.ID {
		t.Errorf("Expected ID %d, got %d", user.ID, retrieved.ID)
	}
	if retrieved.Name != user.Name {
		t.Errorf("Expected Name %s, got %s", user.Name, retrieved.Name)
	}
}

// TestJSONStorage_Map 测试 JSON 存储 Map
func TestJSONStorage_Map(t *testing.T) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	mapStorage := NewTypedJSONStorage[map[string]string](storage)

	key := "test:config"
	config := map[string]string{
		"host":     "localhost",
		"port":     "8080",
		"protocol": "http",
	}

	err := mapStorage.Set(key, config, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	retrieved, err := mapStorage.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(retrieved) != len(config) {
		t.Errorf("Expected map size %d, got %d", len(config), len(retrieved))
	}

	for k, v := range config {
		if retrieved[k] != v {
			t.Errorf("Expected map[%s]=%s, got %s", k, v, retrieved[k])
		}
	}
}

// ============================================================================
// 基准测试
// ============================================================================

// BenchmarkTypedStorage_Set 基准测试类型安全存储的 Set 操作
func BenchmarkTypedStorage_Set(b *testing.B) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	stringStorage, err := NewStringStorage(storage)
	if err != nil {
		b.Fatalf("NewStringStorage failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "bench:key"
		stringStorage.Set(key, "value", 0)
	}
}

// BenchmarkTypedStorage_Get 基准测试类型安全存储的 Get 操作
func BenchmarkTypedStorage_Get(b *testing.B) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	stringStorage, err := NewStringStorage(storage)
	if err != nil {
		b.Fatalf("NewStringStorage failed: %v", err)
	}
	stringStorage.Set("bench:key", "value", 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stringStorage.Get("bench:key")
	}
}

// BenchmarkJSONStorage_Set 基准测试 JSON 存储的 Set 操作
func BenchmarkJSONStorage_Set(b *testing.B) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	userStorage := NewTypedJSONStorage[TestUser](storage)
	user := TestUser{
		ID:    1,
		Name:  "Test User",
		Email: "test@example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userStorage.Set("bench:user", user, 0)
	}
}

// BenchmarkJSONStorage_Get 基准测试 JSON 存储的 Get 操作
func BenchmarkJSONStorage_Get(b *testing.B) {
	storage := NewMemoryStorage(context.Background())
	defer storage.Close()

	userStorage := NewTypedJSONStorage[TestUser](storage)
	user := TestUser{
		ID:    1,
		Name:  "Test User",
		Email: "test@example.com",
	}
	userStorage.Set("bench:user", user, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		userStorage.Get("bench:user")
	}
}
