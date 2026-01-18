package managers

import (
	"context"
	"encoding/json"
	"sync"

	"tunnox-core/internal/constants"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/core/storage"
)

// ConfigManager 配置管理器
type ConfigManager struct {
	*dispose.ManagerBase
	storage  storage.Storage
	config   *ControlConfig
	watchers []ConfigWatcher
	mu       sync.RWMutex
}

// ConfigWatcher 配置变更监听器
type ConfigWatcher interface {
	OnConfigChanged(config *ControlConfig)
}

// NewConfigManager 创建新的配置管理器
func NewConfigManager(storage storage.Storage, config *ControlConfig, parentCtx context.Context) *ConfigManager {
	manager := &ConfigManager{
		ManagerBase: dispose.NewManager("ConfigManager", parentCtx),
		storage:     storage,
		config:      config,
		watchers:    make([]ConfigWatcher, 0),
	}
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
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "marshal config failed")
	}

	key := constants.KeyPrefixConfig + ":config"
	if err := cm.storage.Set(key, string(data), 0); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "save config failed")
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
	key := constants.KeyPrefixConfig + ":config"
	data, err := cm.storage.Get(key)
	if err != nil {
		// 配置不存在，使用默认配置
		return nil
	}

	configData, ok := data.(string)
	if !ok {
		return coreerrors.New(coreerrors.CodeInvalidData, "invalid config data type")
	}

	var config ControlConfig
	if err := json.Unmarshal([]byte(configData), &config); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInvalidData, "unmarshal config failed")
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
