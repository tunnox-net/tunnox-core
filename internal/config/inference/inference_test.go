package inference

import (
	"testing"

	"tunnox-core/internal/config/schema"
)

func TestNewInferenceResult(t *testing.T) {
	result := NewInferenceResult()

	if result == nil {
		t.Fatal("NewInferenceResult returned nil")
	}

	if len(result.Applied) != 0 {
		t.Errorf("Expected empty Applied slice, got %d elements", len(result.Applied))
	}

	if len(result.Skipped) != 0 {
		t.Errorf("Expected empty Skipped slice, got %d elements", len(result.Skipped))
	}

	if len(result.Warnings) != 0 {
		t.Errorf("Expected empty Warnings slice, got %d elements", len(result.Warnings))
	}
}

func TestInferenceResult_AddApplied(t *testing.T) {
	result := NewInferenceResult()
	result.AddApplied("storage.type", "hybrid", "test reason")

	if len(result.Applied) != 1 {
		t.Fatalf("Expected 1 applied action, got %d", len(result.Applied))
	}

	action := result.Applied[0]
	if action.Field != "storage.type" {
		t.Errorf("Expected field 'storage.type', got '%s'", action.Field)
	}
	if action.Value != "hybrid" {
		t.Errorf("Expected value 'hybrid', got '%v'", action.Value)
	}
	if action.Reason != "test reason" {
		t.Errorf("Expected reason 'test reason', got '%s'", action.Reason)
	}
}

func TestInferenceResult_AddSkipped(t *testing.T) {
	result := NewInferenceResult()
	result.AddSkipped("storage.persistence.enabled", true, "user override")

	if len(result.Skipped) != 1 {
		t.Fatalf("Expected 1 skipped action, got %d", len(result.Skipped))
	}

	action := result.Skipped[0]
	if action.Field != "storage.persistence.enabled" {
		t.Errorf("Expected field 'storage.persistence.enabled', got '%s'", action.Field)
	}
}

func TestInferenceResult_AddWarning(t *testing.T) {
	result := NewInferenceResult()
	result.AddWarning("test warning")

	if len(result.Warnings) != 1 {
		t.Fatalf("Expected 1 warning, got %d", len(result.Warnings))
	}

	if result.Warnings[0] != "test warning" {
		t.Errorf("Expected warning 'test warning', got '%s'", result.Warnings[0])
	}
}

func TestInferenceResult_HasChanges(t *testing.T) {
	result := NewInferenceResult()

	if result.HasChanges() {
		t.Error("Expected HasChanges to return false for empty result")
	}

	result.AddApplied("field", "value", "reason")
	if !result.HasChanges() {
		t.Error("Expected HasChanges to return true after adding applied action")
	}
}

func TestInferenceResult_HasWarnings(t *testing.T) {
	result := NewInferenceResult()

	if result.HasWarnings() {
		t.Error("Expected HasWarnings to return false for empty result")
	}

	result.AddWarning("warning")
	if !result.HasWarnings() {
		t.Error("Expected HasWarnings to return true after adding warning")
	}
}

func TestNewEngine(t *testing.T) {
	engine := NewEngine()

	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}

	// Engine should have default rules registered
	if len(engine.rules) == 0 {
		t.Error("Expected default rules to be registered")
	}
}

func TestEngine_Infer_PlatformRequiresRemoteStorage(t *testing.T) {
	engine := NewEngine()
	cfg := &schema.Root{}

	// Enable platform
	cfg.Platform.Enabled = true

	result := engine.Infer(cfg)

	// Should infer storage.remote.enabled = true
	if !cfg.Storage.Remote.Enabled {
		t.Error("Expected storage.remote.enabled to be inferred as true")
	}

	// Should have applied action
	found := false
	for _, action := range result.Applied {
		if action.Field == "storage.remote.enabled" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected applied action for storage.remote.enabled")
	}
}

func TestEngine_Infer_RemoteStorageRequiresPersistence(t *testing.T) {
	engine := NewEngine()
	cfg := &schema.Root{}

	// Enable remote storage
	cfg.Storage.Remote.Enabled = true

	result := engine.Infer(cfg)

	// Should infer persistence.enabled = true
	if !cfg.Storage.Persistence.Enabled {
		t.Error("Expected storage.persistence.enabled to be inferred as true")
	}

	// Should have applied action
	found := false
	for _, action := range result.Applied {
		if action.Field == "storage.persistence.enabled" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected applied action for storage.persistence.enabled")
	}
}

