package repos

import (
	"context"
	"testing"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/storage"
)

// TestPortMappingRepo_GetClientPortMappings_CacheMiss 测试 mapping 索引在缓存 miss 后能否恢复
// 这个测试复现了 mapping 丢失的 bug
func TestPortMappingRepo_GetClientPortMappings_CacheMiss(t *testing.T) {
	ctx := context.Background()

	// 使用临时文件作为持久化存储
	tempFile := "/tmp/test_mapping_repo.json"
	persistent, err := storage.NewJSONStorage(&storage.JSONStorageConfig{
		FilePath:     tempFile,
		AutoSave:     true,
		SaveInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewJSONStorage failed: %v", err)
	}
	defer persistent.Close()

	// 创建本地缓存
	cache := storage.NewMemoryStorage(ctx)

	// 使用默认配置
	config := storage.DefaultHybridConfig()
	config.EnablePersistent = true

	hybridStorage := storage.NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)
	defer hybridStorage.Close()

	// 创建 Repository
	baseRepo := NewRepository(hybridStorage)
	mappingRepo := NewPortMappingRepo(baseRepo)

	// 创建测试 mapping
	clientID := int64(12345678)
	clientIDStr := "12345678"

	mapping := &models.PortMapping{
		ID:             "pmap_test001",
		ListenClientID: 99999999,
		TargetClientID: clientID,
		Protocol:       models.ProtocolTCP,
		SourcePort:     13306,
		TargetHost:     "127.0.0.1",
		TargetPort:     3306,
		ListenAddress:  "0.0.0.0:13306",
		TargetAddress:  "tcp://127.0.0.1:3306",
		Status:         models.MappingStatusActive,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// 保存 mapping
	if err := mappingRepo.SavePortMapping(mapping); err != nil {
		t.Fatalf("SavePortMapping failed: %v", err)
	}

	// 验证写入后立即读取正常
	mappings, err := mappingRepo.GetClientPortMappings(clientIDStr)
	if err != nil {
		t.Fatalf("GetClientPortMappings after write failed: %v", err)
	}
	if len(mappings) != 1 {
		t.Errorf("Expected 1 mapping, got %d", len(mappings))
	}
	t.Logf("After write: got %d mappings", len(mappings))

	// 清空本地缓存中的索引（模拟缓存过期）
	indexKey := "tunnox:client_mappings:" + clientIDStr
	if err := cache.Delete(indexKey); err != nil && err != storage.ErrKeyNotFound {
		t.Logf("cache.Delete warning: %v", err)
	}

	// 重新查询（应该从持久化存储恢复）
	mappings, err = mappingRepo.GetClientPortMappings(clientIDStr)
	if err != nil {
		t.Fatalf("GetClientPortMappings after cache miss failed: %v", err)
	}

	if len(mappings) != 1 {
		t.Errorf("After cache miss: expected 1 mapping, got %d", len(mappings))

		// 调试：直接从持久化存储读取索引
		persistentValue, persistentErr := persistent.Get(indexKey)
		if persistentErr != nil {
			t.Logf("Persistent Get error: %v", persistentErr)
		} else {
			t.Logf("Persistent value type: %T", persistentValue)
			t.Logf("Persistent value: %v", persistentValue)
		}
	} else {
		t.Logf("After cache miss: got %d mappings (correct!)", len(mappings))
		if mappings[0].ID != mapping.ID {
			t.Errorf("Expected mapping ID %s, got %s", mapping.ID, mappings[0].ID)
		}
	}
}

// TestPortMappingRepo_GetClientPortMappings_SimulateRestart 模拟服务重启场景
func TestPortMappingRepo_GetClientPortMappings_SimulateRestart(t *testing.T) {
	ctx := context.Background()

	tempFile := "/tmp/test_mapping_repo_restart.json"
	clientID := int64(88888888)
	clientIDStr := "88888888"
	mappingID := "pmap_restart_test"

	// 第一阶段：写入数据
	{
		persistent, err := storage.NewJSONStorage(&storage.JSONStorageConfig{
			FilePath:     tempFile,
			AutoSave:     true,
			SaveInterval: 100 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("NewJSONStorage failed: %v", err)
		}

		cache := storage.NewMemoryStorage(ctx)
		config := storage.DefaultHybridConfig()
		config.EnablePersistent = true

		hybridStorage := storage.NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)

		baseRepo := NewRepository(hybridStorage)
		mappingRepo := NewPortMappingRepo(baseRepo)

		mapping := &models.PortMapping{
			ID:             mappingID,
			ListenClientID: 77777777,
			TargetClientID: clientID,
			Protocol:       models.ProtocolTCP,
			SourcePort:     13307,
			TargetHost:     "127.0.0.1",
			TargetPort:     3307,
			ListenAddress:  "0.0.0.0:13307",
			TargetAddress:  "tcp://127.0.0.1:3307",
			Status:         models.MappingStatusActive,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}

		if err := mappingRepo.SavePortMapping(mapping); err != nil {
			t.Fatalf("SavePortMapping failed: %v", err)
		}

		// 验证写入成功
		mappings, err := mappingRepo.GetClientPortMappings(clientIDStr)
		if err != nil {
			t.Fatalf("GetClientPortMappings failed: %v", err)
		}
		if len(mappings) != 1 {
			t.Fatalf("Expected 1 mapping, got %d", len(mappings))
		}
		t.Logf("Phase 1: wrote %d mappings", len(mappings))

		// 等待自动保存
		time.Sleep(200 * time.Millisecond)

		// 关闭存储
		hybridStorage.Close()
		persistent.Close()
	}

	// 第二阶段：重新打开，验证数据恢复
	{
		persistent, err := storage.NewJSONStorage(&storage.JSONStorageConfig{
			FilePath:     tempFile,
			AutoSave:     true,
			SaveInterval: 100 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("NewJSONStorage (restart) failed: %v", err)
		}
		defer persistent.Close()

		cache := storage.NewMemoryStorage(ctx)
		config := storage.DefaultHybridConfig()
		config.EnablePersistent = true

		hybridStorage := storage.NewHybridStorageWithSharedCache(ctx, cache, nil, persistent, config)
		defer hybridStorage.Close()

		baseRepo := NewRepository(hybridStorage)
		mappingRepo := NewPortMappingRepo(baseRepo)

		// 从新实例读取
		mappings, err := mappingRepo.GetClientPortMappings(clientIDStr)
		if err != nil {
			t.Fatalf("GetClientPortMappings after restart failed: %v", err)
		}

		if len(mappings) != 1 {
			t.Errorf("After restart: expected 1 mapping, got %d", len(mappings))

			// 调试
			indexKey := "tunnox:client_mappings:" + clientIDStr
			persistentValue, persistentErr := persistent.Get(indexKey)
			if persistentErr != nil {
				t.Logf("Persistent Get error: %v", persistentErr)
			} else {
				t.Logf("Persistent value type: %T", persistentValue)
				t.Logf("Persistent value: %v", persistentValue)
			}
		} else {
			t.Logf("After restart: got %d mappings (correct!)", len(mappings))
			if mappings[0].ID != mappingID {
				t.Errorf("Expected mapping ID %s, got %s", mappingID, mappings[0].ID)
			}
		}
	}
}
