// Package source provides configuration source abstractions and implementations
package source

import (
	"tunnox-core/internal/config/schema"
)

// Source is the interface for configuration sources
// Each source loads configuration into a strongly-typed Root structure
type Source interface {
	// Name returns the source name for logging and error messages
	Name() string

	// Priority returns the source priority (higher = more important)
	// Priority order:
	// 1 - Default values (lowest)
	// 2 - YAML files
	// 3 - .env files
	// 4 - Environment variables
	// 5 - CLI flags (highest)
	Priority() int

	// LoadInto loads configuration into the provided config structure
	// Only non-zero values are set, preserving existing values from lower-priority sources
	LoadInto(cfg *schema.Root) error
}

// SourcePriority constants
const (
	PriorityDefaults = 1
	PriorityYAML     = 2
	PriorityDotEnv   = 3
	PriorityEnv      = 4
	PriorityCLI      = 5
)

// ByPriority implements sort.Interface for []Source based on Priority
type ByPriority []Source

func (a ByPriority) Len() int           { return len(a) }
func (a ByPriority) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByPriority) Less(i, j int) bool { return a[i].Priority() < a[j].Priority() }
