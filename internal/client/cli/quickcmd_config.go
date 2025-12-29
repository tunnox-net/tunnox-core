// Package cli 提供 Tunnox 客户端的配置管理命令
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 配置命令 (tunnox config)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// runConfigCommand 执行 tunnox config <subcommand> 命令
func (r *QuickCommandRunner) runConfigCommand(args []string) (bool, error) {
	if len(args) == 0 {
		r.showConfigHelp()
		return false, nil
	}

	subCmd := strings.ToLower(args[0])
	subArgs := args[1:]

	switch subCmd {
	case "init":
		return r.runConfigInitCommand(subArgs)
	case "show":
		return r.runConfigShowCommand(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown config subcommand: %s\n", subCmd)
		r.showConfigHelp()
		return false, nil
	}
}

// runConfigInitCommand 执行 tunnox config init 命令
func (r *QuickCommandRunner) runConfigInitCommand(args []string) (bool, error) {
	// 确定配置文件路径
	configPath := "config.yaml"
	if len(args) > 0 {
		configPath = args[0]
	}

	// 检查文件是否已存在
	if _, err := os.Stat(configPath); err == nil {
		r.output.Warning("Configuration file already exists: %s", configPath)

		options := []string{"Overwrite", "Cancel"}
		choice, err := PromptSelect("What would you like to do?", options)
		if err != nil || choice != 0 {
			r.output.Info("Operation cancelled")
			return false, nil
		}
	}

	// 创建配置内容
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

server:
  address: https://gw.tunnox.net/_tunnox
  protocol: websocket
log:
  level: info
  format: text
  output: file
  file: tunnox-client.log
`

	// 确保目录存在
	dir := filepath.Dir(configPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			r.output.Error("Failed to create directory: %v", err)
			return false, nil
		}
	}

	// 写入文件
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		r.output.Error("Failed to write config file: %v", err)
		return false, nil
	}

	r.output.Success("Configuration file created: %s", configPath)
	fmt.Println()
	r.output.Info("Edit the file to customize your settings, then run:")
	r.output.Plain("  tunnox start -c %s", configPath)
	fmt.Println()

	return false, nil
}

// runConfigShowCommand 执行 tunnox config show 命令
func (r *QuickCommandRunner) runConfigShowCommand(args []string) (bool, error) {
	r.output.Header("Current Configuration")

	// 显示配置
	r.output.Section("Server")
	serverAddr := r.config.Server.Address
	if serverAddr == "" {
		serverAddr = "(default: https://gw.tunnox.net/_tunnox)"
	}
	protocol := r.config.Server.Protocol
	if protocol == "" {
		protocol = "(default: websocket)"
	}
	r.output.KeyValue("address", serverAddr)
	r.output.KeyValue("protocol", protocol)

	r.output.Section("Client")
	clientID := "N/A (will be assigned on first connection)"
	if r.config.ClientID > 0 {
		clientID = fmt.Sprintf("%d", r.config.ClientID)
	}
	secretKey := "***"
	if r.config.SecretKey == "" {
		secretKey = "N/A (will be assigned on first connection)"
	}
	r.output.KeyValue("client_id", clientID)
	r.output.KeyValue("secret_key", secretKey)

	r.output.Section("Log")
	logLevel := r.config.Log.Level
	if logLevel == "" {
		logLevel = "info"
	}
	logFormat := r.config.Log.Format
	if logFormat == "" {
		logFormat = "text"
	}
	logOutput := r.config.Log.Output
	if logOutput == "" {
		logOutput = "file"
	}
	logFile := r.config.Log.File
	if logFile == "" {
		logFile = "(default location)"
	}
	r.output.KeyValue("level", logLevel)
	r.output.KeyValue("format", logFormat)
	r.output.KeyValue("output", logOutput)
	r.output.KeyValue("file", logFile)

	fmt.Println()

	return false, nil
}

// showConfigHelp 显示配置命令帮助
func (r *QuickCommandRunner) showConfigHelp() {
	fmt.Println(`Usage: tunnox config <command>

Commands:
  init [path]   Generate a configuration file template
  show          Show current configuration

Examples:
  tunnox config init                    # Create config.yaml in current directory
  tunnox config init ~/.tunnox/config.yaml
  tunnox config show                    # Show current configuration`)
}
