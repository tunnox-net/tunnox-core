package schema

import "time"

// SecurityConfig contains security configuration
type SecurityConfig struct {
	JWT           JWTConfig               `yaml:"jwt" json:"jwt"`
	RateLimit     SecurityRateLimitConfig `yaml:"rate_limit" json:"rate_limit"`
	AccessControl AccessControlConfig     `yaml:"access_control" json:"access_control"`
}

// JWTConfig contains JWT settings
type JWTConfig struct {
	SecretKey         Secret        `yaml:"secret_key" json:"secret_key"`
	Expiration        time.Duration `yaml:"expiration" json:"expiration"`
	RefreshExpiration time.Duration `yaml:"refresh_expiration" json:"refresh_expiration"`
	Issuer            string        `yaml:"issuer" json:"issuer"`
}

// SecurityRateLimitConfig contains security rate limit settings
type SecurityRateLimitConfig struct {
	IP     IPRateLimitConfig     `yaml:"ip" json:"ip"`
	Tunnel TunnelRateLimitConfig `yaml:"tunnel" json:"tunnel"`
	Client ClientRateLimitConfig `yaml:"client" json:"client"`
}

// IPRateLimitConfig contains IP-level rate limit settings
type IPRateLimitConfig struct {
	Enabled bool          `yaml:"enabled" json:"enabled"`
	Rate    int           `yaml:"rate" json:"rate"`   // requests per second
	Burst   int           `yaml:"burst" json:"burst"` // burst capacity
	TTL     time.Duration `yaml:"ttl" json:"ttl"`     // bucket TTL
}

// TunnelRateLimitConfig contains tunnel-level rate limit settings
type TunnelRateLimitConfig struct {
	Enabled bool  `yaml:"enabled" json:"enabled"`
	Rate    int64 `yaml:"rate" json:"rate"`   // bytes per second
	Burst   int64 `yaml:"burst" json:"burst"` // burst capacity in bytes
}

// ClientRateLimitConfig contains client-level rate limit settings
type ClientRateLimitConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	Rate    int  `yaml:"rate" json:"rate"`   // requests per second
	Burst   int  `yaml:"burst" json:"burst"` // burst capacity
}

// AccessControlConfig contains access control settings
type AccessControlConfig struct {
	Enabled   bool             `yaml:"enabled" json:"enabled"`
	Whitelist AccessListConfig `yaml:"whitelist" json:"whitelist"`
	Blacklist AccessListConfig `yaml:"blacklist" json:"blacklist"`
}

// AccessListConfig contains whitelist/blacklist settings
type AccessListConfig struct {
	IPs   []string `yaml:"ips" json:"ips"`
	CIDRs []string `yaml:"cidrs" json:"cidrs"`
}
