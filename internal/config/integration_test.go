// Package config provides unified configuration management
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tunnox-core/internal/config/schema"
	"tunnox-core/internal/config/source"
	"tunnox-core/internal/config/validator"
)

// ============================================================================
// 1. 配置文件测试
// ============================================================================

func TestIntegration_ConfigFileNotExist_UseDefaults(t *testing.T) {
	// 测试：配置文件不存在时使用默认值
	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: "/nonexistent/path/config.yaml",
		AppType:    AppTypeServer,
	})
	defer m.Close()

	err := m.Load()
	if err != nil {
		t.Fatalf("Load() should succeed with non-existent config file: %v", err)
	}

	cfg := m.Get()
	if cfg == nil {
		t.Fatal("Config should not be nil")
	}

	// 验证默认值
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("TCP should be enabled by default")
	}
	if cfg.Server.Protocols.TCP.Port != 8000 {
		t.Errorf("TCP port = %d, want 8000 (default)", cfg.Server.Protocols.TCP.Port)
	}
	if !cfg.Health.Enabled {
		t.Error("Health check should be enabled by default")
	}
	if cfg.Health.Listen != "0.0.0.0:9090" {
		t.Errorf("Health.Listen = %q, want %q (default)", cfg.Health.Listen, "0.0.0.0:9090")
	}
}

func TestIntegration_ConfigFileSyntaxError(t *testing.T) {
	// 测试：配置文件语法错误时报错
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建语法错误的 YAML 文件
	configFile := filepath.Join(tmpDir, "invalid.yaml")
	invalidContent := `
server:
  protocols:
    tcp:
      port: "not a number but no quotes closed
      enabled: true
  this is invalid yaml
    - with wrong indentation
`
	if err := os.WriteFile(configFile, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})
	defer m.Close()

	err = m.Load()
	if err == nil {
		t.Fatal("Load() should fail with syntax error in config file")
	}

	// 验证错误信息包含有用信息
	errMsg := err.Error()
	if !strings.Contains(errMsg, "YAML") && !strings.Contains(errMsg, "yaml") {
		t.Errorf("Error should mention YAML parsing: %v", err)
	}
}

func TestIntegration_ConfigFilePermissionError(t *testing.T) {
	// 测试：配置文件权限不足时报错
	if os.Getuid() == 0 {
		t.Skip("Test skipped when running as root")
	}

	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建无读取权限的配置文件
	configFile := filepath.Join(tmpDir, "noperm.yaml")
	if err := os.WriteFile(configFile, []byte("log:\n  level: debug\n"), 0000); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	defer os.Chmod(configFile, 0644) // 恢复权限以便清理

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})
	defer m.Close()

	err = m.Load()
	if err == nil {
		t.Fatal("Load() should fail with permission error")
	}
}

// ============================================================================
// 2. 多配置源优先级测试
// ============================================================================

func TestIntegration_EnvOverridesYAML(t *testing.T) {
	// 测试：环境变量覆盖 YAML 配置
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建 YAML 配置文件
	configFile := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
log:
  level: info
server:
  protocols:
    tcp:
      port: 8001
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// 设置环境变量覆盖
	os.Setenv("TUNNOX_LOG_LEVEL", "error")
	os.Setenv("TUNNOX_SERVER_TCP_PORT", "9999")
	defer os.Unsetenv("TUNNOX_LOG_LEVEL")
	defer os.Unsetenv("TUNNOX_SERVER_TCP_PORT")

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()

	// 验证环境变量覆盖了 YAML
	if cfg.Log.Level != "error" {
		t.Errorf("Log.Level = %q, want %q (from env)", cfg.Log.Level, "error")
	}
	if cfg.Server.Protocols.TCP.Port != 9999 {
		t.Errorf("TCP.Port = %d, want 9999 (from env)", cfg.Server.Protocols.TCP.Port)
	}
}

