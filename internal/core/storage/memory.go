package storage

import (
	"context"
	"sync"
	"time"
	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/utils"
)

// MemoryStorage 内存存储实现
type MemoryStorage struct {
	data           map[string]*storageItem
	mu             sync.RWMutex
	cleanupTicker  *time.Ticker
	cleanupStop    chan struct{}
	cleanupRunning bool
	dispose.Dispose
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
	utils.Infof("MemoryStorage.Set: stored key %s, value type: %T, expiration: %v", key, value, expiration)
	return nil
}

// Get 获取值
func (m *MemoryStorage) Get(key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	utils.Infof("MemoryStorage.Get: retrieving key %s, data map size: %d", key, len(m.data))
	if m.data == nil {
		utils.Debugf("MemoryStorage.Get: data map is nil for key %s", key)
		return nil, ErrKeyNotFound
	}

	item, exists := m.data[key]
	if !exists {
		utils.Debugf("MemoryStorage.Get: key %s not found in data map", key)
		return nil, ErrKeyNotFound
	}

	// 只有 expiration 非零且已过期才删除
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		utils.Infof("MemoryStorage.Get: key %s expired, deleting", key)
		delete(m.data, key)
		return nil, ErrKeyNotFound
	}

	utils.Infof("MemoryStorage.Get: successfully retrieved key %s, value type: %T", key, item.value)
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

	utils.Infof("MemoryStorage.Exists: checking key %s, data map size: %d", key, len(m.data))
	if m.data == nil {
		utils.Debugf("MemoryStorage.Exists: data map is nil for key %s", key)
		return false, nil
	}

	item, exists := m.data[key]
	if !exists {
		utils.Debugf("MemoryStorage.Exists: key %s not found in data map", key)
		return false, nil
	}

	// 修复：零值时间表示永不过期
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		utils.Infof("MemoryStorage.Exists: key %s expired, deleting", key)
		delete(m.data, key)
		return false, nil
	}

	utils.Infof("MemoryStorage.Exists: key %s exists and not expired", key)
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

	// 修复：零值时间表示永不过期
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
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

	// 修复：零值时间表示永不过期
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
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

	// 修复：如果 m.data 为 nil，重新初始化
	if m.data == nil {
		m.data = make(map[string]*storageItem)
	}

	item, exists := m.data[key]
	if !exists {
		item = &storageItem{
			value:      make(map[string]interface{}),
			expiration: time.Now().Add(constants.DefaultDataTTL),
		}
		m.data[key] = item
	}

	// 修复：零值时间表示永不过期
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		item.value = make(map[string]interface{})
		item.expiration = time.Now().Add(constants.DefaultDataTTL)
	}

	// 如果现有值不是map类型，重新初始化为map
	if hash, ok := item.value.(map[string]interface{}); ok {
		hash[field] = value
		return nil
	}

	// 如果类型不匹配，重新初始化为map
	item.value = make(map[string]interface{})
	hash := item.value.(map[string]interface{})
	hash[field] = value
	return nil
}

// GetHash 获取哈希字段
func (m *MemoryStorage) GetHash(key string, field string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, exists := m.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	// 修复：零值时间表示永不过期
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
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

	// 修复：零值时间表示永不过期
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
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

	// 修复：零值时间表示永不过期
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
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

	// 修复：零值时间表示永不过期
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
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

	// 修复：零值时间表示永不过期
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
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
		// 修复：零值时间表示永不过期
		if !item.expiration.IsZero() && now.After(item.expiration) {
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

// SetNX 原子设置，仅当键不存在时
func (m *MemoryStorage) SetNX(key string, value interface{}, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查键是否已存在
	if _, exists := m.data[key]; exists {
		return false, nil // 键已存在，设置失败
	}

	// 键不存在，设置成功
	expiration := time.Now().Add(ttl)
	m.data[key] = &storageItem{
		value:      value,
		expiration: expiration,
	}
	return true, nil
}

// CompareAndSwap 原子比较并交换
func (m *MemoryStorage) CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		// 键不存在，如果oldValue为nil则设置成功
		if oldValue == nil {
			expiration := time.Now().Add(ttl)
			m.data[key] = &storageItem{
				value:      newValue,
				expiration: expiration,
			}
			return true, nil
		}
		return false, nil // 键不存在但期望值不为nil，交换失败
	}

	// 检查是否过期
	if time.Now().After(item.expiration) {
		delete(m.data, key)
		if oldValue == nil {
			expiration := time.Now().Add(ttl)
			m.data[key] = &storageItem{
				value:      newValue,
				expiration: expiration,
			}
			return true, nil
		}
		return false, nil
	}

	// 比较当前值
	if item.value != oldValue {
		return false, nil // 值不匹配，交换失败
	}

	// 值匹配，执行交换
	item.value = newValue
	item.expiration = time.Now().Add(ttl)
	return true, nil
}

// Watch 监听键变化（简化实现，实际应该支持事件通知）
func (m *MemoryStorage) Watch(key string, callback func(interface{})) error {
	// 简化实现：立即执行一次回调
	if item, exists := m.data[key]; exists && time.Now().Before(item.expiration) {
		callback(item.value)
	}
	return nil
}

// Unwatch 取消监听
func (m *MemoryStorage) Unwatch(key string) error {
	// 简化实现：无操作
	return nil
}

// Close 关闭存储（实现Storage接口）
func (m *MemoryStorage) Close() error {
	m.Dispose.Close()
	return nil
}
