package storages

import (
	"context"
	"errors"
	"sync"
	"time"
	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/utils"
)

// 存储相关错误
var (
	ErrKeyNotFound = errors.New("key not found")
	ErrInvalidType = errors.New("invalid type")
)

// Storage 存储接口
type Storage interface {
	// 基础操作
	Set(key string, value interface{}, ttl time.Duration) error
	Get(key string) (interface{}, error)
	Delete(key string) error
	Exists(key string) (bool, error)

	// 列表操作
	SetList(key string, values []interface{}, ttl time.Duration) error
	GetList(key string) ([]interface{}, error)
	AppendToList(key string, value interface{}) error
	RemoveFromList(key string, value interface{}) error

	// 哈希操作
	SetHash(key string, field string, value interface{}) error
	GetHash(key string, field string) (interface{}, error)
	GetAllHash(key string) (map[string]interface{}, error)
	DeleteHash(key string, field string) error

	// 计数器操作
	Incr(key string) (int64, error)
	IncrBy(key string, value int64) (int64, error)

	// 过期时间
	SetExpiration(key string, ttl time.Duration) error
	GetExpiration(key string) (time.Duration, error)

	// 清理过期数据
	CleanupExpired() error

	// 关闭存储
	Close() error
}

// MemoryStorage 内存存储实现
type MemoryStorage struct {
	data           map[string]*storageItem
	mu             sync.RWMutex
	cleanupTicker  *time.Ticker
	cleanupStop    chan struct{}
	cleanupRunning bool
	utils.Dispose
}

type storageItem struct {
	value      interface{}
	expiration time.Time
}

// NewMemoryStorage 创建新的内存存储
func NewMemoryStorage(parentCtx context.Context) *MemoryStorage {
	storage := &MemoryStorage{
		data:        make(map[string]*storageItem),
		cleanupStop: make(chan struct{}),
	}
	storage.SetCtx(parentCtx, storage.onClose)
	return storage
}

// onClose 资源释放回调
func (m *MemoryStorage) onClose() {
	m.StopCleanup()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = nil
}

// Set 设置键值对
func (m *MemoryStorage) Set(key string, value interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	expiration := time.Now().Add(ttl)
	m.data[key] = &storageItem{
		value:      value,
		expiration: expiration,
	}
	return nil
}

// Get 获取值
func (m *MemoryStorage) Get(key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	if time.Now().After(item.expiration) {
		delete(m.data, key)
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
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return false, nil
	}

	if time.Now().After(item.expiration) {
		delete(m.data, key)
		return false, nil
	}

	return true, nil
}

// SetList 设置列表
func (m *MemoryStorage) SetList(key string, values []interface{}, ttl time.Duration) error {
	return m.Set(key, values, ttl)
}

// GetList 获取列表
func (m *MemoryStorage) GetList(key string) ([]interface{}, error) {
	value, err := m.Get(key)
	if err != nil {
		return nil, err
	}

	if list, ok := value.([]interface{}); ok {
		return list, nil
	}

	return nil, ErrInvalidType
}

// AppendToList 追加到列表
func (m *MemoryStorage) AppendToList(key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		m.data[key] = &storageItem{
			value:      []interface{}{value},
			expiration: time.Now().Add(constants.DefaultDataTTL),
		}
		return nil
	}

	if time.Now().After(item.expiration) {
		m.data[key] = &storageItem{
			value:      []interface{}{value},
			expiration: time.Now().Add(constants.DefaultDataTTL),
		}
		return nil
	}

	if list, ok := item.value.([]interface{}); ok {
		item.value = append(list, value)
		return nil
	}

	return ErrInvalidType
}

// RemoveFromList 从列表中移除
func (m *MemoryStorage) RemoveFromList(key string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		return nil
	}

	if time.Now().After(item.expiration) {
		delete(m.data, key)
		return nil
	}

	if list, ok := item.value.([]interface{}); ok {
		newList := make([]interface{}, 0, len(list))
		for _, v := range list {
			if v != value {
				newList = append(newList, v)
			}
		}
		item.value = newList
		return nil
	}

	return ErrInvalidType
}

