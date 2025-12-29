package schema

// ManagementConfig contains management API configuration
type ManagementConfig struct {
	Enabled bool                  `yaml:"enabled" json:"enabled"`
	Listen  string                `yaml:"listen" json:"listen"`
	Auth    ManagementAuthConfig  `yaml:"auth" json:"auth"`
	PProf   ManagementPProfConfig `yaml:"pprof" json:"pprof"`
	CORS    CORSConfig            `yaml:"cors" json:"cors"`
}

// ManagementAuthConfig contains management API auth settings
type ManagementAuthConfig struct {
	Type     string `yaml:"type" json:"type"` // none/bearer/basic
	Token    Secret `yaml:"token" json:"token"`
	Username string `yaml:"username" json:"username"`
	Password Secret `yaml:"password" json:"password"`
}

// ManagementPProfConfig contains pprof settings
type ManagementPProfConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	DataDir string `yaml:"data_dir" json:"data_dir"`
}

// Management auth type constants
const (
	AuthTypeNone   = "none"
	AuthTypeBearer = "bearer"
	AuthTypeBasic  = "basic"
)
