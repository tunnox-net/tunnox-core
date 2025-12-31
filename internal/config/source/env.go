package source

import (
	"os"
	"strconv"
	"strings"
	"time"

	"tunnox-core/internal/config/schema"
)

// EnvSource loads configuration from environment variables
type EnvSource struct {
	prefix string
}

// NewEnvSource creates a new EnvSource with the specified prefix
func NewEnvSource(prefix string) *EnvSource {
	return &EnvSource{
		prefix: prefix,
	}
}

// Name returns the source name
func (s *EnvSource) Name() string {
	return "env"
}

// Priority returns the source priority
func (s *EnvSource) Priority() int {
	return PriorityEnv
}

// LoadInto loads environment variables into the config structure
func (s *EnvSource) LoadInto(cfg *schema.Root) error {
	// Server protocols
	s.loadBool("SERVER_TCP_ENABLED", &cfg.Server.Protocols.TCP.Enabled)
	s.loadInt("SERVER_TCP_PORT", &cfg.Server.Protocols.TCP.Port)
	s.loadString("SERVER_TCP_HOST", &cfg.Server.Protocols.TCP.Host)

	s.loadBool("SERVER_WEBSOCKET_ENABLED", &cfg.Server.Protocols.WebSocket.Enabled)

	s.loadBool("SERVER_KCP_ENABLED", &cfg.Server.Protocols.KCP.Enabled)
	s.loadInt("SERVER_KCP_PORT", &cfg.Server.Protocols.KCP.Port)
	s.loadString("SERVER_KCP_HOST", &cfg.Server.Protocols.KCP.Host)
	s.loadString("SERVER_KCP_MODE", &cfg.Server.Protocols.KCP.Mode)

	s.loadBool("SERVER_QUIC_ENABLED", &cfg.Server.Protocols.QUIC.Enabled)
	s.loadInt("SERVER_QUIC_PORT", &cfg.Server.Protocols.QUIC.Port)
	s.loadString("SERVER_QUIC_HOST", &cfg.Server.Protocols.QUIC.Host)

	// Session
	s.loadDuration("SESSION_HEARTBEAT_TIMEOUT", &cfg.Server.Session.HeartbeatTimeout)
	s.loadDuration("SESSION_CLEANUP_INTERVAL", &cfg.Server.Session.CleanupInterval)
	s.loadInt("SESSION_MAX_CONNECTIONS", &cfg.Server.Session.MaxConnections)
	s.loadInt("SESSION_MAX_CONTROL_CONNECTIONS", &cfg.Server.Session.MaxControlConnections)
	s.loadDuration("SESSION_RECONNECT_WINDOW", &cfg.Server.Session.ReconnectWindow)

	// Client
	s.loadInt64("CLIENT_ID", &cfg.Client.ClientID)
	s.loadSecret("CLIENT_TOKEN", &cfg.Client.AuthToken)
	s.loadBool("CLIENT_ANONYMOUS", &cfg.Client.Anonymous)
	s.loadString("CLIENT_DEVICE_ID", &cfg.Client.DeviceID)
	s.loadSecret("CLIENT_SECRET_KEY", &cfg.Client.SecretKey)
	s.loadString("SERVER_ADDRESS", &cfg.Client.Server.Address)
	s.loadString("SERVER_PROTOCOL", &cfg.Client.Server.Protocol)
	s.loadBool("SERVER_AUTO_RECONNECT", &cfg.Client.Server.AutoReconnect)
	s.loadDuration("SERVER_RECONNECT_INTERVAL", &cfg.Client.Server.ReconnectInterval)
	s.loadDuration("SERVER_CONNECT_TIMEOUT", &cfg.Client.Server.ConnectTimeout)

	// Management
	s.loadBool("MANAGEMENT_ENABLED", &cfg.Management.Enabled)
	s.loadString("MANAGEMENT_LISTEN", &cfg.Management.Listen)
	s.loadString("MANAGEMENT_AUTH_TYPE", &cfg.Management.Auth.Type)
	s.loadSecret("MANAGEMENT_AUTH_TOKEN", &cfg.Management.Auth.Token)
	s.loadString("MANAGEMENT_AUTH_USERNAME", &cfg.Management.Auth.Username)
	s.loadSecret("MANAGEMENT_AUTH_PASSWORD", &cfg.Management.Auth.Password)
	s.loadBool("MANAGEMENT_PPROF_ENABLED", &cfg.Management.PProf.Enabled)
	s.loadString("MANAGEMENT_PPROF_DATA_DIR", &cfg.Management.PProf.DataDir)

	// HTTP
	s.loadBool("HTTP_ENABLED", &cfg.HTTP.Enabled)
	s.loadString("HTTP_LISTEN", &cfg.HTTP.Listen)
	s.loadBool("HTTP_MANAGEMENT_API_ENABLED", &cfg.HTTP.Modules.ManagementAPI.Enabled)
	s.loadString("HTTP_MANAGEMENT_API_PREFIX", &cfg.HTTP.Modules.ManagementAPI.Prefix)
	s.loadBool("HTTP_WEBSOCKET_ENABLED", &cfg.HTTP.Modules.WebSocket.Enabled)
	s.loadString("HTTP_WEBSOCKET_PATH", &cfg.HTTP.Modules.WebSocket.Path)
	s.loadBool("HTTP_DOMAIN_PROXY_ENABLED", &cfg.HTTP.Modules.DomainProxy.Enabled)
	s.loadStringSlice("HTTP_BASE_DOMAINS", &cfg.HTTP.Modules.DomainProxy.BaseDomains)
	s.loadInt("HTTP_SUBDOMAIN_LENGTH", &cfg.HTTP.Modules.DomainProxy.DefaultSubdomainLength)
	s.loadBool("HTTP_SSL_ENABLED", &cfg.HTTP.Modules.DomainProxy.SSL.Enabled)
	s.loadString("HTTP_SSL_CERT_PATH", &cfg.HTTP.Modules.DomainProxy.SSL.CertPath)
	s.loadString("HTTP_SSL_KEY_PATH", &cfg.HTTP.Modules.DomainProxy.SSL.KeyPath)
	s.loadBool("HTTP_CORS_ENABLED", &cfg.HTTP.CORS.Enabled)
	s.loadStringSlice("HTTP_CORS_ORIGINS", &cfg.HTTP.CORS.AllowedOrigins)
	s.loadBool("HTTP_RATE_LIMIT_ENABLED", &cfg.HTTP.RateLimit.Enabled)
	s.loadInt("HTTP_RATE_LIMIT_RPS", &cfg.HTTP.RateLimit.RequestsPerSecond)

	// Storage
	s.loadStringWithTrack("STORAGE_TYPE", &cfg.Storage.Type, &cfg.Storage.TypeSet)
	s.loadBool("REDIS_ENABLED", &cfg.Storage.Redis.Enabled)
	s.loadString("REDIS_ADDR", &cfg.Storage.Redis.Addr)
	s.loadSecret("REDIS_PASSWORD", &cfg.Storage.Redis.Password)
	s.loadInt("REDIS_DB", &cfg.Storage.Redis.DB)
	s.loadInt("REDIS_POOL_SIZE", &cfg.Storage.Redis.PoolSize)
	s.loadBoolWithTrack("PERSISTENCE_ENABLED", &cfg.Storage.Persistence.Enabled, &cfg.Storage.Persistence.EnabledSet)
	s.loadString("PERSISTENCE_FILE", &cfg.Storage.Persistence.File)
	s.loadBool("PERSISTENCE_AUTO_SAVE", &cfg.Storage.Persistence.AutoSave)
	s.loadDuration("PERSISTENCE_SAVE_INTERVAL", &cfg.Storage.Persistence.SaveInterval)
	s.loadBoolWithTrack("STORAGE_REMOTE_ENABLED", &cfg.Storage.Remote.Enabled, &cfg.Storage.Remote.EnabledSet)
	s.loadString("STORAGE_GRPC_ADDRESS", &cfg.Storage.Remote.GRPCAddress)
	s.loadDuration("STORAGE_TIMEOUT", &cfg.Storage.Remote.Timeout)

	// Security
	s.loadSecret("JWT_SECRET_KEY", &cfg.Security.JWT.SecretKey)
	s.loadDuration("JWT_EXPIRATION", &cfg.Security.JWT.Expiration)
	s.loadDuration("JWT_REFRESH_EXPIRATION", &cfg.Security.JWT.RefreshExpiration)
	s.loadString("JWT_ISSUER", &cfg.Security.JWT.Issuer)
	s.loadBool("RATE_LIMIT_IP_ENABLED", &cfg.Security.RateLimit.IP.Enabled)
	s.loadInt("RATE_LIMIT_IP_RATE", &cfg.Security.RateLimit.IP.Rate)
	s.loadInt("RATE_LIMIT_IP_BURST", &cfg.Security.RateLimit.IP.Burst)
	s.loadBool("RATE_LIMIT_TUNNEL_ENABLED", &cfg.Security.RateLimit.Tunnel.Enabled)
	s.loadInt64("RATE_LIMIT_TUNNEL_RATE", &cfg.Security.RateLimit.Tunnel.Rate)

	// Log
	s.loadString("LOG_LEVEL", &cfg.Log.Level)
	s.loadString("LOG_FORMAT", &cfg.Log.Format)
	s.loadString("LOG_FILE", &cfg.Log.File)
	s.loadBool("LOG_CONSOLE", &cfg.Log.Console)
	s.loadBool("LOG_ROTATION_ENABLED", &cfg.Log.Rotation.Enabled)
	s.loadInt("LOG_ROTATION_MAX_SIZE", &cfg.Log.Rotation.MaxSize)
	s.loadInt("LOG_ROTATION_MAX_BACKUPS", &cfg.Log.Rotation.MaxBackups)
	s.loadInt("LOG_ROTATION_MAX_AGE", &cfg.Log.Rotation.MaxAge)
	s.loadBool("LOG_ROTATION_COMPRESS", &cfg.Log.Rotation.Compress)

	// Health
	s.loadBool("HEALTH_ENABLED", &cfg.Health.Enabled)
	s.loadString("HEALTH_LISTEN", &cfg.Health.Listen)
	s.loadString("HEALTH_LIVENESS_PATH", &cfg.Health.Endpoints.Liveness)
	s.loadString("HEALTH_READINESS_PATH", &cfg.Health.Endpoints.Readiness)
	s.loadString("HEALTH_STARTUP_PATH", &cfg.Health.Endpoints.Startup)
	s.loadBool("HEALTH_CHECK_STORAGE", &cfg.Health.Checks.Storage.Enabled)
	s.loadBool("HEALTH_CHECK_REDIS", &cfg.Health.Checks.Redis.Enabled)
	s.loadBool("HEALTH_CHECK_PROTOCOLS", &cfg.Health.Checks.Protocols.Enabled)

	// Platform
	s.loadBool("PLATFORM_ENABLED", &cfg.Platform.Enabled)
	s.loadString("PLATFORM_URL", &cfg.Platform.URL)
	s.loadSecret("PLATFORM_TOKEN", &cfg.Platform.Token)
	s.loadDuration("PLATFORM_TIMEOUT", &cfg.Platform.Timeout)

	return nil
}

