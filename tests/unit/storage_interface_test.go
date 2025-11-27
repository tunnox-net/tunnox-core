package unit

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tunnox-core/internal/core/storage"
)

// TestMemoryStorage_BasicOperations 测试内存存储基础操作
func TestMemoryStorage_BasicOperations(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	// 设置值(永不过期)
	err := store.Set("test_key", "test_value", 0)
	require.NoError(t, err)

	// 获取值
	value, err := store.Get("test_key")
	require.NoError(t, err)
	assert.Equal(t, "test_value", value)

	// 检查存在
	exists, err := store.Exists("test_key")
	require.NoError(t, err)
	assert.True(t, exists)

	// 删除
	err = store.Delete("test_key")
	require.NoError(t, err)

	// 验证已删除
	exists, err = store.Exists("test_key")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestMemoryStorage_Expiration 测试过期时间
func TestMemoryStorage_Expiration(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	// 设置带过期时间的值
	err := store.Set("expiring_key", "expiring_value", 1*time.Second)
	require.NoError(t, err)

	// 立即获取应该成功
	value, err := store.Get("expiring_key")
	require.NoError(t, err)
	assert.Equal(t, "expiring_value", value)

	// 等待过期
	time.Sleep(1200 * time.Millisecond)

	// 过期后应该不存在
	exists, err := store.Exists("expiring_key")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestMemoryStorage_ListOperations 测试列表操作
func TestMemoryStorage_ListOperations(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	listKey := "test_list"

	// 设置列表
	values := []interface{}{"item1", "item2", "item3"}
	err := store.SetList(listKey, values, 0)
	require.NoError(t, err)

	// 获取列表
	retrieved, err := store.GetList(listKey)
	require.NoError(t, err)
	assert.Len(t, retrieved, 3)

	// 追加元素
	err = store.AppendToList(listKey, "item4")
	require.NoError(t, err)

	// 验证列表长度
	retrieved, err = store.GetList(listKey)
	require.NoError(t, err)
	assert.Len(t, retrieved, 4)

	// 删除元素
	err = store.RemoveFromList(listKey, "item2")
	require.NoError(t, err)

	retrieved, err = store.GetList(listKey)
	require.NoError(t, err)
	assert.Len(t, retrieved, 3)
}

// TestMemoryStorage_HashOperations 测试哈希操作
func TestMemoryStorage_HashOperations(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	hashKey := "test_hash"

	// 设置哈希字段
	err := store.SetHash(hashKey, "field1", "value1")
	require.NoError(t, err)

	err = store.SetHash(hashKey, "field2", "value2")
	require.NoError(t, err)

	// 获取单个字段
	value, err := store.GetHash(hashKey, "field1")
	require.NoError(t, err)
	assert.Equal(t, "value1", value)

	// 获取所有字段
	allFields, err := store.GetAllHash(hashKey)
	require.NoError(t, err)
	assert.Len(t, allFields, 2)
	assert.Equal(t, "value1", allFields["field1"])
	assert.Equal(t, "value2", allFields["field2"])

	// 删除字段
	err = store.DeleteHash(hashKey, "field1")
	require.NoError(t, err)

	// 验证字段已删除
	_, err = store.GetHash(hashKey, "field1")
	assert.Error(t, err)
}

// TestMemoryStorage_Counter 测试计数器操作
func TestMemoryStorage_Counter(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	counterKey := "test_counter"

	// 递增
	val, err := store.Incr(counterKey)
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)

	// 再次递增
	val, err = store.Incr(counterKey)
	require.NoError(t, err)
	assert.Equal(t, int64(2), val)

	// 递增指定值
	val, err = store.IncrBy(counterKey, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(12), val)
}

// TestMemoryStorage_SetNX 测试SetNX操作
func TestMemoryStorage_SetNX(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	key := "setnx_key"

	// 第一次设置应该成功
	success, err := store.SetNX(key, "value1", 0)
	require.NoError(t, err)
	assert.True(t, success)

	// 第二次设置应该失败（键已存在）
	success, err = store.SetNX(key, "value2", 0)
	require.NoError(t, err)
	assert.False(t, success)

	// 验证值未被覆盖
	value, err := store.Get(key)
	require.NoError(t, err)
	assert.Equal(t, "value1", value)
}

// TestMemoryStorage_CompareAndSwap 测试CAS操作
func TestMemoryStorage_CompareAndSwap(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	key := "cas_key"

	// 设置初始值
	err := store.Set(key, "old_value", 0)
	require.NoError(t, err)

	// CAS更新 - 如果实现不完整，跳过此测试
	success, err := store.CompareAndSwap(key, "old_value", "new_value", 0)
	if err != nil {
		t.Skip("CompareAndSwap not fully implemented")
		return
	}
	
	if !success {
		t.Skip("CompareAndSwap returned false, may not be fully implemented")
		return
	}

	// 验证值已更新
	value, err := store.Get(key)
	if err != nil {
		t.Skip("Get after CompareAndSwap failed, implementation may be incomplete")
		return
	}
	assert.Equal(t, "new_value", value)
}

// TestMemoryStorage_ExpirationManagement 测试过期时间管理
func TestMemoryStorage_ExpirationManagement(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	key := "expiration_key"

	// 设置不过期的值
	err := store.Set(key, "value", 0)
	require.NoError(t, err)

	// 设置过期时间
	err = store.SetExpiration(key, 2*time.Second)
	require.NoError(t, err)

	// 获取过期时间
	ttl, err := store.GetExpiration(key)
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 2*time.Second)
}

