// Package config provides unified configuration management
package config

import (
	"context"
	"sync"

	"tunnox-core/internal/config/loader"
	"tunnox-core/internal/config/schema"
	"tunnox-core/internal/config/source"
	"tunnox-core/internal/config/validator"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// AppType represents the application type
type AppType string

const (
	// AppTypeServer is the server application type
	AppTypeServer AppType = "server"
	// AppTypeClient is the client application type
	AppTypeClient AppType = "client"
)

// ManagerOptions contains configuration manager options
type ManagerOptions struct {
	// ConfigFile is the path to the configuration file (optional)
	ConfigFile string

	// EnvPrefix is the environment variable prefix (default: TUNNOX)
	EnvPrefix string

	// EnableDotEnv enables .env file loading (default: true)
	EnableDotEnv bool

	// EnableWatch enables configuration file watching (default: false)
	EnableWatch bool

	// AppType is the application type (server/client)
	AppType AppType

	// AppEnv is the application environment (development/production)
	AppEnv string

	// SkipValidation skips configuration validation (default: false)
	SkipValidation bool
}

// Manager is the unified configuration manager
// P0: Follows the Dispose pattern by embedding ServiceBase
type Manager struct {
	*dispose.ResourceBase

	opts       ManagerOptions
	config     *schema.Root
	configMu   sync.RWMutex
	onChange   []func(*schema.Root)
	onChangeMu sync.Mutex
	validator  *validator.Validator
}

// NewManager creates a new configuration Manager
// P0: Follows Dispose pattern with proper context propagation
func NewManager(parentCtx context.Context, opts ManagerOptions) *Manager {
	// Apply defaults
	if opts.EnvPrefix == "" {
		opts.EnvPrefix = "TUNNOX"
	}

	m := &Manager{
		ResourceBase: dispose.NewResourceBase("ConfigManager"),
		opts:         opts,
		onChange:     make([]func(*schema.Root), 0),
		validator:    validator.NewValidator(),
	}

	// Initialize with parent context
	m.ResourceBase.Initialize(parentCtx)

	// Add cleanup handler
	m.AddCleanHandler(m.onClose)

	return m
}

// onClose is called when the manager is disposed
func (m *Manager) onClose() error {
	corelog.Debugf("ConfigManager closing")
	return nil
}

// Load loads configuration from all sources
func (m *Manager) Load() error {
	// Build loader
	loaderBuilder := loader.NewLoaderBuilder().
		WithPrefix(m.opts.EnvPrefix).
		WithConfigFile(m.opts.ConfigFile).
		WithAppType(string(m.opts.AppType)).
		WithAppEnv(m.opts.AppEnv).
		WithDotEnv(m.opts.EnableDotEnv)

	l := loaderBuilder.Build()

	// Load configuration
	cfg, err := l.Load()
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInvalidParam, "failed to load configuration")
	}

	// Validate configuration
	if !m.opts.SkipValidation {
		result := m.validator.Validate(cfg)
		if !result.IsValid() {
			return coreerrors.New(coreerrors.CodeValidationError, result.Error())
		}
	}

	// Store configuration
	m.configMu.Lock()
	m.config = cfg
	m.configMu.Unlock()

	corelog.Infof("Configuration loaded successfully")
	return nil
}

// Get returns the current configuration
func (m *Manager) Get() *schema.Root {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	return m.config
}

// GetServer returns the server configuration
func (m *Manager) GetServer() *schema.ServerConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	return &m.config.Server
}

// GetClient returns the client configuration
func (m *Manager) GetClient() *schema.ClientConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	return &m.config.Client
}

// GetHTTP returns the HTTP configuration
func (m *Manager) GetHTTP() *schema.HTTPConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	return &m.config.HTTP
}

// GetStorage returns the storage configuration
func (m *Manager) GetStorage() *schema.StorageConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	return &m.config.Storage
}

// GetSecurity returns the security configuration
func (m *Manager) GetSecurity() *schema.SecurityConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	return &m.config.Security
}

// GetLog returns the log configuration
func (m *Manager) GetLog() *schema.LogConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	return &m.config.Log
}

// GetHealth returns the health check configuration
func (m *Manager) GetHealth() *schema.HealthConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	return &m.config.Health
}

// GetManagement returns the management API configuration
func (m *Manager) GetManagement() *schema.ManagementConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	return &m.config.Management
}

// GetPlatform returns the platform configuration
func (m *Manager) GetPlatform() *schema.PlatformConfig {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.config == nil {
		return nil
	}
	return &m.config.Platform
}

// Validate validates the current configuration
func (m *Manager) Validate() error {
	m.configMu.RLock()
	cfg := m.config
	m.configMu.RUnlock()

	if cfg == nil {
		return coreerrors.New(coreerrors.CodeInvalidParam, "no configuration loaded")
	}

	result := m.validator.Validate(cfg)
	if !result.IsValid() {
		return coreerrors.New(coreerrors.CodeValidationError, result.Error())
	}

	return nil
}

// OnChange registers a callback for configuration changes
func (m *Manager) OnChange(fn func(*schema.Root)) {
	m.onChangeMu.Lock()
	defer m.onChangeMu.Unlock()
	m.onChange = append(m.onChange, fn)
}

// notifyChange notifies all registered callbacks about configuration changes
func (m *Manager) notifyChange(cfg *schema.Root) {
	m.onChangeMu.Lock()
	callbacks := make([]func(*schema.Root), len(m.onChange))
	copy(callbacks, m.onChange)
	m.onChangeMu.Unlock()

	for _, fn := range callbacks {
		fn(cfg)
	}
}

// Reload reloads the configuration
func (m *Manager) Reload() error {
	// Load new configuration
	if err := m.Load(); err != nil {
		return err
	}

	// Notify callbacks
	m.configMu.RLock()
	cfg := m.config
	m.configMu.RUnlock()

	m.notifyChange(cfg)

	corelog.Infof("Configuration reloaded")
	return nil
}

// Dispose implements the Disposable interface
func (m *Manager) Dispose() error {
	return m.Close()
}

// ============================================================================
// Convenience Functions
// ============================================================================

// LoadServerConfig loads and returns server configuration
func LoadServerConfig(ctx context.Context, configFile string) (*Manager, error) {
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeServer,
	})

	if err := m.Load(); err != nil {
		return nil, err
	}

	return m, nil
}

// LoadClientConfig loads and returns client configuration
func LoadClientConfig(ctx context.Context, configFile string) (*Manager, error) {
	m := NewManager(ctx, ManagerOptions{
		ConfigFile: configFile,
		AppType:    AppTypeClient,
	})

	if err := m.Load(); err != nil {
		return nil, err
	}

	return m, nil
}

// GetDefaultConfig returns the default configuration
func GetDefaultConfig() *schema.Root {
	return source.GetDefaultConfig()
}
