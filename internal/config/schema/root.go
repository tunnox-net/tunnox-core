// Package schema defines configuration structure types
package schema

import "time"

// Root is the top-level configuration structure
type Root struct {
	Server     ServerConfig     `yaml:"server" json:"server"`
	Client     ClientConfig     `yaml:"client" json:"client"`
	Management ManagementConfig `yaml:"management" json:"management"`
	HTTP       HTTPConfig       `yaml:"http" json:"http"`
	Storage    StorageConfig    `yaml:"storage" json:"storage"`
	Security   SecurityConfig   `yaml:"security" json:"security"`
	Log        LogConfig        `yaml:"log" json:"log"`
	Health     HealthConfig     `yaml:"health" json:"health"`
	Platform   PlatformConfig   `yaml:"platform" json:"platform"`
}

// ServerConfig contains server-side configuration
type ServerConfig struct {
	Protocols ProtocolsConfig `yaml:"protocols" json:"protocols"`
	Session   SessionConfig   `yaml:"session" json:"session"`
}

// SessionConfig contains session management settings
type SessionConfig struct {
	HeartbeatTimeout      time.Duration `yaml:"heartbeat_timeout" json:"heartbeat_timeout"`
	CleanupInterval       time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"`
	MaxConnections        int           `yaml:"max_connections" json:"max_connections"`
	MaxControlConnections int           `yaml:"max_control_connections" json:"max_control_connections"`
	ReconnectWindow       time.Duration `yaml:"reconnect_window" json:"reconnect_window"`
}

// PlatformConfig contains cloud platform settings
type PlatformConfig struct {
	Enabled bool          `yaml:"enabled" json:"enabled"`
	URL     string        `yaml:"url" json:"url"`
	Token   Secret        `yaml:"token" json:"token"`
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
	Retry   RetryConfig   `yaml:"retry" json:"retry"`
}

// RetryConfig contains retry settings
type RetryConfig struct {
	MaxRetries    int           `yaml:"max_retries" json:"max_retries"`
	RetryInterval time.Duration `yaml:"retry_interval" json:"retry_interval"`
}
