package storage

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// TestRedisStorage_Basic 测试Redis存储基本功能
func TestRedisStorage_Basic(t *testing.T) {
	// 注意：这个测试需要Redis服务器运行在localhost:6379
	// 如果没有Redis服务器，测试会被跳过

	ctx := context.Background()

	// 创建Redis存储
	config := &RedisConfig{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		PoolSize: 10,
	}

	storage, err := NewRedisStorage(ctx, config)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
	}
	defer storage.Close()

	// 测试基本设置和获取
	t.Run("Basic_Set_Get", func(t *testing.T) {
		key := "test:basic:key"
		value := "test_value"

		// 设置值
		err := storage.Set(key, value, 30*time.Second)
		require.NoError(t, err)

		// 获取值
		retrieved, err := storage.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		// 检查存在性
		exists, err := storage.Exists(key)
		require.NoError(t, err)
		assert.True(t, exists)

		// 删除值
		err = storage.Delete(key)
		require.NoError(t, err)

		// 验证删除
		exists, err = storage.Exists(key)
		require.NoError(t, err)
		assert.False(t, exists)
	})

	// 测试复杂数据类型
	t.Run("Complex_Data_Types", func(t *testing.T) {
		key := "test:complex:key"
		value := map[string]interface{}{
			"string": "hello",
			"number": 42,
			"bool":   true,
			"array":  []interface{}{1, 2, 3},
		}

		// 设置复杂值
		err := storage.Set(key, value, 30*time.Second)
		require.NoError(t, err)

		// 获取复杂值
		retrieved, err := storage.Get(key)
		require.NoError(t, err)

		// 验证结构
		retrievedMap, ok := retrieved.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "hello", retrievedMap["string"])
		assert.Equal(t, float64(42), retrievedMap["number"]) // JSON unmarshal to float64
		assert.Equal(t, true, retrievedMap["bool"])

		// 清理
		storage.Delete(key)
	})

	// 测试过期时间
	t.Run("Expiration", func(t *testing.T) {
		key := "test:expiration:key"
		value := "expire_me"

		// 设置1秒过期
		err := storage.Set(key, value, 1*time.Second)
		require.NoError(t, err)

		// 立即获取应该存在
		retrieved, err := storage.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		// 等待过期
		time.Sleep(2 * time.Second)

		// 过期后应该不存在
		_, err = storage.Get(key)
		assert.Error(t, err)
		assert.Equal(t, ErrKeyNotFound, err)
	})

	// 测试列表操作
	t.Run("List_Operations", func(t *testing.T) {
		key := "test:list:key"
		values := []interface{}{"item1", "item2", "item3"}

		// 设置列表
		err := storage.SetList(key, values, 30*time.Second)
		require.NoError(t, err)

		// 获取列表
		retrieved, err := storage.GetList(key)
		require.NoError(t, err)
		assert.Equal(t, values, retrieved)

		// 追加到列表
		err = storage.AppendToList(key, "item4")
		require.NoError(t, err)

		// 验证追加
		retrieved, err = storage.GetList(key)
		require.NoError(t, err)
		assert.Len(t, retrieved, 4)
		assert.Equal(t, "item4", retrieved[3])

		// 从列表中移除
		err = storage.RemoveFromList(key, "item2")
		require.NoError(t, err)

		// 验证移除
		retrieved, err = storage.GetList(key)
		require.NoError(t, err)
		assert.Len(t, retrieved, 3)
		assert.Equal(t, "item1", retrieved[0])
		assert.Equal(t, "item3", retrieved[1])
		assert.Equal(t, "item4", retrieved[2])

		// 清理
		storage.Delete(key)
	})

	// 测试哈希操作
	t.Run("Hash_Operations", func(t *testing.T) {
		key := "test:hash:key"

		// 设置哈希字段
		err := storage.SetHash(key, "field1", "value1")
		require.NoError(t, err)
		err = storage.SetHash(key, "field2", "value2")
		require.NoError(t, err)

		// 获取单个字段
		value, err := storage.GetHash(key, "field1")
		require.NoError(t, err)
		assert.Equal(t, "value1", value)

		// 获取所有字段
		allFields, err := storage.GetAllHash(key)
		require.NoError(t, err)
		assert.Len(t, allFields, 2)
		assert.Equal(t, "value1", allFields["field1"])
		assert.Equal(t, "value2", allFields["field2"])

		// 删除字段
		err = storage.DeleteHash(key, "field1")
		require.NoError(t, err)

		// 验证删除
		_, err = storage.GetHash(key, "field1")
		assert.Error(t, err)
		assert.Equal(t, ErrKeyNotFound, err)

		// 清理
		storage.Delete(key)
	})

	// 测试计数器操作
	t.Run("Counter_Operations", func(t *testing.T) {
		key := "test:counter:key"

		// 递增
		value, err := storage.Incr(key)
		require.NoError(t, err)
		assert.Equal(t, int64(1), value)

		// 按值递增
		value, err = storage.IncrBy(key, 5)
		require.NoError(t, err)
		assert.Equal(t, int64(6), value)

		// 再次递增
		value, err = storage.Incr(key)
		require.NoError(t, err)
		assert.Equal(t, int64(7), value)

		// 清理
		storage.Delete(key)
	})

	// 测试原子操作
	t.Run("Atomic_Operations", func(t *testing.T) {
		key := "test:atomic:key"

		// 测试SetNX
		success, err := storage.SetNX(key, "initial_value", 30*time.Second)
		require.NoError(t, err)
		assert.True(t, success)

		// 再次尝试SetNX应该失败
		success, err = storage.SetNX(key, "another_value", 30*time.Second)
		require.NoError(t, err)
		assert.False(t, success)

		// 测试CompareAndSwap
		success, err = storage.CompareAndSwap(key, "initial_value", "new_value", 30*time.Second)
		require.NoError(t, err)
		assert.True(t, success)

		// 验证交换结果
		value, err := storage.Get(key)
		require.NoError(t, err)
		assert.Equal(t, "new_value", value)

		// 清理
		storage.Delete(key)
	})

	// 测试过期时间操作
	t.Run("Expiration_Operations", func(t *testing.T) {
		key := "test:expiration_ops:key"
		value := "test_value"

		// 设置值
		err := storage.Set(key, value, 60*time.Second)
		require.NoError(t, err)

		// 获取过期时间
		ttl, err := storage.GetExpiration(key)
		require.NoError(t, err)
		assert.True(t, ttl > 0 && ttl <= 60*time.Second)

		// 设置新的过期时间
		err = storage.SetExpiration(key, 30*time.Second)
		require.NoError(t, err)

		// 验证新的过期时间
		ttl, err = storage.GetExpiration(key)
		require.NoError(t, err)
		assert.True(t, ttl > 0 && ttl <= 30*time.Second)

		// 清理
		storage.Delete(key)
	})
}

