package json

import (
	"encoding/json"
	"strings"
	"time"

	"tunnox-core/internal/core/storage/types"
)

// SetList 设置列表
func (j *Storage) SetList(key string, values []interface{}, ttl time.Duration) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.data[key] = values
	j.dirty = true

	return nil
}

// GetList 获取列表
func (j *Storage) GetList(key string) ([]interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	value, exists := j.data[key]
	if !exists {
		return nil, types.ErrKeyNotFound
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

	return nil, types.ErrInvalidType
}

// AppendToList 追加到列表
func (j *Storage) AppendToList(key string, value interface{}) error {
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
func (j *Storage) RemoveFromList(key string, value interface{}) error {
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
func (j *Storage) SetHash(key string, field string, value interface{}) error {
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
func (j *Storage) GetHash(key string, field string) (interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	value, exists := j.data[key]
	if !exists {
		return nil, types.ErrKeyNotFound
	}

	hash, ok := value.(map[string]interface{})
	if !ok {
		return nil, types.ErrInvalidType
	}

	fieldValue, exists := hash[field]
	if !exists {
		return nil, types.ErrKeyNotFound
	}

	return fieldValue, nil
}

// GetAllHash 获取所有哈希字段
func (j *Storage) GetAllHash(key string) (map[string]interface{}, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	value, exists := j.data[key]
	if !exists {
		return nil, types.ErrKeyNotFound
	}

	hash, ok := value.(map[string]interface{})
	if !ok {
		return nil, types.ErrInvalidType
	}

	// 返回副本，防止外部修改
	result := make(map[string]interface{})
	for k, v := range hash {
		result[k] = v
	}

	return result, nil
}

// DeleteHash 删除哈希字段
func (j *Storage) DeleteHash(key string, field string) error {
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
func (j *Storage) Incr(key string) (int64, error) {
	return j.IncrBy(key, 1)
}

// IncrBy 按指定值递增计数器
func (j *Storage) IncrBy(key string, value int64) (int64, error) {
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
func (j *Storage) SetExpiration(key string, ttl time.Duration) error {
	// JSONStorage 不支持 TTL，此方法为空实现
	return nil
}

// GetExpiration 获取过期时间（JSONStorage 不支持 TTL，返回 0）
func (j *Storage) GetExpiration(key string) (time.Duration, error) {
	// JSONStorage 不支持 TTL，返回 0
	return 0, nil
}

// CleanupExpired 清理过期数据（JSONStorage 不支持 TTL，此方法为空实现）
func (j *Storage) CleanupExpired() error {
	// JSONStorage 不支持 TTL，此方法为空实现
	return nil
}

// SetNX 原子设置，仅当键不存在时
func (j *Storage) SetNX(key string, value interface{}, ttl time.Duration) (bool, error) {
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
func (j *Storage) CompareAndSwap(key string, oldValue, newValue interface{}, ttl time.Duration) (bool, error) {
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
func (j *Storage) Watch(key string, callback func(interface{})) error {
	// JSONStorage 不支持监听，此方法为空实现
	return nil
}

// Unwatch 取消监听（JSONStorage 不支持，此方法为空实现）
func (j *Storage) Unwatch(key string) error {
	// JSONStorage 不支持监听，此方法为空实现
	return nil
}

// QueryByField 按字段查询（扫描匹配前缀的所有键，解析 JSON，过滤字段）
func (j *Storage) QueryByField(keyPrefix string, fieldName string, fieldValue interface{}) ([]string, error) {
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

// QueryByPrefix 按前缀查询所有键值对
// prefix: 键前缀（如 "tunnox:persist:client:config:"）
// limit: 返回结果数量限制，0 表示无限制
// 返回：map[key]jsonValue，key 是完整键名，jsonValue 是 JSON 序列化的值
func (j *Storage) QueryByPrefix(prefix string, limit int) (map[string]string, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	result := make(map[string]string)
	count := 0

	for key, value := range j.data {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		// 序列化值为 JSON 字符串
		var jsonStr string
		if str, ok := value.(string); ok {
			jsonStr = str
		} else {
			jsonBytes, err := json.Marshal(value)
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
