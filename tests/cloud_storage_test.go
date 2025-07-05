package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"tunnox-core/internal/cloud"
)

func TestMemoryStorage_BasicOperations(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	ctx := context.Background()

	// 测试 Set 和 Get
	t.Run("Set and Get", func(t *testing.T) {
		err := storage.Set(ctx, "test_key", "test_value", 1*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		value, err := storage.Get(ctx, "test_key")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if value != "test_value" {
			t.Errorf("Expected 'test_value', got '%v'", value)
		}
	})

	// 测试 Exists
	t.Run("Exists", func(t *testing.T) {
		exists, err := storage.Exists(ctx, "test_key")
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if !exists {
			t.Error("Expected key to exist")
		}

		exists, err = storage.Exists(ctx, "non_existent_key")
		if err != nil {
			t.Fatalf("Exists failed: %v", err)
		}
		if exists {
			t.Error("Expected key to not exist")
		}
	})

	// 测试 Delete
	t.Run("Delete", func(t *testing.T) {
		err := storage.Delete(ctx, "test_key")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err = storage.Get(ctx, "test_key")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})
}

func TestMemoryStorage_ListOperations(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	ctx := context.Background()

	t.Run("SetList and GetList", func(t *testing.T) {
		values := []interface{}{"item1", "item2", "item3"}
		err := storage.SetList(ctx, "test_list", values, 1*time.Hour)
		if err != nil {
			t.Fatalf("SetList failed: %v", err)
		}

		result, err := storage.GetList(ctx, "test_list")
		if err != nil {
			t.Fatalf("GetList failed: %v", err)
		}

		if len(result) != len(values) {
			t.Errorf("Expected %d items, got %d", len(values), len(result))
		}

		for i, expected := range values {
			if result[i] != expected {
				t.Errorf("Expected %v at index %d, got %v", expected, i, result[i])
			}
		}
	})

	t.Run("AppendToList", func(t *testing.T) {
		err := storage.AppendToList(ctx, "test_append", "new_item")
		if err != nil {
			t.Fatalf("AppendToList failed: %v", err)
		}

		result, err := storage.GetList(ctx, "test_append")
		if err != nil {
			t.Fatalf("GetList failed: %v", err)
		}

		if len(result) != 1 || result[0] != "new_item" {
			t.Errorf("Expected ['new_item'], got %v", result)
		}

		// 追加第二个项目
		err = storage.AppendToList(ctx, "test_append", "second_item")
		if err != nil {
			t.Fatalf("AppendToList failed: %v", err)
		}

		result, err = storage.GetList(ctx, "test_append")
		if err != nil {
			t.Fatalf("GetList failed: %v", err)
		}

		expected := []interface{}{"new_item", "second_item"}
		if len(result) != len(expected) {
			t.Errorf("Expected %d items, got %d", len(expected), len(result))
		}
	})

	t.Run("RemoveFromList", func(t *testing.T) {
		err := storage.RemoveFromList(ctx, "test_append", "new_item")
		if err != nil {
			t.Fatalf("RemoveFromList failed: %v", err)
		}

		result, err := storage.GetList(ctx, "test_append")
		if err != nil {
			t.Fatalf("GetList failed: %v", err)
		}

		if len(result) != 1 || result[0] != "second_item" {
			t.Errorf("Expected ['second_item'], got %v", result)
		}
	})
}

func TestMemoryStorage_HashOperations(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	ctx := context.Background()

	t.Run("SetHash and GetHash", func(t *testing.T) {
		err := storage.SetHash(ctx, "test_hash", "field1", "value1")
		if err != nil {
			t.Fatalf("SetHash failed: %v", err)
		}

		value, err := storage.GetHash(ctx, "test_hash", "field1")
		if err != nil {
			t.Fatalf("GetHash failed: %v", err)
		}

		if value != "value1" {
			t.Errorf("Expected 'value1', got '%v'", value)
		}
	})

	t.Run("GetAllHash", func(t *testing.T) {
		err := storage.SetHash(ctx, "test_hash", "field2", "value2")
		if err != nil {
			t.Fatalf("SetHash failed: %v", err)
		}

		all, err := storage.GetAllHash(ctx, "test_hash")
		if err != nil {
			t.Fatalf("GetAllHash failed: %v", err)
		}

		expected := map[string]interface{}{
			"field1": "value1",
			"field2": "value2",
		}

		if len(all) != len(expected) {
			t.Errorf("Expected %d fields, got %d", len(expected), len(all))
		}

		for k, v := range expected {
			if all[k] != v {
				t.Errorf("Expected %v for field %s, got %v", v, k, all[k])
			}
		}
	})

	t.Run("DeleteHash", func(t *testing.T) {
		err := storage.DeleteHash(ctx, "test_hash", "field1")
		if err != nil {
			t.Fatalf("DeleteHash failed: %v", err)
		}

		_, err = storage.GetHash(ctx, "test_hash", "field1")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}

		// field2 应该还存在
		value, err := storage.GetHash(ctx, "test_hash", "field2")
		if err != nil {
			t.Fatalf("GetHash failed: %v", err)
		}
		if value != "value2" {
			t.Errorf("Expected 'value2', got '%v'", value)
		}
	})
}

