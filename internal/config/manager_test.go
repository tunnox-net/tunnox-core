package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestConfig 测试用配置结构
type TestConfig struct {
	Name    string `yaml:"name"`
	Port    int    `yaml:"port"`
	Enabled bool   `yaml:"enabled"`
}

// DefaultTestConfig 返回默认测试配置
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		Name:    "default",
		Port:    8080,
		Enabled: true,
	}
}

func TestTypedConfigLoader_Load_DefaultsWhenFileNotExists(t *testing.T) {
	loader := &TypedConfigLoader[TestConfig]{
		DefaultsProvider: DefaultTestConfig,
	}

	config, err := loader.Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if config.Name != "default" {
		t.Errorf("expected Name='default', got '%s'", config.Name)
	}
	if config.Port != 8080 {
		t.Errorf("expected Port=8080, got %d", config.Port)
	}
	if !config.Enabled {
		t.Error("expected Enabled=true")
	}
}

func TestTypedConfigLoader_Load_FromFile(t *testing.T) {
	// 创建临时目录和配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
name: "test-app"
port: 9090
enabled: false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	loader := &TypedConfigLoader[TestConfig]{
		DefaultsProvider: DefaultTestConfig,
	}

	config, err := loader.Load(configPath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if config.Name != "test-app" {
		t.Errorf("expected Name='test-app', got '%s'", config.Name)
	}
	if config.Port != 9090 {
		t.Errorf("expected Port=9090, got %d", config.Port)
	}
	if config.Enabled {
		t.Error("expected Enabled=false")
	}
}

func TestTypedConfigLoader_Load_WithEnvOverrider(t *testing.T) {
	loader := &TypedConfigLoader[TestConfig]{
		DefaultsProvider: DefaultTestConfig,
		EnvOverrider: func(cfg *TestConfig) error {
			cfg.Port = 3000 // 模拟环境变量覆盖
			return nil
		},
	}

	config, err := loader.Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if config.Port != 3000 {
		t.Errorf("expected Port=3000 after env override, got %d", config.Port)
	}
}

func TestTypedConfigLoader_Load_EnvOverriderError(t *testing.T) {
	loader := &TypedConfigLoader[TestConfig]{
		DefaultsProvider: DefaultTestConfig,
		EnvOverrider: func(cfg *TestConfig) error {
			return errors.New("env override failed")
		},
	}

	_, err := loader.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, errors.Unwrap(err)) {
		// 验证错误包含预期信息
		expectedMsg := "failed to apply env overrides"
		if err.Error()[:len(expectedMsg)] != expectedMsg {
			t.Errorf("expected error to start with '%s', got '%s'", expectedMsg, err.Error())
		}
	}
}

func TestTypedConfigLoader_Load_NoDefaultsProvider(t *testing.T) {
	loader := &TypedConfigLoader[TestConfig]{}

	_, err := loader.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error when no defaults provider and file not exists")
	}
}

