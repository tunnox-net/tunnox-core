package managers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/cloud/storages"
	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
)

// ConfigManager 配置管理器
type ConfigManager struct {
	*dispose.ResourceBase
	storage  storages.Storage
	config   *ControlConfig
	watchers []ConfigWatcher
	mu       sync.RWMutex
}

// ConfigWatcher 配置变更监听器
type ConfigWatcher interface {
	OnConfigChanged(config *ControlConfig)
}

// NewConfigManager 创建新的配置管理器
func NewConfigManager(storage storages.Storage, config *ControlConfig, ctx context.Context) *ConfigManager {
	manager := &ConfigManager{
		ResourceBase: dispose.NewResourceBase("ConfigManager"),
		storage:      storage,
		config:       config,
		watchers:     make([]ConfigWatcher, 0),
	}
	manager.Initialize(ctx)
	return manager
}

// GetConfig 获取当前配置
func (cm *ConfigManager) GetConfig() *ControlConfig {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.config
}

// UpdateConfig 更新配置
func (cm *ConfigManager) UpdateConfig(ctx context.Context, newConfig *ControlConfig) error {
	// 保存配置到存储
	data, err := json.Marshal(newConfig)
	if err != nil {
		return fmt.Errorf("marshal config failed: %w", err)
	}

	key := fmt.Sprintf("%s:config", constants.KeyPrefixConfig)
	if err := cm.storage.Set(key, string(data), 0); err != nil {
		return fmt.Errorf("save config failed: %w", err)
	}

	// 更新内存配置
	cm.mu.Lock()
	cm.config = newConfig
	cm.mu.Unlock()

	// 通知监听器
	cm.notifyWatchers(newConfig)

	return nil
}

// LoadConfig 从存储加载配置
func (cm *ConfigManager) LoadConfig(ctx context.Context) error {
	key := fmt.Sprintf("%s:config", constants.KeyPrefixConfig)
	data, err := cm.storage.Get(key)
	if err != nil {
		// 配置不存在，使用默认配置
		return nil
	}

	configData, ok := data.(string)
	if !ok {
		return fmt.Errorf("invalid config data type")
	}

	var config ControlConfig
	if err := json.Unmarshal([]byte(configData), &config); err != nil {
		return fmt.Errorf("unmarshal config failed: %w", err)
	}

	cm.mu.Lock()
	cm.config = &config
	cm.mu.Unlock()

	return nil
}

// AddWatcher 添加配置变更监听器
func (cm *ConfigManager) AddWatcher(watcher ConfigWatcher) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.watchers = append(cm.watchers, watcher)
}

// RemoveWatcher 移除配置变更监听器
func (cm *ConfigManager) RemoveWatcher(watcher ConfigWatcher) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for i, w := range cm.watchers {
		if w == watcher {
			cm.watchers = append(cm.watchers[:i], cm.watchers[i+1:]...)
			break
		}
	}
}

// notifyWatchers 通知所有监听器
func (cm *ConfigManager) notifyWatchers(config *ControlConfig) {
	cm.mu.RLock()
	watchers := make([]ConfigWatcher, len(cm.watchers))
	copy(watchers, cm.watchers)
	cm.mu.RUnlock()

	for _, watcher := range watchers {
		watcher.OnConfigChanged(config)
	}
}

// watchConfigChanges 监听配置变更
func (cm *ConfigManager) watchConfigChanges() {
	ticker := time.NewTicker(30 * time.Second) // 每30秒检查一次配置变更
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx := context.Background()
			if err := cm.LoadConfig(ctx); err != nil {
				// 记录错误但不中断监听
				continue
			}
		case <-cm.Ctx().Done():
			return
		}
	}
}
