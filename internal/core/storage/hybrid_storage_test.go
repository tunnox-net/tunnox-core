package storage

import (
	"context"
	"testing"
	"time"
)

func TestHybridStorage_MemoryOnlyMode(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	
	// 纯内存模式：persistent = nil
	config := DefaultHybridConfig()
	config.EnablePersistent = false
	
	storage := NewHybridStorage(ctx, cache, nil, config)
	defer storage.Close()
	
	// 测试持久化数据（应该只写缓存）
	key := "tunnox:user:10000001"
	value := "test-user"
	
	if err := storage.Set(key, value, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	
	// 验证可以读取
	got, err := storage.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	
	if got != value {
		t.Errorf("Get returned %v, want %v", got, value)
	}
	
	// 验证数据分类正确
	if !storage.isPersistent(key) {
		t.Error("Expected key to be classified as persistent")
	}
}

func TestHybridStorage_DataCategoryRecognition(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	config := DefaultHybridConfig()
	
	storage := NewHybridStorage(ctx, cache, nil, config)
	defer storage.Close()
	
	tests := []struct {
		key        string
		wantPersistent bool
	}{
		{"tunnox:user:123", true},
		{"tunnox:client:456", true},
		{"tunnox:mapping:789", true},
		{"tunnox:node:abc", true},
		{"tunnox:stats:xyz", true},
		{"tunnox:runtime:key:123", false},
		{"tunnox:session:abc", false},
		{"tunnox:jwt:123", false},
		{"tunnox:route:client:456", false},
		{"tunnox:temp:xyz", false},
		{"other:key", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := storage.isPersistent(tt.key)
			if got != tt.wantPersistent {
				t.Errorf("isPersistent(%q) = %v, want %v", tt.key, got, tt.wantPersistent)
			}
		})
	}
}

func TestHybridStorage_RuntimeData(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	config := DefaultHybridConfig()
	
	storage := NewHybridStorage(ctx, cache, nil, config)
	defer storage.Close()
	
	// 测试运行时数据
	key := "tunnox:runtime:key:mapping123"
	value := "encryption-key-abc"
	ttl := 1 * time.Hour
	
	if err := storage.Set(key, value, ttl); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	
	// 验证可以读取
	got, err := storage.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	
	if got != value {
		t.Errorf("Get returned %v, want %v", got, value)
	}
	
	// 验证数据分类
	if storage.isPersistent(key) {
		t.Error("Expected key to be classified as runtime")
	}
}

func TestHybridStorage_ExplicitMethods(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	config := DefaultHybridConfig()
	
	storage := NewHybridStorage(ctx, cache, nil, config)
	defer storage.Close()
	
	// 测试显式持久化方法
	persistentKey := "test:persistent"
	persistentValue := "persistent-data"
	
	if err := storage.SetPersistent(persistentKey, persistentValue); err != nil {
		t.Fatalf("SetPersistent failed: %v", err)
	}
	
	got, err := storage.Get(persistentKey)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	
	if got != persistentValue {
		t.Errorf("Get returned %v, want %v", got, persistentValue)
	}
	
	// 测试显式运行时方法
	runtimeKey := "test:runtime"
	runtimeValue := "runtime-data"
	
	if err := storage.SetRuntime(runtimeKey, runtimeValue, 1*time.Hour); err != nil {
		t.Fatalf("SetRuntime failed: %v", err)
	}
	
	got, err = storage.Get(runtimeKey)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	
	if got != runtimeValue {
		t.Errorf("Get returned %v, want %v", got, runtimeValue)
	}
}