func TestMemoryStorage_CounterOperations(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	ctx := context.Background()

	t.Run("Incr", func(t *testing.T) {
		value, err := storage.Incr(ctx, "test_counter")
		if err != nil {
			t.Fatalf("Incr failed: %v", err)
		}
		if value != 1 {
			t.Errorf("Expected 1, got %d", value)
		}

		value, err = storage.Incr(ctx, "test_counter")
		if err != nil {
			t.Fatalf("Incr failed: %v", err)
		}
		if value != 2 {
			t.Errorf("Expected 2, got %d", value)
		}
	})

	t.Run("IncrBy", func(t *testing.T) {
		value, err := storage.IncrBy(ctx, "test_counter", 5)
		if err != nil {
			t.Fatalf("IncrBy failed: %v", err)
		}
		if value != 7 {
			t.Errorf("Expected 7, got %d", value)
		}
	})
}

func TestMemoryStorage_Expiration(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	ctx := context.Background()

	t.Run("TTL Expiration", func(t *testing.T) {
		// 设置一个短期过期的键
		err := storage.Set(ctx, "expire_key", "expire_value", 10*time.Millisecond)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// 立即获取应该成功
		value, err := storage.Get(ctx, "expire_key")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if value != "expire_value" {
			t.Errorf("Expected 'expire_value', got '%v'", value)
		}

		// 等待过期
		time.Sleep(20 * time.Millisecond)

		// 再次获取应该失败
		_, err = storage.Get(ctx, "expire_key")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("SetExpiration", func(t *testing.T) {
		err := storage.Set(ctx, "extend_key", "extend_value", 1*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// 设置短期过期
		err = storage.SetExpiration(ctx, "extend_key", 10*time.Millisecond)
		if err != nil {
			t.Fatalf("SetExpiration failed: %v", err)
		}

		// 等待过期
		time.Sleep(20 * time.Millisecond)

		// 应该已过期
		_, err = storage.Get(ctx, "extend_key")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("GetExpiration", func(t *testing.T) {
		err := storage.Set(ctx, "ttl_key", "ttl_value", 1*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		ttl, err := storage.GetExpiration(ctx, "ttl_key")
		if err != nil {
			t.Fatalf("GetExpiration failed: %v", err)
		}

		// TTL 应该在 1 小时左右
		if ttl < 59*time.Minute || ttl > 61*time.Minute {
			t.Errorf("Expected TTL around 1 hour, got %v", ttl)
		}
	})
}

func TestMemoryStorage_CleanupExpired(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	ctx := context.Background()

	t.Run("Manual Cleanup", func(t *testing.T) {
		// 设置一些过期的键
		err := storage.Set(ctx, "expired1", "value1", 1*time.Millisecond)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}
		err = storage.Set(ctx, "expired2", "value2", 1*time.Millisecond)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// 设置一个未过期的键
		err = storage.Set(ctx, "valid_key", "valid_value", 1*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// 等待过期
		time.Sleep(10 * time.Millisecond)

		// 手动清理
		err = storage.CleanupExpired(ctx)
		if err != nil {
			t.Fatalf("CleanupExpired failed: %v", err)
		}

		// 过期的键应该不存在
		_, err = storage.Get(ctx, "expired1")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound for expired1, got %v", err)
		}

		_, err = storage.Get(ctx, "expired2")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound for expired2, got %v", err)
		}

		// 有效的键应该存在
		value, err := storage.Get(ctx, "valid_key")
		if err != nil {
			t.Fatalf("Get valid_key failed: %v", err)
		}
		if value != "valid_value" {
			t.Errorf("Expected 'valid_value', got '%v'", value)
		}
	})
}

func TestMemoryStorage_AutoCleanup(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	ctx := context.Background()

	t.Run("Auto Cleanup", func(t *testing.T) {
		// 启动自动清理，每 50ms 清理一次
		storage.StartCleanup(50 * time.Millisecond)

		// 设置一个短期过期的键
		err := storage.Set(ctx, "auto_expire", "auto_value", 20*time.Millisecond)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// 立即获取应该成功
		value, err := storage.Get(ctx, "auto_expire")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if value != "auto_value" {
			t.Errorf("Expected 'auto_value', got '%v'", value)
		}

		// 等待自动清理
		time.Sleep(100 * time.Millisecond)

		// 应该已被自动清理
		_, err = storage.Get(ctx, "auto_expire")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}

		// 停止自动清理
		storage.StopCleanup()
	})
}

func TestMemoryStorage_Concurrency(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	ctx := context.Background()

	t.Run("Concurrent Operations", func(t *testing.T) {
		const numGoroutines = 10
		const numOperations = 100

		// 并发写入
		done := make(chan bool, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("concurrent_key_%d_%d", id, j)
					err := storage.Set(ctx, key, fmt.Sprintf("value_%d_%d", id, j), 1*time.Hour)
					if err != nil {
						t.Errorf("Set failed: %v", err)
					}
				}
				done <- true
			}(i)
		}

		// 等待所有 goroutine 完成
		for i := 0; i < numGoroutines; i++ {
			<-done
		}

		// 验证所有值都正确写入
		for i := 0; i < numGoroutines; i++ {
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("concurrent_key_%d_%d", i, j)
				expected := fmt.Sprintf("value_%d_%d", i, j)
				value, err := storage.Get(ctx, key)
				if err != nil {
					t.Errorf("Get failed for key %s: %v", key, err)
					continue
				}
				if value != expected {
					t.Errorf("Expected %s for key %s, got %v", expected, key, value)
				}
			}
		}
	})
}

