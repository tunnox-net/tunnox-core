// Package schema defines configuration structure types
package schema

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// Secret wraps sensitive configuration values with automatic masking
// for logging and serialization
type Secret string

// String returns a masked representation of the secret for safe logging
func (s Secret) String() string {
	if len(s) == 0 {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return string(s[:2]) + "****" + string(s[len(s)-2:])
}

// Value returns the actual secret value
// Use with caution - only when the actual value is needed
func (s Secret) Value() string {
	return string(s)
}

// IsEmpty returns true if the secret is empty
func (s Secret) IsEmpty() bool {
	return len(s) == 0
}

// MarshalJSON implements json.Marshaler with masking
func (s Secret) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// UnmarshalJSON implements json.Unmarshaler
func (s *Secret) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*s = Secret(str)
	return nil
}

// MarshalYAML implements yaml.Marshaler with masking
func (s Secret) MarshalYAML() (interface{}, error) {
	return s.String(), nil
}

// UnmarshalYAML implements yaml.Unmarshaler
func (s *Secret) UnmarshalYAML(node *yaml.Node) error {
	*s = Secret(node.Value)
	return nil
}

// NewSecret creates a new Secret from a string
func NewSecret(value string) Secret {
	return Secret(value)
}
