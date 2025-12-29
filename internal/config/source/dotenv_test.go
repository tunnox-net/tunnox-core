package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDotEnvSource_Name(t *testing.T) {
	s := NewDotEnvSource("TUNNOX", nil, "")
	if s.Name() != "dotenv" {
		t.Errorf("Name() = %q, want %q", s.Name(), "dotenv")
	}
}

func TestDotEnvSource_Priority(t *testing.T) {
	s := NewDotEnvSource("TUNNOX", nil, "")
	if s.Priority() != PriorityDotEnv {
		t.Errorf("Priority() = %d, want %d", s.Priority(), PriorityDotEnv)
	}
}

func TestParseEnvLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{"simple", "KEY=value", "KEY", "value", true},
		{"with spaces", "KEY = value", "KEY", "value", true},
		{"quoted value", `KEY="quoted value"`, "KEY", "quoted value", true},
		{"single quoted", `KEY='single quoted'`, "KEY", "single quoted", true},
		{"empty value", "KEY=", "KEY", "", true},
		{"no equals", "INVALID", "", "", false},
		{"empty key", "=value", "", "", false},
		{"complex value", "KEY=http://example.com?foo=bar", "KEY", "http://example.com?foo=bar", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, ok := parseEnvLine(tt.line)
			if ok != tt.wantOK {
				t.Errorf("parseEnvLine(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			}
			if key != tt.wantKey {
				t.Errorf("parseEnvLine(%q) key = %q, want %q", tt.line, key, tt.wantKey)
			}
			if value != tt.wantValue {
				t.Errorf("parseEnvLine(%q) value = %q, want %q", tt.line, value, tt.wantValue)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"simple", "a\nb\nc", []string{"a", "b", "c"}},
		{"with crlf", "a\r\nb\r\nc", []string{"a", "b", "c"}},
		{"empty lines", "a\n\nb", []string{"a", "", "b"}},
		{"single line", "single", []string{"single"}},
		{"trailing newline", "a\nb\n", []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLines(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("splitLines() len = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i, v := range tt.expected {
				if result[i] != v {
					t.Errorf("splitLines()[%d] = %q, want %q", i, result[i], v)
				}
			}
		})
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  hello  ", "hello"},
		{"\thello\t", "hello"},
		{"hello", "hello"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		result := trimSpace(tt.input)
		if result != tt.expected {
			t.Errorf("trimSpace(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestDotEnvSource_LoadEnvFile(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .env file
	envFile := filepath.Join(tmpDir, ".env")
	content := `
# Comment
TUNNOX_TEST_VAR1=value1
TUNNOX_TEST_VAR2="quoted value"

# Another comment
TUNNOX_TEST_VAR3=123
`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write .env file: %v", err)
	}

	// Clear any existing test vars
	os.Unsetenv("TUNNOX_TEST_VAR1")
	os.Unsetenv("TUNNOX_TEST_VAR2")
	os.Unsetenv("TUNNOX_TEST_VAR3")

	s := NewDotEnvSource("TUNNOX", []string{tmpDir}, "")
	err = s.loadEnvFile(envFile)
	if err != nil {
		t.Fatalf("loadEnvFile() error = %v", err)
	}

	// Verify environment variables were set
	if v := os.Getenv("TUNNOX_TEST_VAR1"); v != "value1" {
		t.Errorf("TUNNOX_TEST_VAR1 = %q, want %q", v, "value1")
	}
	if v := os.Getenv("TUNNOX_TEST_VAR2"); v != "quoted value" {
		t.Errorf("TUNNOX_TEST_VAR2 = %q, want %q", v, "quoted value")
	}
	if v := os.Getenv("TUNNOX_TEST_VAR3"); v != "123" {
		t.Errorf("TUNNOX_TEST_VAR3 = %q, want %q", v, "123")
	}

	// Cleanup
	os.Unsetenv("TUNNOX_TEST_VAR1")
	os.Unsetenv("TUNNOX_TEST_VAR2")
	os.Unsetenv("TUNNOX_TEST_VAR3")
}

func TestDotEnvSource_ExistingEnvNotOverwritten(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set existing env var
	os.Setenv("TUNNOX_TEST_EXISTING", "original")
	defer os.Unsetenv("TUNNOX_TEST_EXISTING")

	// Create .env file trying to override
	envFile := filepath.Join(tmpDir, ".env")
	content := `TUNNOX_TEST_EXISTING=overridden`
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write .env file: %v", err)
	}

	s := NewDotEnvSource("TUNNOX", []string{tmpDir}, "")
	err = s.loadEnvFile(envFile)
	if err != nil {
		t.Fatalf("loadEnvFile() error = %v", err)
	}

	// Original value should be preserved
	if v := os.Getenv("TUNNOX_TEST_EXISTING"); v != "original" {
		t.Errorf("TUNNOX_TEST_EXISTING = %q, want %q (should not be overwritten)", v, "original")
	}
}

func TestFindDotEnvDirs(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "tunnox-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configFile := filepath.Join(tmpDir, "config.yaml")

	dirs := FindDotEnvDirs(configFile)

	// Should include config file directory
	found := false
	for _, d := range dirs {
		if d == tmpDir {
			found = true
			break
		}
	}
	if !found {
		t.Error("FindDotEnvDirs() should include config file directory")
	}

	// Should include cwd
	cwd, _ := os.Getwd()
	found = false
	for _, d := range dirs {
		if d == cwd {
			found = true
			break
		}
	}
	if !found {
		t.Error("FindDotEnvDirs() should include current working directory")
	}
}
