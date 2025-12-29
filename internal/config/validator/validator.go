// Package validator provides configuration validation
package validator

import (
	"fmt"
	"net"
	"strings"

	"tunnox-core/internal/config/schema"
)

// ValidationError represents a single validation error
type ValidationError struct {
	Field   string // Field path (e.g., "server.protocols.tcp.port")
	Value   string // Current value (masked for secrets)
	Message string // Error message
	Hint    string // Fix suggestion
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult contains all validation errors
type ValidationResult struct {
	Errors []ValidationError
}

// IsValid returns true if there are no validation errors
func (r *ValidationResult) IsValid() bool {
	return len(r.Errors) == 0
}

// Error returns a formatted error message
func (r *ValidationResult) Error() string {
	if r.IsValid() {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Configuration validation failed:\n\n")

	for i, err := range r.Errors {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Field))
		if err.Value != "" {
			sb.WriteString(fmt.Sprintf("     Current value: %s\n", err.Value))
		}
		sb.WriteString(fmt.Sprintf("     Error: %s\n", err.Message))
		if err.Hint != "" {
			sb.WriteString(fmt.Sprintf("     Hint: %s\n", err.Hint))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// AddError adds a validation error
func (r *ValidationResult) AddError(field, value, message, hint string) {
	r.Errors = append(r.Errors, ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
		Hint:    hint,
	})
}

// Validator validates configuration
type Validator struct {
	rules []ValidationRule
}

// ValidationRule is a function that validates configuration
type ValidationRule func(cfg *schema.Root, result *ValidationResult)

// NewValidator creates a new Validator with default rules
func NewValidator() *Validator {
	v := &Validator{
		rules: make([]ValidationRule, 0),
	}

	// Add default rules
	v.AddRule(validateServerProtocols)
	v.AddRule(validateSession)
	v.AddRule(validateClient)
	v.AddRule(validateHTTP)
	v.AddRule(validateStorage)
	v.AddRule(validateSecurity)
	v.AddRule(validateLog)
	v.AddRule(validateHealth)
	v.AddRule(validateManagement)
	v.AddRule(validateDependencies)

	return v
}

// AddRule adds a validation rule
func (v *Validator) AddRule(rule ValidationRule) {
	v.rules = append(v.rules, rule)
}

// Validate validates the configuration
func (v *Validator) Validate(cfg *schema.Root) *ValidationResult {
	result := &ValidationResult{
		Errors: make([]ValidationError, 0),
	}

	for _, rule := range v.rules {
		rule(cfg, result)
	}

	return result
}

// ValidateConfig is a convenience function that creates a validator and validates
func ValidateConfig(cfg *schema.Root) *ValidationResult {
	return NewValidator().Validate(cfg)
}

// ============================================================================
// Validation Rules
// ============================================================================

func validateServerProtocols(cfg *schema.Root, result *ValidationResult) {
	// TCP
	if cfg.Server.Protocols.TCP.Enabled {
		validatePort("server.protocols.tcp.port", cfg.Server.Protocols.TCP.Port, result)
		validateHost("server.protocols.tcp.host", cfg.Server.Protocols.TCP.Host, result)
	}

	// KCP
	if cfg.Server.Protocols.KCP.Enabled {
		validatePort("server.protocols.kcp.port", cfg.Server.Protocols.KCP.Port, result)
		validateHost("server.protocols.kcp.host", cfg.Server.Protocols.KCP.Host, result)
		validateKCPMode("server.protocols.kcp.mode", cfg.Server.Protocols.KCP.Mode, result)
	}

	// QUIC
	if cfg.Server.Protocols.QUIC.Enabled {
		validatePort("server.protocols.quic.port", cfg.Server.Protocols.QUIC.Port, result)
		validateHost("server.protocols.quic.host", cfg.Server.Protocols.QUIC.Host, result)
	}
}

func validateSession(cfg *schema.Root, result *ValidationResult) {
	if cfg.Server.Session.HeartbeatTimeout < 10e9 { // 10 seconds in nanoseconds
		result.AddError("server.session.heartbeat_timeout",
			cfg.Server.Session.HeartbeatTimeout.String(),
			"heartbeat_timeout must be at least 10s",
			"Set a value >= 10s, e.g., 60s")
	}

	if cfg.Server.Session.CleanupInterval < 5e9 { // 5 seconds
		result.AddError("server.session.cleanup_interval",
			cfg.Server.Session.CleanupInterval.String(),
			"cleanup_interval must be at least 5s",
			"Set a value >= 5s, e.g., 15s")
	}

	if cfg.Server.Session.CleanupInterval >= cfg.Server.Session.HeartbeatTimeout {
		result.AddError("server.session.cleanup_interval",
			cfg.Server.Session.CleanupInterval.String(),
			"cleanup_interval must be less than heartbeat_timeout",
			"Set cleanup_interval < heartbeat_timeout")
	}

	if cfg.Server.Session.MaxConnections < 1 {
		result.AddError("server.session.max_connections",
			fmt.Sprintf("%d", cfg.Server.Session.MaxConnections),
			"max_connections must be at least 1",
			"Set a positive value")
	}

	if cfg.Server.Session.MaxControlConnections > cfg.Server.Session.MaxConnections {
		result.AddError("server.session.max_control_connections",
			fmt.Sprintf("%d", cfg.Server.Session.MaxControlConnections),
			"max_control_connections must not exceed max_connections",
			"Set max_control_connections <= max_connections")
	}
}

func validateClient(cfg *schema.Root, result *ValidationResult) {
	if !cfg.Client.Anonymous {
		if cfg.Client.ClientID <= 0 {
			result.AddError("client.client_id",
				fmt.Sprintf("%d", cfg.Client.ClientID),
				"client_id is required when not in anonymous mode",
				"Set a valid client_id or enable anonymous mode")
		}
		if cfg.Client.AuthToken.IsEmpty() {
			result.AddError("client.auth_token",
				"",
				"auth_token is required when not in anonymous mode",
				"Set a valid auth_token or enable anonymous mode")
		}
	}

	// Validate protocol
	validProtocols := map[string]bool{
		schema.ProtocolTCP:       true,
		schema.ProtocolWebSocket: true,
		schema.ProtocolKCP:       true,
		schema.ProtocolQUIC:      true,
		schema.ProtocolAuto:      true,
	}
	if !validProtocols[cfg.Client.Server.Protocol] {
		result.AddError("client.server.protocol",
			cfg.Client.Server.Protocol,
			"invalid protocol",
			"Use one of: tcp, websocket, kcp, quic, auto")
	}

	// Validate log level
	validateLogLevel("client.log.level", cfg.Client.Log.Level, result)
	validateLogFormat("client.log.format", cfg.Client.Log.Format, result)
}

func validateHTTP(cfg *schema.Root, result *ValidationResult) {
	if !cfg.HTTP.Enabled {
		return
	}

	// Validate domain proxy
	if cfg.HTTP.Modules.DomainProxy.Enabled {
		if len(cfg.HTTP.Modules.DomainProxy.BaseDomains) == 0 {
			result.AddError("http.modules.domain_proxy.base_domains",
				"[]",
				"base_domains is required when domain_proxy is enabled",
				fmt.Sprintf("Add at least one domain, e.g., %q", schema.DefaultBaseDomain))
		}

		// Validate SSL settings
		if cfg.HTTP.Modules.DomainProxy.SSL.Enabled {
			if cfg.HTTP.Modules.DomainProxy.SSL.CertPath == "" {
				result.AddError("http.modules.domain_proxy.ssl.cert_path",
					"",
					"cert_path is required when SSL is enabled",
					"Set the path to SSL certificate file")
			}
			if cfg.HTTP.Modules.DomainProxy.SSL.KeyPath == "" {
				result.AddError("http.modules.domain_proxy.ssl.key_path",
					"",
					"key_path is required when SSL is enabled",
					"Set the path to SSL private key file")
			}
		}
	}

	// Validate CORS
	if cfg.HTTP.CORS.MaxAge < 0 {
		result.AddError("http.cors.max_age",
			fmt.Sprintf("%d", cfg.HTTP.CORS.MaxAge),
			"max_age must be non-negative",
			"Set a value >= 0")
	}
}

func validateStorage(cfg *schema.Root, result *ValidationResult) {
	// Validate storage type
	validTypes := map[string]bool{
		schema.StorageTypeMemory: true,
		schema.StorageTypeRedis:  true,
		schema.StorageTypeHybrid: true,
	}
	if !validTypes[cfg.Storage.Type] && cfg.Storage.Type != "" {
		result.AddError("storage.type",
			cfg.Storage.Type,
			"invalid storage type",
			"Use one of: memory, redis, hybrid")
	}

	// Redis validation
	if cfg.Storage.Redis.Enabled {
		if cfg.Storage.Redis.Addr == "" {
			result.AddError("storage.redis.addr",
				"",
				"redis.addr is required when Redis is enabled",
				"Set redis address, e.g., localhost:6379")
		}
		if cfg.Storage.Redis.DB < 0 || cfg.Storage.Redis.DB > 15 {
			result.AddError("storage.redis.db",
				fmt.Sprintf("%d", cfg.Storage.Redis.DB),
				"redis.db must be between 0 and 15",
				"Set a value between 0 and 15")
		}
	}

	// Remote storage validation
	if cfg.Storage.Remote.Enabled {
		if cfg.Storage.Remote.GRPCAddress == "" {
			result.AddError("storage.remote.grpc_address",
				"",
				"grpc_address is required when remote storage is enabled",
				"Set the gRPC server address")
		}
	}
}

func validateSecurity(cfg *schema.Root, result *ValidationResult) {
	// Rate limit validation
	if cfg.Security.RateLimit.IP.Enabled {
		if cfg.Security.RateLimit.IP.Rate <= 0 {
			result.AddError("security.rate_limit.ip.rate",
				fmt.Sprintf("%d", cfg.Security.RateLimit.IP.Rate),
				"rate must be positive",
				"Set a value > 0")
		}
		if cfg.Security.RateLimit.IP.Burst < cfg.Security.RateLimit.IP.Rate {
			result.AddError("security.rate_limit.ip.burst",
				fmt.Sprintf("%d", cfg.Security.RateLimit.IP.Burst),
				"burst must be >= rate",
				"Set burst >= rate")
		}
	}
}

func validateLog(cfg *schema.Root, result *ValidationResult) {
	validateLogLevel("log.level", cfg.Log.Level, result)
	validateLogFormat("log.format", cfg.Log.Format, result)

	if cfg.Log.Rotation.Enabled {
		if cfg.Log.Rotation.MaxSize <= 0 {
			result.AddError("log.rotation.max_size",
				fmt.Sprintf("%d", cfg.Log.Rotation.MaxSize),
				"max_size must be positive",
				"Set a value > 0")
		}
	}
}

func validateHealth(cfg *schema.Root, result *ValidationResult) {
	// Health check validation is minimal
	// Just ensure listen address is valid if enabled
	if cfg.Health.Enabled && cfg.Health.Listen != "" {
		if _, err := net.ResolveTCPAddr("tcp", cfg.Health.Listen); err != nil {
			result.AddError("health.listen",
				cfg.Health.Listen,
				"invalid listen address",
				"Use format host:port, e.g., 0.0.0.0:9090")
		}
	}
}

func validateManagement(cfg *schema.Root, result *ValidationResult) {
	if !cfg.Management.Enabled {
		return
	}

	// Validate auth type
	validAuthTypes := map[string]bool{
		schema.AuthTypeNone:   true,
		schema.AuthTypeBearer: true,
		schema.AuthTypeBasic:  true,
	}
	if !validAuthTypes[cfg.Management.Auth.Type] && cfg.Management.Auth.Type != "" {
		result.AddError("management.auth.type",
			cfg.Management.Auth.Type,
			"invalid auth type",
			"Use one of: none, bearer, basic")
	}

	// Validate basic auth requires username and password
	if cfg.Management.Auth.Type == schema.AuthTypeBasic {
		if cfg.Management.Auth.Username == "" {
			result.AddError("management.auth.username",
				"",
				"username is required for basic auth",
				"Set a username")
		}
		if cfg.Management.Auth.Password.IsEmpty() {
			result.AddError("management.auth.password",
				"",
				"password is required for basic auth",
				"Set a password")
		}
	}
}

// validateDependencies validates configuration dependencies
func validateDependencies(cfg *schema.Root, result *ValidationResult) {
	// WebSocket requires HTTP to be enabled
	if cfg.Server.Protocols.WebSocket.Enabled && !cfg.HTTP.Enabled {
		result.AddError("server.protocols.websocket.enabled",
			"true",
			"WebSocket protocol requires HTTP service to be enabled",
			"Set http.enabled = true")
	}

	if cfg.Server.Protocols.WebSocket.Enabled && !cfg.HTTP.Modules.WebSocket.Enabled {
		result.AddError("server.protocols.websocket.enabled",
			"true",
			"WebSocket protocol requires HTTP WebSocket module to be enabled",
			"Set http.modules.websocket.enabled = true")
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func validatePort(field string, port int, result *ValidationResult) {
	if port < 1 || port > 65535 {
		result.AddError(field,
			fmt.Sprintf("%d", port),
			"port must be between 1 and 65535",
			"Use a valid port number, e.g., 8000")
	}
	if port < 1024 {
		result.AddError(field,
			fmt.Sprintf("%d", port),
			"port below 1024 requires root privileges",
			"Use a port >= 1024 or run as root")
	}
}

func validateHost(field, host string, result *ValidationResult) {
	if host == "" {
		return
	}
	if host != "0.0.0.0" && host != "localhost" && host != "127.0.0.1" && host != "::" {
		if net.ParseIP(host) == nil {
			result.AddError(field,
				host,
				"invalid host address",
				"Use a valid IP address or 0.0.0.0")
		}
	}
}

func validateKCPMode(field, mode string, result *ValidationResult) {
	validModes := map[string]bool{
		schema.KCPModeNormal: true,
		schema.KCPModeFast:   true,
		schema.KCPModeFast2:  true,
		schema.KCPModeFast3:  true,
	}
	if !validModes[mode] && mode != "" {
		result.AddError(field,
			mode,
			"invalid KCP mode",
			"Use one of: normal, fast, fast2, fast3")
	}
}

func validateLogLevel(field, level string, result *ValidationResult) {
	validLevels := map[string]bool{
		schema.LogLevelDebug: true,
		schema.LogLevelInfo:  true,
		schema.LogLevelWarn:  true,
		schema.LogLevelError: true,
	}
	if !validLevels[level] && level != "" {
		result.AddError(field,
			level,
			"invalid log level",
			"Use one of: debug, info, warn, error")
	}
}

func validateLogFormat(field, format string, result *ValidationResult) {
	validFormats := map[string]bool{
		schema.LogFormatText: true,
		schema.LogFormatJSON: true,
	}
	if !validFormats[format] && format != "" {
		result.AddError(field,
			format,
			"invalid log format",
			"Use one of: text, json")
	}
}
