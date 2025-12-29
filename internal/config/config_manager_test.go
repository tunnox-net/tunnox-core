package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tunnox-core/internal/config/schema"
)

func TestNewManager(t *testing.T) {
	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{})

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}

	// Verify Dispose pattern is followed
	if m.ResourceBase == nil {
		t.Error("ResourceBase should not be nil")
	}

	// Clean up
	m.Close()
}

func TestManager_Load(t *testing.T) {
	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		AppType: AppTypeServer,
	})
	defer m.Close()

	err := m.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()
	if cfg == nil {
		t.Fatal("Get() returned nil after Load()")
	}

	// Verify defaults are applied
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("TCP should be enabled by default")
	}
}

func TestManager_LoadWithConfigFile(t *testing.T) {
	// Create temp config file
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `
server:
  protocols:
    tcp:
      port: 9999

log:
  level: debug
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})
	defer m.Close()

	err = m.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()
	if cfg.Server.Protocols.TCP.Port != 9999 {
		t.Errorf("TCP.Port = %d, want 9999", cfg.Server.Protocols.TCP.Port)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
}

func TestManager_LoadWithEnvOverride(t *testing.T) {
	os.Setenv("TUNNOX_LOG_LEVEL", "error")
	defer os.Unsetenv("TUNNOX_LOG_LEVEL")

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		AppType: AppTypeServer,
	})
	defer m.Close()

	err := m.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	cfg := m.Get()
	if cfg.Log.Level != "error" {
		t.Errorf("Log.Level = %q, want %q (from env)", cfg.Log.Level, "error")
	}
}

func TestManager_GetAccessors(t *testing.T) {
	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		AppType: AppTypeServer,
	})
	defer m.Close()

	err := m.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Test all accessors
	if m.GetServer() == nil {
		t.Error("GetServer() returned nil")
	}
	if m.GetClient() == nil {
		t.Error("GetClient() returned nil")
	}
	if m.GetHTTP() == nil {
		t.Error("GetHTTP() returned nil")
	}
	if m.GetStorage() == nil {
		t.Error("GetStorage() returned nil")
	}
	if m.GetSecurity() == nil {
		t.Error("GetSecurity() returned nil")
	}
	if m.GetLog() == nil {
		t.Error("GetLog() returned nil")
	}
	if m.GetHealth() == nil {
		t.Error("GetHealth() returned nil")
	}
	if m.GetManagement() == nil {
		t.Error("GetManagement() returned nil")
	}
	if m.GetPlatform() == nil {
		t.Error("GetPlatform() returned nil")
	}
}

func TestManager_GetAccessors_BeforeLoad(t *testing.T) {
	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{})
	defer m.Close()

	// Accessors should return nil before Load
	if m.Get() != nil {
		t.Error("Get() should return nil before Load()")
	}
	if m.GetServer() != nil {
		t.Error("GetServer() should return nil before Load()")
	}
}

func TestManager_Validate(t *testing.T) {
	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		AppType: AppTypeServer,
	})
	defer m.Close()

	// Validate before load
	err := m.Validate()
	if err == nil {
		t.Error("Validate() should error before Load()")
	}

	// Load and validate
	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	err = m.Validate()
	if err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestManager_OnChange(t *testing.T) {
	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		AppType: AppTypeServer,
	})
	defer m.Close()

	callbackCalled := false
	var receivedConfig *schema.Root

	m.OnChange(func(cfg *schema.Root) {
		callbackCalled = true
		receivedConfig = cfg
	})

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Trigger reload
	if err := m.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if !callbackCalled {
		t.Error("OnChange callback should be called after Reload()")
	}
	if receivedConfig == nil {
		t.Error("OnChange should receive config")
	}
}

func TestManager_Reload(t *testing.T) {
	// Create temp config file
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `log:
  level: info
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
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

	// Verify initial config
	if m.GetLog().Level != "info" {
		t.Errorf("Initial Log.Level = %q, want %q", m.GetLog().Level, "info")
	}

	// Update config file
	newContent := `log:
  level: debug
`
	if err := os.WriteFile(configFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("Failed to update config: %v", err)
	}

	// Reload
	if err := m.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	// Verify reloaded config
	if m.GetLog().Level != "debug" {
		t.Errorf("After reload Log.Level = %q, want %q", m.GetLog().Level, "debug")
	}
}

func TestManager_Dispose(t *testing.T) {
	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{})

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Dispose should not error
	err := m.Dispose()
	if err != nil {
		t.Errorf("Dispose() error = %v", err)
	}

	// Manager should be closed
	if !m.IsClosed() {
		t.Error("Manager should be closed after Dispose()")
	}
}

func TestManager_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := NewManager(ctx, ManagerOptions{})

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Cancel context
	cancel()

	// Give some time for cleanup
	time.Sleep(10 * time.Millisecond)

	// Manager should be closed due to context cancellation
	if !m.IsClosed() {
		t.Error("Manager should be closed after context cancellation")
	}
}

func TestManager_SkipValidation(t *testing.T) {
	// Create config with invalid settings
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.yaml")
	content := `
server:
  protocols:
    tcp:
      port: 0  # Invalid port
`
	if err := os.WriteFile(configFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Without SkipValidation, should fail
	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})

	err = m.Load()
	if err == nil {
		t.Error("Load() should fail with invalid config")
	}
	m.Close()

	// With SkipValidation, should succeed
	m = NewManager(ctx, ManagerOptions{
		ConfigFile:     configFile,
		AppType:        AppTypeServer,
		SkipValidation: true,
	})
	defer m.Close()

	err = m.Load()
	if err != nil {
		t.Errorf("Load() with SkipValidation should not fail: %v", err)
	}
}

func TestLoadServerConfig_Convenience(t *testing.T) {
	ctx := context.Background()
	m, err := LoadServerConfig(ctx, "")

	if err != nil {
		t.Fatalf("LoadServerConfig() error = %v", err)
	}
	defer m.Close()

	if m.Get() == nil {
		t.Error("LoadServerConfig() should return loaded config")
	}
}

func TestLoadClientConfig_Convenience(t *testing.T) {
	ctx := context.Background()
	m, err := LoadClientConfig(ctx, "")

	if err != nil {
		t.Fatalf("LoadClientConfig() error = %v", err)
	}
	defer m.Close()

	if m.Get() == nil {
		t.Error("LoadClientConfig() should return loaded config")
	}
}

func TestGetDefaultConfig_Function(t *testing.T) {
	cfg := GetDefaultConfig()

	if cfg == nil {
		t.Fatal("GetDefaultConfig() returned nil")
	}

	// Verify some defaults
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("TCP should be enabled by default")
	}
	if !cfg.Health.Enabled {
		t.Error("Health should be enabled by default")
	}
}

func TestManager_CustomEnvPrefix(t *testing.T) {
	os.Setenv("CUSTOM_LOG_LEVEL", "warn")
	defer os.Unsetenv("CUSTOM_LOG_LEVEL")

	ctx := context.Background()
	m := NewManager(ctx, ManagerOptions{
		EnvPrefix: "CUSTOM",
		AppType:   AppTypeServer,
	})
	defer m.Close()

	if err := m.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if m.GetLog().Level != "warn" {
		t.Errorf("Log.Level = %q, want %q (from CUSTOM_LOG_LEVEL)", m.GetLog().Level, "warn")
	}
}
