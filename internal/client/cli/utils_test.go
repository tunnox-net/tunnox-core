package cli

import (
	"testing"
)

func TestParseIntWithDefault(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		defaultVal  int
		expected    int
		expectError bool
	}{
		{"empty string", "", 10, 10, false},
		{"valid number", "42", 10, 42, false},
		{"zero", "0", 10, 0, false},
		{"negative", "-5", 10, -5, false},
		{"invalid", "abc", 10, 0, true},
		// Note: fmt.Sscanf("3.14", "%d", &val) will parse as 3 (not error)
		{"float", "3.14", 10, 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseIntWithDefault(tt.input, tt.defaultVal)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("expected %d, got %d", tt.expected, result)
				}
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"needs truncation", "hello world", 8, "hello .."},
		{"maxLen too small", "hello", 2, "he"},
		{"maxLen zero", "hello", 0, ""},
		{"empty string", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"0 bytes", 0, "0 B"},
		{"bytes", 512, "512 B"},
		{"1 KB", 1024, "1.0 KB"},
		{"1.5 KB", 1536, "1.5 KB"},
		{"1 MB", 1024 * 1024, "1.0 MB"},
		{"1 GB", 1024 * 1024 * 1024, "1.0 GB"},
		{"1.2 GB", 1288490188, "1.2 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", "N/A"},
		{"RFC3339", "2025-11-28T15:30:00Z", "2025-11-28 15:30"},
		// Note: time.Parse converts to local time zone
		{"RFC3339 with TZ", "2025-11-28T15:30:45+08:00", "2025-11-28 15:30"},
		{"long format", "2025-11-28 15:30:45.123456", "2025-11-28 15:30"},
		{"short format", "2025-11-28", "2025-11-28"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTime(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
