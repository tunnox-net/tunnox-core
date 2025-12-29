package storage

import (
	"context"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
)

// storageItem 存储项
type storageItem struct {
	value      interface{}
	expiration time.Time
}

// MemoryStorage 内存存储实现
type MemoryStorage struct {
	*dispose.ManagerBase
	data           map[string]*storageItem
	mu             sync.RWMutex
	cleanupTicker  *time.Ticker
	cleanupStop    chan struct{}
	cleanupRunning bool
}

// NewMemoryStorage 创建内存存储
func NewMemoryStorage(parentCtx context.Context) Storage {
	storage := &MemoryStorage{
		ManagerBase: dispose.NewManager("MemoryStorage", parentCtx),
		data:        make(map[string]*storageItem),
		cleanupStop: make(chan struct{}),
	}
	return storage
}

// onClose 资源释放回调
func (m *MemoryStorage) onClose() error {
	m.StopCleanup()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = nil
	return nil
}

// Set 设置键值对
func (m *MemoryStorage) Set(key string, value interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 修复：如果 m.data 为 nil，重新初始化
	if m.data == nil {
		m.data = make(map[string]*storageItem)
	}

	var expiration time.Time
	if ttl <= 0 {
		expiration = time.Time{} // 零值，表示永不过期
	} else {
		expiration = time.Now().Add(ttl)
	}

	m.data[key] = &storageItem{
		value:      value,
		expiration: expiration,
	}
	// Stored successfully
	return nil
}

// Get 获取值
func (m *MemoryStorage) Get(key string) (interface{}, error) {
	m.mu.RLock()
	if m.data == nil {
		m.mu.RUnlock()
		return nil, ErrKeyNotFound
	}

	item, exists := m.data[key]
	if !exists {
		m.mu.RUnlock()
		return nil, ErrKeyNotFound
	}

	// 检查是否过期（在 RLock 下检查，不执行删除）
	expired := !item.expiration.IsZero() && time.Now().After(item.expiration)
	m.mu.RUnlock()

	// 如果过期，需要升级为写锁来删除
	if expired {
		m.mu.Lock()
		// 再次检查（可能已被其他 goroutine 删除）
		if item, exists := m.data[key]; exists {
			if !item.expiration.IsZero() && time.Now().After(item.expiration) {
				delete(m.data, key)
			}
		}
		m.mu.Unlock()
		return nil, ErrKeyNotFound
	}
	return item.value, nil
}

// Delete 删除键
func (m *MemoryStorage) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

// Exists 检查键是否存在
func (m *MemoryStorage) Exists(key string) (bool, error) {
	m.mu.RLock()
	if m.data == nil {
		m.mu.RUnlock()
		return false, nil
	}

	item, exists := m.data[key]
	if !exists {
		m.mu.RUnlock()
		return false, nil
	}

	// 检查是否过期（在 RLock 下检查，不执行删除）
	expired := !item.expiration.IsZero() && time.Now().After(item.expiration)
	m.mu.RUnlock()

	// 如果过期，需要升级为写锁来删除
	if expired {
		m.mu.Lock()
		// 再次检查（可能已被其他 goroutine 删除）
		if item, exists := m.data[key]; exists {
			if !item.expiration.IsZero() && time.Now().After(item.expiration) {
				delete(m.data, key)
			}
		}
		m.mu.Unlock()
		return false, nil
	}
	return true, nil
}
