package cloud

import (
	"context"
	"time"
)

// DistributedLock 分布式锁接口
type DistributedLock interface {
	// Acquire 获取锁，返回是否成功获取
	Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error)
	// Release 释放锁
	Release(ctx context.Context, key string) error
	// IsLocked 检查锁是否被持有
	IsLocked(ctx context.Context, key string) (bool, error)
}

// MemoryLock 内存锁实现（用于单机开发/测试）
type MemoryLock struct {
	locks map[string]*lockInfo
}

type lockInfo struct {
	owner     string
	expiresAt time.Time
}

// NewMemoryLock 创建内存锁
func NewMemoryLock() *MemoryLock {
	return &MemoryLock{
		locks: make(map[string]*lockInfo),
	}
}

// Acquire 获取内存锁
func (m *MemoryLock) Acquire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	now := time.Now()

	// 检查锁是否存在且未过期
	if info, exists := m.locks[key]; exists {
		if now.Before(info.expiresAt) {
			return false, nil // 锁被持有
		}
		// 锁已过期，删除
		delete(m.locks, key)
	}

	// 创建新锁
	m.locks[key] = &lockInfo{
		owner:     "memory-lock", // 简化实现，实际应该有唯一标识
		expiresAt: now.Add(ttl),
	}

	return true, nil
}

// Release 释放内存锁
func (m *MemoryLock) Release(ctx context.Context, key string) error {
	delete(m.locks, key)
	return nil
}

// IsLocked 检查内存锁是否被持有
func (m *MemoryLock) IsLocked(ctx context.Context, key string) (bool, error) {
	now := time.Now()

	if info, exists := m.locks[key]; exists {
		if now.Before(info.expiresAt) {
			return true, nil // 锁被持有
		}
		// 锁已过期，删除
		delete(m.locks, key)
	}

	return false, nil
}
