package schema

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestSecret_String(t *testing.T) {
	tests := []struct {
		name     string
		secret   Secret
		expected string
	}{
		{
			name:     "empty secret",
			secret:   Secret(""),
			expected: "",
		},
		{
			name:     "short secret (1 char)",
			secret:   Secret("a"),
			expected: "****",
		},
		{
			name:     "short secret (4 chars)",
			secret:   Secret("abcd"),
			expected: "****",
		},
		{
			name:     "normal secret (8 chars)",
			secret:   Secret("password"),
			expected: "pa****rd",
		},
		{
			name:     "long secret",
			secret:   Secret("supersecretpassword123"),
			expected: "su****23",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.secret.String()
			if result != tt.expected {
				t.Errorf("String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSecret_Value(t *testing.T) {
	secret := Secret("mysecret")
	if secret.Value() != "mysecret" {
		t.Errorf("Value() = %q, want %q", secret.Value(), "mysecret")
	}
}

func TestSecret_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		secret   Secret
		expected bool
	}{
		{"empty", Secret(""), true},
		{"non-empty", Secret("secret"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := tt.secret.IsEmpty(); result != tt.expected {
				t.Errorf("IsEmpty() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestSecret_MarshalJSON(t *testing.T) {
	secret := Secret("password123")
	data, err := json.Marshal(secret)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	// Should be masked
	expected := `"pa****23"`
	if string(data) != expected {
		t.Errorf("MarshalJSON() = %s, want %s", string(data), expected)
	}
}

func TestSecret_UnmarshalJSON(t *testing.T) {
	var secret Secret
	err := json.Unmarshal([]byte(`"mypassword"`), &secret)
	if err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}

	if secret.Value() != "mypassword" {
		t.Errorf("UnmarshalJSON() value = %q, want %q", secret.Value(), "mypassword")
	}
}

func TestSecret_MarshalYAML(t *testing.T) {
	secret := Secret("password123")
	data, err := yaml.Marshal(secret)
	if err != nil {
		t.Fatalf("MarshalYAML() error = %v", err)
	}

	// Should be masked (YAML adds newline)
	expected := "pa****23\n"
	if string(data) != expected {
		t.Errorf("MarshalYAML() = %q, want %q", string(data), expected)
	}
}

func TestSecret_UnmarshalYAML(t *testing.T) {
	type TestConfig struct {
		Password Secret `yaml:"password"`
	}

	yamlData := `password: mypassword123`
	var cfg TestConfig
	err := yaml.Unmarshal([]byte(yamlData), &cfg)
	if err != nil {
		t.Fatalf("UnmarshalYAML() error = %v", err)
	}

	if cfg.Password.Value() != "mypassword123" {
		t.Errorf("UnmarshalYAML() value = %q, want %q", cfg.Password.Value(), "mypassword123")
	}
}

func TestNewSecret(t *testing.T) {
	secret := NewSecret("test")
	if secret.Value() != "test" {
		t.Errorf("NewSecret() value = %q, want %q", secret.Value(), "test")
	}
}

func TestSecret_StringDoesNotLeakValue(t *testing.T) {
	secret := Secret("mysupersecretpassword")
	str := secret.String()

	// Ensure the full value is not in the string
	if str == secret.Value() {
		t.Error("String() returned the actual secret value - security issue!")
	}
}
