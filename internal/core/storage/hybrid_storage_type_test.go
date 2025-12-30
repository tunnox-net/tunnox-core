package storage

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestHybridStorage_GetList_AfterJSONReload 测试 JSON 重载后的列表类型
func TestHybridStorage_GetList_AfterJSONReload(t *testing.T) {
	ctx := context.Background()
	tempFile := "/tmp/test_list_type.json"

	// 清理旧文件
	os.Remove(tempFile)

	// 第一阶段：写入列表数据
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

		hybridStorage := NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)

		// 使用 SharedPersistent 类型的 key
		key := "tunnox:client_mappings:12345678"
		mappingJSON := `{"id":"pmap_test","listen_client_id":12345678,"target_client_id":87654321}`

		// 追加到列表
		if err := hybridStorage.AppendToList(key, mappingJSON); err != nil {
			t.Fatalf("AppendToList failed: %v", err)
		}

		// 验证写入后可以读取
		list, err := hybridStorage.GetList(key)
		if err != nil {
			t.Fatalf("GetList after write failed: %v", err)
		}
		if len(list) != 1 {
			t.Fatalf("Expected 1 item, got %d", len(list))
		}
		t.Logf("Phase 1: wrote list with %d items, first item type: %T", len(list), list[0])

		// 等待自动保存
		time.Sleep(200 * time.Millisecond)

		hybridStorage.Close()
		persistent.Close()
	}

	// 检查 JSON 文件内容
	data, err := os.ReadFile(tempFile)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}
	t.Logf("JSON file content: %s", string(data))

	// 解析 JSON 文件查看实际存储的数据类型
	var jsonData map[string]interface{}
	if err := json.Unmarshal(data, &jsonData); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	key := "tunnox:client_mappings:12345678"
	if val, ok := jsonData[key]; ok {
		t.Logf("Stored value type: %T, value: %v", val, val)
	}

	// 第二阶段：重新打开（模拟服务重启）
	{
		persistent, err := NewJSONStorage(&JSONStorageConfig{
			FilePath:     tempFile,
			AutoSave:     true,
			SaveInterval: 100 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("NewJSONStorage (reload) failed: %v", err)
		}
		defer persistent.Close()

		cache := NewMemoryStorage(ctx)
		config := DefaultHybridConfig()
		config.EnablePersistent = true

		hybridStorage := NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)
		defer hybridStorage.Close()

		// 直接从持久化存储读取（绕过缓存）
		value, err := persistent.Get(key)
		if err != nil {
			t.Fatalf("Persistent.Get failed: %v", err)
		}
		t.Logf("From persistent - type: %T, value: %v", value, value)

		// 尝试类型断言
		if list, ok := value.([]interface{}); ok {
			t.Logf("Type assertion to []interface{} succeeded, len=%d", len(list))
			if len(list) > 0 {
				t.Logf("First item type: %T, value: %v", list[0], list[0])
			}
		} else {
			t.Errorf("Type assertion to []interface{} failed! value type: %T", value)
		}

		// 通过 HybridStorage.GetList 读取
		list, err := hybridStorage.GetList(key)
		if err != nil {
			t.Errorf("GetList after reload failed: %v", err)
		} else {
			t.Logf("GetList succeeded, len=%d", len(list))
			if len(list) > 0 {
				t.Logf("First item type: %T, value: %v", list[0], list[0])
			}
		}
	}
}

// TestHybridStorage_GenericRepository_ListDeserialization 测试 GenericRepository 的列表反序列化
func TestHybridStorage_GenericRepository_ListDeserialization(t *testing.T) {
	ctx := context.Background()
	tempFile := "/tmp/test_list_deserialize.json"

	os.Remove(tempFile)

	// 模拟写入一个包含 JSON 字符串的列表
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

		hybridStorage := NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)

		key := "tunnox:client_mappings:99999999"

		// 模拟 Repository 的行为：将 struct 序列化为 JSON 字符串存入列表
		type TestMapping struct {
			ID             string `json:"id"`
			ListenClientID int64  `json:"listen_client_id"`
			TargetClientID int64  `json:"target_client_id"`
		}

		mapping := TestMapping{
			ID:             "pmap_test_deserialize",
			ListenClientID: 99999999,
			TargetClientID: 88888888,
		}

		mappingJSON, _ := json.Marshal(mapping)
		mappingStr := string(mappingJSON)

		t.Logf("Storing JSON string: %s", mappingStr)

		if err := hybridStorage.AppendToList(key, mappingStr); err != nil {
			t.Fatalf("AppendToList failed: %v", err)
		}

		// 验证
		list, err := hybridStorage.GetList(key)
		if err != nil {
			t.Fatalf("GetList after write failed: %v", err)
		}
		t.Logf("After write: len=%d, first item type: %T", len(list), list[0])

		// 模拟 GenericRepository.List 的反序列化逻辑
		for i, item := range list {
			if entityData, ok := item.(string); ok {
				t.Logf("Item[%d] is string, can unmarshal: %s", i, entityData)
			} else {
				t.Errorf("Item[%d] is NOT string, type: %T", i, item)
			}
		}

		time.Sleep(200 * time.Millisecond)
		hybridStorage.Close()
		persistent.Close()
	}

	// 第二阶段：重新打开
	{
		persistent, err := NewJSONStorage(&JSONStorageConfig{
			FilePath:     tempFile,
			AutoSave:     true,
			SaveInterval: 100 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("NewJSONStorage (reload) failed: %v", err)
		}
		defer persistent.Close()

		cache := NewMemoryStorage(ctx)
		config := DefaultHybridConfig()
		config.EnablePersistent = true

		hybridStorage := NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)
		defer hybridStorage.Close()

		key := "tunnox:client_mappings:99999999"

		list, err := hybridStorage.GetList(key)
		if err != nil {
			t.Fatalf("GetList after reload failed: %v", err)
		}
		t.Logf("After reload: len=%d", len(list))

		// 模拟 GenericRepository.List 的反序列化逻辑
		for i, item := range list {
			if entityData, ok := item.(string); ok {
				t.Logf("After reload - Item[%d] is string, can unmarshal: %s", i, entityData)
			} else {
				t.Errorf("After reload - Item[%d] is NOT string, type: %T, value: %v", i, item, item)
			}
		}
	}
}
