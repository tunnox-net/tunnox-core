package server

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"tunnox-core/internal/constants"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils"

	"gopkg.in/yaml.v3"
)

// ProtocolConfig 协议配置
type ProtocolConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port,omitempty"`
	Host    string `yaml:"host,omitempty"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Protocols map[string]ProtocolConfig `yaml:"protocols"`
}

// ManagementConfig 管理服务配置
type ManagementConfig struct {
	Listen string      `yaml:"listen"`
	Auth   AuthConfig  `yaml:"auth"`
	PProf  PProfConfig `yaml:"pprof"`
}

// PProfConfig PProf 性能分析配置
type PProfConfig struct {
	Enabled     bool   `yaml:"enabled"`
	DataDir     string `yaml:"data_dir"`
	Retention   int    `yaml:"retention"`
	AutoCapture bool   `yaml:"auto_capture"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Type  string `yaml:"type"`  // bearer | basic | none
	Token string `yaml:"token"` // Bearer token
}

// LogConfig 日志配置
type LogConfig struct {
	Level    string            `yaml:"level"`    // debug, info, warn, error
	File     string            `yaml:"file"`     // 日志文件路径
	Rotation LogRotationConfig `yaml:"rotation"` // 日志轮转配置
}

// LogRotationConfig 日志轮转配置
type LogRotationConfig struct {
	MaxSize    int  `yaml:"max_size"`    // 单个文件最大大小(MB)
	MaxBackups int  `yaml:"max_backups"` // 保留的旧文件数量
	MaxAge     int  `yaml:"max_age"`     // 保留天数
	Compress   bool `yaml:"compress"`    // 是否压缩
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// PersistenceConfig 持久化配置
type PersistenceConfig struct {
	Enabled      bool   `yaml:"enabled"`
	File         string `yaml:"file"`
	AutoSave     bool   `yaml:"auto_save"`
	SaveInterval int    `yaml:"save_interval"` // 秒
}

// StorageConfig 远程存储配置
type StorageConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	Token   string `yaml:"token"`
	Timeout int    `yaml:"timeout"` // 秒
}

// PlatformConfig 云控平台配置
type PlatformConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	Token   string `yaml:"token"`
	Timeout int    `yaml:"timeout"` // 秒
}

// SecurityConfig 安全配置
type SecurityConfig struct {
	ReconnectTokenSecret string `yaml:"reconnect_token_secret"` // 重连Token HMAC密钥，为空时自动生成
	ReconnectTokenTTL    int    `yaml:"reconnect_token_ttl"`    // 重连Token有效期（秒），默认30
}

// Config 应用配置
type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Management  ManagementConfig  `yaml:"management"`
	Log         LogConfig         `yaml:"log"`
	Redis       RedisConfig       `yaml:"redis"`
	Persistence PersistenceConfig `yaml:"persistence"`
	Storage     StorageConfig     `yaml:"storage"`
	Platform    PlatformConfig    `yaml:"platform"`
	Security    SecurityConfig    `yaml:"security"`
}

// LoadConfig 加载配置文件
// 如果配置文件不存在,使用默认配置(不自动生成文件)
func LoadConfig(configPath string) (*Config, error) {
	// 如果配置文件不存在,使用默认配置
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		corelog.Warnf(constants.MsgConfigFileNotFound, configPath)
		corelog.Infof("Using default configuration (no config file generated)")
		config := GetDefaultConfig()
		ApplyEnvOverrides(config)
		return config, nil
	}

	// 读取配置文件
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeConfigError, constants.MsgFailedToReadConfigFile, configPath, err)
	}

	// 解析YAML
	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeConfigError, constants.MsgFailedToParseConfigFile, configPath, err)
	}

	// 应用环境变量覆盖（环境变量优先级高于配置文件）
	ApplyEnvOverrides(&config)

	// 确保日志文件路径已设置
	if config.Log.File == "" {
		config.Log.File = utils.GetDefaultServerLogPath()
	} else {
		// 展开路径（支持 ~ 和相对路径）
		expandedPath, err := utils.ExpandPath(config.Log.File)
		if err != nil {
			return nil, coreerrors.Wrapf(err, coreerrors.CodeConfigError, "failed to expand log file path %q", config.Log.File)
		}
		config.Log.File = expandedPath
	}

	// 确保日志目录存在
	logDir := filepath.Dir(config.Log.File)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeConfigError, "failed to create log directory %q", logDir)
	}

	// 验证配置
	if err := ValidateConfig(&config); err != nil {
		return nil, coreerrors.Wrapf(err, coreerrors.CodeConfigError, constants.MsgInvalidConfiguration, err)
	}

	corelog.Infof(constants.MsgConfigLoadedFrom, configPath)
	return &config, nil
}

