package health

import (
	"context"
	"fmt"

	"tunnox-core/internal/core/storage"
)

// StorageAdapter 存储适配器（将 Storage 接口适配为 StorageChecker）
type StorageAdapter struct {
	storage storage.Storage
}

// NewStorageAdapter 创建存储适配器
func NewStorageAdapter(s storage.Storage) *StorageAdapter {
	return &StorageAdapter{storage: s}
}

// Ping 检查存储连接（使用 Exists 方法）
func (a *StorageAdapter) Ping(ctx context.Context) error {
	if a.storage == nil {
		return fmt.Errorf("storage is nil")
	}

	// 使用一个测试键来检查存储是否可用
	testKey := "tunnox:health:check"
	_, err := a.storage.Exists(testKey)
	return err
}

