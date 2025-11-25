package infrastructure

import (
	"context"
	"tunnox-core/internal/core/dispose"
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

// storageManager 存储管理器实现
type storageManager struct {
	*dispose.ManagerBase
	storage storage.Storage
}

// NewStorageManager 创建新的存储管理器
func NewStorageManager(storage storage.Storage, parentCtx context.Context) *storageManager {
	manager := &storageManager{
		ManagerBase: dispose.NewManager("StorageManager", parentCtx),
		storage:     storage,
	}
	return manager
}

// GetStorage 获取存储实例
func (sm *storageManager) GetStorage() storage.Storage {
	return sm.storage
}

// Initialize 初始化存储
func (sm *storageManager) Initialize(ctx context.Context) error {
	// 这里可以添加存储初始化逻辑
	return nil
}

// Close 关闭存储
func (sm *storageManager) Close() error {
	// 这里可以添加存储关闭逻辑
	return nil
}

// HealthCheck 健康检查
func (sm *storageManager) HealthCheck() error {
	// 这里可以添加存储健康检查逻辑
	return nil
}
