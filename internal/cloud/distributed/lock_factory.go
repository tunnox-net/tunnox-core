package distributed

import (
	"fmt"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/utils"
)

// LockType 锁类型
type LockType string

const (
	LockTypeMemory  LockType = "memory"  // 内存锁
	LockTypeStorage LockType = "storage" // 基于存储的锁
)

// LockFactory 分布式锁工厂
type LockFactory struct {
	storage storages.Storage
}

// NewLockFactory 创建分布式锁工厂
func NewLockFactory(storage storages.Storage) *LockFactory {
	return &LockFactory{
		storage: storage,
	}
}

// CreateLock 创建分布式锁
func (f *LockFactory) CreateLock(lockType LockType, owner string) (DistributedLock, error) {
	switch lockType {
	case LockTypeMemory:
		utils.Infof("Creating memory-based distributed lock")
		return NewMemoryLock(), nil
	case LockTypeStorage:
		utils.Infof("Creating storage-based distributed lock for owner: %s", owner)
		return NewStorageBasedLock(f.storage, owner), nil
	default:
		return nil, fmt.Errorf("unsupported lock type: %s", lockType)
	}
}

// CreateDefaultLock 创建默认分布式锁（基于存储）
func (f *LockFactory) CreateDefaultLock(owner string) DistributedLock {
	lock, err := f.CreateLock(LockTypeStorage, owner)
	if err != nil {
		utils.Errorf("Failed to create default lock, falling back to memory lock: %v", err)
		return NewMemoryLock()
	}
	return lock
}

// CreateLockWithRetry 创建带重试机制的分布式锁
func (f *LockFactory) CreateLockWithRetry(lockType LockType, owner string, maxRetries int) (DistributedLock, error) {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		lock, err := f.CreateLock(lockType, owner)
		if err == nil {
			return lock, nil
		}
		lastErr = err
		utils.Warnf("Failed to create lock (attempt %d/%d): %v", i+1, maxRetries, err)
	}

	return nil, fmt.Errorf("failed to create lock after %d attempts: %w", maxRetries, lastErr)
}
