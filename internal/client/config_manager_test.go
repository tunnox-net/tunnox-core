package client

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewConfigManager 测试配置管理器创建
func TestNewConfigManager(t *testing.T) {
	cm := NewConfigManager()

	if cm == nil {
		t.Fatal("Expected non-nil ConfigManager")
	}

	if len(cm.searchPaths) == 0 {
		t.Error("Expected non-empty search paths")
	}

	if len(cm.savePaths) == 0 {
		t.Error("Expected non-empty save paths")
	}

	// 验证路径包含预期的文件名
	foundConfigFile := false
	for _, path := range cm.searchPaths {
		if filepath.Base(path) == "client-config.yaml" {
			foundConfigFile = true
			break
		}
	}

	if !foundConfigFile {
		t.Error("Expected search paths to contain client-config.yaml")
	}
}

// TestLoadConfig_DefaultConfig 测试加载默认配置
func TestLoadConfig_DefaultConfig(t *testing.T) {
	cm := &ConfigManager{
		searchPaths: []string{
			"/non/existent/path/config.yaml",
		},
	}

	config, err := cm.LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if config == nil {
		t.Fatal("Expected non-nil config")
	}

	// 验证默认配置的特征
	if config.ClientID != 0 {
		t.Logf("Default config ClientID: %d", config.ClientID)
	}
}

// TestLoadConfig_FromFile 测试从文件加载配置
func TestLoadConfig_FromFile(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")

	configContent := `client_id: 12345
server:
  address: test.example.com:8080
  protocol: tcp
auth_token: test-token-123
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	cm := &ConfigManager{
		searchPaths: []string{configPath},
	}

	config, err := cm.LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if config.ClientID != 12345 {
		t.Errorf("Expected ClientID=12345, got %d", config.ClientID)
	}

	// 验证服务器地址包含主机名
	if config.Server.Address == "" {
		t.Error("Expected non-empty server address")
	}
}

// TestLoadConfig_CommandLinePath 测试命令行指定路径优先级
func TestLoadConfig_CommandLinePath(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建两个配置文件
	cmdConfigPath := filepath.Join(tmpDir, "cmd-config.yaml")
	searchConfigPath := filepath.Join(tmpDir, "search-config.yaml")

	// 命令行配置
	cmdContent := `client_id: 999`
	err := os.WriteFile(cmdConfigPath, []byte(cmdContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create cmd config: %v", err)
	}

	// 搜索路径配置
	searchContent := `client_id: 111`
	err = os.WriteFile(searchConfigPath, []byte(searchContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create search config: %v", err)
	}

	cm := &ConfigManager{
		searchPaths: []string{searchConfigPath},
	}

	// 命令行路径应该优先
	config, err := cm.LoadConfig(cmdConfigPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if config.ClientID != 999 {
		t.Errorf("Expected ClientID=999 (from cmd path), got %d", config.ClientID)
	}
}

// TestSaveConfig 测试保存配置
func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "save-config.yaml")

	cm := &ConfigManager{
		savePaths: []string{configPath},
	}

	config := &ClientConfig{
		ClientID: 54321,
		Server: struct {
			Address  string `yaml:"address"`
			Protocol string `yaml:"protocol"`
		}{
			Address:  "save.example.com:9090",
			Protocol: "tcp",
		},
	}

	err := cm.SaveConfig(config)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// 验证文件已创建
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// 验证可以读取回来
	cm2 := &ConfigManager{
		searchPaths: []string{configPath},
	}

	loadedConfig, err := cm2.LoadConfig("")
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loadedConfig.ClientID != 54321 {
		t.Errorf("Expected ClientID=54321, got %d", loadedConfig.ClientID)
	}
}

// TestGetWorkingDir 测试获取工作目录
func TestGetWorkingDir(t *testing.T) {
	workDir := getWorkingDir()
	if workDir == "" {
		t.Error("Expected non-empty working directory")
	}
	t.Logf("Working dir: %s", workDir)
}
