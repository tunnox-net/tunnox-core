// Package inference provides configuration auto-inference capabilities
package inference

import (
	"tunnox-core/internal/config/schema"
)

// Inferencer defines the interface for configuration inference
type Inferencer interface {
	// Infer applies inference rules to the configuration
	Infer(cfg *schema.Root) *InferenceResult
}

// InferenceResult contains the result of configuration inference
type InferenceResult struct {
	Applied  []InferenceAction // Successfully applied inferences
	Skipped  []InferenceAction // Skipped inferences (user already set)
	Warnings []string          // Warnings during inference
}

// InferenceAction represents a single inference action
type InferenceAction struct {
	Field  string      // Configuration field path (e.g., "storage.remote.enabled")
	Value  interface{} // The inferred value
	Reason string      // Why this inference was made
}

// NewInferenceResult creates a new empty InferenceResult
func NewInferenceResult() *InferenceResult {
	return &InferenceResult{
		Applied:  make([]InferenceAction, 0),
		Skipped:  make([]InferenceAction, 0),
		Warnings: make([]string, 0),
	}
}

// AddApplied adds an applied inference action
func (r *InferenceResult) AddApplied(field string, value interface{}, reason string) {
	r.Applied = append(r.Applied, InferenceAction{
		Field:  field,
		Value:  value,
		Reason: reason,
	})
}

// AddSkipped adds a skipped inference action
func (r *InferenceResult) AddSkipped(field string, value interface{}, reason string) {
	r.Skipped = append(r.Skipped, InferenceAction{
		Field:  field,
		Value:  value,
		Reason: reason,
	})
}

// AddWarning adds a warning message
func (r *InferenceResult) AddWarning(message string) {
	r.Warnings = append(r.Warnings, message)
}

// HasChanges returns true if any inferences were applied
func (r *InferenceResult) HasChanges() bool {
	return len(r.Applied) > 0
}

// HasWarnings returns true if there are warnings
func (r *InferenceResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// Engine is the default implementation of Inferencer
type Engine struct {
	rules []InferenceRule
}

// NewEngine creates a new inference engine with default rules
func NewEngine() *Engine {
	e := &Engine{
		rules: make([]InferenceRule, 0),
	}
	// Register default rules
	e.registerDefaultRules()
	return e
}

// AddRule adds an inference rule to the engine
func (e *Engine) AddRule(rule InferenceRule) {
	e.rules = append(e.rules, rule)
}

// Infer applies all inference rules to the configuration
func (e *Engine) Infer(cfg *schema.Root) *InferenceResult {
	result := NewInferenceResult()

	for _, rule := range e.rules {
		rule.Apply(cfg, result)
	}

	return result
}

// registerDefaultRules registers all default inference rules
func (e *Engine) registerDefaultRules() {
	e.AddRule(NewPlatformRequiresRemoteStorageRule())
	e.AddRule(NewRemoteStorageRequiresPersistenceRule())
	e.AddRule(NewRemoteStorageRequiresHybridRule())
	e.AddRule(NewRedisEnabledRequiresRedisTypeRule())
}
