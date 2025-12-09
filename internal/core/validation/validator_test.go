package validation

import (
	"testing"
	"tunnox-core/internal/core/errors"

	"github.com/stretchr/testify/assert"
)

func TestValidationResult_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		result   *ValidationResult
		expected bool
	}{
		{
			name:     "valid with no errors",
			result:   &ValidationResult{Errors: []error{}},
			expected: true,
		},
		{
			name: "invalid with errors",
			result: &ValidationResult{
				Errors: []error{errors.New(errors.ErrorTypePermanent, "error")},
			},
			expected: false,
		},
		{
			name:     "valid with nil errors",
			result:   &ValidationResult{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.result.IsValid())
		})
	}
}

func TestValidationResult_Error(t *testing.T) {
	t.Run("no errors returns empty string", func(t *testing.T) {
		result := &ValidationResult{}
		assert.Equal(t, "", result.Error())
	})

	t.Run("with errors returns joined messages", func(t *testing.T) {
		result := &ValidationResult{
			Errors: []error{
				errors.New(errors.ErrorTypePermanent, "error 1"),
				errors.New(errors.ErrorTypePermanent, "error 2"),
			},
		}
		errMsg := result.Error()
		assert.Contains(t, errMsg, "error 1")
		assert.Contains(t, errMsg, "error 2")
		assert.Contains(t, errMsg, "; ")
	})
}

func TestValidationResult_AddError(t *testing.T) {
	result := &ValidationResult{}

	result.AddError(errors.New(errors.ErrorTypePermanent, "error 1"))
	assert.Len(t, result.Errors, 1)

	result.AddError(errors.New(errors.ErrorTypePermanent, "error 2"))
	assert.Len(t, result.Errors, 2)

	// Adding nil should not increase count
	result.AddError(nil)
	assert.Len(t, result.Errors, 2)
}