// SetHash 设置哈希字段
func (m *MemoryStorage) SetHash(key string, field string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		item = &storageItem{
			value:      make(map[string]interface{}),
			expiration: time.Now().Add(constants.DefaultDataTTL),
		}
		m.data[key] = item
	}

	if time.Now().After(item.expiration) {
		item.value = make(map[string]interface{})
		item.expiration = time.Now().Add(constants.DefaultDataTTL)
	}

	if hash, ok := item.value.(map[string]interface{}); ok {
		hash[field] = value
		return nil
	}

	return ErrInvalidType
}

// GetHash 获取哈希字段
func (m *MemoryStorage) GetHash(key string, field string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	if time.Now().After(item.expiration) {
		delete(m.data, key)
		return nil, ErrKeyNotFound
	}

	if hash, ok := item.value.(map[string]interface{}); ok {
		if value, exists := hash[field]; exists {
			return value, nil
		}
		return nil, ErrKeyNotFound
	}

	return nil, ErrInvalidType
}

// GetAllHash 获取所有哈希字段
func (m *MemoryStorage) GetAllHash(key string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	if time.Now().After(item.expiration) {
		delete(m.data, key)
		return nil, ErrKeyNotFound
	}

	if hash, ok := item.value.(map[string]interface{}); ok {
		result := make(map[string]interface{})
		for k, v := range hash {
			result[k] = v
		}
		return result, nil
	}

	return nil, ErrInvalidType
}

// DeleteHash 删除哈希字段
func (m *MemoryStorage) DeleteHash(key string, field string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		return nil
	}

	if time.Now().After(item.expiration) {
		delete(m.data, key)
		return nil
	}

	if hash, ok := item.value.(map[string]interface{}); ok {
		delete(hash, field)
		return nil
	}

	return ErrInvalidType
}

// Incr 递增计数器
func (m *MemoryStorage) Incr(key string) (int64, error) {
	return m.IncrBy(key, 1)
}

// IncrBy 按指定值递增
func (m *MemoryStorage) IncrBy(key string, value int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		item = &storageItem{
			value:      int64(0),
			expiration: time.Now().Add(constants.DefaultDataTTL),
		}
		m.data[key] = item
	}

	if time.Now().After(item.expiration) {
		item.value = int64(0)
		item.expiration = time.Now().Add(constants.DefaultDataTTL)
	}

	if counter, ok := item.value.(int64); ok {
		newValue := counter + value
		item.value = newValue
		return newValue, nil
	}

	return 0, ErrInvalidType
}

// SetExpiration 设置过期时间
func (m *MemoryStorage) SetExpiration(key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		return ErrKeyNotFound
	}

	item.expiration = time.Now().Add(ttl)
	return nil
}

// GetExpiration 获取过期时间
func (m *MemoryStorage) GetExpiration(key string) (time.Duration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return 0, ErrKeyNotFound
	}

	if time.Now().After(item.expiration) {
		delete(m.data, key)
		return 0, ErrKeyNotFound
	}

	return time.Until(item.expiration), nil
}

// CleanupExpired 清理过期数据
func (m *MemoryStorage) CleanupExpired() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, item := range m.data {
		if now.After(item.expiration) {
			delete(m.data, key)
		}
	}

	return nil
}

// StartCleanup 启动定时清理协程
func (m *MemoryStorage) StartCleanup(interval time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cleanupRunning {
		return
	}

	m.cleanupTicker = time.NewTicker(interval)
	m.cleanupRunning = true

	go func() {
		for {
			select {
			case <-m.cleanupTicker.C:
				if err := m.CleanupExpired(); err != nil {
					// 记录错误但不中断清理
					// 这里可以添加日志记录
				}
			case <-m.cleanupStop:
				return
			}
		}
	}()
}

// StopCleanup 停止定时清理协程
func (m *MemoryStorage) StopCleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.cleanupRunning {
		return
	}

	if m.cleanupTicker != nil {
		m.cleanupTicker.Stop()
	}
	close(m.cleanupStop)
	m.cleanupRunning = false
}

// Close 关闭存储（实现Storage接口）
func (m *MemoryStorage) Close() error {
	m.Dispose.Close()
	return nil
}