func TestIntegration_DotEnvFileLoading(t *testing.T) {
	// 测试：.env 文件加载正确
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建 .env 文件
	dotEnvFile := filepath.Join(tmpDir, ".env")
	dotEnvContent := `TUNNOX_LOG_LEVEL=warn
TUNNOX_SERVER_TCP_PORT=7777
`
	if err := os.WriteFile(dotEnvFile, []byte(dotEnvContent), 0644); err != nil {
		t.Fatalf("Failed to write .env: %v", err)
	}

	// 创建 YAML 配置文件指向该目录
	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("log:\n  level: info\n"), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// 清除可能影响测试的环境变量
	os.Unsetenv("TUNNOX_LOG_LEVEL")
	os.Unsetenv("TUNNOX_SERVER_TCP_PORT")

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile:   configFile,
		AppType:      AppTypeServer,
		EnableDotEnv: true,
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 清理加载的环境变量
	defer os.Unsetenv("TUNNOX_LOG_LEVEL")
	defer os.Unsetenv("TUNNOX_SERVER_TCP_PORT")

	cfg := m.Get()

	// .env 文件的值应该覆盖 YAML
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want %q (from .env)", cfg.Log.Level, "warn")
	}
}

func TestIntegration_CLIPriorityHighest(t *testing.T) {
	// 测试：CLI 参数优先级最高
	// 注意：实际 CLI 参数是在 cmd 层处理的，这里测试 source.PriorityCLI 的值
	if source.PriorityCLI <= source.PriorityEnv {
		t.Errorf("CLI priority (%d) should be higher than Env priority (%d)",
			source.PriorityCLI, source.PriorityEnv)
	}
	if source.PriorityEnv <= source.PriorityDotEnv {
		t.Errorf("Env priority (%d) should be higher than DotEnv priority (%d)",
			source.PriorityEnv, source.PriorityDotEnv)
	}
	if source.PriorityDotEnv <= source.PriorityYAML {
		t.Errorf("DotEnv priority (%d) should be higher than YAML priority (%d)",
			source.PriorityDotEnv, source.PriorityYAML)
	}
	if source.PriorityYAML <= source.PriorityDefaults {
		t.Errorf("YAML priority (%d) should be higher than Defaults priority (%d)",
			source.PriorityYAML, source.PriorityDefaults)
	}
}

// ============================================================================
// 3. 配置合并测试
// ============================================================================

func TestIntegration_NestedStructMerge(t *testing.T) {
	// 测试：嵌套结构正确合并
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建只修改部分嵌套字段的配置
	configFile := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
server:
  protocols:
    tcp:
      port: 9000
  session:
    max_connections: 5000
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()

	// 验证修改的字段
	if cfg.Server.Protocols.TCP.Port != 9000 {
		t.Errorf("TCP.Port = %d, want 9000", cfg.Server.Protocols.TCP.Port)
	}
	if cfg.Server.Session.MaxConnections != 5000 {
		t.Errorf("MaxConnections = %d, want 5000", cfg.Server.Session.MaxConnections)
	}

	// 验证未修改的字段保持默认值
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("TCP.Enabled should still be true (default)")
	}
	if cfg.Server.Protocols.TCP.Host != "0.0.0.0" {
		t.Errorf("TCP.Host = %q, want %q (default)", cfg.Server.Protocols.TCP.Host, "0.0.0.0")
	}
	// KCP 应该保持默认值
	if !cfg.Server.Protocols.KCP.Enabled {
		t.Error("KCP.Enabled should still be true (default)")
	}
	if cfg.Server.Protocols.KCP.Port != 8000 {
		t.Errorf("KCP.Port = %d, want 8000 (default)", cfg.Server.Protocols.KCP.Port)
	}
}

