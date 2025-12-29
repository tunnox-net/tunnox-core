package validator

import (
	"testing"
	"time"

	"tunnox-core/internal/config/schema"
	"tunnox-core/internal/config/source"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	if v == nil {
		t.Fatal("NewValidator() returned nil")
	}
	if len(v.rules) == 0 {
		t.Error("NewValidator() should have default rules")
	}
}

func TestValidationResult_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		errors []ValidationError
		want   bool
	}{
		{"no errors", nil, true},
		{"empty errors", []ValidationError{}, true},
		{"has errors", []ValidationError{{Field: "test", Message: "error"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ValidationResult{Errors: tt.errors}
			if got := r.IsValid(); got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidationResult_Error(t *testing.T) {
	r := &ValidationResult{}
	if r.Error() != "" {
		t.Error("Error() should return empty string when valid")
	}

	r.AddError("field", "value", "message", "hint")
	errStr := r.Error()
	if errStr == "" {
		t.Error("Error() should return non-empty string when invalid")
	}
	if len(errStr) == 0 {
		t.Error("Error message should not be empty")
	}
}

func TestValidator_ValidateDefaultConfig(t *testing.T) {
	cfg := source.GetDefaultConfig()
	v := NewValidator()
	result := v.Validate(cfg)

	if !result.IsValid() {
		t.Errorf("Default config should be valid, got errors: %s", result.Error())
	}
}

func TestValidator_ValidatePort(t *testing.T) {
	cfg := source.GetDefaultConfig()

	// Invalid port
	cfg.Server.Protocols.TCP.Port = 0
	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Port 0 should be invalid")
	}

	// Port below 1024 (requires root)
	cfg.Server.Protocols.TCP.Port = 80
	result = v.Validate(cfg)
	if result.IsValid() {
		t.Error("Port 80 should warn about root privileges")
	}
}

func TestValidator_ValidateSession(t *testing.T) {
	cfg := source.GetDefaultConfig()

	// Heartbeat too short
	cfg.Server.Session.HeartbeatTimeout = 5 * time.Second
	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("HeartbeatTimeout < 10s should be invalid")
	}

	// Reset and test cleanup interval
	cfg = source.GetDefaultConfig()
	cfg.Server.Session.CleanupInterval = cfg.Server.Session.HeartbeatTimeout + time.Second
	result = v.Validate(cfg)

	if result.IsValid() {
		t.Error("CleanupInterval >= HeartbeatTimeout should be invalid")
	}
}

func TestValidator_ValidateClient(t *testing.T) {
	cfg := source.GetDefaultConfig()

	// Non-anonymous mode without client_id
	cfg.Client.Anonymous = false
	cfg.Client.ClientID = 0
	cfg.Client.AuthToken = ""

	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Non-anonymous mode without client_id should be invalid")
	}

	hasClientIDError := false
	hasTokenError := false
	for _, e := range result.Errors {
		if e.Field == "client.client_id" {
			hasClientIDError = true
		}
		if e.Field == "client.auth_token" {
			hasTokenError = true
		}
	}

	if !hasClientIDError {
		t.Error("Should have client_id validation error")
	}
	if !hasTokenError {
		t.Error("Should have auth_token validation error")
	}
}

func TestValidator_ValidateHTTP_DomainProxy(t *testing.T) {
	cfg := source.GetDefaultConfig()

	// Enable domain proxy without base domains
	cfg.HTTP.Modules.DomainProxy.Enabled = true
	cfg.HTTP.Modules.DomainProxy.BaseDomains = []string{}

	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Domain proxy enabled without base_domains should be invalid")
	}
}

func TestValidator_ValidateHTTP_SSL(t *testing.T) {
	cfg := source.GetDefaultConfig()

	// Enable SSL without cert paths
	cfg.HTTP.Modules.DomainProxy.Enabled = true
	cfg.HTTP.Modules.DomainProxy.SSL.Enabled = true
	cfg.HTTP.Modules.DomainProxy.SSL.CertPath = ""
	cfg.HTTP.Modules.DomainProxy.SSL.KeyPath = ""

	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("SSL enabled without cert_path and key_path should be invalid")
	}
}

func TestValidator_ValidateStorage_Redis(t *testing.T) {
	cfg := source.GetDefaultConfig()

	// Enable Redis without addr
	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Redis.Addr = ""

	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Redis enabled without addr should be invalid")
	}
}

func TestValidator_ValidateStorage_RedisDB(t *testing.T) {
	cfg := source.GetDefaultConfig()

	cfg.Storage.Redis.Enabled = true
	cfg.Storage.Redis.Addr = "localhost:6379"
	cfg.Storage.Redis.DB = 16 // Invalid

	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Redis DB > 15 should be invalid")
	}
}