func TestValidationResult_AddErrorf(t *testing.T) {
	result := &ValidationResult{}

	result.AddErrorf(errors.ErrorTypePermanent, "error %d", 1)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Error(), "error 1")

	result.AddErrorf(errors.ErrorTypeNetwork, "network error: %s", "timeout")
	assert.Len(t, result.Errors, 2)
	assert.Contains(t, result.Errors[1].Error(), "timeout")
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		fieldName   string
		expectError bool
	}{
		{"valid port 80", 80, "port", false},
		{"valid port 443", 443, "port", false},
		{"valid port 1", 1, "port", false},
		{"valid port 65535", 65535, "port", false},
		{"invalid port 0", 0, "port", true},
		{"invalid port -1", -1, "port", true},
		{"invalid port 65536", 65536, "port", true},
		{"invalid port 100000", 100000, "port", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port, tt.fieldName)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.fieldName)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePortOrZero(t *testing.T) {
	tests := []struct {
		name        string
		port        int
		expectError bool
	}{
		{"valid port 0", 0, false},
		{"valid port 1", 1, false},
		{"valid port 8080", 8080, false},
		{"valid port 65535", 65535, false},
		{"invalid port -1", -1, true},
		{"invalid port 65536", 65536, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePortOrZero(tt.port, "port")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateTimeout(t *testing.T) {
	tests := []struct {
		name        string
		timeout     int
		expectError bool
	}{
		{"valid timeout 0", 0, false},
		{"valid timeout 30", 30, false},
		{"valid timeout 3600", 3600, false},
		{"invalid negative timeout", -1, true},
		{"invalid negative timeout -100", -100, true},
		{"invalid very large timeout", 365*24*3600 + 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeout(tt.timeout, "timeout")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateDuration(t *testing.T) {
	// ValidateDuration is an alias for ValidateTimeout
	err := ValidateDuration(30, "duration")
	assert.NoError(t, err)

	err = ValidateDuration(-1, "duration")
	assert.Error(t, err)
}

func TestValidateNonEmptyString(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		expectError bool
	}{
		{"valid non-empty", "test", false},
		{"valid with spaces", "test value", false},
		{"invalid empty string", "", true},
		{"invalid only spaces", "   ", true},
		{"invalid only tabs", "\t\t", true},
		{"valid single character", "a", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNonEmptyString(tt.value, "field")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateStringInList(t *testing.T) {
	allowedValues := []string{"tcp", "udp", "http"}

	tests := []struct {
		name        string
		value       string
		expectError bool
	}{
		{"valid tcp", "tcp", false},
		{"valid udp", "udp", false},
		{"valid http", "http", false},
		{"empty value allowed", "", false},
		{"invalid value", "invalid", true},
		{"case sensitive", "TCP", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStringInList(tt.value, "protocol", allowedValues)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "protocol")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateHost(t *testing.T) {
	tests := []struct {
		name        string
		host        string
		expectError bool
	}{
		{"valid IPv4", "192.168.1.1", false},
		{"valid IPv4 loopback", "127.0.0.1", false},
		{"valid IPv4 any", "0.0.0.0", false},
		{"valid IPv6 loopback", "::1", false},
		{"valid IPv6 any", "::", false},
		{"valid localhost", "localhost", false},
		{"valid hostname", "example.com", false},
		{"empty host", "", true},
		{"host with space", "invalid host", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHost(tt.host, "host")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateAddress(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		expectError bool
	}{
		{"valid address", "localhost:8080", false},
		{"valid IPv4:port", "192.168.1.1:443", false},
		{"valid IPv6:port", "[::1]:8080", false},
		{"valid with service name", "localhost:http", false},
		{"empty address", "", true},
		{"missing port", "localhost", true},
		{"invalid port", "localhost:99999", true},
		{"negative port", "localhost:-1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAddress(tt.addr, "address")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePositiveInt(t *testing.T) {
	tests := []struct {
		name        string
		value       int
		expectError bool
	}{
		{"valid positive", 1, false},
		{"valid large positive", 1000000, false},
		{"invalid zero", 0, true},
		{"invalid negative", -1, true},
		{"invalid large negative", -1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePositiveInt(tt.value, "value")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNonNegativeInt(t *testing.T) {
	tests := []struct {
		name        string
		value       int
		expectError bool
	}{
		{"valid zero", 0, false},
		{"valid positive", 100, false},
		{"valid large positive", 1000000, false},
		{"invalid negative", -1, true},
		{"invalid large negative", -1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNonNegativeInt(tt.value, "value")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateIntRange(t *testing.T) {
	tests := []struct {
		name        string
		value       int
		min         int
		max         int
		expectError bool
	}{
		{"valid in range", 50, 0, 100, false},
		{"valid at min", 0, 0, 100, false},
		{"valid at max", 100, 0, 100, false},
		{"invalid below min", -1, 0, 100, true},
		{"invalid above max", 101, 0, 100, true},
		{"valid single value range", 5, 5, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIntRange(tt.value, tt.min, tt.max, "value")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateInt64Range(t *testing.T) {
	tests := []struct {
		name        string
		value       int64
		min         int64
		max         int64
		expectError bool
	}{
		{"valid in range", 500, 0, 1000, false},
		{"valid at min", 0, 0, 1000, false},
		{"valid at max", 1000, 0, 1000, false},
		{"invalid below min", -1, 0, 1000, true},
		{"invalid above max", 1001, 0, 1000, true},
		{"valid large int64", 1<<50, 0, 1<<60, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInt64Range(tt.value, tt.min, tt.max, "value")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
	}{
		{"valid http URL", "http://example.com", false},
		{"valid https URL", "https://example.com", false},
		{"valid http with path", "http://example.com/path", false},
		{"valid https with port", "https://example.com:8080", false},
		{"invalid empty", "", true},
		{"invalid no protocol", "example.com", true},
		{"invalid ftp protocol", "ftp://example.com", true},
		{"invalid mailto", "mailto:test@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateURL(tt.url, "url")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateCompressionLevel(t *testing.T) {
	tests := []struct {
		name        string
		level       int
		expectError bool
	}{
		{"valid level 1", 1, false},
		{"valid level 5", 5, false},
		{"valid level 9", 9, false},
		{"invalid level 0", 0, true},
		{"invalid level 10", 10, true},
		{"invalid negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCompressionLevel(tt.level, "level")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateBandwidthLimit(t *testing.T) {
	tests := []struct {
		name        string
		limit       int64
		expectError bool
	}{
		{"valid zero (no limit)", 0, false},
		{"valid 1KB/s", 1024, false},
		{"valid 1MB/s", 1024 * 1024, false},
		{"valid 1GB/s", 1024 * 1024 * 1024, false},
		{"invalid negative", -1, true},
		{"invalid too large", 1024*1024*1024*1024 + 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBandwidthLimit(tt.limit, "bandwidth")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateMaxConnections(t *testing.T) {
	tests := []struct {
		name        string
		maxConn     int
		expectError bool
	}{
		{"valid zero (no limit)", 0, false},
		{"valid 100", 100, false},
		{"valid 10000", 10000, false},
		{"valid 1 million", 1000000, false},
		{"invalid negative", -1, true},
		{"invalid too large", 10*1000*1000 + 1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMaxConnections(tt.maxConn, "maxConn")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePortList(t *testing.T) {
	tests := []struct {
		name        string
		ports       []int
		expectError bool
	}{
		{"valid empty list", []int{}, false},
		{"valid single port", []int{8080}, false},
		{"valid multiple ports", []int{80, 443, 8080}, false},
		{"invalid with 0", []int{80, 0, 8080}, true},
		{"invalid with negative", []int{80, -1, 8080}, true},
		{"invalid with out of range", []int{80, 65536}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePortList(tt.ports, "ports")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRequired(t *testing.T) {
	tests := []struct {
		name        string
		value       interface{}
		expectError bool
	}{
		{"valid non-empty string", "test", false},
		{"valid non-zero int", 123, false},
		{"valid non-zero int64", int64(456), false},
		{"invalid nil", nil, true},
		{"invalid empty string", "", true},
		{"invalid zero int", 0, true},
		{"invalid zero int64", int64(0), true},
		{"valid struct", struct{}{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequired(tt.value, "field")
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidation_EdgeCases(t *testing.T) {
	t.Run("validate port with zero field name", func(t *testing.T) {
		err := ValidatePort(8080, "")
		assert.NoError(t, err)
	})

	t.Run("validate string in list with empty list", func(t *testing.T) {
		err := ValidateStringInList("value", "field", []string{})
		assert.Error(t, err)
	})

	t.Run("validate int range with inverted min/max", func(t *testing.T) {
		err := ValidateIntRange(50, 100, 0, "value")
		assert.Error(t, err)
	})

	t.Run("validate required with whitespace string", func(t *testing.T) {
		err := ValidateRequired("   ", "field")
		assert.Error(t, err)
	})
}

func TestValidation_Integration(t *testing.T) {
	// Simulate validation of a complete configuration
	result := &ValidationResult{}

	// Validate multiple fields
	result.AddError(ValidatePort(8080, "server.port"))
	result.AddError(ValidateHost("localhost", "server.host"))
	result.AddError(ValidateTimeout(30, "server.timeout"))
	result.AddError(ValidateNonEmptyString("test-server", "server.name"))
	result.AddError(ValidateCompressionLevel(6, "compression.level"))

	assert.True(t, result.IsValid())
	assert.Empty(t, result.Error())

	// Add an invalid value
	result.AddError(ValidatePort(0, "invalid.port"))

	assert.False(t, result.IsValid())
	assert.NotEmpty(t, result.Error())
	assert.Contains(t, result.Error(), "invalid.port")
}