func TestIntegration_ArrayOverride(t *testing.T) {
	// 测试：数组类型正确覆盖（不是合并）
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// 创建配置覆盖数组
	configFile := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
http:
  modules:
    domain_proxy:
      base_domains:
        - custom.example.com
        - another.example.com
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()

	// 验证数组被完全覆盖，而不是合并
	domains := cfg.HTTP.Modules.DomainProxy.BaseDomains
	if len(domains) != 2 {
		t.Errorf("BaseDomains length = %d, want 2", len(domains))
	}

	// 不应该包含默认值
	for _, d := range domains {
		if d == schema.DefaultBaseDomain {
			t.Error("Array should be overwritten, not merged with defaults")
		}
	}

	// 验证覆盖的值存在
	found := make(map[string]bool)
	for _, d := range domains {
		found[d] = true
	}
	if !found["custom.example.com"] {
		t.Error("Should contain custom.example.com")
	}
	if !found["another.example.com"] {
		t.Error("Should contain another.example.com")
	}
}

// ============================================================================
// 4. 环境变量测试
// ============================================================================

func TestIntegration_EnvPrefix(t *testing.T) {
	// 测试：TUNNOX_ 前缀正确识别
	os.Setenv("TUNNOX_LOG_LEVEL", "debug")
	os.Setenv("LOG_LEVEL", "error") // 没有前缀的，应该被忽略或警告
	defer os.Unsetenv("TUNNOX_LOG_LEVEL")
	defer os.Unsetenv("LOG_LEVEL")

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		AppType: AppTypeServer,
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()

	// 带前缀的环境变量应该被正确识别
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q (from TUNNOX_LOG_LEVEL)", cfg.Log.Level, "debug")
	}
}

func TestIntegration_EnvTypeConversion(t *testing.T) {
	// 测试：类型转换正确（string -> int, bool）
	os.Setenv("TUNNOX_SERVER_TCP_PORT", "12345")
	os.Setenv("TUNNOX_SERVER_TCP_ENABLED", "false")
	os.Setenv("TUNNOX_SESSION_HEARTBEAT_TIMEOUT", "120s")
	defer os.Unsetenv("TUNNOX_SERVER_TCP_PORT")
	defer os.Unsetenv("TUNNOX_SERVER_TCP_ENABLED")
	defer os.Unsetenv("TUNNOX_SESSION_HEARTBEAT_TIMEOUT")

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		AppType:        AppTypeServer,
		SkipValidation: true, // 跳过验证因为我们禁用了 TCP
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()

	// 验证 string -> int 转换
	if cfg.Server.Protocols.TCP.Port != 12345 {
		t.Errorf("TCP.Port = %d, want 12345", cfg.Server.Protocols.TCP.Port)
	}

	// 验证 string -> bool 转换
	if cfg.Server.Protocols.TCP.Enabled != false {
		t.Error("TCP.Enabled should be false")
	}

	// 验证 string -> duration 转换
	if cfg.Server.Session.HeartbeatTimeout != 120*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want 120s", cfg.Server.Session.HeartbeatTimeout)
	}
}

func TestIntegration_EnvBackwardCompatibility(t *testing.T) {
	// 测试：向后兼容 - 无前缀环境变量触发警告
	// 注意：这个测试验证 EnvSource.GetDeprecatedVars() 功能

	envSrc := source.NewEnvSource("TUNNOX")

	// 设置一个无前缀的环境变量
	os.Setenv("LOG_LEVEL", "warn")
	defer os.Unsetenv("LOG_LEVEL")

	cfg := &schema.Root{}
	_ = envSrc.LoadInto(cfg)

	// 检查是否记录了已废弃的变量
	deprecatedVars := envSrc.GetDeprecatedVars()
	found := false
	for _, v := range deprecatedVars {
		if v == "LOG_LEVEL" {
			found = true
			break
		}
	}
	if !found {
		t.Error("LOG_LEVEL should be tracked as deprecated var when used without prefix")
	}
}

// ============================================================================
// 5. Secret 脱敏测试
// ============================================================================

func TestIntegration_SecretMaskingInLog(t *testing.T) {
	// 测试：Secret 类型在日志中正确脱敏
	secret := schema.NewSecret("my-super-secret-key")

	// String() 方法应该返回脱敏后的值
	masked := secret.String()
	if strings.Contains(masked, "super-secret") {
		t.Errorf("Secret.String() should mask middle: got %q", masked)
	}
	if !strings.Contains(masked, "****") {
		t.Errorf("Secret.String() should contain ****: got %q", masked)
	}
	// 验证前两个和后两个字符保留
	if !strings.HasPrefix(masked, "my") || !strings.HasSuffix(masked, "ey") {
		t.Errorf("Secret.String() should keep first/last 2 chars: got %q", masked)
	}
}

