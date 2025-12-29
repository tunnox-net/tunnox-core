package schema

import "time"

// HealthConfig contains health check configuration
type HealthConfig struct {
	Enabled   bool                  `yaml:"enabled" json:"enabled"`
	Listen    string                `yaml:"listen" json:"listen"`
	Endpoints HealthEndpointsConfig `yaml:"endpoints" json:"endpoints"`
	Checks    HealthChecksConfig    `yaml:"checks" json:"checks"`
}

// HealthEndpointsConfig contains health endpoint paths
type HealthEndpointsConfig struct {
	Liveness  string `yaml:"liveness" json:"liveness"`   // K8s liveness probe
	Readiness string `yaml:"readiness" json:"readiness"` // K8s readiness probe
	Startup   string `yaml:"startup" json:"startup"`     // K8s startup probe
}

// HealthChecksConfig contains health check settings
type HealthChecksConfig struct {
	Storage   HealthCheckConfig `yaml:"storage" json:"storage"`
	Redis     HealthCheckConfig `yaml:"redis" json:"redis"`
	Protocols HealthCheckConfig `yaml:"protocols" json:"protocols"`
}

// HealthCheckConfig contains individual health check settings
type HealthCheckConfig struct {
	Enabled bool          `yaml:"enabled" json:"enabled"`
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}
