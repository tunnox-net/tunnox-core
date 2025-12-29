package schema

// LogConfig contains logging configuration
type LogConfig struct {
	Level    string            `yaml:"level" json:"level"`     // debug/info/warn/error
	Format   string            `yaml:"format" json:"format"`   // text/json
	File     string            `yaml:"file" json:"file"`       // log file path
	Console  bool              `yaml:"console" json:"console"` // also output to console
	Rotation LogRotationConfig `yaml:"rotation" json:"rotation"`
}

// LogRotationConfig contains log rotation settings
type LogRotationConfig struct {
	Enabled    bool `yaml:"enabled" json:"enabled"`
	MaxSize    int  `yaml:"max_size" json:"max_size"`       // max size in MB
	MaxBackups int  `yaml:"max_backups" json:"max_backups"` // max number of backups
	MaxAge     int  `yaml:"max_age" json:"max_age"`         // max age in days
	Compress   bool `yaml:"compress" json:"compress"`       // compress old logs
}
