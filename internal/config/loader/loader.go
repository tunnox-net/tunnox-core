// Package loader provides multi-source configuration loading
package loader

import (
	"sort"

	"tunnox-core/internal/config/inference"
	"tunnox-core/internal/config/schema"
	"tunnox-core/internal/config/source"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// Loader loads configuration from multiple sources in priority order
type Loader struct {
	sources       []source.Source
	skipInference bool
}

// NewLoader creates a new Loader
func NewLoader() *Loader {
	return &Loader{
		sources: make([]source.Source, 0),
	}
}

// AddSource adds a configuration source
func (l *Loader) AddSource(s source.Source) {
	l.sources = append(l.sources, s)
}

// SetSkipInference disables the inference phase
func (l *Loader) SetSkipInference(skip bool) {
	l.skipInference = skip
}

// Load loads configuration from all sources in priority order
// Lower priority sources are loaded first, then higher priority sources override
func (l *Loader) Load() (*schema.Root, error) {
	if len(l.sources) == 0 {
		return nil, coreerrors.New(coreerrors.CodeInvalidParam, "no configuration sources registered")
	}

	// Sort sources by priority (ascending)
	sorted := make([]source.Source, len(l.sources))
	copy(sorted, l.sources)
	sort.Sort(source.ByPriority(sorted))

	// Create empty config
	cfg := &schema.Root{}

	// Load from each source in priority order
	for _, s := range sorted {
		corelog.Debugf("Loading configuration from source: %s (priority %d)", s.Name(), s.Priority())
		if err := s.LoadInto(cfg); err != nil {
			return nil, coreerrors.Wrapf(err, coreerrors.CodeInvalidParam,
				"failed to load configuration from source %s", s.Name())
		}
	}

	// Apply inference phase (post-processing)
	if !l.skipInference {
		engine := inference.NewEngine()
		result := engine.Infer(cfg)
		inference.PrintReport(result)
	}

	return cfg, nil
}

// LoaderBuilder helps build a Loader with common configurations
type LoaderBuilder struct {
	loader        *Loader
	prefix        string
	configFile    string
	appType       string
	appEnv        string
	enableDotEnv  bool
	skipInference bool
}

// NewLoaderBuilder creates a new LoaderBuilder
func NewLoaderBuilder() *LoaderBuilder {
	return &LoaderBuilder{
		loader:       NewLoader(),
		prefix:       "TUNNOX",
		enableDotEnv: true,
	}
}

// WithPrefix sets the environment variable prefix
func (b *LoaderBuilder) WithPrefix(prefix string) *LoaderBuilder {
	b.prefix = prefix
	return b
}

// WithConfigFile sets the configuration file path
func (b *LoaderBuilder) WithConfigFile(path string) *LoaderBuilder {
	b.configFile = path
	return b
}

// WithAppType sets the application type (server/client)
func (b *LoaderBuilder) WithAppType(appType string) *LoaderBuilder {
	b.appType = appType
	return b
}

// WithAppEnv sets the application environment (development/production)
func (b *LoaderBuilder) WithAppEnv(env string) *LoaderBuilder {
	b.appEnv = env
	return b
}

// WithDotEnv enables or disables .env file loading
func (b *LoaderBuilder) WithDotEnv(enabled bool) *LoaderBuilder {
	b.enableDotEnv = enabled
	return b
}

// WithSkipInference enables or disables the inference phase
func (b *LoaderBuilder) WithSkipInference(skip bool) *LoaderBuilder {
	b.skipInference = skip
	return b
}

// Build creates the configured Loader
func (b *LoaderBuilder) Build() *Loader {
	// 1. Add default source (lowest priority)
	b.loader.AddSource(source.NewDefaultSource())

	// 2. Find and add YAML source
	configFile := source.FindConfigFile(b.configFile, b.appType)
	if configFile != "" {
		b.loader.AddSource(source.NewYAMLSource(configFile))
		corelog.Debugf("Using config file: %s", configFile)
	}

	// 3. Add .env source if enabled
	if b.enableDotEnv {
		dirs := source.FindDotEnvDirs(configFile)
		b.loader.AddSource(source.NewDotEnvSource(b.prefix, dirs, b.appEnv))
	}

	// 4. Add environment variable source (highest priority before CLI)
	b.loader.AddSource(source.NewEnvSource(b.prefix))

	// 5. Configure inference phase
	b.loader.SetSkipInference(b.skipInference)

	return b.loader
}

// Load is a convenience function that creates a loader and loads configuration
func Load(configFile, appType string) (*schema.Root, error) {
	loader := NewLoaderBuilder().
		WithConfigFile(configFile).
		WithAppType(appType).
		Build()

	return loader.Load()
}

// LoadServer loads server configuration
func LoadServer(configFile string) (*schema.Root, error) {
	return Load(configFile, "server")
}

// LoadClient loads client configuration
func LoadClient(configFile string) (*schema.Root, error) {
	return Load(configFile, "client")
}