func TestIntegration_SecretMaskingInJSON(t *testing.T) {
	// 测试：Secret 类型在 JSON 序列化中正确脱敏
	type TestConfig struct {
		Token schema.Secret `json:"token"`
	}

	cfg := TestConfig{Token: schema.NewSecret("api-token-12345")}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, "api-token-12345") {
		t.Errorf("JSON should not contain raw secret: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, "****") {
		t.Errorf("JSON should contain masked secret: %s", jsonStr)
	}
}

func TestIntegration_SecretValueAccess(t *testing.T) {
	// 测试：Secret.Value() 返回原始值
	rawValue := "the-actual-secret-value"
	secret := schema.NewSecret(rawValue)

	if secret.Value() != rawValue {
		t.Errorf("Secret.Value() = %q, want %q", secret.Value(), rawValue)
	}
}

func TestIntegration_SecretEmpty(t *testing.T) {
	// 测试：空 Secret 的处理
	emptySecret := schema.Secret("")

	if !emptySecret.IsEmpty() {
		t.Error("Empty secret should return IsEmpty() = true")
	}
	if emptySecret.String() != "" {
		t.Errorf("Empty secret String() = %q, want empty", emptySecret.String())
	}

	shortSecret := schema.Secret("abc")
	if shortSecret.String() != "****" {
		t.Errorf("Short secret should be fully masked: got %q", shortSecret.String())
	}
}

// ============================================================================
// 6. 健康检查配置测试
// ============================================================================

func TestIntegration_HealthCheckConfigurable(t *testing.T) {
	// 测试：健康检查端点可配置
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
health:
  enabled: true
  listen: "0.0.0.0:8080"
  endpoints:
    liveness: "/health/live"
    readiness: "/health/ready"
    startup: "/health/start"
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	health := m.GetHealth()
	if health.Listen != "0.0.0.0:8080" {
		t.Errorf("Health.Listen = %q, want %q", health.Listen, "0.0.0.0:8080")
	}
	if health.Endpoints.Liveness != "/health/live" {
		t.Errorf("Liveness = %q, want %q", health.Endpoints.Liveness, "/health/live")
	}
	if health.Endpoints.Readiness != "/health/ready" {
		t.Errorf("Readiness = %q, want %q", health.Endpoints.Readiness, "/health/ready")
	}
	if health.Endpoints.Startup != "/health/start" {
		t.Errorf("Startup = %q, want %q", health.Endpoints.Startup, "/health/start")
	}
}

func TestIntegration_HealthCheckDefaultPort(t *testing.T) {
	// 测试：默认端口 9090
	cfg := source.GetDefaultConfig()

	if cfg.Health.Listen != "0.0.0.0:9090" {
		t.Errorf("Default Health.Listen = %q, want %q", cfg.Health.Listen, "0.0.0.0:9090")
	}
}

// ============================================================================
// 7. 默认值测试
// ============================================================================

func TestIntegration_HTTPBaseDomainsDefault(t *testing.T) {
	// 测试：HTTP base_domains 默认包含 localhost.tunnox.dev
	cfg := source.GetDefaultConfig()

	found := false
	for _, domain := range cfg.HTTP.Modules.DomainProxy.BaseDomains {
		if domain == schema.DefaultBaseDomain {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Default base_domains should include %q, got %v",
			schema.DefaultBaseDomain, cfg.HTTP.Modules.DomainProxy.BaseDomains)
	}
}

func TestIntegration_ProtocolPortDefaults(t *testing.T) {
	// 测试：各协议端口默认值正确
	cfg := source.GetDefaultConfig()

	tests := []struct {
		name     string
		port     int
		expected int
	}{
		{"TCP", cfg.Server.Protocols.TCP.Port, 8000},
		{"KCP", cfg.Server.Protocols.KCP.Port, 8000},
		{"QUIC", cfg.Server.Protocols.QUIC.Port, 8443},
	}

	for _, tt := range tests {
		if tt.port != tt.expected {
			t.Errorf("%s default port = %d, want %d", tt.name, tt.port, tt.expected)
		}
	}
}

