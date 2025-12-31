package client

import (
	"os"
	"path/filepath"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils"

	"gopkg.in/yaml.v3"
)

// ConfigManager 客户端配置管理器
type ConfigManager struct {
	searchPaths []string // 配置文件搜索路径（按优先级排序）
	savePaths   []string // 配置文件保存路径（按优先级排序）
}

// NewConfigManager 创建配置管理器
// 只从工作目录读取和保存配置文件
func NewConfigManager() *ConfigManager {
	workDir := getWorkingDir()
	configPath := filepath.Join(workDir, "client-config.yaml")

	return &ConfigManager{
		searchPaths: []string{configPath},
		savePaths:   []string{configPath},
	}
}

// NewConfigManagerWithPath 创建配置管理器（使用指定的配置文件路径）
// configFilePath: 用户通过 -c 指定的配置文件路径
// 如果指定了路径，则优先使用该路径进行加载和保存
func NewConfigManagerWithPath(configFilePath string) *ConfigManager {
	if configFilePath == "" {
		return NewConfigManager()
	}

	// 使用用户指定的路径作为首选
	workDir := getWorkingDir()
	defaultPath := filepath.Join(workDir, "client-config.yaml")

	return &ConfigManager{
		searchPaths: []string{configFilePath, defaultPath},
		savePaths:   []string{configFilePath}, // 保存时只使用用户指定的路径
	}
}

// LoadConfig 加载配置（按优先级尝试多个路径）
func (cm *ConfigManager) LoadConfig(cmdConfigPath string) (*ClientConfig, error) {
	// 1. 命令行指定的配置文件
	if cmdConfigPath != "" {
		config, err := cm.loadConfigFromFile(cmdConfigPath)
		if err != nil {
			return nil, coreerrors.Wrapf(err, coreerrors.CodeStorageError, "failed to load config from %s", cmdConfigPath)
		}
		corelog.Infof("ConfigManager: loaded config from %s (command line)", cmdConfigPath)
		return config, nil
	}

	// 2. 尝试标准搜索路径
	for _, path := range cm.searchPaths {
		config, err := cm.loadConfigFromFile(path)
		if err == nil {
			corelog.Infof("ConfigManager: loaded config from %s", path)
			return config, nil
		}
		// 文件不存在是正常情况，继续尝试下一个
		if !os.IsNotExist(err) {
			corelog.Warnf("ConfigManager: failed to load config from %s: %v", path, err)
		}
	}

	// 3. 所有路径都没有配置文件，返回空配置
	// 首次连接时服务端会分配 clientId + secretKey
	corelog.Infof("ConfigManager: no config file found, using empty config")
	return &ClientConfig{
		// 不设置 ClientID 和 SecretKey，首次连接时服务端会分配
		// 不设置 Server.Address，让自动连接机制处理
	}, nil
}

// SaveConfig 保存配置（按优先级尝试多个路径，权限不足时降级）
// 注意：此方法会保留已存在的配置文件中的 Server.Address 和 Server.Protocol
// 除非明确指定 allowUpdateServerConfig=true，否则不会更新服务器配置
func (cm *ConfigManager) SaveConfig(config *ClientConfig) error {
	return cm.SaveConfigWithOptions(config, false)
}

// SaveConfigWithOptions 保存配置（带选项）
// allowUpdateServerConfig: 是否允许更新 Server.Address 和 Server.Protocol
func (cm *ConfigManager) SaveConfigWithOptions(config *ClientConfig, allowUpdateServerConfig bool) error {
	// 尝试从已存在的配置文件中加载，以便合并配置
	var existingConfig *ClientConfig
	for _, path := range cm.searchPaths {
		if cfg, err := cm.loadConfigFromFile(path); err == nil {
			existingConfig = cfg
			break
		}
	}

	if existingConfig != nil {
		// 如果不允许更新服务器配置，保留现有的服务器地址和协议
		if !allowUpdateServerConfig {
			config.Server.Address = existingConfig.Server.Address
			config.Server.Protocol = existingConfig.Server.Protocol
		}

		// 合并凭据配置：优先使用传入的有效值，否则保留现有值
		// 这确保了首次获取的 ClientID 和 SecretKey 不会在后续保存时丢失
		if config.ClientID == 0 && existingConfig.ClientID > 0 {
			config.ClientID = existingConfig.ClientID
		}
		if config.SecretKey == "" && existingConfig.SecretKey != "" {
			config.SecretKey = existingConfig.SecretKey
		}
	}

	var lastErr error

	for _, path := range cm.savePaths {
		// 确保目录存在
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			corelog.Warnf("ConfigManager: failed to create directory %s: %v, trying next...", dir, err)
			lastErr = err
			continue
		}

		// 尝试写入配置
		if err := cm.saveConfigToFile(path, config); err != nil {
			corelog.Warnf("ConfigManager: failed to save config to %s: %v, trying next...", path, err)
			lastErr = err
			continue
		}

		corelog.Infof("ConfigManager: config saved to %s", path)
		return nil
	}

	// 所有路径都失败
	if lastErr != nil {
		return coreerrors.Wrap(lastErr, coreerrors.CodeStorageError, "failed to save config to any location")
	}
	return coreerrors.New(coreerrors.CodeStorageError, "failed to save config to any location")
}

// loadConfigFromFile 从文件加载配置
func (cm *ConfigManager) loadConfigFromFile(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config ClientConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidData, "failed to parse config")
	}

	return &config, nil
}

// saveConfigToFile 保存配置到文件
func (cm *ConfigManager) saveConfigToFile(path string, config *ClientConfig) error {
	// ✅ 在保存前，确保日志配置有默认值（不保存空值）
	if config.Log.Level == "" {
		config.Log.Level = "info"
	}
	if config.Log.Format == "" {
		config.Log.Format = "text"
	}
	// ✅ output 字段不保存到配置文件，由系统根据运行模式自动控制
	// CLI模式：只写文件，不输出到控制台
	// Daemon模式：同时写文件和输出到控制台
	config.Log.Output = "" // 清空output字段，不保存

	if config.Log.File == "" {
		// 使用默认日志路径
		candidates := utils.GetDefaultClientLogPath(false)
		if len(candidates) > 0 {
			config.Log.File = candidates[0]
		}
	}

	// 序列化为 YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal config")
	}

	// 写入临时文件
	tempFile := path + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to write temp file")
	}

	// 原子替换
	if err := os.Rename(tempFile, path); err != nil {
		os.Remove(tempFile) // 清理临时文件
		return coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to rename temp file")
	}

	return nil
}

// getExecutableDir 获取可执行文件所在目录
func getExecutableDir() string {
	execPath, err := os.Executable()
	if err != nil {
		corelog.Warnf("ConfigManager: failed to get executable path: %v", err)
		return "."
	}
	return filepath.Dir(execPath)
}

// getWorkingDir 获取工作目录
func getWorkingDir() string {
	workDir, err := os.Getwd()
	if err != nil {
		corelog.Warnf("ConfigManager: failed to get working directory: %v", err)
		return "."
	}
	return workDir
}

// getUserHomeDir 获取用户主目录
func getUserHomeDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		corelog.Warnf("ConfigManager: failed to get user home directory: %v", err)
		return "."
	}
	return homeDir
}

// getDefaultConfig 获取默认配置
func getDefaultConfig() *ClientConfig {
	config := &ClientConfig{}
	// 默认使用 WebSocket 连接到公共服务器
	config.Server.Address = "https://gw.tunnox.net/_tunnox"
	config.Server.Protocol = "websocket"
	return config
}
