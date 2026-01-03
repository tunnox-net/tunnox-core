package memory

import (
	"encoding/json"
	"strings"
	"time"

	"tunnox-core/internal/cloud/constants"
	"tunnox-core/internal/core/storage/types"
)

// SetList 设置列表
func (m *Storage) SetList(key string, values []any, ttl time.Duration) error {
	return m.Set(key, values, ttl)
}

// GetList 获取列表
func (m *Storage) GetList(key string) ([]any, error) {
	value, err := m.Get(key)
	if err != nil {
		return nil, err
	}

	if list, ok := value.([]any); ok {
		return list, nil
	}

	return nil, types.ErrInvalidType
}

// AppendToList 追加到列表
func (m *Storage) AppendToList(key string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		m.data[key] = &StorageItem{
			Value:      []any{value},
			Expiration: time.Now().Add(constants.DefaultDataTTL),
		}
		return nil
	}

	// 修复：零值时间表示永不过期
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		m.data[key] = &StorageItem{
			Value:      []any{value},
			Expiration: time.Now().Add(constants.DefaultDataTTL),
		}
		return nil
	}

	if list, ok := item.Value.([]any); ok {
		item.Value = append(list, value)
		return nil
	}

	return types.ErrInvalidType
}

// RemoveFromList 从列表中移除
func (m *Storage) RemoveFromList(key string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		return nil
	}

	// 修复：零值时间表示永不过期
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		delete(m.data, key)
		return nil
	}

	if list, ok := item.Value.([]any); ok {
		newList := make([]any, 0, len(list))
		for _, v := range list {
			if v != value {
				newList = append(newList, v)
			}
		}
		item.Value = newList
		return nil
	}

	return types.ErrInvalidType
}

// SetHash 设置哈希字段
func (m *Storage) SetHash(key string, field string, value any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 修复：如果 m.data 为 nil，重新初始化
	if m.data == nil {
		m.data = make(map[string]*StorageItem)
	}

	item, exists := m.data[key]
	if !exists {
		item = &StorageItem{
			Value:      make(map[string]any),
			Expiration: time.Now().Add(constants.DefaultDataTTL),
		}
		m.data[key] = item
	}

	// 修复：零值时间表示永不过期
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		item.Value = make(map[string]any)
		item.Expiration = time.Now().Add(constants.DefaultDataTTL)
	}

	// 如果现有值不是map类型，重新初始化为map
	if hash, ok := item.Value.(map[string]any); ok {
		hash[field] = value
		return nil
	}

	// 如果类型不匹配，重新初始化为map
	item.Value = make(map[string]any)
	hash := item.Value.(map[string]any)
	hash[field] = value
	return nil
}

// GetHash 获取哈希字段
func (m *Storage) GetHash(key string, field string) (any, error) {
	m.mu.RLock()
	item, exists := m.data[key]
	if !exists {
		m.mu.RUnlock()
		return nil, types.ErrKeyNotFound
	}

	// 检查是否过期（在 RLock 下检查，不执行删除）
	expired := !item.Expiration.IsZero() && time.Now().After(item.Expiration)
	m.mu.RUnlock()

	// 如果过期，需要升级为写锁来删除
	if expired {
		m.mu.Lock()
		// 再次检查（可能已被其他 goroutine 删除）
		if item, exists := m.data[key]; exists {
			if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
				delete(m.data, key)
			}
		}
		m.mu.Unlock()
		return nil, types.ErrKeyNotFound
	}

	if hash, ok := item.Value.(map[string]any); ok {
		if value, exists := hash[field]; exists {
			return value, nil
		}
		return nil, types.ErrKeyNotFound
	}

	return nil, types.ErrInvalidType
}

// GetAllHash 获取所有哈希字段
func (m *Storage) GetAllHash(key string) (map[string]any, error) {
	m.mu.RLock()
	item, exists := m.data[key]
	if !exists {
		m.mu.RUnlock()
		return nil, types.ErrKeyNotFound
	}

	// 检查是否过期（在 RLock 下检查，不执行删除）
	expired := !item.Expiration.IsZero() && time.Now().After(item.Expiration)
	m.mu.RUnlock()

	// 如果过期，需要升级为写锁来删除
	if expired {
		m.mu.Lock()
		// 再次检查（可能已被其他 goroutine 删除）
		if item, exists := m.data[key]; exists {
			if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
				delete(m.data, key)
			}
		}
		m.mu.Unlock()
		return nil, types.ErrKeyNotFound
	}

	if hash, ok := item.Value.(map[string]any); ok {
		result := make(map[string]any)
		for k, v := range hash {
			result[k] = v
		}
		return result, nil
	}

	return nil, types.ErrInvalidType
}

// DeleteHash 删除哈希字段
func (m *Storage) DeleteHash(key string, field string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		return nil
	}

	// 修复：零值时间表示永不过期
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		delete(m.data, key)
		return nil
	}

	if hash, ok := item.Value.(map[string]any); ok {
		delete(hash, field)
		return nil
	}

	return types.ErrInvalidType
}

// Incr 递增计数器
func (m *Storage) Incr(key string) (int64, error) {
	return m.IncrBy(key, 1)
}

