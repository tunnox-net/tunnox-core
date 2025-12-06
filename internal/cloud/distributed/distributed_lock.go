package distributed

import (
	"sync"
	"time"
)

// DistributedLock 分布式锁接口
type DistributedLock interface {
	// Acquire 获取锁，返回是否成功获取
	Acquire(key string, ttl time.Duration) (bool, error)
	// Release 释放锁
	Release(key string) error
	// IsLocked 检查锁是否被持有
	IsLocked(key string) (bool, error)
}

// MemoryLock 内存锁实现（用于单机开发/测试）
type MemoryLock struct {
	mu    sync.RWMutex
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
func (m *MemoryLock) Acquire(key string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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
func (m *MemoryLock) Release(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.locks, key)
	return nil
}

// IsLocked 检查内存锁是否被持有
func (m *MemoryLock) IsLocked(key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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
