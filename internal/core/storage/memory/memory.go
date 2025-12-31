package memory

import (
	"context"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/core/storage/types"
)

// StorageItem 存储项
type StorageItem struct {
	Value      any
	Expiration time.Time
}

// Storage 内存存储实现
type Storage struct {
	*dispose.ManagerBase
	data           map[string]*StorageItem
	mu             sync.RWMutex
	cleanupTicker  *time.Ticker
	cleanupStop    chan struct{}
	cleanupRunning bool
}

// New 创建内存存储
func New(parentCtx context.Context) *Storage {
	storage := &Storage{
		ManagerBase: dispose.NewManager("MemoryStorage", parentCtx),
		data:        make(map[string]*StorageItem),
		cleanupStop: make(chan struct{}),
	}
	// 绑定 onClose 回调，确保资源正确释放
	storage.AddCleanHandler(storage.onClose)
	return storage
}

// onClose 资源释放回调
func (m *Storage) onClose() error {
	m.StopCleanup()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = nil
	return nil
}

// Set 设置键值对
func (m *Storage) Set(key string, value any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 修复：如果 m.data 为 nil，重新初始化
	if m.data == nil {
		m.data = make(map[string]*StorageItem)
	}

	var expiration time.Time
	if ttl <= 0 {
		expiration = time.Time{} // 零值，表示永不过期
	} else {
		expiration = time.Now().Add(ttl)
	}

	m.data[key] = &StorageItem{
		Value:      value,
		Expiration: expiration,
	}
	// Stored successfully
	return nil
}

// Get 获取值
// 采用懒删除策略：检测到过期时只返回"不存在"，不执行删除
// 删除操作由后台 CleanupExpired 定时器统一处理
func (m *Storage) Get(key string) (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.data == nil {
		return nil, types.ErrKeyNotFound
	}

	item, exists := m.data[key]
	if !exists {
		return nil, types.ErrKeyNotFound
	}

	// 检查是否过期，过期则返回不存在（懒删除，不升级写锁）
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		return nil, types.ErrKeyNotFound
	}

	return item.Value, nil
}

// Delete 删除键
func (m *Storage) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return nil
}

// Exists 检查键是否存在
// 采用懒删除策略：检测到过期时只返回 false，不执行删除
// 删除操作由后台 CleanupExpired 定时器统一处理
func (m *Storage) Exists(key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.data == nil {
		return false, nil
	}

	item, exists := m.data[key]
	if !exists {
		return false, nil
	}

	// 检查是否过期，过期则返回不存在（懒删除，不升级写锁）
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		return false, nil
	}

	return true, nil
}
