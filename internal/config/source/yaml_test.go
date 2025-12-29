package source

import (
	"os"
	"path/filepath"
	"testing"

	"tunnox-core/internal/config/schema"
)

func TestYAMLSource_Name(t *testing.T) {
	s := NewYAMLSource("config.yaml")
	if s.Name() != "yaml" {
		t.Errorf("Name() = %q, want %q", s.Name(), "yaml")
	}
}

func TestYAMLSource_Priority(t *testing.T) {
	s := NewYAMLSource("config.yaml")
	if s.Priority() != PriorityYAML {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityYAML)
	}
}

func TestYAMLSource_LoadInto_NonExistent(t *testing.T) {
	cfg := &schema.Root{}
	s := NewYAMLSource("/nonexistent/path/config.yaml")

	err := s.LoadInto(cfg)
	if err != nil {
		t.Errorf("LoadInto() should not error on non-existent file, got %v", err)
	}
}

func TestYAMLSource_LoadInto_ValidFile(t *testing.T) {
	// Create temp directory and file
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.yaml")
	yamlContent := `
server:
  protocols:
    tcp:
      enabled: true
      port: 9999
      host: "127.0.0.1"
    kcp:
      mode: "fast2"

log:
  level: debug
  format: json

storage:
  type: redis
  redis:
    enabled: true
    addr: "redis:6379"
`
	if err := os.WriteFile(configFile, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg := &schema.Root{}
	s := NewYAMLSource(configFile)

	err = s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	// Verify values
	if cfg.Server.Protocols.TCP.Port != 9999 {
		t.Errorf("TCP.Port = %d, want 9999", cfg.Server.Protocols.TCP.Port)
	}
	if cfg.Server.Protocols.TCP.Host != "127.0.0.1" {
		t.Errorf("TCP.Host = %q, want %q", cfg.Server.Protocols.TCP.Host, "127.0.0.1")
	}
	if cfg.Server.Protocols.KCP.Mode != "fast2" {
		t.Errorf("KCP.Mode = %q, want %q", cfg.Server.Protocols.KCP.Mode, "fast2")
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
	if cfg.Storage.Type != "redis" {
		t.Errorf("Storage.Type = %q, want %q", cfg.Storage.Type, "redis")
	}
	if cfg.Storage.Redis.Addr != "redis:6379" {
		t.Errorf("Redis.Addr = %q, want %q", cfg.Storage.Redis.Addr, "redis:6379")
	}
}

func TestYAMLSource_LoadInto_InvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "invalid.yaml")
	invalidContent := `
server:
  protocols:
    tcp:
      port: [invalid yaml here
`
	if err := os.WriteFile(configFile, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg := &schema.Root{}
	s := NewYAMLSource(configFile)

	err = s.LoadInto(cfg)
	if err == nil {
		t.Error("LoadInto() should error on invalid YAML")
	}
}

func TestYAMLSource_LoadInto_MultipleFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// First file sets base config
	configFile1 := filepath.Join(tmpDir, "config.yaml")
	content1 := `
server:
  protocols:
    tcp:
      port: 8000

log:
  level: info
`
	if err := os.WriteFile(configFile1, []byte(content1), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Second file overrides some values
	configFile2 := filepath.Join(tmpDir, "config.local.yaml")
	content2 := `
log:
  level: debug
`
	if err := os.WriteFile(configFile2, []byte(content2), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg := &schema.Root{}
	s := NewYAMLSource(configFile1, configFile2)

	err = s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	// First file's value should be preserved
	if cfg.Server.Protocols.TCP.Port != 8000 {
		t.Errorf("TCP.Port = %d, want 8000", cfg.Server.Protocols.TCP.Port)
	}

	// Second file should override log level
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q (overridden by local)", cfg.Log.Level, "debug")
	}
}

func TestExpandPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"empty", "", false},
		{"relative", "./config.yaml", false},
		{"absolute", "/etc/tunnox/config.yaml", false},
		{"home dir", "~/config.yaml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := expandPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("expandPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if tt.path == "~/config.yaml" && result == tt.path {
				t.Error("expandPath should expand ~")
			}
		})
	}
}

func TestFindConfigFile(t *testing.T) {
	// Create temp directory with config file
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("server: {}"), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Test with explicit path
	result := FindConfigFile(configFile, "server")
	if result != configFile {
		t.Errorf("FindConfigFile() = %q, want %q", result, configFile)
	}

	// Test with non-existent explicit path
	result = FindConfigFile("/nonexistent/config.yaml", "server")
	if result != "/nonexistent/config.yaml" {
		t.Errorf("FindConfigFile() should return explicit path even if not found")
	}

	// Test with empty path (will search standard locations)
	result = FindConfigFile("", "server")
	// Result depends on current directory, just ensure no panic
}
