package source

import (
	"os"
	"testing"
	"time"

	"tunnox-core/internal/config/schema"
)

func TestEnvSource_Name(t *testing.T) {
	s := NewEnvSource("TUNNOX")
	if s.Name() != "env" {
		t.Errorf("Name() = %q, want %q", s.Name(), "env")
	}
}

func TestEnvSource_Priority(t *testing.T) {
	s := NewEnvSource("TUNNOX")
	if s.Priority() != PriorityEnv {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityEnv)
	}
}

func TestEnvSource_LoadInto_String(t *testing.T) {
	// Set up test environment
	os.Setenv("TUNNOX_LOG_LEVEL", "debug")
	defer os.Unsetenv("TUNNOX_LOG_LEVEL")

	cfg := &schema.Root{}
	s := NewEnvSource("TUNNOX")

	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
}

func TestEnvSource_LoadInto_Int(t *testing.T) {
	os.Setenv("TUNNOX_SERVER_TCP_PORT", "9000")
	defer os.Unsetenv("TUNNOX_SERVER_TCP_PORT")

	cfg := &schema.Root{}
	s := NewEnvSource("TUNNOX")

	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	if cfg.Server.Protocols.TCP.Port != 9000 {
		t.Errorf("TCP.Port = %d, want 9000", cfg.Server.Protocols.TCP.Port)
	}
}

func TestEnvSource_LoadInto_Bool(t *testing.T) {
	os.Setenv("TUNNOX_SERVER_TCP_ENABLED", "false")
	defer os.Unsetenv("TUNNOX_SERVER_TCP_ENABLED")

	cfg := &schema.Root{}
	cfg.Server.Protocols.TCP.Enabled = true // Set initial value

	s := NewEnvSource("TUNNOX")
	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	if cfg.Server.Protocols.TCP.Enabled != false {
		t.Error("TCP.Enabled should be false")
	}
}

func TestEnvSource_LoadInto_Duration(t *testing.T) {
	os.Setenv("TUNNOX_SESSION_HEARTBEAT_TIMEOUT", "120s")
	defer os.Unsetenv("TUNNOX_SESSION_HEARTBEAT_TIMEOUT")

	cfg := &schema.Root{}
	s := NewEnvSource("TUNNOX")

	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	if cfg.Server.Session.HeartbeatTimeout != 120*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want 120s", cfg.Server.Session.HeartbeatTimeout)
	}
}

func TestEnvSource_LoadInto_StringSlice(t *testing.T) {
	os.Setenv("TUNNOX_HTTP_BASE_DOMAINS", "example.com, test.dev, localhost.tunnox.dev")
	defer os.Unsetenv("TUNNOX_HTTP_BASE_DOMAINS")

	cfg := &schema.Root{}
	s := NewEnvSource("TUNNOX")

	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	expected := []string{"example.com", "test.dev", "localhost.tunnox.dev"}
	if len(cfg.HTTP.Modules.DomainProxy.BaseDomains) != len(expected) {
		t.Errorf("BaseDomains length = %d, want %d",
			len(cfg.HTTP.Modules.DomainProxy.BaseDomains), len(expected))
	}

	for i, v := range expected {
		if cfg.HTTP.Modules.DomainProxy.BaseDomains[i] != v {
			t.Errorf("BaseDomains[%d] = %q, want %q",
				i, cfg.HTTP.Modules.DomainProxy.BaseDomains[i], v)
		}
	}
}

func TestEnvSource_LoadInto_Secret(t *testing.T) {
	os.Setenv("TUNNOX_REDIS_PASSWORD", "secretpassword")
	defer os.Unsetenv("TUNNOX_REDIS_PASSWORD")

	cfg := &schema.Root{}
	s := NewEnvSource("TUNNOX")

	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	if cfg.Storage.Redis.Password.Value() != "secretpassword" {
		t.Errorf("Redis.Password = %q, want %q",
			cfg.Storage.Redis.Password.Value(), "secretpassword")
	}
}

// P0: Test backward compatible fallback without prefix
func TestEnvSource_BackwardCompatibleFallback(t *testing.T) {
	// Set env var without prefix (deprecated)
	os.Setenv("LOG_LEVEL", "warn")
	defer os.Unsetenv("LOG_LEVEL")

	cfg := &schema.Root{}
	s := NewEnvSource("TUNNOX")

	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	// Should still load from non-prefixed env var
	if cfg.Log.Level != "warn" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "warn")
	}

	// Check that deprecated var was tracked
	deprecatedVars := s.GetDeprecatedVars()
	found := false
	for _, v := range deprecatedVars {
		if v == "LOG_LEVEL" {
			found = true
			break
		}
	}
	if !found {
		t.Error("LOG_LEVEL should be tracked as deprecated")
	}
}

// P0: Test that prefixed env var takes precedence
func TestEnvSource_PrefixedTakesPrecedence(t *testing.T) {
	os.Setenv("LOG_LEVEL", "warn")
	os.Setenv("TUNNOX_LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("TUNNOX_LOG_LEVEL")
	}()

	cfg := &schema.Root{}
	s := NewEnvSource("TUNNOX")

	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	// Prefixed should take precedence
	if cfg.Log.Level != "debug" {
		t.Errorf("Log.Level = %q, want %q", cfg.Log.Level, "debug")
	}
}

func TestEnvSource_InvalidValueIgnored(t *testing.T) {
	os.Setenv("TUNNOX_SERVER_TCP_PORT", "not-a-number")
	defer os.Unsetenv("TUNNOX_SERVER_TCP_PORT")

	cfg := &schema.Root{}
	cfg.Server.Protocols.TCP.Port = 8000 // Set initial value

	s := NewEnvSource("TUNNOX")
	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	// Invalid value should be ignored, keep original
	if cfg.Server.Protocols.TCP.Port != 8000 {
		t.Errorf("TCP.Port = %d, want 8000 (unchanged)", cfg.Server.Protocols.TCP.Port)
	}
}