// ValidateConfig 验证配置
func ValidateConfig(config *Config) error {
	// 验证协议配置
	if config.Server.Protocols == nil {
		config.Server.Protocols = make(map[string]ProtocolConfig)
	}

	// 设置默认协议配置
	defaultProtocols := map[string]ProtocolConfig{
		"tcp": {
			Enabled: true,
			Port:    8000,
			Host:    "0.0.0.0",
		},
		"kcp": {
			Enabled: true,
			Port:    8000,
			Host:    "0.0.0.0",
		},
		"quic": {
			Enabled: true,
			Port:    8443,
			Host:    "0.0.0.0",
		},
		"websocket": {
			Enabled: true,
		},
	}

	// 合并默认配置
	for name, defaultConfig := range defaultProtocols {
		if userConfig, exists := config.Server.Protocols[name]; exists {
			if userConfig.Port <= 0 {
				userConfig.Port = defaultConfig.Port
			}
			if userConfig.Host == "" {
				userConfig.Host = defaultConfig.Host
			}
			config.Server.Protocols[name] = userConfig
		} else {
			config.Server.Protocols[name] = defaultConfig
		}
	}

	// 验证 Management 配置
	if config.Management.Listen == "" {
		config.Management.Listen = "0.0.0.0:9000"
	}
	if config.Management.Auth.Type == "" {
		config.Management.Auth.Type = "bearer"
	}
	if config.Management.PProf.DataDir == "" {
		config.Management.PProf.DataDir = "logs/pprof"
	}
	if config.Management.PProf.Retention <= 0 {
		config.Management.PProf.Retention = 10
	}

	// 验证日志配置
	if config.Log.Level == "" {
		config.Log.Level = "info"
	}
	if config.Log.File == "" {
		config.Log.File = "logs/server.log"
	}
	// 设置日志轮转默认值
	if config.Log.Rotation.MaxSize <= 0 {
		config.Log.Rotation.MaxSize = 100
	}
	if config.Log.Rotation.MaxBackups <= 0 {
		config.Log.Rotation.MaxBackups = 10
	}
	if config.Log.Rotation.MaxAge <= 0 {
		config.Log.Rotation.MaxAge = 30
	}

	// 验证 Redis 配置
	if config.Redis.Enabled && config.Redis.Addr == "" {
		return coreerrors.New(coreerrors.CodeConfigError, "redis.addr is required when redis.enabled is true")
	}

	// 验证 Persistence 配置
	if config.Persistence.Enabled && config.Persistence.File == "" {
		config.Persistence.File = "data/tunnox.json"
	}
	if config.Persistence.SaveInterval <= 0 {
		config.Persistence.SaveInterval = 30
	}

	// 验证 Storage 配置
	if config.Storage.Enabled && config.Storage.URL == "" {
		return coreerrors.New(coreerrors.CodeConfigError, "storage.url is required when storage.enabled is true")
	}
	if config.Storage.Timeout <= 0 {
		config.Storage.Timeout = 10
	}

	// 验证 Platform 配置
	if config.Platform.Enabled && config.Platform.URL == "" {
		return coreerrors.New(coreerrors.CodeConfigError, "platform.url is required when platform.enabled is true")
	}
	if config.Platform.Timeout <= 0 {
		config.Platform.Timeout = 10
	}

	// 验证 Security 配置
	if config.Security.ReconnectTokenSecret == "" {
		// 未配置密钥时自动生成随机密钥
		secret, err := generateRandomSecret(32)
		if err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeConfigError, "failed to generate random secret")
		}
		config.Security.ReconnectTokenSecret = secret
		corelog.Warnf("security.reconnect_token_secret not configured, using auto-generated random secret (not recommended for production cluster)")
	}
	if config.Security.ReconnectTokenTTL <= 0 {
		config.Security.ReconnectTokenTTL = 30 // 默认30秒
	}

	return nil
}

