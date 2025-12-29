package source

import (
	"time"

	"tunnox-core/internal/config/schema"
)

// DefaultSource provides default configuration values
type DefaultSource struct{}

// NewDefaultSource creates a new DefaultSource
func NewDefaultSource() *DefaultSource {
	return &DefaultSource{}
}

// Name returns the source name
func (s *DefaultSource) Name() string {
	return "defaults"
}

// Priority returns the source priority
func (s *DefaultSource) Priority() int {
	return PriorityDefaults
}

// LoadInto loads default values into the configuration
func (s *DefaultSource) LoadInto(cfg *schema.Root) error {
	// Server defaults
	cfg.Server.Protocols.TCP.Enabled = true
	cfg.Server.Protocols.TCP.Port = 8000
	cfg.Server.Protocols.TCP.Host = "0.0.0.0"

	cfg.Server.Protocols.WebSocket.Enabled = true

	cfg.Server.Protocols.KCP.Enabled = true
	cfg.Server.Protocols.KCP.Port = 8000
	cfg.Server.Protocols.KCP.Host = "0.0.0.0"
	cfg.Server.Protocols.KCP.Mode = schema.KCPModeFast
	cfg.Server.Protocols.KCP.SndWnd = 1024
	cfg.Server.Protocols.KCP.RcvWnd = 1024
	cfg.Server.Protocols.KCP.MTU = 1400

	cfg.Server.Protocols.QUIC.Enabled = true
	cfg.Server.Protocols.QUIC.Port = 8443
	cfg.Server.Protocols.QUIC.Host = "0.0.0.0"
	cfg.Server.Protocols.QUIC.MaxStreams = 100
	cfg.Server.Protocols.QUIC.IdleTimeout = 30 * time.Second

	cfg.Server.Session.HeartbeatTimeout = 60 * time.Second
	cfg.Server.Session.CleanupInterval = 15 * time.Second
	cfg.Server.Session.MaxConnections = 10000
	cfg.Server.Session.MaxControlConnections = 5000
	cfg.Server.Session.ReconnectWindow = 300 * time.Second

	// Client defaults
	cfg.Client.Anonymous = true
	cfg.Client.DeviceID = "auto"
	cfg.Client.Server.Address = "https://gw.tunnox.net/_tunnox"
	cfg.Client.Server.Protocol = schema.ProtocolWebSocket
	cfg.Client.Server.AutoReconnect = true
	cfg.Client.Server.ReconnectInterval = 5 * time.Second
	cfg.Client.Server.ConnectTimeout = 30 * time.Second
	cfg.Client.Log.Level = schema.LogLevelInfo
	cfg.Client.Log.Format = schema.LogFormatText
	cfg.Client.Stream.CompressionLevel = 6
	cfg.Client.Stream.BufferSize = 4096

	// Management defaults
	cfg.Management.Enabled = true
	cfg.Management.Listen = "0.0.0.0:9000"
	cfg.Management.Auth.Type = schema.AuthTypeBearer
	cfg.Management.PProf.Enabled = true
	cfg.Management.PProf.DataDir = "logs/pprof"
	cfg.Management.CORS.Enabled = true
	cfg.Management.CORS.AllowedOrigins = []string{"*"}

	// HTTP defaults
	cfg.HTTP.Enabled = true
	cfg.HTTP.Listen = "0.0.0.0:9000"
	cfg.HTTP.Modules.ManagementAPI.Enabled = true
	cfg.HTTP.Modules.ManagementAPI.Prefix = "/_api"
	cfg.HTTP.Modules.WebSocket.Enabled = true
	cfg.HTTP.Modules.WebSocket.Path = "/_tunnox"
	cfg.HTTP.Modules.DomainProxy.DefaultSubdomainLength = 8
	// P0: Default base_domains includes localhost.tunnox.dev
	cfg.HTTP.Modules.DomainProxy.BaseDomains = []string{schema.DefaultBaseDomain}
	cfg.HTTP.CORS.Enabled = true
	cfg.HTTP.CORS.AllowedOrigins = []string{"*"}
	cfg.HTTP.CORS.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	cfg.HTTP.CORS.AllowedHeaders = []string{"*"}
	cfg.HTTP.CORS.MaxAge = 86400
	cfg.HTTP.RateLimit.RequestsPerSecond = 100
	cfg.HTTP.RateLimit.Burst = 200

	// Storage defaults
	cfg.Storage.Type = schema.StorageTypeMemory
	cfg.Storage.Redis.Addr = "localhost:6379"
	cfg.Storage.Redis.PoolSize = 10
	cfg.Storage.Redis.MinIdleConns = 5
	cfg.Storage.Redis.MaxRetries = 3
	cfg.Storage.Redis.DialTimeout = 5 * time.Second
	cfg.Storage.Redis.ReadTimeout = 3 * time.Second
	cfg.Storage.Redis.WriteTimeout = 3 * time.Second
	cfg.Storage.Persistence.Enabled = true
	cfg.Storage.Persistence.File = "data/tunnox.json"
	cfg.Storage.Persistence.AutoSave = true
	cfg.Storage.Persistence.SaveInterval = 30 * time.Second
	cfg.Storage.Remote.Timeout = 5 * time.Second
	cfg.Storage.Remote.MaxRetries = 3
	cfg.Storage.Hybrid.CacheType = schema.StorageTypeMemory
	cfg.Storage.Hybrid.DefaultCacheTTL = time.Hour
	cfg.Storage.Hybrid.PersistentCacheTTL = 24 * time.Hour
	cfg.Storage.Hybrid.SharedCacheTTL = 5 * time.Minute

	// Security defaults
	cfg.Security.JWT.Expiration = 24 * time.Hour
	cfg.Security.JWT.RefreshExpiration = 168 * time.Hour
	cfg.Security.JWT.Issuer = "tunnox"
	cfg.Security.RateLimit.IP.Enabled = true
	cfg.Security.RateLimit.IP.Rate = 10
	cfg.Security.RateLimit.IP.Burst = 20
	cfg.Security.RateLimit.IP.TTL = 5 * time.Minute
	cfg.Security.RateLimit.Tunnel.Rate = 1048576   // 1MB/s
	cfg.Security.RateLimit.Tunnel.Burst = 10485760 // 10MB
	cfg.Security.RateLimit.Client.Rate = 100
	cfg.Security.RateLimit.Client.Burst = 200

	// Log defaults
	cfg.Log.Level = schema.LogLevelInfo
	cfg.Log.Format = schema.LogFormatText
	cfg.Log.File = "logs/server.log"
	cfg.Log.Console = true
	cfg.Log.Rotation.Enabled = true
	cfg.Log.Rotation.MaxSize = 100
	cfg.Log.Rotation.MaxBackups = 10
	cfg.Log.Rotation.MaxAge = 30

	// Health defaults
	cfg.Health.Enabled = true
	cfg.Health.Listen = "0.0.0.0:9090"
	cfg.Health.Endpoints.Liveness = "/healthz"
	cfg.Health.Endpoints.Readiness = "/ready"
	cfg.Health.Endpoints.Startup = "/startup"
	cfg.Health.Checks.Storage.Enabled = true
	cfg.Health.Checks.Storage.Timeout = 3 * time.Second
	cfg.Health.Checks.Redis.Enabled = true
	cfg.Health.Checks.Redis.Timeout = 3 * time.Second
	cfg.Health.Checks.Protocols.Enabled = true
	cfg.Health.Checks.Protocols.Timeout = 3 * time.Second

	// Platform defaults
	cfg.Platform.Timeout = 10 * time.Second
	cfg.Platform.Retry.MaxRetries = 3
	cfg.Platform.Retry.RetryInterval = time.Second

	return nil
}

// GetDefaultConfig returns a fully initialized default configuration
func GetDefaultConfig() *schema.Root {
	cfg := &schema.Root{}
	source := NewDefaultSource()
	_ = source.LoadInto(cfg)
	return cfg
}