// getEnv gets environment variable with the configured prefix
func (s *EnvSource) getEnv(key string) (string, bool) {
	prefixedKey := s.prefix + "_" + key
	if v := os.Getenv(prefixedKey); v != "" {
		return v, true
	}
	return "", false
}

func (s *EnvSource) loadString(key string, target *string) {
	if v, ok := s.getEnv(key); ok {
		*target = v
	}
}

func (s *EnvSource) loadStringWithTrack(key string, target *string, setFlag **bool) {
	if v, ok := s.getEnv(key); ok {
		*target = v
		trueVal := true
		*setFlag = &trueVal
	}
}

func (s *EnvSource) loadSecret(key string, target *schema.Secret) {
	if v, ok := s.getEnv(key); ok {
		*target = schema.Secret(v)
	}
}

func (s *EnvSource) loadBool(key string, target *bool) {
	if v, ok := s.getEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			*target = b
		}
	}
}

func (s *EnvSource) loadBoolWithTrack(key string, target *bool, setFlag **bool) {
	if v, ok := s.getEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			*target = b
			trueVal := true
			*setFlag = &trueVal
		}
	}
}

func (s *EnvSource) loadInt(key string, target *int) {
	if v, ok := s.getEnv(key); ok {
		if i, err := strconv.Atoi(v); err == nil {
			*target = i
		}
	}
}

func (s *EnvSource) loadInt64(key string, target *int64) {
	if v, ok := s.getEnv(key); ok {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			*target = i
		}
	}
}

func (s *EnvSource) loadDuration(key string, target *time.Duration) {
	if v, ok := s.getEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			*target = d
		}
	}
}

func (s *EnvSource) loadStringSlice(key string, target *[]string) {
	if v, ok := s.getEnv(key); ok {
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			*target = result
		}
	}
}

