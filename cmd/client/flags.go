package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tunnox-core/internal/client"
	"tunnox-core/internal/utils"
)

// loadOrCreateConfig 加载或创建配置
func loadOrCreateConfig(configFile, protocol, serverAddr string, clientID int64, secretKey string, isCLIMode bool) (*client.ClientConfig, error) {
	// 使用配置管理器加载配置
	configManager := client.NewConfigManager()
	config, err := configManager.LoadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// 保存配置文件中的原始值（在命令行参数覆盖之前）
	configFileHasAddress := config.Server.Address != ""
	configFileHasProtocol := config.Server.Protocol != ""

	// 命令行参数覆盖配置文件
	if protocol != "" {
		config.Server.Protocol = normalizeProtocol(protocol)
	}
	if serverAddr != "" {
		config.Server.Address = serverAddr
	}
	if clientID > 0 {
		config.ClientID = clientID
	}
	if secretKey != "" {
		config.SecretKey = secretKey
	}

	// 检测是否需要自动连接（符合设计文档的条件）：
	// 1. 以cli的方式启动（runInteractive == true）
	// 2. 配置文件中没有指定服务器地址（检查原始值，而不是被命令行覆盖后的值）
	// 3. 配置文件中没有指定协议（检查原始值，而不是被命令行覆盖后的值）
	// 4. 命令行参数中没有指定服务器地址（-s 参数）
	// 5. 命令行参数中没有指定协议（-p 参数）
	// 注意：如果配置文件中指定了地址或协议，或者命令行中指定了地址或协议，都不能启用自动连接
	needsAutoConnect := isCLIMode &&
		!configFileHasAddress &&
		!configFileHasProtocol &&
		serverAddr == "" &&
		protocol == ""

	// 验证配置（如果不需要自动连接，则设置默认值）
	if err := validateConfig(config, !needsAutoConnect); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// validateConfig 验证配置
func validateConfig(config *client.ClientConfig, setDefaults bool) error {
	// 如果需要设置默认值（非自动连接模式）
	if setDefaults {
		// 如果地址为空，使用默认 WebSocket 地址
		if config.Server.Address == "" {
			config.Server.Address = "https://gw.tunnox.net/_tunnox"
			config.Server.Protocol = "websocket"
		}

		// 如果协议为空，使用默认 WebSocket 协议
		if config.Server.Protocol == "" {
			config.Server.Protocol = "websocket"
		}
	}

	// 规范化协议名称（如果有协议）
	if config.Server.Protocol != "" {
		config.Server.Protocol = normalizeProtocol(config.Server.Protocol)

		// 验证协议
		validProtocols := []string{"tcp", "websocket", "kcp", "quic"}
		valid := false
		for _, p := range validProtocols {
			if config.Server.Protocol == p {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid protocol: %s (must be one of: tcp, websocket, kcp, quic)", config.Server.Protocol)
		}
	}
	// 如果协议为空且地址也为空，说明是自动连接模式，协议会在自动连接时确定

	// ClientID 和 SecretKey 可以为空，首次连接时由服务端分配
	return nil
}

// normalizeProtocol 规范化协议名称
func normalizeProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	// 支持简写
	if protocol == "ws" {
		return "websocket"
	}
	return protocol
}

// showHelp 显示帮助信息
func showHelp() {
	fmt.Println(`Tunnox Client - Port Mapping Client

USAGE:
    tunnox-client [OPTIONS]

OPTIONS:
    Connection:
      -config <file>     Path to config file (optional)
      -p <protocol>      Protocol: tcp/websocket/ws/kcp/quic
      -s <address>       Server address (e.g., localhost:7001)
      -id <client_id>    Client ID (auto-assigned on first connect)
      -key <secret>      Secret key (auto-assigned on first connect)
      -log <file>        Log file path (overrides config file)

    Mode:
      -interactive       Run in interactive mode with CLI (default)
      -daemon            Run in daemon mode (no CLI, for background service)

    Help:
      -h                 Show this help

EXAMPLES:
    # Interactive mode (default) - with CLI
    tunnox-client -p quic -s localhost:7003

    # Daemon mode - no CLI, runs in background
    tunnox-client -p quic -s localhost:7003 -daemon

    # Use config file
    tunnox-client -config client-config.yaml

    # Quick start with QUIC (recommended)
    tunnox-client -p quic -s localhost:7003

INTERACTIVE MODE:
    In interactive mode, you can use commands like:
      - generate-code     Generate a connection code (TargetClient)
      - use-code <code>   Use a connection code (ListenClient)
      - list-mappings     List all tunnel mappings
      - help              Show all available commands
      - exit              Quit the client

DAEMON MODE:
    Use -daemon flag for:
      - Running as a system service
      - Background processes
      - Automated deployments

NOTES:
    - Command line options override config file settings
    - Default mode is interactive (with CLI)
    - Default server: https://gw.tunnox.net/_tunnox (WebSocket)
    - Default protocol is websocket if not specified
    - ClientID and SecretKey are auto-assigned on first connection`)
}

// configureLogging 配置日志输出
//
// 返回：日志文件路径（如果输出到文件）和可能的错误
// CLI模式：只写文件，不输出到控制台（避免干扰用户）
// Daemon模式：同时写文件和输出到控制台
func configureLogging(config *client.ClientConfig, interactive bool) (string, error) {
	logConfig := &client.LogConfig{
		Level:  "info",
		Format: "text",
	}

	// 从配置文件读取日志配置（如果有）
	if config.Log.Level != "" {
		logConfig.Level = config.Log.Level
	}
	if config.Log.Format != "" {
		logConfig.Format = config.Log.Format
	}

	// 根据运行模式设置日志输出
	if interactive {
		// CLI模式：只写文件，不输出到控制台
		logConfig.Output = "file"
	} else {
		// Daemon模式：只写文件，避免 stderr 输出干扰进程管理
		// 进程状态应该通过日志文件或健康检查端点来监控
		logConfig.Output = "file"
	}

	// 确定日志文件路径
	logFile := config.Log.File
	if logFile == "" {
		// 使用默认路径列表（按优先级）
		candidates := utils.GetDefaultClientLogPath(interactive)
		var err error
		logFile, err = utils.ResolveLogPath(candidates)
		if err != nil {
			return "", fmt.Errorf("failed to resolve log path: %w", err)
		}
	} else {
		// 展开路径（支持 ~ 和相对路径）
		expandedPath, err := utils.ExpandPath(logFile)
		if err != nil {
			return "", fmt.Errorf("failed to expand log file path %q: %w", logFile, err)
		}
		logFile = expandedPath

		// 确保日志目录存在
		logDir := filepath.Dir(logFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create log directory %q: %w", logDir, err)
		}
	}

	logConfig.File = logFile

	// 初始化日志
	if err := utils.InitLogger((*utils.LogConfig)(logConfig)); err != nil {
		return "", err
	}

	// 返回日志文件路径（总是输出到文件）
	if logConfig.File != "" {
		return logConfig.File, nil
	}
	return "", nil
}