// TestRedisStorage_Concurrency 测试Redis存储并发操作
func TestRedisStorage_Concurrency(t *testing.T) {
	ctx := context.Background()

	config := &RedisConfig{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
		PoolSize: 10,
	}

	storage, err := NewRedisStorage(ctx, config)
	if err != nil {
		t.Skipf("Redis not available, skipping test: %v", err)
	}
	defer storage.Close()

	t.Run("Concurrent_Operations", func(t *testing.T) {
		const numGoroutines = 10
		const numOperations = 100

		// 并发设置和获取
		done := make(chan bool, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer func() { done <- true }()

				for j := 0; j < numOperations; j++ {
					key := fmt.Sprintf("concurrent:key:%d:%d", id, j)
					value := fmt.Sprintf("value:%d:%d", id, j)

					// 设置值
					err := storage.Set(key, value, 30*time.Second)
					if err != nil {
						t.Errorf("Failed to set key %s: %v", key, err)
						return
					}

					// 获取值
					retrieved, err := storage.Get(key)
					if err != nil {
						t.Errorf("Failed to get key %s: %v", key, err)
						return
					}

					if retrieved != value {
						t.Errorf("Value mismatch for key %s: expected %s, got %v", key, value, retrieved)
						return
					}

					// 删除值
					err = storage.Delete(key)
					if err != nil {
						t.Errorf("Failed to delete key %s: %v", key, err)
						return
					}
				}
			}(i)
		}

		// 等待所有goroutine完成
		for i := 0; i < numGoroutines; i++ {
			<-done
		}
	})
}

// TestRedisStorage_Factory 测试存储工厂
func TestRedisStorage_Factory(t *testing.T) {
	ctx := context.Background()

	factory := NewStorageFactory(ctx)

	t.Run("Factory_Create_Redis", func(t *testing.T) {
		config := &StorageConfigMap{
			Type:     StorageTypeRedis,
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
			PoolSize: 10,
		}

		storage, err := factory.CreateStorageWithConfigMap(config)
		if err != nil {
			t.Skipf("Redis not available, skipping test: %v", err)
		}
		defer storage.Close()

		// 测试基本功能
		key := "factory:test:key"
		value := "factory_test_value"

		err = storage.Set(key, value, 30*time.Second)
		require.NoError(t, err)

		retrieved, err := storage.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		storage.Delete(key)
	})

	t.Run("Factory_Create_Memory", func(t *testing.T) {
		config := &StorageConfigMap{
			Type: StorageTypeMemory,
		}

		storage, err := factory.CreateStorageWithConfigMap(config)
		require.NoError(t, err)
		defer storage.Close()

		// 测试基本功能
		key := "factory:memory:key"
		value := "factory_memory_value"

		err = storage.Set(key, value, 30*time.Second)
		require.NoError(t, err)

		retrieved, err := storage.Get(key)
		require.NoError(t, err)
		assert.Equal(t, value, retrieved)

		storage.Delete(key)
	})
}
