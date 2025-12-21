package httpservice

import "time"

// HTTPServiceConfig HTTP 服务配置
type HTTPServiceConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ListenAddr string `yaml:"listen_addr"`

	// 模块配置
	Modules ModulesConfig `yaml:"modules"`

	// 通用配置
	CORS      CORSConfig      `yaml:"cors"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
}

// ModulesConfig 模块配置
type ModulesConfig struct {
	ManagementAPI  ManagementAPIModuleConfig  `yaml:"management_api"`
	HTTPPoll       HTTPPollModuleConfig       `yaml:"httppoll"`
	WebSocket      WebSocketModuleConfig      `yaml:"websocket"`
	DomainProxy    DomainProxyModuleConfig    `yaml:"domain_proxy"`
	WebSocketProxy WebSocketProxyModuleConfig `yaml:"websocket_proxy"`
}

// ManagementAPIModuleConfig 管理 API 模块配置
type ManagementAPIModuleConfig struct {
	Enabled bool        `yaml:"enabled"`
	Auth    AuthConfig  `yaml:"auth"`
	PProf   PProfConfig `yaml:"pprof"`
}

// HTTPPollModuleConfig HTTP 长轮询传输模块配置
type HTTPPollModuleConfig struct {
	Enabled        bool `yaml:"enabled"`
	MaxRequestSize int  `yaml:"max_request_size"` // 最大请求大小（字节）
	DefaultTimeout int  `yaml:"default_timeout"`  // 默认超时（秒）
	MaxTimeout     int  `yaml:"max_timeout"`      // 最大超时（秒）
}

// DomainProxyModuleConfig 域名代理模块配置
type DomainProxyModuleConfig struct {
	Enabled              bool             `yaml:"enabled"`
	BaseDomains          []string         `yaml:"base_domains"`           // 基础域名，如 ["tunnel.example.com"]
	DefaultScheme        string           `yaml:"default_scheme"`         // 默认协议: http/https
	CommandModeThreshold int64            `yaml:"command_mode_threshold"` // 命令模式阈值（字节），小于此值用命令模式
	TunnelPool           TunnelPoolConfig `yaml:"tunnel_pool"`            // 隧道复用配置
	RequestTimeout       time.Duration    `yaml:"request_timeout"`        // 请求超时
}

// TunnelPoolConfig 隧道池配置
type TunnelPoolConfig struct {
	Enabled             bool          `yaml:"enabled"`
	IdleTimeout         time.Duration `yaml:"idle_timeout"`           // 空闲超时
	MaxTunnelsPerClient int           `yaml:"max_tunnels_per_client"` // 每个 Client 最大隧道数
}

// WebSocketProxyModuleConfig WebSocket 代理模块配置
type WebSocketProxyModuleConfig struct {
	Enabled bool `yaml:"enabled"`
}

// WebSocketModuleConfig WebSocket 传输模块配置（客户端控制连接）
type WebSocketModuleConfig struct {
	Enabled bool `yaml:"enabled"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Type   string `yaml:"type"`   // api_key / jwt / bearer / none
	Secret string `yaml:"secret"` // API 密钥或 JWT 密钥
}

// CORSConfig CORS 配置
type CORSConfig struct {
	Enabled        bool     `yaml:"enabled"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	AllowedMethods []string `yaml:"allowed_methods"`
	AllowedHeaders []string `yaml:"allowed_headers"`
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerSecond int  `yaml:"requests_per_second"`
	Burst             int  `yaml:"burst"`
}

// PProfConfig PProf 性能分析配置
type PProfConfig struct {
	Enabled     bool   `yaml:"enabled"`
	DataDir     string `yaml:"data_dir"`
	Retention   int    `yaml:"retention"`
	AutoCapture bool   `yaml:"auto_capture"`
}

// DefaultHTTPServiceConfig 返回默认配置
func DefaultHTTPServiceConfig() *HTTPServiceConfig {
	return &HTTPServiceConfig{
		Enabled:    true,
		ListenAddr: "0.0.0.0:9000",
		Modules: ModulesConfig{
			ManagementAPI: ManagementAPIModuleConfig{
				Enabled: true,
				Auth: AuthConfig{
					Type: "bearer",
				},
				PProf: PProfConfig{
					Enabled:     false,
					DataDir:     "logs/pprof",
					Retention:   10,
					AutoCapture: true,
				},
			},
			HTTPPoll: HTTPPollModuleConfig{
				Enabled:        true,
				MaxRequestSize: 1048576, // 1MB
				DefaultTimeout: 30,
				MaxTimeout:     60,
			},
			WebSocket: WebSocketModuleConfig{
				Enabled: true,
			},
			DomainProxy: DomainProxyModuleConfig{
				Enabled:              false,
				DefaultScheme:        "http",
				CommandModeThreshold: 1048576, // 1MB
				TunnelPool: TunnelPoolConfig{
					Enabled:             true,
					IdleTimeout:         30 * time.Second,
					MaxTunnelsPerClient: 100,
				},
				RequestTimeout: 30 * time.Second,
			},
			WebSocketProxy: WebSocketProxyModuleConfig{
				Enabled: false,
			},
		},
		CORS: CORSConfig{
			Enabled:        true,
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type", "Authorization", "X-Request-ID"},
		},
		RateLimit: RateLimitConfig{
			Enabled:           false,
			RequestsPerSecond: 100,
			Burst:             200,
		},
	}
}
