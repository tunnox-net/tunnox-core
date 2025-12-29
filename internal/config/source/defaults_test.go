package source

import (
	"testing"
	"time"

	"tunnox-core/internal/config/schema"
)

func TestDefaultSource_Name(t *testing.T) {
	s := NewDefaultSource()
	if s.Name() != "defaults" {
		t.Errorf("Name() = %q, want %q", s.Name(), "defaults")
	}
}

func TestDefaultSource_Priority(t *testing.T) {
	s := NewDefaultSource()
	if s.Priority() != PriorityDefaults {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityDefaults)
	}
}

func TestDefaultSource_LoadInto(t *testing.T) {
	cfg := &schema.Root{}
	s := NewDefaultSource()

	err := s.LoadInto(cfg)
	if err != nil {
		t.Fatalf("LoadInto() error = %v", err)
	}

	// Verify server protocol defaults
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("TCP should be enabled by default")
	}
	if cfg.Server.Protocols.TCP.Port != 8000 {
		t.Errorf("TCP port = %d, want 8000", cfg.Server.Protocols.TCP.Port)
	}
	if cfg.Server.Protocols.TCP.Host != "0.0.0.0" {
		t.Errorf("TCP host = %q, want %q", cfg.Server.Protocols.TCP.Host, "0.0.0.0")
	}

	if !cfg.Server.Protocols.WebSocket.Enabled {
		t.Error("WebSocket should be enabled by default")
	}

	if !cfg.Server.Protocols.KCP.Enabled {
		t.Error("KCP should be enabled by default")
	}
	if cfg.Server.Protocols.KCP.Mode != schema.KCPModeFast {
		t.Errorf("KCP mode = %q, want %q", cfg.Server.Protocols.KCP.Mode, schema.KCPModeFast)
	}

	if !cfg.Server.Protocols.QUIC.Enabled {
		t.Error("QUIC should be enabled by default")
	}
	if cfg.Server.Protocols.QUIC.Port != 8443 {
		t.Errorf("QUIC port = %d, want 8443", cfg.Server.Protocols.QUIC.Port)
	}

	// Verify session defaults
	if cfg.Server.Session.HeartbeatTimeout != 60*time.Second {
		t.Errorf("HeartbeatTimeout = %v, want 60s", cfg.Server.Session.HeartbeatTimeout)
	}
	if cfg.Server.Session.MaxConnections != 10000 {
		t.Errorf("MaxConnections = %d, want 10000", cfg.Server.Session.MaxConnections)
	}

	// Verify client defaults
	if !cfg.Client.Anonymous {
		t.Error("Client should be anonymous by default")
	}
	if cfg.Client.Server.Protocol != schema.ProtocolWebSocket {
		t.Errorf("Client protocol = %q, want %q", cfg.Client.Server.Protocol, schema.ProtocolWebSocket)
	}

	// Verify HTTP defaults - P0: base_domains should have default
	if len(cfg.HTTP.Modules.DomainProxy.BaseDomains) == 0 {
		t.Error("HTTP base_domains should have default value")
	}
	if cfg.HTTP.Modules.DomainProxy.BaseDomains[0] != schema.DefaultBaseDomain {
		t.Errorf("HTTP base_domains[0] = %q, want %q",
			cfg.HTTP.Modules.DomainProxy.BaseDomains[0], schema.DefaultBaseDomain)
	}

	// Verify health defaults
	if !cfg.Health.Enabled {
		t.Error("Health should be enabled by default")
	}
	if cfg.Health.Listen != "0.0.0.0:9090" {
		t.Errorf("Health listen = %q, want %q", cfg.Health.Listen, "0.0.0.0:9090")
	}
	if cfg.Health.Endpoints.Liveness != "/healthz" {
		t.Errorf("Liveness endpoint = %q, want %q", cfg.Health.Endpoints.Liveness, "/healthz")
	}

	// Verify log defaults
	if cfg.Log.Level != schema.LogLevelInfo {
		t.Errorf("Log level = %q, want %q", cfg.Log.Level, schema.LogLevelInfo)
	}
	if cfg.Log.Rotation.MaxSize != 100 {
		t.Errorf("Log rotation max_size = %d, want 100", cfg.Log.Rotation.MaxSize)
	}

	// Verify storage defaults
	if cfg.Storage.Type != schema.StorageTypeMemory {
		t.Errorf("Storage type = %q, want %q", cfg.Storage.Type, schema.StorageTypeMemory)
	}
}

func TestGetDefaultConfig(t *testing.T) {
	cfg := GetDefaultConfig()

	if cfg == nil {
		t.Fatal("GetDefaultConfig() returned nil")
	}

	// Just verify a few key values
	if !cfg.Server.Protocols.TCP.Enabled {
		t.Error("TCP should be enabled by default")
	}
	if !cfg.Health.Enabled {
		t.Error("Health should be enabled by default")
	}
}