func TestValidator_ValidateStorage_Remote(t *testing.T) {
	cfg := source.GetDefaultConfig()

	cfg.Storage.Remote.Enabled = true
	cfg.Storage.Remote.GRPCAddress = ""

	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Remote storage enabled without grpc_address should be invalid")
	}
}

func TestValidator_ValidateSecurity_RateLimit(t *testing.T) {
	cfg := source.GetDefaultConfig()

	cfg.Security.RateLimit.IP.Enabled = true
	cfg.Security.RateLimit.IP.Rate = 0

	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Rate limit with rate 0 should be invalid")
	}

	// Reset and test burst < rate
	cfg = source.GetDefaultConfig()
	cfg.Security.RateLimit.IP.Enabled = true
	cfg.Security.RateLimit.IP.Rate = 100
	cfg.Security.RateLimit.IP.Burst = 50

	result = v.Validate(cfg)
	if result.IsValid() {
		t.Error("Burst < rate should be invalid")
	}
}

func TestValidator_ValidateLog(t *testing.T) {
	cfg := source.GetDefaultConfig()

	// Invalid log level
	cfg.Log.Level = "invalid"
	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Invalid log level should be invalid")
	}

	// Invalid log format
	cfg = source.GetDefaultConfig()
	cfg.Log.Format = "xml"
	result = v.Validate(cfg)

	if result.IsValid() {
		t.Error("Invalid log format should be invalid")
	}
}

func TestValidator_ValidateManagement_BasicAuth(t *testing.T) {
	cfg := source.GetDefaultConfig()

	cfg.Management.Enabled = true
	cfg.Management.Auth.Type = schema.AuthTypeBasic
	cfg.Management.Auth.Username = ""
	cfg.Management.Auth.Password = ""

	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Basic auth without username/password should be invalid")
	}
}

func TestValidator_ValidateDependencies_WebSocket(t *testing.T) {
	cfg := source.GetDefaultConfig()

	// WebSocket enabled but HTTP disabled
	cfg.Server.Protocols.WebSocket.Enabled = true
	cfg.HTTP.Enabled = false

	v := NewValidator()
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("WebSocket without HTTP should be invalid")
	}

	// WebSocket enabled but HTTP WebSocket module disabled
	cfg = source.GetDefaultConfig()
	cfg.Server.Protocols.WebSocket.Enabled = true
	cfg.HTTP.Enabled = true
	cfg.HTTP.Modules.WebSocket.Enabled = false

	result = v.Validate(cfg)
	if result.IsValid() {
		t.Error("WebSocket without HTTP WebSocket module should be invalid")
	}
}

func TestValidateConfig_Convenience(t *testing.T) {
	cfg := source.GetDefaultConfig()
	result := ValidateConfig(cfg)

	if !result.IsValid() {
		t.Errorf("Default config should be valid: %s", result.Error())
	}
}

func TestValidator_AddRule(t *testing.T) {
	v := NewValidator()
	initialRules := len(v.rules)

	customRule := func(cfg *schema.Root, result *ValidationResult) {
		if cfg.Log.Level == "custom" {
			result.AddError("log.level", "custom", "custom level not allowed", "use debug/info/warn/error")
		}
	}

	v.AddRule(customRule)

	if len(v.rules) != initialRules+1 {
		t.Error("AddRule should add a rule")
	}

	// Test custom rule is applied
	cfg := source.GetDefaultConfig()
	cfg.Log.Level = "custom"
	result := v.Validate(cfg)

	if result.IsValid() {
		t.Error("Custom rule should have triggered")
	}
}

func TestValidationResult_AddError(t *testing.T) {
	r := &ValidationResult{}
	r.AddError("field", "value", "message", "hint")

	if len(r.Errors) != 1 {
		t.Fatalf("AddError() should add one error, got %d", len(r.Errors))
	}

	e := r.Errors[0]
	if e.Field != "field" {
		t.Errorf("Field = %q, want %q", e.Field, "field")
	}
	if e.Value != "value" {
		t.Errorf("Value = %q, want %q", e.Value, "value")
	}
	if e.Message != "message" {
		t.Errorf("Message = %q, want %q", e.Message, "message")
	}
	if e.Hint != "hint" {
		t.Errorf("Hint = %q, want %q", e.Hint, "hint")
	}
}

func TestValidationError_Error(t *testing.T) {
	e := &ValidationError{
		Field:   "test.field",
		Message: "test message",
	}

	expected := "test.field: test message"
	if e.Error() != expected {
		t.Errorf("Error() = %q, want %q", e.Error(), expected)
	}
}