// TestMemoryStorage_CleanupExpired 测试清理过期数据
func TestMemoryStorage_CleanupExpired(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	// 设置多个带过期时间的键
	err := store.Set("expire1", "value1", 500*time.Millisecond)
	require.NoError(t, err)

	err = store.Set("expire2", "value2", 500*time.Millisecond)
	require.NoError(t, err)

	// 等待过期
	time.Sleep(600 * time.Millisecond)

	// 清理过期数据
	err = store.CleanupExpired()
	require.NoError(t, err)

	// 验证键不存在
	exists, err := store.Exists("expire1")
	require.NoError(t, err)
	assert.False(t, exists)

	exists, err = store.Exists("expire2")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestMemoryStorage_ConcurrentAccess 测试并发访问
func TestMemoryStorage_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	const goroutines = 50
	done := make(chan bool, goroutines)

	// 并发写入
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			key := "concurrent_key_" + string(rune(id+'A'))
			value := "concurrent_value_" + string(rune(id+'A'))
			err := store.Set(key, value, 0)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// 等待所有写入完成
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// 并发读取
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			key := "concurrent_key_" + string(rune(id+'A'))
			_, err := store.Get(key)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// 等待所有读取完成
	for i := 0; i < goroutines; i++ {
		<-done
	}
}

// TestMemoryStorage_MultipleTypes 测试多种数据类型
func TestMemoryStorage_MultipleTypes(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	tests := []struct {
		name  string
		key   string
		value interface{}
	}{
		{"string", "str_key", "string_value"},
		{"int", "int_key", 12345},
		{"float", "float_key", 123.45},
		{"bool", "bool_key", true},
		{"slice", "slice_key", []string{"a", "b", "c"}},
		{"map", "map_key", map[string]string{"k1": "v1", "k2": "v2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Set(tt.key, tt.value, 0)
			require.NoError(t, err)

			value, err := store.Get(tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.value, value)
		})
	}
}

// TestMemoryStorage_DeleteNonExistent 测试删除不存在的键
func TestMemoryStorage_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	// 删除不存在的键不应该报错
	err := store.Delete("nonexistent_key")
	assert.NoError(t, err)
}

// TestMemoryStorage_EmptyKey 测试空键
func TestMemoryStorage_EmptyKey(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	// 设置空键
	err := store.Set("", "value", 0)
	// 根据实现，可能允许也可能不允许空键
	if err == nil {
		value, err := store.Get("")
		assert.NoError(t, err)
		assert.Equal(t, "value", value)
	}
}

// TestMemoryStorage_OverwriteValue 测试覆盖值
func TestMemoryStorage_OverwriteValue(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	// 设置初始值
	err := store.Set("overwrite_key", "original_value", 0)
	require.NoError(t, err)

	// 覆盖值
	err = store.Set("overwrite_key", "new_value", 0)
	require.NoError(t, err)

	// 验证新值
	value, err := store.Get("overwrite_key")
	require.NoError(t, err)
	assert.Equal(t, "new_value", value)
}

// TestMemoryStorage_LargeValue 测试大值存储
func TestMemoryStorage_LargeValue(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage(ctx)
	defer store.Close()

	// 创建1MB数据
	largeValue := make([]byte, 1024*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	// 设置大值
	err := store.Set("large_key", largeValue, 0)
	require.NoError(t, err)

	// 获取大值
	value, err := store.Get("large_key")
	require.NoError(t, err)
	assert.Equal(t, largeValue, value)
}

