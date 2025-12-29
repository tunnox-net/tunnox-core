package schema

// HTTPConfig contains HTTP service configuration
type HTTPConfig struct {
	Enabled   bool                `yaml:"enabled" json:"enabled"`
	Listen    string              `yaml:"listen" json:"listen"`
	Modules   HTTPModulesConfig   `yaml:"modules" json:"modules"`
	CORS      CORSConfig          `yaml:"cors" json:"cors"`
	RateLimit HTTPRateLimitConfig `yaml:"rate_limit" json:"rate_limit"`
}

// HTTPModulesConfig contains HTTP module configurations
type HTTPModulesConfig struct {
	ManagementAPI  ManagementAPIConfig   `yaml:"management_api" json:"management_api"`
	WebSocket      WebSocketModuleConfig `yaml:"websocket" json:"websocket"`
	DomainProxy    DomainProxyConfig     `yaml:"domain_proxy" json:"domain_proxy"`
	WebSocketProxy WebSocketProxyConfig  `yaml:"websocket_proxy" json:"websocket_proxy"`
}

// ManagementAPIConfig contains management API settings
type ManagementAPIConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Prefix  string `yaml:"prefix" json:"prefix"`
}

// WebSocketModuleConfig contains WebSocket module settings
type WebSocketModuleConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Path    string `yaml:"path" json:"path"`
}

// DomainProxyConfig contains domain proxy settings
type DomainProxyConfig struct {
	Enabled                bool                 `yaml:"enabled" json:"enabled"`
	BaseDomains            []string             `yaml:"base_domains" json:"base_domains"`
	DefaultSubdomainLength int                  `yaml:"default_subdomain_length" json:"default_subdomain_length"`
	SSL                    DomainProxySSLConfig `yaml:"ssl" json:"ssl"`
}

// DomainProxySSLConfig contains SSL settings for domain proxy
type DomainProxySSLConfig struct {
	Enabled  bool   `yaml:"enabled" json:"enabled"`
	CertPath string `yaml:"cert_path" json:"cert_path"`
	KeyPath  string `yaml:"key_path" json:"key_path"`
	AutoSSL  bool   `yaml:"auto_ssl" json:"auto_ssl"`
}

// WebSocketProxyConfig contains WebSocket proxy settings
type WebSocketProxyConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// CORSConfig contains CORS settings
type CORSConfig struct {
	Enabled        bool     `yaml:"enabled" json:"enabled"`
	AllowedOrigins []string `yaml:"allowed_origins" json:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods" json:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers" json:"allowed_headers"`
	MaxAge         int      `yaml:"max_age" json:"max_age"`
}

// HTTPRateLimitConfig contains HTTP rate limit settings
type HTTPRateLimitConfig struct {
	Enabled           bool `yaml:"enabled" json:"enabled"`
	RequestsPerSecond int  `yaml:"requests_per_second" json:"requests_per_second"`
	Burst             int  `yaml:"burst" json:"burst"`
}

// DefaultBaseDomain is the default base domain for domain proxy
const DefaultBaseDomain = "localhost.tunnox.dev"