func TestTypedConfigExporter_Export(t *testing.T) {
	tmpDir := t.TempDir()
	exportPath := filepath.Join(tmpDir, "exported.yaml")

	exporter := &TypedConfigExporter[TestConfig]{}

	config := TestConfig{
		Name:    "exported-app",
		Port:    7070,
		Enabled: true,
	}

	err := exporter.Export(config, exportPath, ExportOptions{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(exportPath); os.IsNotExist(err) {
		t.Fatal("exported file does not exist")
	}

	// 读取并验证内容
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	content := string(data)
	if !contains(content, "name: exported-app") {
		t.Error("exported content does not contain expected name")
	}
	if !contains(content, "port: 7070") {
		t.Error("exported content does not contain expected port")
	}
}

func TestTypedConfigExporter_ExportTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	exportPath := filepath.Join(tmpDir, "template.yaml")

	templateContent := "# Template config\nname: ${NAME}\nport: ${PORT}"

	exporter := &TypedConfigExporter[TestConfig]{
		TemplateProvider: func() string {
			return templateContent
		},
	}

	err := exporter.Export(TestConfig{}, exportPath, ExportOptions{Template: true})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	if string(data) != templateContent {
		t.Errorf("expected template content, got: %s", string(data))
	}
}

func TestTypedConfigExporter_ExportCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedDir := filepath.Join(tmpDir, "nested", "dir")
	exportPath := filepath.Join(nestedDir, "config.yaml")

	exporter := &TypedConfigExporter[TestConfig]{}

	err := exporter.Export(TestConfig{Name: "test"}, exportPath, ExportOptions{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if _, err := os.Stat(exportPath); os.IsNotExist(err) {
		t.Fatal("exported file does not exist in nested directory")
	}
}

func TestTypedConfigManager_Load(t *testing.T) {
	loader := &TypedConfigLoader[TestConfig]{
		DefaultsProvider: DefaultTestConfig,
	}

	manager := NewTypedConfigManager[TestConfig](loader, nil, nil)

	config, err := manager.Load("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if config.Name != "default" {
		t.Errorf("expected Name='default', got '%s'", config.Name)
	}
}

func TestTypedConfigManager_LoadWithValidator(t *testing.T) {
	loader := &TypedConfigLoader[TestConfig]{
		DefaultsProvider: func() *TestConfig {
			return &TestConfig{Port: 0} // 无效端口
		},
	}

	validator := ValidatorFunc[TestConfig](func(cfg TestConfig) error {
		if cfg.Port <= 0 {
			return errors.New("port must be positive")
		}
		return nil
	})

	manager := NewTypedConfigManager[TestConfig](loader, validator, nil)

	_, err := manager.Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	if !contains(err.Error(), "validation failed") {
		t.Errorf("expected validation error message, got: %v", err)
	}
}

func TestTypedConfigManager_Validate(t *testing.T) {
	validator := ValidatorFunc[TestConfig](func(cfg TestConfig) error {
		if cfg.Name == "" {
			return errors.New("name is required")
		}
		return nil
	})

	manager := NewTypedConfigManager[TestConfig](nil, validator, nil)

	// 测试有效配置
	err := manager.Validate(TestConfig{Name: "test"})
	if err != nil {
		t.Errorf("expected no error for valid config, got: %v", err)
	}

	// 测试无效配置
	err = manager.Validate(TestConfig{Name: ""})
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestTypedConfigManager_ValidateWithoutValidator(t *testing.T) {
	manager := NewTypedConfigManager[TestConfig](nil, nil, nil)

	// 没有验证器时应该返回 nil
	err := manager.Validate(TestConfig{})
	if err != nil {
		t.Errorf("expected nil when no validator, got: %v", err)
	}
}

func TestTypedConfigManager_Export(t *testing.T) {
	tmpDir := t.TempDir()
	exportPath := filepath.Join(tmpDir, "config.yaml")

	exporter := &TypedConfigExporter[TestConfig]{}
	manager := NewTypedConfigManager[TestConfig](nil, nil, exporter)

	err := manager.Export(TestConfig{Name: "test"}, exportPath, ExportOptions{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if _, err := os.Stat(exportPath); os.IsNotExist(err) {
		t.Fatal("exported file does not exist")
	}
}

func TestTypedConfigManager_ExportWithoutExporter(t *testing.T) {
	manager := NewTypedConfigManager[TestConfig](nil, nil, nil)

	err := manager.Export(TestConfig{}, "/some/path.yaml", ExportOptions{})
	if err == nil {
		t.Fatal("expected error when no exporter configured")
	}
}

func TestValidatorFunc(t *testing.T) {
	validatorFn := ValidatorFunc[TestConfig](func(cfg TestConfig) error {
		if cfg.Port < 1024 {
			return errors.New("port must be >= 1024")
		}
		return nil
	})

	// 测试接口实现
	var _ TypedValidator[TestConfig] = validatorFn

	err := validatorFn.Validate(TestConfig{Port: 80})
	if err == nil {
		t.Error("expected error for port < 1024")
	}

	err = validatorFn.Validate(TestConfig{Port: 8080})
	if err != nil {
		t.Errorf("expected no error for port >= 1024, got: %v", err)
	}
}

// 辅助函数：检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