func TestEngine_Infer_RemoteStorageRequiresHybrid(t *testing.T) {
	engine := NewEngine()
	cfg := &schema.Root{}

	// Enable remote storage
	cfg.Storage.Remote.Enabled = true

	result := engine.Infer(cfg)

	// Should infer storage.type = hybrid
	if cfg.Storage.Type != schema.StorageTypeHybrid {
		t.Errorf("Expected storage.type to be 'hybrid', got '%s'", cfg.Storage.Type)
	}

	// Should have applied action
	found := false
	for _, action := range result.Applied {
		if action.Field == "storage.type" && action.Value == schema.StorageTypeHybrid {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected applied action for storage.type = hybrid")
	}
}

func TestEngine_Infer_RedisEnabledRequiresRedisType(t *testing.T) {
	engine := NewEngine()
	cfg := &schema.Root{}

	// Enable redis but not remote storage
	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Remote.Enabled = false

	result := engine.Infer(cfg)

	// Should infer storage.type = redis
	if cfg.Storage.Type != schema.StorageTypeRedis {
		t.Errorf("Expected storage.type to be 'redis', got '%s'", cfg.Storage.Type)
	}

	// Should have applied action
	found := false
	for _, action := range result.Applied {
		if action.Field == "storage.type" && action.Value == schema.StorageTypeRedis {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected applied action for storage.type = redis")
	}
}

func TestEngine_Infer_UserOverride(t *testing.T) {
	engine := NewEngine()
	cfg := &schema.Root{}

	// Enable platform
	cfg.Platform.Enabled = true

	// User explicitly set storage.remote.enabled to true
	trueVal := true
	cfg.Storage.Remote.EnabledSet = &trueVal
	cfg.Storage.Remote.Enabled = true

	result := engine.Infer(cfg)

	// Should skip inference for storage.remote.enabled
	found := false
	for _, action := range result.Skipped {
		if action.Field == "storage.remote.enabled" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected skipped action for storage.remote.enabled when user override is set")
	}
}

func TestEngine_Infer_UserOverrideWithConflict(t *testing.T) {
	engine := NewEngine()
	cfg := &schema.Root{}

	// Enable platform
	cfg.Platform.Enabled = true

	// User explicitly set storage.remote.enabled to false (conflict)
	trueVal := true
	cfg.Storage.Remote.EnabledSet = &trueVal
	cfg.Storage.Remote.Enabled = false

	result := engine.Infer(cfg)

	// Should add warning about conflict
	if !result.HasWarnings() {
		t.Error("Expected warning about platform.enabled=true but storage.remote.enabled=false")
	}
}

func TestEngine_Infer_ChainedInference(t *testing.T) {
	engine := NewEngine()
	cfg := &schema.Root{}

	// Enable platform - should trigger chain of inferences
	cfg.Platform.Enabled = true

	engine.Infer(cfg)

	// Should infer storage.remote.enabled = true
	if !cfg.Storage.Remote.Enabled {
		t.Error("Expected storage.remote.enabled to be true")
	}

	// Should infer storage.persistence.enabled = true
	if !cfg.Storage.Persistence.Enabled {
		t.Error("Expected storage.persistence.enabled to be true")
	}

	// Should infer storage.type = hybrid
	if cfg.Storage.Type != schema.StorageTypeHybrid {
		t.Errorf("Expected storage.type to be 'hybrid', got '%s'", cfg.Storage.Type)
	}
}

func TestEngine_Infer_NoChangesWhenNotNeeded(t *testing.T) {
	engine := NewEngine()
	cfg := &schema.Root{}

	// Don't enable anything that triggers inference
	cfg.Platform.Enabled = false
	cfg.Storage.Remote.Enabled = false
	cfg.Storage.Redis.Enabled = false

	result := engine.Infer(cfg)

	if result.HasChanges() {
		t.Error("Expected no changes when nothing triggers inference")
	}
}
