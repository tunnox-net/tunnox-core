package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
