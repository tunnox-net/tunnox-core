package distributed

import (
	"fmt"
	"time"
	"tunnox-core/internal/core/storage"
	"tunnox-core/internal/utils"
)

// StorageBasedLock 基于存储的分布式锁实现
type StorageBasedLock struct {
	storage storage.Storage
	owner   string
}

// NewStorageBasedLock 创建基于存储的分布式锁
func NewStorageBasedLock(storage storage.Storage, owner string) *StorageBasedLock {
	return &StorageBasedLock{
		storage: storage,
		owner:   owner,
	}
}

// Acquire 获取锁
func (s *StorageBasedLock) Acquire(key string, ttl time.Duration) (bool, error) {
	lockKey := fmt.Sprintf("lock:%s", key)
	lockValue := fmt.Sprintf("%s:%d", s.owner, time.Now().UnixNano())

	// 使用存储层的原子操作尝试获取锁
	acquired, err := s.storage.SetNX(lockKey, lockValue, ttl)
	if err != nil {
		utils.Errorf("Failed to acquire lock %s: %v", key, err)
		return false, fmt.Errorf("acquire lock failed: %w", err)
	}

	if acquired {
		utils.Infof("Successfully acquired lock %s by %s", key, s.owner)
	}

	return acquired, nil
}

// Release 释放锁
func (s *StorageBasedLock) Release(key string) error {
	lockKey := fmt.Sprintf("lock:%s", key)

	// 获取当前锁的值
	currentValue, err := s.storage.Get(lockKey)
	if err != nil {
		// 锁可能已经不存在或过期
		utils.Infof("Lock %s not found or expired, skipping release", key)
		return nil
	}

	// 检查锁是否属于当前所有者
	if lockValue, ok := currentValue.(string); ok {
		if s.isOwner(lockValue) {
			// 使用原子操作删除锁
			err = s.storage.Delete(lockKey)
			if err != nil {
				utils.Errorf("Failed to release lock %s: %v", key, err)
				return fmt.Errorf("release lock failed: %w", err)
			}
			utils.Infof("Successfully released lock %s by %s", key, s.owner)
		} else {
			utils.Warnf("Attempted to release lock %s owned by another process", key)
			return fmt.Errorf("lock %s is not owned by current process", key)
		}
	}

	return nil
}

// IsLocked 检查锁是否被持有
func (s *StorageBasedLock) IsLocked(key string) (bool, error) {
	lockKey := fmt.Sprintf("lock:%s", key)

	// 直接检查键是否存在，存储层会自动处理过期
	exists, err := s.storage.Exists(lockKey)
	if err != nil {
		return false, fmt.Errorf("check lock status failed: %w", err)
	}

	return exists, nil
}

// GetLockOwner 获取锁的所有者信息
func (s *StorageBasedLock) GetLockOwner(key string) (string, error) {
	lockKey := fmt.Sprintf("lock:%s", key)

	value, err := s.storage.Get(lockKey)
	if err != nil {
		return "", fmt.Errorf("get lock owner failed: %w", err)
	}

	if lockValue, ok := value.(string); ok {
		return s.extractOwner(lockValue), nil
	}

	return "", fmt.Errorf("invalid lock value format")
}

// TryAcquire 尝试获取锁，带重试机制
func (s *StorageBasedLock) TryAcquire(key string, ttl time.Duration, maxRetries int, retryDelay time.Duration) (bool, error) {
	for i := 0; i < maxRetries; i++ {
		acquired, err := s.Acquire(key, ttl)
		if err != nil {
			return false, err
		}

		if acquired {
			return true, nil
		}

		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}

	return false, nil
}

// RenewLock 续期锁
func (s *StorageBasedLock) RenewLock(key string, ttl time.Duration) (bool, error) {
	lockKey := fmt.Sprintf("lock:%s", key)

	// 获取当前锁的值
	currentValue, err := s.storage.Get(lockKey)
	if err != nil {
		return false, fmt.Errorf("lock not found: %w", err)
	}

	if lockValue, ok := currentValue.(string); ok {
		if s.isOwner(lockValue) {
			// 使用原子操作续期锁
			success, err := s.storage.CompareAndSwap(lockKey, lockValue, lockValue, ttl)
			if err != nil {
				return false, fmt.Errorf("renew lock failed: %w", err)
			}

			if success {
				utils.Infof("Successfully renewed lock %s by %s", key, s.owner)
			}

			return success, nil
		}
	}

	return false, fmt.Errorf("lock %s is not owned by current process", key)
}

// isOwner 检查锁值是否属于当前所有者
func (s *StorageBasedLock) isOwner(lockValue string) bool {
	owner := s.extractOwner(lockValue)
	return owner == s.owner
}

// extractOwner 从锁值中提取所有者信息
func (s *StorageBasedLock) extractOwner(lockValue string) string {
	// 锁值格式: "owner:timestamp"
	for i, char := range lockValue {
		if char == ':' {
			return lockValue[:i]
		}
	}
	return lockValue
}