func TestMemoryStorage_ErrorHandling(t *testing.T) {
	storage := cloud.NewMemoryStorage(context.Background())
	defer storage.Close()

	ctx := context.Background()

	t.Run("Get Non-existent Key", func(t *testing.T) {
		_, err := storage.Get(ctx, "non_existent")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("Get Hash Non-existent Key", func(t *testing.T) {
		_, err := storage.GetHash(ctx, "non_existent", "field")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("Get Hash Non-existent Field", func(t *testing.T) {
		err := storage.SetHash(ctx, "test_hash", "field1", "value1")
		if err != nil {
			t.Fatalf("SetHash failed: %v", err)
		}

		_, err = storage.GetHash(ctx, "test_hash", "non_existent_field")
		if err != cloud.ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("Invalid Type Operations", func(t *testing.T) {
		// 设置一个字符串值
		err := storage.Set(ctx, "string_key", "string_value", 1*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// 尝试作为列表获取
		_, err = storage.GetList(ctx, "string_key")
		if err != cloud.ErrInvalidType {
			t.Errorf("Expected ErrInvalidType, got %v", err)
		}

		// 尝试作为哈希获取
		_, err = storage.GetHash(ctx, "string_key", "field")
		if err != cloud.ErrInvalidType {
			t.Errorf("Expected ErrInvalidType, got %v", err)
		}
	})
}