// generateRandomSecret 生成随机密钥
func generateRandomSecret(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GetDefaultConfig 获取默认配置
func GetDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Protocols: map[string]ProtocolConfig{
				"tcp": {
					Enabled: true,
					Port:    8000,
					Host:    "0.0.0.0",
				},
				"kcp": {
					Enabled: true,
					Port:    8000,
					Host:    "0.0.0.0",
				},
				"quic": {
					Enabled: true,
					Port:    8443,
					Host:    "0.0.0.0",
				},
				"websocket": {
					Enabled: true,
				},
			},
		},
		Management: ManagementConfig{
			Listen: "0.0.0.0:9000",
			Auth: AuthConfig{
				Type:  "bearer",
				Token: "",
			},
			PProf: PProfConfig{
				Enabled:     true,
				DataDir:     "logs/pprof",
				Retention:   10,
				AutoCapture: true,
			},
		},
		Log: LogConfig{
			Level: "info",
			File:  "logs/server.log",
			Rotation: LogRotationConfig{
				MaxSize:    100,
				MaxBackups: 10,
				MaxAge:     30,
				Compress:   false,
			},
		},
		Redis: RedisConfig{
			Enabled:  false,
			Addr:     "redis:6379",
			Password: "",
			DB:       0,
		},
		Persistence: PersistenceConfig{
			Enabled:      true,
			File:         "data/tunnox.json",
			AutoSave:     true,
			SaveInterval: 30,
		},
		Storage: StorageConfig{
			Enabled: false,
			URL:     "http://tunnox-storage:8080",
			Token:   "",
			Timeout: 10,
		},
		Platform: PlatformConfig{
			Enabled: false,
			URL:     "http://tunnox-platform:8080",
			Token:   "",
			Timeout: 10,
		},
		Security: SecurityConfig{
			ReconnectTokenSecret: "", // 为空时自动生成
			ReconnectTokenTTL:    30,
		},
	}
}

// SaveConfig 保存配置到文件
func SaveConfig(configPath string, config *Config) error {
	// 确保目录存在
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeConfigError, "failed to create config directory")
	}

	// 序列化为 YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeConfigError, "failed to marshal config")
	}

	// 写入临时文件
	tempFile := configPath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeConfigError, "failed to write temp file")
	}

	// 原子替换
	if err := os.Rename(tempFile, configPath); err != nil {
		os.Remove(tempFile) // 清理临时文件
		return coreerrors.Wrap(err, coreerrors.CodeConfigError, "failed to rename temp file")
	}

	return nil
}

// SaveMinimalConfig 保存简洁的配置模板到文件
func SaveMinimalConfig(configPath string) error {
	return ExportConfigTemplate(configPath)
}

// ExportConfigTemplate 导出配置模板
func ExportConfigTemplate(configPath string) error {
	// 确保目录存在
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeConfigError, "failed to create config directory")
	}

	// 简洁的配置模板
	template := GetConfigTemplate()

	// 写入文件
	if err := os.WriteFile(configPath, []byte(template), 0644); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeConfigError, "failed to write config file")
	}

	return nil
}

// GetConfigTemplate 获取配置模板
func GetConfigTemplate() string {
	return `# Tunnox Core Server Configuration
# 只需配置需要修改的部分，其他保持默认即可

# ============================================
# 协议配置
# ============================================
server:
  protocols:
    tcp:
      enabled: true
      port: 8000
      host: "0.0.0.0"
    kcp:
      enabled: true
      port: 8000
      host: "0.0.0.0"
    quic:
      enabled: true
      port: 8443
      host: "0.0.0.0"
    websocket:
      enabled: true

# ============================================
# HTTP 管理服务
# ============================================
management:
  listen: "0.0.0.0:9000"
  auth:
    type: bearer
    token: ""  # 设置 API 访问令牌
  pprof:
    enabled: true
    data_dir: "logs/pprof"
    retention: 10
    auto_capture: true

# ============================================
# 日志配置
# ============================================
log:
  level: info  # debug, info, warn, error
  file: "logs/server.log"
  rotation:
    max_size: 100     # MB
    max_backups: 10
    max_age: 30       # days
    compress: false

# ============================================
# Redis 配置 - 可选
# 启用后自动切换为集群模式
# ============================================
redis:
  enabled: false
  addr: "redis:6379"
  password: ""
  db: 0

# ============================================
# 本地持久化 - 可选
# 单节点模式下推荐启用
# ============================================
persistence:
  enabled: true
  file: "data/tunnox.json"
  auto_save: true
  save_interval: 30  # seconds

# ============================================
# 远程存储 - 可选
# 连接 tunnox-storage 服务
# ============================================
storage:
  enabled: false
  url: "http://tunnox-storage:8080"
  token: ""
  timeout: 10  # seconds

# ============================================
# 云控平台 - 可选
# 连接 tunnox-platform 服务
# ============================================
platform:
  enabled: false
  url: "http://tunnox-platform:8080"
  token: ""
  timeout: 10  # seconds

# ============================================
# 安全配置
# ============================================
security:
  reconnect_token_secret: ""  # 重连Token HMAC密钥，为空时自动生成随机密钥
  reconnect_token_ttl: 30     # 重连Token有效期（秒）
`
}
