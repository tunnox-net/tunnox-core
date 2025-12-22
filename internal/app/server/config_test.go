package server

import (
	"testing"
)

// TestAuthConfig 测试认证配置
func TestAuthConfig(t *testing.T) {
	config := AuthConfig{
		Type:  "bearer",
		Token: "test-token",
	}

	if config.Type != "bearer" {
		t.Errorf("Expected Type to be 'bearer', got '%s'", config.Type)
	}
	if config.Token != "test-token" {
		t.Errorf("Expected Token to be 'test-token', got '%s'", config.Token)
	}
}

// TestRedisConfig 测试 Redis 配置
func TestRedisConfig(t *testing.T) {
	config := RedisConfig{
		Enabled:  true,
		Addr:     "localhost:6379",
		Password: "password",
		DB:       0,
	}

	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if config.Addr != "localhost:6379" {
		t.Errorf("Expected Addr to be 'localhost:6379', got '%s'", config.Addr)
	}
	if config.Password != "password" {
		t.Errorf("Expected Password to be 'password', got '%s'", config.Password)
	}
	if config.DB != 0 {
		t.Errorf("Expected DB to be 0, got %d", config.DB)
	}
}

// TestStorageConfig 测试存储配置
func TestStorageConfig(t *testing.T) {
	config := StorageConfig{
		Enabled: true,
		URL:     "http://tunnox-storage:8080",
		Token:   "test-token",
		Timeout: 10,
	}

	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if config.URL != "http://tunnox-storage:8080" {
		t.Errorf("Expected URL to be 'http://tunnox-storage:8080', got '%s'", config.URL)
	}
	if config.Token != "test-token" {
		t.Errorf("Expected Token to be 'test-token', got '%s'", config.Token)
	}
	if config.Timeout != 10 {
		t.Errorf("Expected Timeout to be 10, got %d", config.Timeout)
	}
}

// TestPlatformConfig 测试平台配置
func TestPlatformConfig(t *testing.T) {
	config := PlatformConfig{
		Enabled: true,
		URL:     "http://tunnox-platform:8080",
		Token:   "test-token",
		Timeout: 10,
	}

	if !config.Enabled {
		t.Error("Expected Enabled to be true")
	}
	if config.URL != "http://tunnox-platform:8080" {
		t.Errorf("Expected URL to be 'http://tunnox-platform:8080', got '%s'", config.URL)
	}
	if config.Token != "test-token" {
		t.Errorf("Expected Token to be 'test-token', got '%s'", config.Token)
	}
	if config.Timeout != 10 {
		t.Errorf("Expected Timeout to be 10, got %d", config.Timeout)
	}
}

// TestManagementConfig 测试管理配置
func TestManagementConfig(t *testing.T) {
	config := ManagementConfig{
		Listen: "0.0.0.0:9000",
		Auth: AuthConfig{
			Type:  "bearer",
			Token: "secret-token",
		},
		PProf: PProfConfig{
			Enabled:     true,
			DataDir:     "logs/pprof",
			Retention:   10,
			AutoCapture: true,
		},
	}

	if config.Listen != "0.0.0.0:9000" {
		t.Errorf("Expected Listen to be '0.0.0.0:9000', got '%s'", config.Listen)
	}
	if config.Auth.Type != "bearer" {
		t.Errorf("Expected Auth.Type to be 'bearer', got '%s'", config.Auth.Type)
	}
	if config.Auth.Token != "secret-token" {
		t.Errorf("Expected Auth.Token to be 'secret-token', got '%s'", config.Auth.Token)
	}
	if !config.PProf.Enabled {
		t.Error("Expected PProf.Enabled to be true")
	}
}

// TestGetDefaultConfig 测试获取默认配置
func TestGetDefaultConfig(t *testing.T) {
	config := GetDefaultConfig()

	if config == nil {
		t.Fatal("Expected config to be non-nil")
	}

	// 验证默认协议配置
	if config.Server.Protocols == nil {
		t.Fatal("Expected protocols to be non-nil")
	}

	tcpConfig, exists := config.Server.Protocols["tcp"]
	if !exists {
		t.Error("Expected TCP protocol to exist")
	}
	if !tcpConfig.Enabled {
		t.Error("Expected TCP to be enabled by default")
	}
	if tcpConfig.Port != 8000 {
		t.Errorf("Expected TCP port to be 8000, got %d", tcpConfig.Port)
	}

	// 验证管理配置
	if config.Management.Listen != "0.0.0.0:9000" {
		t.Errorf("Expected Management.Listen to be '0.0.0.0:9000', got '%s'", config.Management.Listen)
	}

	// 验证日志配置
	if config.Log.Level != "info" {
		t.Errorf("Expected Log.Level to be 'info', got '%s'", config.Log.Level)
	}
}

// TestValidateConfig 测试配置验证
func TestValidateConfig(t *testing.T) {
	config := GetDefaultConfig()

	err := ValidateConfig(config)
	if err != nil {
		t.Errorf("Expected no error for default config, got: %v", err)
	}

	// 测试无效的 Redis 配置
	invalidConfig := &Config{
		Redis: RedisConfig{
			Enabled: true,
			Addr:    "", // 缺少地址
		},
	}

	err = ValidateConfig(invalidConfig)
	if err == nil {
		t.Error("Expected error for invalid Redis config")
	}

	// 测试无效的 Storage 配置
	invalidConfig2 := &Config{
		Storage: StorageConfig{
			Enabled: true,
			URL:     "", // 缺少 URL
		},
	}

	err = ValidateConfig(invalidConfig2)
	if err == nil {
		t.Error("Expected error for invalid Storage config")
	}
}
