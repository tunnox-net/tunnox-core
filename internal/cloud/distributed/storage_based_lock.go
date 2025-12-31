package distributed

import (
	"fmt"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/storage"
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
	casStore, ok := s.storage.(storage.CASStore)
	if !ok {
		return false, coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support CAS operations")
	}
	acquired, err := casStore.SetNX(lockKey, lockValue, ttl)
	if err != nil {
		corelog.Errorf("Failed to acquire lock %s: %v", key, err)
		return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "acquire lock failed")
	}

	if acquired {
		corelog.Infof("Successfully acquired lock %s by %s", key, s.owner)
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
		corelog.Infof("Lock %s not found or expired, skipping release", key)
		return nil
	}

	// 检查锁是否属于当前所有者
	if lockValue, ok := currentValue.(string); ok {
		if s.isOwner(lockValue) {
			// 使用原子操作删除锁
			err = s.storage.Delete(lockKey)
			if err != nil {
				corelog.Errorf("Failed to release lock %s: %v", key, err)
				return coreerrors.Wrap(err, coreerrors.CodeStorageError, "release lock failed")
			}
			corelog.Infof("Successfully released lock %s by %s", key, s.owner)
		} else {
			corelog.Warnf("Attempted to release lock %s owned by another process", key)
			return coreerrors.Newf(coreerrors.CodeForbidden, "lock %s is not owned by current process", key)
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
		return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "check lock status failed")
	}

	return exists, nil
}

// GetLockOwner 获取锁的所有者信息
func (s *StorageBasedLock) GetLockOwner(key string) (string, error) {
	lockKey := fmt.Sprintf("lock:%s", key)

	value, err := s.storage.Get(lockKey)
	if err != nil {
		return "", coreerrors.Wrap(err, coreerrors.CodeStorageError, "get lock owner failed")
	}

	if lockValue, ok := value.(string); ok {
		return s.extractOwner(lockValue), nil
	}

	return "", coreerrors.New(coreerrors.CodeInvalidData, "invalid lock value format")
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
		return false, coreerrors.Wrap(err, coreerrors.CodeNotFound, "lock not found")
	}

	if lockValue, ok := currentValue.(string); ok {
		if s.isOwner(lockValue) {
			// 使用原子操作续期锁
			casStore, ok := s.storage.(storage.CASStore)
			if !ok {
				return false, coreerrors.New(coreerrors.CodeNotConfigured, "storage does not support CAS operations")
			}
			success, err := casStore.CompareAndSwap(lockKey, lockValue, lockValue, ttl)
			if err != nil {
				return false, coreerrors.Wrap(err, coreerrors.CodeStorageError, "renew lock failed")
			}

			if success {
				corelog.Infof("Successfully renewed lock %s by %s", key, s.owner)
			}

			return success, nil
		}
	}

	return false, coreerrors.Newf(coreerrors.CodeForbidden, "lock %s is not owned by current process", key)
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
