package client

import (
corelog "tunnox-core/internal/core/log"
	"fmt"
	"os"
	"path/filepath"
	
	"gopkg.in/yaml.v3"
)

// ConfigManager 客户端配置管理器
type ConfigManager struct {
	searchPaths []string // 配置文件搜索路径（按优先级排序）
	savePaths   []string // 配置文件保存路径（按优先级排序）
}

// NewConfigManager 创建配置管理器
func NewConfigManager() *ConfigManager {
	execDir := getExecutableDir()
	workDir := getWorkingDir()
	homeDir := getUserHomeDir()
	
	return &ConfigManager{
		searchPaths: []string{
			filepath.Join(execDir, "client-config.yaml"),
			filepath.Join(workDir, "client-config.yaml"),
			filepath.Join(homeDir, ".tunnox", "client-config.yaml"),
		},
		savePaths: []string{
			filepath.Join(execDir, "client-config.yaml"),
			filepath.Join(workDir, "client-config.yaml"),
			filepath.Join(homeDir, ".tunnox", "client-config.yaml"),
		},
	}
}

// LoadConfig 加载配置（按优先级尝试多个路径）
func (cm *ConfigManager) LoadConfig(cmdConfigPath string) (*ClientConfig, error) {
	// 1. 命令行指定的配置文件
	if cmdConfigPath != "" {
		config, err := cm.loadConfigFromFile(cmdConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", cmdConfigPath, err)
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
	
	// 3. 所有路径都没有配置文件，返回空配置（不设置默认地址）
	// 这样可以在 CLI 模式下触发自动连接
	corelog.Infof("ConfigManager: no config file found, using empty config")
	return &ClientConfig{
		Anonymous: true,
		DeviceID:  "anonymous-device",
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
	// 如果不允许更新服务器配置，先尝试从已存在的配置文件中加载，保留 Server.Address 和 Server.Protocol
	if !allowUpdateServerConfig {
		var existingConfig *ClientConfig
		for _, path := range cm.searchPaths {
			if cfg, err := cm.loadConfigFromFile(path); err == nil {
				existingConfig = cfg
				break
			}
		}
		
		// 如果存在已加载的配置，保留其 Server.Address 和 Server.Protocol
		if existingConfig != nil {
			config.Server.Address = existingConfig.Server.Address
			config.Server.Protocol = existingConfig.Server.Protocol
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
		return fmt.Errorf("failed to save config to any location: %w", lastErr)
	}
	return fmt.Errorf("failed to save config to any location")
}

// loadConfigFromFile 从文件加载配置
func (cm *ConfigManager) loadConfigFromFile(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	
	var config ClientConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	
	return &config, nil
}

// saveConfigToFile 保存配置到文件
func (cm *ConfigManager) saveConfigToFile(path string, config *ClientConfig) error {
	// 序列化为 YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	// 写入临时文件
	tempFile := path + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	
	// 原子替换
	if err := os.Rename(tempFile, path); err != nil {
		os.Remove(tempFile) // 清理临时文件
		return fmt.Errorf("failed to rename temp file: %w", err)
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
	config := &ClientConfig{
		Anonymous: true,
		DeviceID:  "anonymous-device",
	}
	// 默认使用 WebSocket 连接到公共服务器
	config.Server.Address = "https://gw.tunnox.net/_tunnox"
	config.Server.Protocol = "websocket"
	return config
}

