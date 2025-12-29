package loader

import (
	"os"
	"path/filepath"
	"testing"

	"tunnox-core/internal/config/source"
)

func TestLoader_NewLoader(t *testing.T) {
	l := NewLoader()
	if l == nil {
		t.Fatal("NewLoader() returned nil")
	}
	if len(l.sources) != 0 {
		t.Errorf("NewLoader() sources = %d, want 0", len(l.sources))
	}
}

func TestLoader_AddSource(t *testing.T) {
	l := NewLoader()
	l.AddSource(source.NewDefaultSource())
	l.AddSource(source.NewEnvSource("TUNNOX"))

	if len(l.sources) != 2 {
		t.Errorf("AddSource() sources = %d, want 2", len(l.sources))
	}
}

func TestLoader_Load_NoSources(t *testing.T) {
	l := NewLoader()
	_, err := l.Load()
	if err == nil {
		t.Error("Load() should error when no sources are registered")
	}
}

func TestLoader_Load_DefaultsOnly(t *testing.T) {
	l := NewLoader()
	l.AddSource(source.NewDefaultSource())

	cfg, err := l.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify defaults are applied
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("TCP should be enabled by default")
	}
	if cfg.Server.Protocols.TCP.Port != 8000 {
		t.Errorf("TCP.Port = %d, want 8000", cfg.Server.Protocols.TCP.Port)
	}
}

func TestLoader_Load_PriorityOrder(t *testing.T) {
	// Create temp config file
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
log:
  level: warn
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Set env var (higher priority than YAML)
	os.Setenv("TUNNOX_LOG_LEVEL", "error")
	defer os.Unsetenv("TUNNOX_LOG_LEVEL")

	l := NewLoader()
	l.AddSource(source.NewDefaultSource())        // info
	l.AddSource(source.NewYAMLSource(configFile)) // warn
	l.AddSource(source.NewEnvSource("TUNNOX"))    // error

	cfg, err := l.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Env should win (highest priority)
	if cfg.Log.Level != "error" {
		t.Errorf("Log.Level = %q, want %q (env should override yaml)", cfg.Log.Level, "error")
	}
}

func TestLoaderBuilder(t *testing.T) {
	builder := NewLoaderBuilder()
	if builder == nil {
		t.Fatal("NewLoaderBuilder() returned nil")
	}

	builder.WithPrefix("TEST").
		WithAppType("server").
		WithDotEnv(false)

	l := builder.Build()
	if l == nil {
		t.Fatal("Build() returned nil")
	}

	// Should have at least defaults and env source
	if len(l.sources) < 2 {
		t.Errorf("Build() sources = %d, want >= 2", len(l.sources))
	}
}

func TestLoad_Convenience(t *testing.T) {
	cfg, err := Load("", "server")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should have defaults applied
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("Load() should apply defaults")
	}
}

func TestLoadServer_Convenience(t *testing.T) {
	cfg, err := LoadServer("")
	if err != nil {
		t.Fatalf("LoadServer() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("LoadServer() returned nil")
	}
}

func TestLoadClient_Convenience(t *testing.T) {
	cfg, err := LoadClient("")
	if err != nil {
		t.Fatalf("LoadClient() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("LoadClient() returned nil")
	}
}

// Test integration of multiple sources
func TestLoader_Integration(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create config.yaml
	configFile := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
server:
  protocols:
    tcp:
      port: 9000

log:
  level: info

storage:
  type: redis
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Create .env file
	envFile := filepath.Join(tmpDir, ".env")
	envContent := `TUNNOX_LOG_LEVEL=debug`
	if err := os.WriteFile(envFile, []byte(envContent), 0644); err != nil {
		t.Fatalf("Failed to write .env: %v", err)
	}

	// Clear and set real env var (highest priority)
	os.Unsetenv("TUNNOX_LOG_LEVEL")
	os.Setenv("TUNNOX_STORAGE_TYPE", "memory")
	defer os.Unsetenv("TUNNOX_STORAGE_TYPE")

	// Build loader
	l := NewLoaderBuilder().
		WithConfigFile(configFile).
		WithAppType("server").
		Build()

	cfg, err := l.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify layered config
	// TCP port from YAML
	if cfg.Server.Protocols.TCP.Port != 9000 {
		t.Errorf("TCP.Port = %d, want 9000 (from YAML)", cfg.Server.Protocols.TCP.Port)
	}

	// Storage type from real env var (highest priority)
	if cfg.Storage.Type != "memory" {
		t.Errorf("Storage.Type = %q, want %q (from env)", cfg.Storage.Type, "memory")
	}
}
