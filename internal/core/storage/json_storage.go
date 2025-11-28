package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tunnox-core/internal/core/dispose"
)

// JSONStorage JSON 文件持久化存储
// 适合单机部署，数据存储在本地 JSON 文件中
type JSONStorage struct {
	filePath string
	data     map[string]interface{}
	mu       sync.RWMutex

	// 自动保存
	autoSave     bool
	saveInterval time.Duration
	stopChan     chan struct{}
	dirty        bool // 标记是否有未保存的更改
}

// JSONStorageConfig JSON 存储配置
type JSONStorageConfig struct {
	FilePath     string        // JSON 文件路径
	AutoSave     bool          // 是否自动保存
	SaveInterval time.Duration // 自动保存间隔
}

// NewJSONStorage 创建 JSON 存储
func NewJSONStorage(config *JSONStorageConfig) (*JSONStorage, error) {
	if config == nil {
		config = &JSONStorageConfig{
			FilePath:     "data/tunnox-data.json",
			AutoSave:     true,
			SaveInterval: 30 * time.Second,
		}
	}

	// 确保目录存在
	dir := filepath.Dir(config.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	storage := &JSONStorage{
		filePath:     config.FilePath,
		data:         make(map[string]interface{}),
		autoSave:     config.AutoSave,
		saveInterval: config.SaveInterval,
		stopChan:     make(chan struct{}),
	}

	// 加载现有数据
	if err := storage.load(); err != nil {
		dispose.Warnf("JSONStorage: failed to load existing data: %v, starting with empty data", err)
	}

	// 启动自动保存
	if storage.autoSave && storage.saveInterval > 0 {
		go storage.autoSaveLoop()
	}

	dispose.Infof("JSONStorage: initialized with file %s", config.FilePath)
	return storage, nil
}

// load 从文件加载数据
func (j *JSONStorage) load() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	// 检查文件是否存在
	if _, err := os.Stat(j.filePath); os.IsNotExist(err) {
		dispose.Infof("JSONStorage: file %s does not exist, starting with empty data", j.filePath)
		return nil
	}

	// 读取文件
	data, err := os.ReadFile(j.filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// 解析 JSON
	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &j.data); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	dispose.Infof("JSONStorage: loaded %d keys from %s", len(j.data), j.filePath)
	return nil
}

// save 保存数据到文件
func (j *JSONStorage) save() error {
	j.mu.RLock()
	defer j.mu.RUnlock()

	// 序列化为 JSON（格式化输出，便于阅读）
	data, err := json.MarshalIndent(j.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// 写入临时文件
	tempFile := j.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// 原子替换
	if err := os.Rename(tempFile, j.filePath); err != nil {
		os.Remove(tempFile) // 清理临时文件
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// autoSaveLoop 自动保存循环
func (j *JSONStorage) autoSaveLoop() {
	ticker := time.NewTicker(j.saveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			j.mu.RLock()
			dirty := j.dirty
			j.mu.RUnlock()

			if dirty {
				if err := j.save(); err != nil {
					dispose.Errorf("JSONStorage: auto-save failed: %v", err)
				} else {
					j.mu.Lock()
					j.dirty = false
					j.mu.Unlock()
					dispose.Debugf("JSONStorage: auto-saved to %s", j.filePath)
				}
			}
		case <-j.stopChan:
			return
		}
	}
}

// Set 设置键值对
func (j *JSONStorage) Set(key string, value interface{}) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.data[key] = value
	j.dirty = true

	return nil
}

// Get 获取值
func (j *JSONStorage) Get(key string) (interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	value, exists := j.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	return value, nil
}

// Delete 删除键
func (j *JSONStorage) Delete(key string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	delete(j.data, key)
	j.dirty = true

	return nil
}

// Exists 检查键是否存在
func (j *JSONStorage) Exists(key string) (bool, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	_, exists := j.data[key]
	return exists, nil
}

// BatchSet 批量设置
func (j *JSONStorage) BatchSet(items map[string]interface{}) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	for key, value := range items {
		j.data[key] = value
	}
	j.dirty = true

	return nil
}

// BatchGet 批量获取
func (j *JSONStorage) BatchGet(keys []string) (map[string]interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	result := make(map[string]interface{})
	for _, key := range keys {
		if value, exists := j.data[key]; exists {
			result[key] = value
		}
	}

	return result, nil
}

// BatchDelete 批量删除
func (j *JSONStorage) BatchDelete(keys []string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	for _, key := range keys {
		delete(j.data, key)
	}
	j.dirty = true

	return nil
}

// Flush 立即保存到文件
func (j *JSONStorage) Flush() error {
	if err := j.save(); err != nil {
		return err
	}

	j.mu.Lock()
	j.dirty = false
	j.mu.Unlock()

	return nil
}