// IncrBy 按指定值递增
func (m *Storage) IncrBy(key string, value int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		item = &StorageItem{
			Value:      int64(0),
			Expiration: time.Now().Add(constants.DefaultDataTTL),
		}
		m.data[key] = item
	}

	// 修复：零值时间表示永不过期
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		item.Value = int64(0)
		item.Expiration = time.Now().Add(constants.DefaultDataTTL)
	}

	if counter, ok := item.Value.(int64); ok {
		newValue := counter + value
		item.Value = newValue
		return newValue, nil
	}

	return 0, types.ErrInvalidType
}

// SetExpiration 设置过期时间
func (m *Storage) SetExpiration(key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		return types.ErrKeyNotFound
	}

	item.Expiration = time.Now().Add(ttl)
	return nil
}

// GetExpiration 获取过期时间
func (m *Storage) GetExpiration(key string) (time.Duration, error) {
	m.mu.RLock()
	item, exists := m.data[key]
	if !exists {
		m.mu.RUnlock()
		return 0, types.ErrKeyNotFound
	}

	// 检查是否过期（在 RLock 下检查，不执行删除）
	expired := !item.Expiration.IsZero() && time.Now().After(item.Expiration)
	expiration := item.Expiration
	m.mu.RUnlock()

	// 如果过期，需要升级为写锁来删除
	if expired {
		m.mu.Lock()
		// 再次检查（可能已被其他 goroutine 删除）
		if item, exists := m.data[key]; exists {
			if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
				delete(m.data, key)
			}
		}
		m.mu.Unlock()
		return 0, types.ErrKeyNotFound
	}

	return time.Until(expiration), nil
}

// CleanupExpired 清理过期数据
func (m *Storage) CleanupExpired() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for key, item := range m.data {
		// 修复：零值时间表示永不过期
		if !item.Expiration.IsZero() && now.After(item.Expiration) {
			delete(m.data, key)
		}
	}

	return nil
}

// StartCleanup 启动定时清理协程
func (m *Storage) StartCleanup(interval time.Duration) {
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
func (m *Storage) StopCleanup() {
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
// 注意：已过期的键视为不存在，允许重新设置
func (m *Storage) SetNX(key string, value any, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 检查键是否已存在且未过期
	if item, exists := m.data[key]; exists {
		// 如果键存在但已过期，视为不存在，允许覆盖
		if item.Expiration.IsZero() || time.Now().Before(item.Expiration) {
			return false, nil // 键存在且未过期，设置失败
		}
		// 键已过期，删除后继续设置
		delete(m.data, key)
	}

	// 键不存在或已过期，设置成功
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
	return true, nil
}

// CompareAndSwap 原子比较并交换
func (m *Storage) CompareAndSwap(key string, oldValue, newValue any, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, exists := m.data[key]
	if !exists {
		// 键不存在，如果oldValue为nil则设置成功
		if oldValue == nil {
			expiration := time.Now().Add(ttl)
			m.data[key] = &StorageItem{
				Value:      newValue,
				Expiration: expiration,
			}
			return true, nil
		}
		return false, nil // 键不存在但期望值不为nil，交换失败
	}

	// 检查是否过期
	if time.Now().After(item.Expiration) {
		delete(m.data, key)
		if oldValue == nil {
			expiration := time.Now().Add(ttl)
			m.data[key] = &StorageItem{
				Value:      newValue,
				Expiration: expiration,
			}
			return true, nil
		}
		return false, nil
	}

	// 比较当前值
	if item.Value != oldValue {
		return false, nil // 值不匹配，交换失败
	}

	// 值匹配，执行交换
	item.Value = newValue
	item.Expiration = time.Now().Add(ttl)
	return true, nil
}

// Watch 监听键变化（简化实现，实际应该支持事件通知）
func (m *Storage) Watch(key string, callback func(any)) error {
	// 简化实现：立即执行一次回调
	if item, exists := m.data[key]; exists && time.Now().Before(item.Expiration) {
		callback(item.Value)
	}
	return nil
}

// Unwatch 取消监听
func (m *Storage) Unwatch(key string) error {
	// 简化实现：无操作
	return nil
}

// Close 关闭存储（实现Storage接口）
func (m *Storage) Close() error {
	m.Dispose.Close()
	return nil
}

// QueryByPrefix 按前缀查询所有键值对
// prefix: 键前缀（如 "tunnox:persist:client:config:"）
// limit: 返回结果数量限制，0 表示无限制
// 返回：map[key]jsonValue，key 是完整键名，jsonValue 是 JSON 序列化的值
func (m *Storage) QueryByPrefix(prefix string, limit int) (map[string]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.data == nil {
		return make(map[string]string), nil
	}

	result := make(map[string]string)
	count := 0
	now := time.Now()

	for key, item := range m.data {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		// 检查是否过期
		if !item.Expiration.IsZero() && now.After(item.Expiration) {
			continue
		}

		// 序列化值为 JSON 字符串
		var jsonStr string
		if str, ok := item.Value.(string); ok {
			jsonStr = str
		} else {
			jsonBytes, err := json.Marshal(item.Value)
			if err != nil {
				continue
			}
			jsonStr = string(jsonBytes)
		}

		result[key] = jsonStr
		count++

		// 检查限制
		if limit > 0 && count >= limit {
			break
		}
	}

	return result, nil
}