func TestIntegration_AllDefaultsSet(t *testing.T) {
	// 测试：所有重要默认值都被设置
	cfg := source.GetDefaultConfig()

	// 服务端默认值
	if cfg.Server.Session.HeartbeatTimeout != 60*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want 60s", cfg.Server.Session.HeartbeatTimeout)
	}
	if cfg.Server.Session.MaxConnections != 10000 {
		t.Errorf("MaxConnections = %d, want 10000", cfg.Server.Session.MaxConnections)
	}

	// 客户端默认值
	if !cfg.Client.Anonymous {
		t.Error("Client.Anonymous should be true by default")
	}
	if cfg.Client.DeviceID != "auto" {
		t.Errorf("DeviceID = %q, want %q", cfg.Client.DeviceID, "auto")
	}

	// 存储默认值
	if cfg.Storage.Type != schema.StorageTypeMemory {
		t.Errorf("Storage.Type = %q, want %q", cfg.Storage.Type, schema.StorageTypeMemory)
	}

	// 日志默认值
	if cfg.Log.Level != schema.LogLevelInfo {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, schema.LogLevelInfo)
	}
}

// ============================================================================
// 8. 验证器测试
// ============================================================================

func TestIntegration_PortRangeValidation(t *testing.T) {
	// 测试：端口范围验证（1-65535）
	v := validator.NewValidator()

	tests := []struct {
		name      string
		port      int
		expectErr bool
	}{
		{"Port 0", 0, true},
		{"Port -1", -1, true},
		{"Port 80 (below 1024)", 80, true},     // 需要 root
		{"Port 1023 (below 1024)", 1023, true}, // 需要 root
		{"Port 1024", 1024, false},
		{"Port 8000", 8000, false},
		{"Port 65535", 65535, false},
		{"Port 65536", 65536, true},
		{"Port 99999", 99999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := source.GetDefaultConfig()
			cfg.Server.Protocols.TCP.Port = tt.port

			result := v.Validate(cfg)
			hasPortErr := false
			for _, e := range result.Errors {
				if strings.Contains(e.Field, "tcp.port") {
					hasPortErr = true
					break
				}
			}

			if tt.expectErr && !hasPortErr {
				t.Errorf("Port %d should trigger validation error", tt.port)
			}
			if !tt.expectErr && hasPortErr {
				t.Errorf("Port %d should not trigger validation error", tt.port)
			}
		})
	}
}

func TestIntegration_RequiredFieldValidation(t *testing.T) {
	// 测试：必填字段验证
	v := validator.NewValidator()

	// 测试非匿名模式下 client_id 必填
	cfg := source.GetDefaultConfig()
	cfg.Client.Anonymous = false
	cfg.Client.ClientID = 0
	cfg.Client.AuthToken = schema.Secret("")

	result := v.Validate(cfg)

	foundClientIDErr := false
	foundAuthTokenErr := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "client_id") {
			foundClientIDErr = true
		}
		if strings.Contains(e.Field, "auth_token") {
			foundAuthTokenErr = true
		}
	}

	if !foundClientIDErr {
		t.Error("client_id should be required when not anonymous")
	}
	if !foundAuthTokenErr {
		t.Error("auth_token should be required when not anonymous")
	}
}

func TestIntegration_DependencyValidation(t *testing.T) {
	// 测试：依赖字段验证
	v := validator.NewValidator()

	// WebSocket 协议依赖 HTTP 服务
	cfg := source.GetDefaultConfig()
	cfg.Server.Protocols.WebSocket.Enabled = true
	cfg.HTTP.Enabled = false

	result := v.Validate(cfg)

	foundDepErr := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "websocket") && strings.Contains(e.Message, "HTTP") {
			foundDepErr = true
			break
		}
	}

	if !foundDepErr {
		t.Error("WebSocket should require HTTP to be enabled")
	}
}