// GetStats 获取统计信息
func (j *JSONStorage) GetStats() map[string]interface{} {
	j.mu.RLock()
	defer j.mu.RUnlock()

	return map[string]interface{}{
		"file_path": j.filePath,
		"key_count": len(j.data),
		"auto_save": j.autoSave,
		"dirty":     j.dirty,
	}
}

// SetList 设置列表
func (j *JSONStorage) SetList(key string, values []interface{}, ttl time.Duration) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.data[key] = values
	j.dirty = true

	return nil
}

// GetList 获取列表
func (j *JSONStorage) GetList(key string) ([]interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	value, exists := j.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	if list, ok := value.([]interface{}); ok {
		return list, nil
	}

	// 尝试从 JSON 字符串解析（兼容旧数据格式）
	if str, ok := value.(string); ok {
		var list []interface{}
		if err := json.Unmarshal([]byte(str), &list); err == nil {
			return list, nil
		}
	}

	return nil, ErrInvalidType
}

// AppendToList 追加到列表
func (j *JSONStorage) AppendToList(key string, value interface{}) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	existing, exists := j.data[key]
	if !exists {
		j.data[key] = []interface{}{value}
		j.dirty = true
		return nil
	}

	var list []interface{}
	if existingList, ok := existing.([]interface{}); ok {
		list = existingList
	} else if str, ok := existing.(string); ok {
		// 尝试从 JSON 字符串解析（兼容旧数据格式）
		if err := json.Unmarshal([]byte(str), &list); err != nil {
			list = []interface{}{}
		}
	} else {
		list = []interface{}{}
	}

	list = append(list, value)
	j.data[key] = list
	j.dirty = true

	return nil
}

// RemoveFromList 从列表中移除
func (j *JSONStorage) RemoveFromList(key string, value interface{}) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	existing, exists := j.data[key]
	if !exists {
		return nil
	}

	var list []interface{}
	if existingList, ok := existing.([]interface{}); ok {
		list = existingList
	} else if str, ok := existing.(string); ok {
		// 尝试从 JSON 字符串解析（兼容旧数据格式）
		if err := json.Unmarshal([]byte(str), &list); err != nil {
			return nil
		}
	} else {
		return nil
	}

	// 序列化 value 用于比较
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// 移除所有匹配的值
	newList := make([]interface{}, 0, len(list))
	for _, item := range list {
		itemJSON, err := json.Marshal(item)
		if err != nil {
			continue
		}
		if string(itemJSON) != string(valueJSON) {
			newList = append(newList, item)
		}
	}

	j.data[key] = newList
	j.dirty = true

	return nil
}

// SetHash 设置哈希字段
func (j *JSONStorage) SetHash(key string, field string, value interface{}) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	existing, exists := j.data[key]
	var hash map[string]interface{}

	if !exists {
		hash = make(map[string]interface{})
	} else if existingHash, ok := existing.(map[string]interface{}); ok {
		hash = existingHash
	} else {
		hash = make(map[string]interface{})
	}

	hash[field] = value
	j.data[key] = hash
	j.dirty = true

	return nil
}

// GetHash 获取哈希字段
func (j *JSONStorage) GetHash(key string, field string) (interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	value, exists := j.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	hash, ok := value.(map[string]interface{})
	if !ok {
		return nil, ErrInvalidType
	}

	fieldValue, exists := hash[field]
	if !exists {
		return nil, ErrKeyNotFound
	}

	return fieldValue, nil
}

// GetAllHash 获取所有哈希字段
func (j *JSONStorage) GetAllHash(key string) (map[string]interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	value, exists := j.data[key]
	if !exists {
		return nil, ErrKeyNotFound
	}

	hash, ok := value.(map[string]interface{})
	if !ok {
		return nil, ErrInvalidType
	}

	// 返回副本，防止外部修改
	result := make(map[string]interface{})
	for k, v := range hash {
		result[k] = v
	}

	return result, nil
}

// DeleteHash 删除哈希字段
func (j *JSONStorage) DeleteHash(key string, field string) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	existing, exists := j.data[key]
	if !exists {
		return nil
	}

	hash, ok := existing.(map[string]interface{})
	if !ok {
		return nil
	}

	delete(hash, field)
	j.data[key] = hash
	j.dirty = true

	return nil
}

// Incr 递增计数器
func (j *JSONStorage) Incr(key string) (int64, error) {
	return j.IncrBy(key, 1)
}