func TestHybridStorage_Delete(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	config := DefaultHybridConfig()
	
	storage := NewHybridStorage(ctx, cache, nil, config)
	defer storage.Close()
	
	// 设置数据
	key := "tunnox:user:123"
	value := "test-user"
	
	if err := storage.Set(key, value, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	
	// 删除数据
	if err := storage.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	
	// 验证已删除
	_, err := storage.Get(key)
	if err != ErrKeyNotFound {
		t.Errorf("Expected ErrKeyNotFound, got %v", err)
	}
}

func TestHybridStorage_Exists(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	config := DefaultHybridConfig()
	
	storage := NewHybridStorage(ctx, cache, nil, config)
	defer storage.Close()
	
	key := "tunnox:client:456"
	value := "test-client"
	
	// 键不存在
	exists, err := storage.Exists(key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Expected key to not exist")
	}
	
	// 设置键
	if err := storage.Set(key, value, 0); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	
	// 键存在
	exists, err = storage.Exists(key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Expected key to exist")
	}
}

func TestHybridStorage_ConfigUpdate(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	config := DefaultHybridConfig()
	
	storage := NewHybridStorage(ctx, cache, nil, config)
	defer storage.Close()
	
	// 更新持久化前缀
	newPrefixes := []string{"custom:prefix:"}
	storage.UpdatePersistentPrefixes(newPrefixes)
	
	// 验证配置已更新
	updatedConfig := storage.GetConfig()
	if len(updatedConfig.PersistentPrefixes) != 1 {
		t.Errorf("Expected 1 prefix, got %d", len(updatedConfig.PersistentPrefixes))
	}
	if updatedConfig.PersistentPrefixes[0] != "custom:prefix:" {
		t.Errorf("Expected prefix 'custom:prefix:', got %s", updatedConfig.PersistentPrefixes[0])
	}
	
	// 验证新前缀生效
	if !storage.isPersistent("custom:prefix:test") {
		t.Error("Expected key with new prefix to be persistent")
	}
}

func TestHybridStorage_IsPersistentEnabled(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	
	// 纯内存模式
	config1 := DefaultHybridConfig()
	config1.EnablePersistent = false
	storage1 := NewHybridStorage(ctx, cache, nil, config1)
	defer storage1.Close()
	
	if storage1.IsPersistentEnabled() {
		t.Error("Expected persistent to be disabled")
	}
	
	// 持久化模式（使用 NullPersistentStorage）
	config2 := DefaultHybridConfig()
	config2.EnablePersistent = true
	storage2 := NewHybridStorage(ctx, cache, NewNullPersistentStorage(), config2)
	defer storage2.Close()
	
	// NullPersistentStorage 不是 nil，所以持久化应该启用
	if !storage2.IsPersistentEnabled() {
		t.Error("Expected persistent to be enabled with NullPersistentStorage")
	}
}

func TestHybridStorage_ListOperations(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	config := DefaultHybridConfig()
	
	storage := NewHybridStorage(ctx, cache, nil, config)
	defer storage.Close()
	
	key := "test:list"
	values := []interface{}{"item1", "item2", "item3"}
	
	// 设置列表
	if err := storage.SetList(key, values, 1*time.Hour); err != nil {
		t.Fatalf("SetList failed: %v", err)
	}
	
	// 获取列表
	got, err := storage.GetList(key)
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}
	
	if len(got) != len(values) {
		t.Errorf("GetList returned %d items, want %d", len(got), len(values))
	}
	
	// 追加到列表
	if err := storage.AppendToList(key, "item4"); err != nil {
		t.Fatalf("AppendToList failed: %v", err)
	}
	
	got, err = storage.GetList(key)
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}
	
	if len(got) != 4 {
		t.Errorf("After append, list has %d items, want 4", len(got))
	}
	
	// 从列表移除
	if err := storage.RemoveFromList(key, "item2"); err != nil {
		t.Fatalf("RemoveFromList failed: %v", err)
	}
	
	got, err = storage.GetList(key)
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}
	
	if len(got) != 3 {
		t.Errorf("After remove, list has %d items, want 3", len(got))
	}
}

func TestHybridStorage_CounterOperations(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryStorage(ctx)
	config := DefaultHybridConfig()
	
	storage := NewHybridStorage(ctx, cache, nil, config)
	defer storage.Close()
	
	key := "test:counter"
	
	// Incr
	count, err := storage.Incr(key)
	if err != nil {
		t.Fatalf("Incr failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Incr returned %d, want 1", count)
	}
	
	// IncrBy
	count, err = storage.IncrBy(key, 5)
	if err != nil {
		t.Fatalf("IncrBy failed: %v", err)
	}
	if count != 6 {
		t.Errorf("IncrBy returned %d, want 6", count)
	}
}

