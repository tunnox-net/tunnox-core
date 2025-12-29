package schema

import "time"

// ClientConfig contains client-side configuration
type ClientConfig struct {
	ClientID  int64  `yaml:"client_id" json:"client_id"`
	AuthToken Secret `yaml:"auth_token" json:"auth_token"`
	Anonymous bool   `yaml:"anonymous" json:"anonymous"`
	DeviceID  string `yaml:"device_id" json:"device_id"`
	SecretKey Secret `yaml:"secret_key" json:"secret_key"`

	Server ClientServerConfig `yaml:"server" json:"server"`
	Log    ClientLogConfig    `yaml:"log" json:"log"`
	Stream ClientStreamConfig `yaml:"stream" json:"stream"`
}

// ClientServerConfig contains client's server connection settings
type ClientServerConfig struct {
	Address           string        `yaml:"address" json:"address"`
	Protocol          string        `yaml:"protocol" json:"protocol"` // tcp/websocket/kcp/quic/auto
	AutoReconnect     bool          `yaml:"auto_reconnect" json:"auto_reconnect"`
	ReconnectInterval time.Duration `yaml:"reconnect_interval" json:"reconnect_interval"`
	ConnectTimeout    time.Duration `yaml:"connect_timeout" json:"connect_timeout"`
}

// ClientLogConfig contains client's log settings
type ClientLogConfig struct {
	Level  string `yaml:"level" json:"level"`   // debug/info/warn/error
	Format string `yaml:"format" json:"format"` // text/json
	File   string `yaml:"file" json:"file"`
}

// ClientStreamConfig contains client's stream settings
type ClientStreamConfig struct {
	EnableCompression bool `yaml:"enable_compression" json:"enable_compression"`
	CompressionLevel  int  `yaml:"compression_level" json:"compression_level"`
	BufferSize        int  `yaml:"buffer_size" json:"buffer_size"`
}

// Protocol constants
const (
	ProtocolTCP       = "tcp"
	ProtocolWebSocket = "websocket"
	ProtocolKCP       = "kcp"
	ProtocolQUIC      = "quic"
	ProtocolAuto      = "auto"
)

// Log level constants
const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelWarn  = "warn"
	LogLevelError = "error"
)

// Log format constants
const (
	LogFormatText = "text"
	LogFormatJSON = "json"
)
