package inference

import (
	"tunnox-core/internal/config/schema"
)

// InferenceRule defines an inference rule
type InferenceRule interface {
	// Name returns the rule name
	Name() string
	// Apply applies the rule to the configuration
	Apply(cfg *schema.Root, result *InferenceResult)
}

// ============================================================================
// Rule: Platform requires remote storage
// ============================================================================

// platformRequiresRemoteStorageRule implements the rule:
// platform.enabled=true -> storage.remote.enabled=true
type platformRequiresRemoteStorageRule struct{}

// NewPlatformRequiresRemoteStorageRule creates a new rule
func NewPlatformRequiresRemoteStorageRule() InferenceRule {
	return &platformRequiresRemoteStorageRule{}
}

func (r *platformRequiresRemoteStorageRule) Name() string {
	return "platform-requires-remote-storage"
}

func (r *platformRequiresRemoteStorageRule) Apply(cfg *schema.Root, result *InferenceResult) {
	if !cfg.Platform.Enabled {
		return
	}

	field := "storage.remote.enabled"
	inferredValue := true
	reason := "platform.enabled=true requires remote storage"

	if cfg.Storage.Remote.EnabledSet != nil && *cfg.Storage.Remote.EnabledSet {
		// User explicitly set the value
		if !cfg.Storage.Remote.Enabled {
			// User explicitly set to false, add warning
			result.AddWarning("platform.enabled=true but storage.remote.enabled=false may cause issues")
		}
		result.AddSkipped(field, inferredValue, reason)
		return
	}

	// Apply inference
	cfg.Storage.Remote.Enabled = true
	result.AddApplied(field, inferredValue, reason)
}

// ============================================================================
// Rule: Remote storage requires persistence
// ============================================================================

// remoteStorageRequiresPersistenceRule implements the rule:
// storage.remote.enabled=true -> persistence.enabled=true
type remoteStorageRequiresPersistenceRule struct{}

// NewRemoteStorageRequiresPersistenceRule creates a new rule
func NewRemoteStorageRequiresPersistenceRule() InferenceRule {
	return &remoteStorageRequiresPersistenceRule{}
}

func (r *remoteStorageRequiresPersistenceRule) Name() string {
	return "remote-storage-requires-persistence"
}

func (r *remoteStorageRequiresPersistenceRule) Apply(cfg *schema.Root, result *InferenceResult) {
	if !cfg.Storage.Remote.Enabled {
		return
	}

	field := "storage.persistence.enabled"
	inferredValue := true
	reason := "storage.remote.enabled=true requires local persistence for caching"

	if cfg.Storage.Persistence.EnabledSet != nil && *cfg.Storage.Persistence.EnabledSet {
		// User explicitly set the value
		result.AddSkipped(field, inferredValue, reason)
		return
	}

	// Apply inference
	cfg.Storage.Persistence.Enabled = true
	result.AddApplied(field, inferredValue, reason)
}

// ============================================================================
// Rule: Remote storage requires hybrid storage type
// ============================================================================

// remoteStorageRequiresHybridRule implements the rule:
// storage.remote.enabled=true -> storage.type=hybrid
type remoteStorageRequiresHybridRule struct{}

// NewRemoteStorageRequiresHybridRule creates a new rule
func NewRemoteStorageRequiresHybridRule() InferenceRule {
	return &remoteStorageRequiresHybridRule{}
}

func (r *remoteStorageRequiresHybridRule) Name() string {
	return "remote-storage-requires-hybrid"
}

func (r *remoteStorageRequiresHybridRule) Apply(cfg *schema.Root, result *InferenceResult) {
	if !cfg.Storage.Remote.Enabled {
		return
	}

	field := "storage.type"
	inferredValue := schema.StorageTypeHybrid
	reason := "storage.remote.enabled=true requires hybrid storage mode"

	if cfg.Storage.TypeSet != nil && *cfg.Storage.TypeSet {
		// User explicitly set the storage type
		if cfg.Storage.Type != schema.StorageTypeHybrid {
			result.AddWarning("storage.remote.enabled=true but storage.type is not hybrid, this may cause issues")
		}
		result.AddSkipped(field, inferredValue, reason)
		return
	}

	// Apply inference
	cfg.Storage.Type = schema.StorageTypeHybrid
	result.AddApplied(field, inferredValue, reason)
}

// ============================================================================
// Rule: Redis enabled requires redis storage type
// ============================================================================

// redisEnabledRequiresRedisTypeRule implements the rule:
// redis.enabled=true -> storage.type=redis (if remote storage is not enabled)
type redisEnabledRequiresRedisTypeRule struct{}

// NewRedisEnabledRequiresRedisTypeRule creates a new rule
func NewRedisEnabledRequiresRedisTypeRule() InferenceRule {
	return &redisEnabledRequiresRedisTypeRule{}
}

func (r *redisEnabledRequiresRedisTypeRule) Name() string {
	return "redis-enabled-requires-redis-type"
}

func (r *redisEnabledRequiresRedisTypeRule) Apply(cfg *schema.Root, result *InferenceResult) {
	// Only apply if redis is enabled and remote storage is not enabled
	if !cfg.Storage.Redis.Enabled || cfg.Storage.Remote.Enabled {
		return
	}

	field := "storage.type"
	inferredValue := schema.StorageTypeRedis
	reason := "storage.redis.enabled=true requires redis storage mode"

	if cfg.Storage.TypeSet != nil && *cfg.Storage.TypeSet {
		// User explicitly set the storage type
		if cfg.Storage.Type == schema.StorageTypeMemory {
			result.AddWarning("storage.redis.enabled=true but storage.type=memory, redis will not be used")
		}
		result.AddSkipped(field, inferredValue, reason)
		return
	}

	// Apply inference
	cfg.Storage.Type = schema.StorageTypeRedis
	result.AddApplied(field, inferredValue, reason)
}
