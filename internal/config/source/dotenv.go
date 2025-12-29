package source

import (
	"os"
	"path/filepath"

	"tunnox-core/internal/config/schema"
	corelog "tunnox-core/internal/core/log"
)

// DotEnvSource loads configuration from .env files
type DotEnvSource struct {
	dirs   []string // directories to search for .env files
	appEnv string   // application environment (e.g., production, development)
	prefix string   // environment variable prefix for loading
}

// NewDotEnvSource creates a new DotEnvSource
func NewDotEnvSource(prefix string, dirs []string, appEnv string) *DotEnvSource {
	return &DotEnvSource{
		dirs:   dirs,
		appEnv: appEnv,
		prefix: prefix,
	}
}

// Name returns the source name
func (s *DotEnvSource) Name() string {
	return "dotenv"
}

// Priority returns the source priority
func (s *DotEnvSource) Priority() int {
	return PriorityDotEnv
}

// LoadInto loads .env files and then applies env vars to config
func (s *DotEnvSource) LoadInto(cfg *schema.Root) error {
	// Load .env files into process environment
	if err := s.loadDotEnvFiles(); err != nil {
		return err
	}

	// Then use EnvSource to load into config
	// Note: We don't actually call EnvSource here because
	// the env vars are already in the environment and will be
	// picked up by the EnvSource in the loading chain
	return nil
}

// loadDotEnvFiles loads .env files into the process environment
func (s *DotEnvSource) loadDotEnvFiles() error {
	// Build list of .env files to load (from lowest to highest priority)
	files := []string{
		".env",
		".env.local",
	}

	if s.appEnv != "" {
		files = append(files, ".env."+s.appEnv)
		files = append(files, ".env."+s.appEnv+".local")
	}

	// Load files from each directory
	for _, dir := range s.dirs {
		for _, file := range files {
			path := filepath.Join(dir, file)
			if err := s.loadEnvFile(path); err != nil {
				// Log warning but continue with other files
				corelog.Debugf("Failed to load %s: %v", path, err)
			}
		}
	}

	return nil
}

// loadEnvFile loads a single .env file
func (s *DotEnvSource) loadEnvFile(path string) error {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // Skip non-existent files
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Parse and set environment variables
	lines := splitLines(string(data))
	for _, line := range lines {
		line = trimSpace(line)

		// Skip empty lines and comments
		if line == "" || line[0] == '#' {
			continue
		}

		// Parse key=value
		key, value, ok := parseEnvLine(line)
		if !ok {
			continue
		}

		// Only set if not already set (allow env vars to override .env files)
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				corelog.Warnf("Failed to set env var %s from %s: %v", key, path, err)
			}
		}
	}

	corelog.Debugf("Loaded env file: %s", path)
	return nil
}

// parseEnvLine parses a line from a .env file
func parseEnvLine(line string) (key, value string, ok bool) {
	// Find the first = sign
	idx := -1
	for i := 0; i < len(line); i++ {
		if line[i] == '=' {
			idx = i
			break
		}
	}
	if idx == -1 {
		return "", "", false
	}

	key = trimSpace(line[:idx])
	value = trimSpace(line[idx+1:])

	// Remove quotes from value
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}

	// Skip empty keys
	if key == "" {
		return "", "", false
	}

	return key, value, true
}

// splitLines splits a string into lines
func splitLines(s string) []string {
	var lines []string
	var current string
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, current)
			current = ""
		} else if s[i] == '\r' {
			// Skip carriage return
			continue
		} else {
			current += string(s[i])
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

// trimSpace removes leading and trailing whitespace
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// FindDotEnvDirs finds directories that might contain .env files
func FindDotEnvDirs(configFile string) []string {
	var dirs []string

	// Config file directory
	if configFile != "" {
		if dir := filepath.Dir(configFile); dir != "" && dir != "." {
			dirs = append(dirs, dir)
		}
	}

	// Current working directory
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, cwd)
	}

	// User home directory
	if home, err := os.UserHomeDir(); err == nil {
		tunnoxDir := filepath.Join(home, ".tunnox")
		dirs = append(dirs, tunnoxDir)
	}

	return dirs
}
