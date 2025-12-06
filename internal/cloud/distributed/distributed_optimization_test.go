package distributed

import (
	"context"
	"testing"
	"time"
	"tunnox-core/internal/core/storage"
)

func TestStorageBasedLock(t *testing.T) {
	storage := storage.NewMemoryStorage(context.Background())
	defer storage.Close()

	// 创建基于存储的分布式锁
	lock1 := NewStorageBasedLock(storage, "test_owner_1")
	lock2 := NewStorageBasedLock(storage, "test_owner_2")

	t.Run("Basic Lock Operations", func(t *testing.T) {
		// 测试获取锁
		acquired, err := lock1.Acquire("test_key", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}
		if !acquired {
			t.Fatal("Expected to acquire lock")
		}

		// 测试重复获取锁失败
		acquired, err = lock1.Acquire("test_key", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to check lock: %v", err)
		}
		if acquired {
			t.Fatal("Expected lock acquisition to fail")
		}

		// 测试其他所有者无法获取锁
		acquired, err = lock2.Acquire("test_key", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to check lock: %v", err)
		}
		if acquired {
			t.Fatal("Expected lock acquisition to fail for different owner")
		}

		// 测试释放锁
		err = lock1.Release("test_key")
		if err != nil {
			t.Fatalf("Failed to release lock: %v", err)
		}

		// 测试锁释放后可以重新获取
		acquired, err = lock2.Acquire("test_key", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to acquire lock after release: %v", err)
		}
		if !acquired {
			t.Fatal("Expected to acquire lock after release")
		}
	})

	t.Run("Lock Expiration", func(t *testing.T) {
		// 获取短期锁
		acquired, err := lock1.Acquire("expire_key", 100*time.Millisecond)
		if err != nil {
			t.Fatalf("Failed to acquire lock: %v", err)
		}
		if !acquired {
			t.Fatal("Expected to acquire lock")
		}

		// 等待锁过期
		time.Sleep(200 * time.Millisecond)

		// 检查锁是否已过期
		isLocked, err := lock1.IsLocked("expire_key")
		if err != nil {
			t.Fatalf("Failed to check lock status: %v", err)
		}
		if isLocked {
			t.Log("Lock is still held, waiting a bit more...")
			time.Sleep(100 * time.Millisecond)
		}

		// 测试锁过期后可以重新获取
		acquired, err = lock2.Acquire("expire_key", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to acquire expired lock: %v", err)
		}
		if !acquired {
			t.Fatal("Expected to acquire expired lock")
		}
	})
}

func TestStorageAtomicOperations(t *testing.T) {
	storage := storage.NewMemoryStorage(context.Background())
	defer storage.Close()

	t.Run("SetNX Operations", func(t *testing.T) {
		casStore, ok := storage.(interface {
			SetNX(key string, value interface{}, ttl time.Duration) (bool, error)
			CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error)
		})
		if !ok {
			t.Skip("storage does not support CAS operations")
		}
		// 测试SetNX成功
		success, err := casStore.SetNX("test_key", "test_value", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to SetNX: %v", err)
		}
		if !success {
			t.Fatal("Expected SetNX to succeed")
		}

		// 测试SetNX失败（键已存在）
		success, err = casStore.SetNX("test_key", "another_value", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to SetNX: %v", err)
		}
		if success {
			t.Fatal("Expected SetNX to fail")
		}

		// 验证值没有被修改
		value, err := storage.Get("test_key")
		if err != nil {
			t.Fatalf("Failed to get value: %v", err)
		}
		if value != "test_value" {
			t.Fatalf("Expected 'test_value', got '%v'", value)
		}
	})

	t.Run("CompareAndSwap Operations", func(t *testing.T) {
		// 设置初始值
		err := storage.Set("cas_key", "old_value", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to set value: %v", err)
		}

		// 测试CompareAndSwap成功
		casStore, ok := storage.(interface {
			SetNX(key string, value interface{}, ttl time.Duration) (bool, error)
			CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error)
		})
		if !ok {
			t.Skip("storage does not support CAS operations")
		}
		success, err := casStore.CompareAndSwap("cas_key", "old_value", "new_value", 5*time.Second)
		if err != nil {
			t.Fatalf("Failed to CompareAndSwap: %v", err)
		}
		if !success {
			t.Fatal("Expected CompareAndSwap to succeed")
		}

		// 验证值被修改
		value, err := storage.Get("cas_key")
		if err != nil {
			t.Fatalf("Failed to get value: %v", err)
		}
		if value != "new_value" {
			t.Fatalf("Expected 'new_value', got '%v'", value)
		}
	})
}
