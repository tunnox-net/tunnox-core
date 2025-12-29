package inference

import (
	"testing"

	"tunnox-core/internal/config/schema"
)

func TestPlatformRequiresRemoteStorageRule_Name(t *testing.T) {
	rule := NewPlatformRequiresRemoteStorageRule()
	if rule.Name() != "platform-requires-remote-storage" {
		t.Errorf("Expected rule name 'platform-requires-remote-storage', got '%s'", rule.Name())
	}
}

func TestPlatformRequiresRemoteStorageRule_Apply_NotTriggered(t *testing.T) {
	rule := NewPlatformRequiresRemoteStorageRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	// Platform is disabled
	cfg.Platform.Enabled = false

	rule.Apply(cfg, result)

	if cfg.Storage.Remote.Enabled {
		t.Error("Should not enable remote storage when platform is disabled")
	}
	if len(result.Applied) != 0 {
		t.Error("Should not have any applied actions")
	}
}

func TestPlatformRequiresRemoteStorageRule_Apply_Triggered(t *testing.T) {
	rule := NewPlatformRequiresRemoteStorageRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	// Platform is enabled
	cfg.Platform.Enabled = true

	rule.Apply(cfg, result)

	if !cfg.Storage.Remote.Enabled {
		t.Error("Should enable remote storage when platform is enabled")
	}
	if len(result.Applied) != 1 {
		t.Errorf("Expected 1 applied action, got %d", len(result.Applied))
	}
}

func TestPlatformRequiresRemoteStorageRule_Apply_UserOverride(t *testing.T) {
	rule := NewPlatformRequiresRemoteStorageRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	cfg.Platform.Enabled = true
	trueVal := true
	cfg.Storage.Remote.EnabledSet = &trueVal
	cfg.Storage.Remote.Enabled = true

	rule.Apply(cfg, result)

	// Should skip, not apply
	if len(result.Applied) != 0 {
		t.Error("Should not have applied actions when user override is set")
	}
	if len(result.Skipped) != 1 {
		t.Errorf("Expected 1 skipped action, got %d", len(result.Skipped))
	}
}

func TestPlatformRequiresRemoteStorageRule_Apply_UserOverrideConflict(t *testing.T) {
	rule := NewPlatformRequiresRemoteStorageRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	cfg.Platform.Enabled = true
	trueVal := true
	cfg.Storage.Remote.EnabledSet = &trueVal
	cfg.Storage.Remote.Enabled = false // Conflict!

	rule.Apply(cfg, result)

	// Should have warning
	if len(result.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestRemoteStorageRequiresPersistenceRule_Name(t *testing.T) {
	rule := NewRemoteStorageRequiresPersistenceRule()
	if rule.Name() != "remote-storage-requires-persistence" {
		t.Errorf("Expected rule name 'remote-storage-requires-persistence', got '%s'", rule.Name())
	}
}

func TestRemoteStorageRequiresPersistenceRule_Apply_Triggered(t *testing.T) {
	rule := NewRemoteStorageRequiresPersistenceRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	cfg.Storage.Remote.Enabled = true

	rule.Apply(cfg, result)

	if !cfg.Storage.Persistence.Enabled {
		t.Error("Should enable persistence when remote storage is enabled")
	}
}

func TestRemoteStorageRequiresHybridRule_Name(t *testing.T) {
	rule := NewRemoteStorageRequiresHybridRule()
	if rule.Name() != "remote-storage-requires-hybrid" {
		t.Errorf("Expected rule name 'remote-storage-requires-hybrid', got '%s'", rule.Name())
	}
}

func TestRemoteStorageRequiresHybridRule_Apply_Triggered(t *testing.T) {
	rule := NewRemoteStorageRequiresHybridRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	cfg.Storage.Remote.Enabled = true

	rule.Apply(cfg, result)

	if cfg.Storage.Type != schema.StorageTypeHybrid {
		t.Errorf("Expected storage.type to be 'hybrid', got '%s'", cfg.Storage.Type)
	}
}

func TestRemoteStorageRequiresHybridRule_Apply_UserOverrideConflict(t *testing.T) {
	rule := NewRemoteStorageRequiresHybridRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	cfg.Storage.Remote.Enabled = true
	trueVal := true
	cfg.Storage.TypeSet = &trueVal
	cfg.Storage.Type = schema.StorageTypeMemory // Conflict!

	rule.Apply(cfg, result)

	// Should have warning
	if len(result.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(result.Warnings))
	}

	// Should skip
	if len(result.Skipped) != 1 {
		t.Errorf("Expected 1 skipped action, got %d", len(result.Skipped))
	}

	// Type should remain as user set
	if cfg.Storage.Type != schema.StorageTypeMemory {
		t.Errorf("Expected storage.type to remain 'memory', got '%s'", cfg.Storage.Type)
	}
}

func TestRedisEnabledRequiresRedisTypeRule_Name(t *testing.T) {
	rule := NewRedisEnabledRequiresRedisTypeRule()
	if rule.Name() != "redis-enabled-requires-redis-type" {
		t.Errorf("Expected rule name 'redis-enabled-requires-redis-type', got '%s'", rule.Name())
	}
}

func TestRedisEnabledRequiresRedisTypeRule_Apply_Triggered(t *testing.T) {
	rule := NewRedisEnabledRequiresRedisTypeRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Remote.Enabled = false

	rule.Apply(cfg, result)

	if cfg.Storage.Type != schema.StorageTypeRedis {
		t.Errorf("Expected storage.type to be 'redis', got '%s'", cfg.Storage.Type)
	}
}

func TestRedisEnabledRequiresRedisTypeRule_Apply_NotTriggeredWhenRemoteEnabled(t *testing.T) {
	rule := NewRedisEnabledRequiresRedisTypeRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Remote.Enabled = true // Remote takes precedence

	rule.Apply(cfg, result)

	// Should not change type since remote storage takes precedence
	if len(result.Applied) != 0 {
		t.Error("Should not apply when remote storage is enabled")
	}
}

func TestRedisEnabledRequiresRedisTypeRule_Apply_UserOverrideConflict(t *testing.T) {
	rule := NewRedisEnabledRequiresRedisTypeRule()
	cfg := &schema.Root{}
	result := NewInferenceResult()

	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Remote.Enabled = false
	trueVal := true
	cfg.Storage.TypeSet = &trueVal
	cfg.Storage.Type = schema.StorageTypeMemory // Conflict!

	rule.Apply(cfg, result)

	// Should have warning
	if len(result.Warnings) != 1 {
		t.Errorf("Expected 1 warning, got %d", len(result.Warnings))
	}
}
