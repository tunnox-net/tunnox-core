package schema

import "time"

// ProtocolsConfig contains all protocol configurations
type ProtocolsConfig struct {
	TCP       TCPConfig       `yaml:"tcp" json:"tcp"`
	WebSocket WebSocketConfig `yaml:"websocket" json:"websocket"`
	KCP       KCPConfig       `yaml:"kcp" json:"kcp"`
	QUIC      QUICConfig      `yaml:"quic" json:"quic"`
}

// TCPConfig contains TCP protocol settings
type TCPConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Port    int    `yaml:"port" json:"port"`
	Host    string `yaml:"host" json:"host"`
}

// WebSocketConfig contains WebSocket protocol settings
type WebSocketConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// WebSocket uses HTTP service, no separate port needed
}

// KCPConfig contains KCP protocol settings
type KCPConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	Port    int    `yaml:"port" json:"port"`
	Host    string `yaml:"host" json:"host"`
	Mode    string `yaml:"mode" json:"mode"` // normal/fast/fast2/fast3
	SndWnd  int    `yaml:"snd_wnd" json:"snd_wnd"`
	RcvWnd  int    `yaml:"rcv_wnd" json:"rcv_wnd"`
	MTU     int    `yaml:"mtu" json:"mtu"`
}

// QUICConfig contains QUIC protocol settings
type QUICConfig struct {
	Enabled     bool          `yaml:"enabled" json:"enabled"`
	Port        int           `yaml:"port" json:"port"`
	Host        string        `yaml:"host" json:"host"`
	MaxStreams  int           `yaml:"max_streams" json:"max_streams"`
	IdleTimeout time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
}

// KCPMode constants
const (
	KCPModeNormal = "normal"
	KCPModeFast   = "fast"
	KCPModeFast2  = "fast2"
	KCPModeFast3  = "fast3"
)
