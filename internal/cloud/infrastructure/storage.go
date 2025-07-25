package infrastructure

import (
	"context"
	"tunnox-core/internal/core/storage"
)

// StorageManager 存储管理器
type StorageManager interface {
	// 获取存储实例
	GetStorage() storage.Storage

	// 初始化存储
	Initialize(ctx context.Context) error

	// 关闭存储
	Close() error

	// 健康检查
	HealthCheck() error
}

// StorageManagerImpl 存储管理器实现
type StorageManagerImpl struct {
	storage storage.Storage
}

// NewStorageManager 创建新的存储管理器
func NewStorageManager(storage storage.Storage) *StorageManagerImpl {
	return &StorageManagerImpl{
		storage: storage,
	}
}

// GetStorage 获取存储实例
func (sm *StorageManagerImpl) GetStorage() storage.Storage {
	return sm.storage
}

// Initialize 初始化存储
func (sm *StorageManagerImpl) Initialize(ctx context.Context) error {
	// 这里可以添加存储初始化逻辑
	return nil
}

// Close 关闭存储
func (sm *StorageManagerImpl) Close() error {
	// 这里可以添加存储关闭逻辑
	return nil
}

// HealthCheck 健康检查
func (sm *StorageManagerImpl) HealthCheck() error {
	// 这里可以添加存储健康检查逻辑
	return nil
}
