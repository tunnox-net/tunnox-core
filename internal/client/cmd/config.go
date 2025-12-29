package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"tunnox-core/internal/client/cli"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// configCmd 配置管理命令组
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long: `Manage Tunnox client configuration.

Commands:
  init      Generate a configuration file template
  show      Show current configuration
  edit      Edit configuration interactively`,
}

// configInitCmd 生成配置文件模板
var configInitCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Generate a configuration file template",
	Long: `Generate a configuration file template with default values.

Example:
  tunnox config init                    # Create config.yaml in current directory
  tunnox config init ~/.tunnox/config.yaml`,
	Args: cobra.MaximumNArgs(1),
	Run:  runConfigInit,
}

// configShowCmd 显示当前配置
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long: `Show the current configuration settings.

Example:
  tunnox config show`,
	Run: runConfigShow,
}

func init() {
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
}

// defaultConfig 默认配置模板
type defaultConfig struct {
	Server struct {
		Address  string `yaml:"address"`
		Protocol string `yaml:"protocol"`
	} `yaml:"server"`
	Log struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
		Output string `yaml:"output"`
		File   string `yaml:"file,omitempty"`
	} `yaml:"log"`
}

func runConfigInit(cmd *cobra.Command, args []string) {
	output := cli.NewOutput(false)

	// 确定配置文件路径
	configPath := "config.yaml"
	if len(args) > 0 {
		configPath = args[0]
	}

	// 检查文件是否已存在
	if _, err := os.Stat(configPath); err == nil {
		output.Warning("Configuration file already exists: %s", configPath)

		options := []string{"Overwrite", "Cancel"}
		choice, err := cli.PromptSelect("What would you like to do?", options)
		if err != nil || choice != 0 {
			output.Info("Operation cancelled")
			return
		}
	}

	// 创建默认配置
	config := defaultConfig{}
	config.Server.Address = "https://gw.tunnox.net/_tunnox"
	config.Server.Protocol = "websocket"
	config.Log.Level = "info"
	config.Log.Format = "text"
	config.Log.Output = "file"
	config.Log.File = "tunnox-client.log"

	// 生成 YAML
	data, err := yaml.Marshal(&config)
	if err != nil {
		output.Error("Failed to generate config: %v", err)
		os.Exit(1)
	}

	// 添加注释
	configContent := `# Tunnox Client Configuration
#
# Server settings
# - address: Server address (can include protocol prefix like https://)
# - protocol: Transport protocol (tcp/websocket/kcp/quic)
#
# Client settings (auto-assigned on first connection)
# - client_id: Client ID (server-assigned)
# - secret_key: Authentication key (server-assigned)
#
# Log settings
# - level: Log level (debug/info/warn/error)
# - format: Log format (text/json)
# - output: Output destination (stdout/file/both)
# - file: Log file path (when output includes file)

` + string(data)

	// 确保目录存在
	dir := filepath.Dir(configPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			output.Error("Failed to create directory: %v", err)
			os.Exit(1)
		}
	}

	// 写入文件
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		output.Error("Failed to write config file: %v", err)
		os.Exit(1)
	}

	output.Success("Configuration file created: %s", configPath)
	fmt.Println()
	output.Info("Edit the file to customize your settings, then run:")
	output.Plain("  tunnox start -c %s", configPath)
	fmt.Println()
}

func runConfigShow(cmd *cobra.Command, args []string) {
	output := cli.NewOutput(false)
	output.Header("Current Configuration")

	// 加载配置
	config, err := loadConfig()
	if err != nil {
		output.Error("Failed to load config: %v", err)
		os.Exit(1)
	}

	// 显示配置
	output.Section("Server")
	serverAddr := config.Server.Address
	if serverAddr == "" {
		serverAddr = "(default: https://gw.tunnox.net/_tunnox)"
	}
	protocol := config.Server.Protocol
	if protocol == "" {
		protocol = "(default: websocket)"
	}
	output.KeyValue("address", serverAddr)
	output.KeyValue("protocol", protocol)

	output.Section("Client")
	clientID := "N/A (will be assigned on first connection)"
	if config.ClientID > 0 {
		clientID = fmt.Sprintf("%d", config.ClientID)
	}
	secretKey := "***"
	if config.SecretKey == "" {
		secretKey = "N/A (will be assigned on first connection)"
	}
	output.KeyValue("client_id", clientID)
	output.KeyValue("secret_key", secretKey)

	output.Section("Log")
	logLevel := config.Log.Level
	if logLevel == "" {
		logLevel = "info"
	}
	logFormat := config.Log.Format
	if logFormat == "" {
		logFormat = "text"
	}
	logOutput := config.Log.Output
	if logOutput == "" {
		logOutput = "file"
	}
	logFile := config.Log.File
	if logFile == "" {
		logFile = "(default location)"
	}
	output.KeyValue("level", logLevel)
	output.KeyValue("format", logFormat)
	output.KeyValue("output", logOutput)
	output.KeyValue("file", logFile)

	fmt.Println()
}
