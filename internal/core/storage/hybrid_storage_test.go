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
		key            string
		wantPersistent bool
	}{
		{"tunnox:user:123", true},
		{"tunnox:client:456", true},
		{"tunnox:mapping:789", true},
		{"tunnox:node:abc", false}, // 节点信息现在是共享数据，不是持久化数据
		{"tunnox:stats:persistent:xyz", true},
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

// TestHybridStorage_SharedPersistentList_CacheMiss 测试 SharedPersistent 类型的列表数据
// 在缓存 miss 后能否从持久化存储正确恢复
// 这个测试复现了 mapping 索引丢失的 bug
func TestHybridStorage_SharedPersistentList_CacheMiss(t *testing.T) {
	ctx := context.Background()

	// 使用临时文件作为持久化存储
	tempFile := "/tmp/test_hybrid_storage_list.json"
	persistent, err := NewJSONStorage(&JSONStorageConfig{
		FilePath:     tempFile,
		AutoSave:     true,
		SaveInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewJSONStorage failed: %v", err)
	}
	defer persistent.Close()

	// 创建本地缓存
	cache := NewMemoryStorage(ctx)

	// 使用默认配置（包含 SharedPersistentPrefixes）
	config := DefaultHybridConfig()
	config.EnablePersistent = true

	storage := NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)
	defer storage.Close()

	// 使用 SharedPersistent 前缀的 key（tunnox:client_mappings:）
	key := "tunnox:client_mappings:12345678"

	// 验证 key 被正确识别为 SharedPersistent
	if !storage.isSharedPersistent(key) {
		t.Fatalf("Expected key %s to be SharedPersistent", key)
	}

	// 第一步：追加数据到列表
	item1 := `{"id":"mapping1","target_client_id":12345678}`
	item2 := `{"id":"mapping2","target_client_id":12345678}`

	if err := storage.AppendToList(key, item1); err != nil {
		t.Fatalf("AppendToList item1 failed: %v", err)
	}
	if err := storage.AppendToList(key, item2); err != nil {
		t.Fatalf("AppendToList item2 failed: %v", err)
	}

	// 验证写入后立即读取正常
	list, err := storage.GetList(key)
	if err != nil {
		t.Fatalf("GetList after write failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("Expected 2 items, got %d", len(list))
	}
	t.Logf("After write: list has %d items", len(list))

	// 第二步：清空本地缓存（模拟缓存过期或服务重启）
	if err := cache.Delete(key); err != nil && err != ErrKeyNotFound {
		t.Fatalf("cache.Delete failed: %v", err)
	}

	// 验证缓存已清空
	_, err = cache.Get(key)
	if err != ErrKeyNotFound {
		t.Fatalf("Expected cache to be empty, got err: %v", err)
	}
	t.Log("Cache cleared successfully")

	// 第三步：重新读取（应该从持久化存储恢复）
	list, err = storage.GetList(key)
	if err != nil {
		t.Fatalf("GetList after cache miss failed: %v", err)
	}

	// 这是关键断言：缓存 miss 后应该能从持久化存储恢复数据
	if len(list) != 2 {
		t.Errorf("After cache miss: expected 2 items, got %d", len(list))
		t.Logf("Persistent storage data check...")

		// 调试：直接从持久化存储读取
		persistentValue, persistentErr := persistent.Get(key)
		if persistentErr != nil {
			t.Logf("Persistent Get error: %v", persistentErr)
		} else {
			t.Logf("Persistent value type: %T", persistentValue)
			t.Logf("Persistent value: %v", persistentValue)
		}
	} else {
		t.Logf("After cache miss: list has %d items (correct!)", len(list))
	}
}

// TestHybridStorage_SharedPersistentList_SimulateRestart 模拟服务重启场景
// 创建新的 HybridStorage 实例，验证能否读取之前持久化的数据
func TestHybridStorage_SharedPersistentList_SimulateRestart(t *testing.T) {
	ctx := context.Background()

	// 使用临时文件作为持久化存储
	tempFile := "/tmp/test_hybrid_storage_restart.json"

	// 第一阶段：写入数据
	{
		persistent, err := NewJSONStorage(&JSONStorageConfig{
			FilePath:     tempFile,
			AutoSave:     true,
			SaveInterval: 100 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("NewJSONStorage failed: %v", err)
		}

		cache := NewMemoryStorage(ctx)
		config := DefaultHybridConfig()
		config.EnablePersistent = true

		storage := NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)

		key := "tunnox:client_mappings:99999999"
		item := `{"id":"pmap_test","target_client_id":99999999}`

		if err := storage.AppendToList(key, item); err != nil {
			t.Fatalf("AppendToList failed: %v", err)
		}

		// 验证写入成功
		list, err := storage.GetList(key)
		if err != nil {
			t.Fatalf("GetList failed: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("Expected 1 item, got %d", len(list))
		}
		t.Logf("Phase 1: wrote %d items", len(list))

		// 关闭存储（模拟服务停止）
		storage.Close()
		persistent.Close()
	}

	// 第二阶段：重新打开（模拟服务重启），验证数据恢复
	{
		persistent, err := NewJSONStorage(&JSONStorageConfig{
			FilePath:     tempFile,
			AutoSave:     true,
			SaveInterval: 100 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("NewJSONStorage (restart) failed: %v", err)
		}
		defer persistent.Close()

		// 新的缓存实例（空的）
		cache := NewMemoryStorage(ctx)
		config := DefaultHybridConfig()
		config.EnablePersistent = true

		storage := NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)
		defer storage.Close()

		key := "tunnox:client_mappings:99999999"

		// 从新实例读取（缓存为空，应该从持久化存储恢复）
		list, err := storage.GetList(key)
		if err != nil {
			t.Fatalf("GetList after restart failed: %v", err)
		}

		if len(list) != 1 {
			t.Errorf("After restart: expected 1 item, got %d", len(list))

			// 调试信息
			persistentValue, persistentErr := persistent.Get(key)
			if persistentErr != nil {
				t.Logf("Persistent Get error: %v", persistentErr)
			} else {
				t.Logf("Persistent value type: %T", persistentValue)
				t.Logf("Persistent value: %v", persistentValue)
			}
		} else {
			t.Logf("After restart: list has %d items (correct!)", len(list))
		}
	}
}