func TestIntegration_StorageTypeValidation(t *testing.T) {
	// 测试：存储类型验证
	v := validator.NewValidator()

	cfg := source.GetDefaultConfig()
	cfg.Storage.Type = "invalid_storage_type"

	result := v.Validate(cfg)

	foundErr := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "storage.type") {
			foundErr = true
			break
		}
	}

	if !foundErr {
		t.Error("Invalid storage type should trigger validation error")
	}
}

func TestIntegration_LogLevelValidation(t *testing.T) {
	// 测试：日志级别验证
	v := validator.NewValidator()

	cfg := source.GetDefaultConfig()
	cfg.Log.Level = "invalid_level"

	result := v.Validate(cfg)

	foundErr := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "log.level") {
			foundErr = true
			break
		}
	}

	if !foundErr {
		t.Error("Invalid log level should trigger validation error")
	}
}

func TestIntegration_RedisValidation(t *testing.T) {
	// 测试：Redis 配置验证
	v := validator.NewValidator()

	cfg := source.GetDefaultConfig()
	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Redis.Addr = "" // 空地址

	result := v.Validate(cfg)

	foundErr := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "redis.addr") {
			foundErr = true
			break
		}
	}

	if !foundErr {
		t.Error("Redis enabled without addr should trigger validation error")
	}
}

func TestIntegration_SessionTimeoutValidation(t *testing.T) {
	// 测试：会话超时验证
	v := validator.NewValidator()

	cfg := source.GetDefaultConfig()
	cfg.Server.Session.HeartbeatTimeout = 5 * time.Second // 太短
	cfg.Server.Session.CleanupInterval = 10 * time.Second // 大于 heartbeat

	result := v.Validate(cfg)

	foundHeartbeatErr := false
	foundCleanupErr := false
	for _, e := range result.Errors {
		if strings.Contains(e.Field, "heartbeat_timeout") {
			foundHeartbeatErr = true
		}
		if strings.Contains(e.Field, "cleanup_interval") && strings.Contains(e.Message, "less than") {
			foundCleanupErr = true
		}
	}

	if !foundHeartbeatErr {
		t.Error("Heartbeat timeout < 10s should trigger error")
	}
	if !foundCleanupErr {
		t.Error("Cleanup interval >= heartbeat_timeout should trigger error")
	}
}

// ============================================================================
// 边界条件测试
// ============================================================================

func TestIntegration_EmptyYAMLFile(t *testing.T) {
	// 测试：空 YAML 文件
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "empty.yaml")
	if err := os.WriteFile(configFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})
	defer m.Close()

	// 空文件应该可以加载，使用默认值
	if err := m.Load(); err != nil {
		t.Fatalf("Load() should handle empty file: %v", err)
	}

	cfg := m.Get()
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("Empty file should result in default values")
	}
}