// IncrBy 按指定值递增计数器
func (j *JSONStorage) IncrBy(key string, value int64) (int64, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	existing, exists := j.data[key]
	var current int64

	if !exists {
		current = 0
	} else {
		switch v := existing.(type) {
		case int64:
			current = v
		case int:
			current = int64(v)
		case float64:
			current = int64(v)
		default:
			current = 0
		}
	}

	current += value
	j.data[key] = current
	j.dirty = true

	return current, nil
}

// SetExpiration 设置过期时间（JSONStorage 不支持 TTL，此方法为空实现）
func (j *JSONStorage) SetExpiration(key string, ttl time.Duration) error {
	// JSONStorage 不支持 TTL，此方法为空实现
	return nil
}

// GetExpiration 获取过期时间（JSONStorage 不支持 TTL，返回 0）
func (j *JSONStorage) GetExpiration(key string) (time.Duration, error) {
	// JSONStorage 不支持 TTL，返回 0
	return 0, nil
}

// CleanupExpired 清理过期数据（JSONStorage 不支持 TTL，此方法为空实现）
func (j *JSONStorage) CleanupExpired() error {
	// JSONStorage 不支持 TTL，此方法为空实现
	return nil
}

// SetNX 原子设置，仅当键不存在时
func (j *JSONStorage) SetNX(key string, value interface{}, ttl time.Duration) (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if _, exists := j.data[key]; exists {
		return false, nil
	}

	j.data[key] = value
	j.dirty = true
	return true, nil
}

// CompareAndSwap 原子比较并交换
func (j *JSONStorage) CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	existing, exists := j.data[key]
	if !exists {
		return false, nil
	}

	// 序列化比较
	oldJSON, err := json.Marshal(oldValue)
	if err != nil {
		return false, err
	}

	existingJSON, err := json.Marshal(existing)
	if err != nil {
		return false, err
	}

	if string(existingJSON) != string(oldJSON) {
		return false, nil
	}

	j.data[key] = newValue
	j.dirty = true
	return true, nil
}

// Watch 监听键变化（JSONStorage 不支持，此方法为空实现）
func (j *JSONStorage) Watch(key string, callback func(interface{})) error {
	// JSONStorage 不支持监听，此方法为空实现
	return nil
}

// Unwatch 取消监听（JSONStorage 不支持，此方法为空实现）
func (j *JSONStorage) Unwatch(key string) error {
	// JSONStorage 不支持监听，此方法为空实现
	return nil
}

// QueryByField 按字段查询（扫描匹配前缀的所有键，解析 JSON，过滤字段）
func (j *JSONStorage) QueryByField(keyPrefix string, fieldName string, fieldValue interface{}) ([]string, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	var results []string

	// 扫描所有匹配前缀的键
	for key, value := range j.data {
		if !strings.HasPrefix(key, keyPrefix) {
			continue
		}

		// 解析 JSON 字符串
		var jsonStr string
		if str, ok := value.(string); ok {
			jsonStr = str
		} else {
			// 如果不是字符串，尝试序列化
			jsonBytes, err := json.Marshal(value)
			if err != nil {
				continue
			}
			jsonStr = string(jsonBytes)
		}

		// 解析为 map 以检查字段
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &obj); err != nil {
			continue
		}

		// 检查字段值是否匹配
		if fieldVal, exists := obj[fieldName]; exists {
			// 类型转换比较
			if matchesFieldValue(fieldVal, fieldValue) {
				results = append(results, jsonStr)
			}
		}
	}

	return results, nil
}

// matchesFieldValue 比较字段值是否匹配（支持 int64、string、float64 等类型）
func matchesFieldValue(actual, expected interface{}) bool {
	// 直接比较
	if actual == expected {
		return true
	}

	// 类型转换比较
	switch expectedVal := expected.(type) {
	case int64:
		switch actualVal := actual.(type) {
		case int64:
			return actualVal == expectedVal
		case float64:
			return int64(actualVal) == expectedVal
		case int:
			return int64(actualVal) == expectedVal
		}
	case string:
		if actualStr, ok := actual.(string); ok {
			return actualStr == expectedVal
		}
	case float64:
		switch actualVal := actual.(type) {
		case float64:
			return actualVal == expectedVal
		case int64:
			return float64(actualVal) == expectedVal
		case int:
			return float64(actualVal) == expectedVal
		}
	}

	return false
}

// Close 关闭存储（保存数据）
func (j *JSONStorage) Close() error {
	// 停止自动保存
	close(j.stopChan)

	// 最后保存一次
	j.mu.RLock()
	dirty := j.dirty
	j.mu.RUnlock()

	if dirty {
		if err := j.save(); err != nil {
			return fmt.Errorf("failed to save on close: %w", err)
		}
		dispose.Infof("JSONStorage: saved %d keys to %s on close", len(j.data), j.filePath)
	}

	return nil
}
