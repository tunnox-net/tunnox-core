package source

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"tunnox-core/internal/config/schema"
	coreerrors "tunnox-core/internal/core/errors"
)

// YAMLSource loads configuration from YAML files
type YAMLSource struct {
	paths []string // list of YAML file paths to load
}

// NewYAMLSource creates a new YAMLSource with the specified file paths
func NewYAMLSource(paths ...string) *YAMLSource {
	return &YAMLSource{
		paths: paths,
	}
}

// Name returns the source name
func (s *YAMLSource) Name() string {
	return "yaml"
}

// Priority returns the source priority
func (s *YAMLSource) Priority() int {
	return PriorityYAML
}

// LoadInto loads YAML configuration into the config structure
// Files are loaded in order, with later files overriding earlier ones
func (s *YAMLSource) LoadInto(cfg *schema.Root) error {
	for _, path := range s.paths {
		if path == "" {
			continue
		}

		// Expand path (handle ~ and relative paths)
		expandedPath, err := expandPath(path)
		if err != nil {
			return coreerrors.Wrapf(err, coreerrors.CodeInvalidParam, "failed to expand path %q", path)
		}

		// Check if file exists
		if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
			// Skip non-existent files silently
			continue
		}

		// Read file
		data, err := os.ReadFile(expandedPath)
		if err != nil {
			return coreerrors.Wrapf(err, coreerrors.CodeStorageError, "failed to read config file %q", expandedPath)
		}

		// Parse YAML
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return coreerrors.Wrapf(err, coreerrors.CodeInvalidParam, "failed to parse YAML file %q", expandedPath)
		}

		// Track which fields were explicitly set
		if err := trackExplicitlySetFields(data, cfg); err != nil {
			// Non-fatal: just log and continue
			continue
		}
	}

	return nil
}

// trackExplicitlySetFields parses YAML to track which fields were explicitly set
func trackExplicitlySetFields(data []byte, cfg *schema.Root) error {
	// Parse into a raw map to check field presence
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}

	trueVal := true

	// Check storage.type
	if storage, ok := raw["storage"].(map[string]interface{}); ok {
		if _, hasType := storage["type"]; hasType {
			cfg.Storage.TypeSet = &trueVal
		}

		// Check storage.persistence.enabled
		if persistence, ok := storage["persistence"].(map[string]interface{}); ok {
			if _, hasEnabled := persistence["enabled"]; hasEnabled {
				cfg.Storage.Persistence.EnabledSet = &trueVal
			}
		}

		// Check storage.remote.enabled
		if remote, ok := storage["remote"].(map[string]interface{}); ok {
			if _, hasEnabled := remote["enabled"]; hasEnabled {
				cfg.Storage.Remote.EnabledSet = &trueVal
			}
		}
	}

	return nil
}

// FindConfigFile searches for a configuration file in standard locations
// Returns the first found file path, or empty string if none found
func FindConfigFile(configFile string, appType string) string {
	// If explicitly specified, use that
	if configFile != "" {
		expanded, err := expandPath(configFile)
		if err == nil {
			if _, err := os.Stat(expanded); err == nil {
				return expanded
			}
		}
		return configFile
	}

	// Determine search paths based on app type
	var searchPaths []string

	if appType == "server" {
		searchPaths = []string{
			"./config.yaml",
			"./server.yaml",
		}

		// Add executable directory
		if execPath, err := os.Executable(); err == nil {
			execDir := filepath.Dir(execPath)
			searchPaths = append(searchPaths, filepath.Join(execDir, "config.yaml"))
			searchPaths = append(searchPaths, filepath.Join(execDir, "server.yaml"))
		}

		// Add system config directory
		searchPaths = append(searchPaths, "/etc/tunnox/config.yaml")
	} else if appType == "client" {
		searchPaths = []string{
			"./client-config.yaml",
			"./client.yaml",
		}

		// Add executable directory
		if execPath, err := os.Executable(); err == nil {
			execDir := filepath.Dir(execPath)
			searchPaths = append(searchPaths, filepath.Join(execDir, "client-config.yaml"))
			searchPaths = append(searchPaths, filepath.Join(execDir, "client.yaml"))
		}

		// Add user config directory
		if homeDir, err := os.UserHomeDir(); err == nil {
			searchPaths = append(searchPaths, filepath.Join(homeDir, ".tunnox", "client-config.yaml"))
			searchPaths = append(searchPaths, filepath.Join(homeDir, ".tunnox", "client.yaml"))
		}
	}

	// Search for first existing file
	for _, path := range searchPaths {
		expanded, err := expandPath(path)
		if err != nil {
			continue
		}
		if _, err := os.Stat(expanded); err == nil {
			return expanded
		}
	}

	return ""
}

// expandPath expands ~ to user home directory
func expandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get user home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[1:])
	}

	return filepath.Clean(path), nil
}