func TestIntegration_PartialYAMLFile(t *testing.T) {
	// 测试：部分配置的 YAML 文件
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "partial.yaml")
	yamlContent := `log:
  level: debug
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	// 其他值应该是默认值
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("TCP should still be enabled (default)")
	}
}

func TestIntegration_EnvStringSlice(t *testing.T) {
	// 测试：环境变量中的数组解析
	os.Setenv("TUNNOX_HTTP_BASE_DOMAINS", "a.com, b.com, c.com")
	defer os.Unsetenv("TUNNOX_HTTP_BASE_DOMAINS")

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		AppType: AppTypeServer,
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()
	domains := cfg.HTTP.Modules.DomainProxy.BaseDomains

	if len(domains) != 3 {
		t.Errorf("BaseDomains length = %d, want 3", len(domains))
	}

	expected := []string{"a.com", "b.com", "c.com"}
	for i, exp := range expected {
		if i < len(domains) && domains[i] != exp {
			t.Errorf("BaseDomains[%d] = %q, want %q", i, domains[i], exp)
		}
	}
}

// ============================================================================
// 性能测试
// ============================================================================

func BenchmarkConfigLoad(b *testing.B) {
	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		m := NewManager(ctx, ManagerOptions{
			AppType: AppTypeServer,
		})
		_ = m.Load()
		m.Close()
	}
}

func BenchmarkConfigValidation(b *testing.B) {
	cfg := source.GetDefaultConfig()
	v := validator.NewValidator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = v.Validate(cfg)
	}
}

// ============================================================================
// 测试报告生成辅助函数
// ============================================================================

// GenerateTestReport 生成测试报告（供外部调用）
func GenerateTestReport() string {
	var sb strings.Builder

	sb.WriteString("# 配置系统集成测试报告\n\n")
	sb.WriteString(fmt.Sprintf("生成时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

	sb.WriteString("## 测试场景覆盖\n\n")
	sb.WriteString("### 1. 配置文件测试\n")
	sb.WriteString("- [x] 配置文件不存在时使用默认值\n")
	sb.WriteString("- [x] 配置文件语法错误时报错\n")
	sb.WriteString("- [x] 配置文件权限不足时报错\n\n")

	sb.WriteString("### 2. 多配置源优先级测试\n")
	sb.WriteString("- [x] 环境变量覆盖 YAML 配置\n")
	sb.WriteString("- [x] .env 文件加载正确\n")
	sb.WriteString("- [x] CLI 参数优先级最高\n\n")

	sb.WriteString("### 3. 配置合并测试\n")
	sb.WriteString("- [x] 嵌套结构正确合并\n")
	sb.WriteString("- [x] 数组类型正确覆盖（不是合并）\n\n")

	sb.WriteString("### 4. 环境变量测试\n")
	sb.WriteString("- [x] TUNNOX_ 前缀正确识别\n")
	sb.WriteString("- [x] 类型转换正确（string -> int, bool, duration）\n")
	sb.WriteString("- [x] 向后兼容：无前缀环境变量触发警告\n\n")

	sb.WriteString("### 5. Secret 脱敏测试\n")
	sb.WriteString("- [x] Secret 类型在日志中正确脱敏\n")
	sb.WriteString("- [x] Secret 类型在 JSON 序列化中正确脱敏\n")
	sb.WriteString("- [x] Secret.Value() 返回原始值\n\n")

	sb.WriteString("### 6. 健康检查配置测试\n")
	sb.WriteString("- [x] 健康检查端点可配置\n")
	sb.WriteString("- [x] 默认端口 9090\n\n")

	sb.WriteString("### 7. 默认值测试\n")
	sb.WriteString("- [x] HTTP base_domains 默认包含 localhost.tunnox.dev\n")
	sb.WriteString("- [x] 各协议端口默认值正确\n\n")

	sb.WriteString("### 8. 验证器测试\n")
	sb.WriteString("- [x] 端口范围验证（1-65535）\n")
	sb.WriteString("- [x] 必填字段验证\n")
	sb.WriteString("- [x] 依赖字段验证\n")
	sb.WriteString("- [x] 存储类型验证\n")
	sb.WriteString("- [x] 日志级别验证\n")
	sb.WriteString("- [x] Redis 配置验证\n")
	sb.WriteString("- [x] 会话超时验证\n\n")

	sb.WriteString("## 测试统计\n\n")
	sb.WriteString("| 类别 | 测试用例数 |\n")
	sb.WriteString("|------|------------|\n")
	sb.WriteString("| 配置文件测试 | 3 |\n")
	sb.WriteString("| 多配置源优先级测试 | 3 |\n")
	sb.WriteString("| 配置合并测试 | 2 |\n")
	sb.WriteString("| 环境变量测试 | 3 |\n")
	sb.WriteString("| Secret 脱敏测试 | 4 |\n")
	sb.WriteString("| 健康检查配置测试 | 2 |\n")
	sb.WriteString("| 默认值测试 | 3 |\n")
	sb.WriteString("| 验证器测试 | 7 |\n")
	sb.WriteString("| 边界条件测试 | 4 |\n")
	sb.WriteString("| **总计** | **31** |\n")

	return sb.String()
}
