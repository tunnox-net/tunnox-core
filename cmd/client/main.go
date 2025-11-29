package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"tunnox-core/internal/client"
	"tunnox-core/internal/client/cli"
	"tunnox-core/internal/utils"
)

func main() {
	// 解析命令行参数
	configFile := flag.String("config", "", "path to config file (optional)")
	protocol := flag.String("p", "", "protocol: tcp/websocket/ws/udp/quic (overrides config)")
	serverAddr := flag.String("s", "", "server address (e.g., localhost:7001, overrides config)")
	clientID := flag.Int64("id", 0, "client ID (overrides config)")
	deviceID := flag.String("device", "", "device ID for anonymous mode (overrides config)")
	authToken := flag.String("token", "", "auth token (overrides config)")
	anonymous := flag.Bool("anonymous", false, "use anonymous mode (overrides config)")
	daemon := flag.Bool("daemon", false, "run in daemon mode (no interactive CLI)")
	interactive := flag.Bool("interactive", true, "run in interactive mode with CLI (default)")
	help := flag.Bool("h", false, "show help")

	flag.Parse()

	// 显示帮助
	if *help {
		showHelp()
		os.Exit(0)
	}

	// 决定运行模式
	runInteractive := *interactive && !*daemon

	// 加载配置
	config, err := loadOrCreateConfig(*configFile, *protocol, *serverAddr, *clientID, *deviceID, *authToken, *anonymous, runInteractive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 配置日志输出
	logFile, err := configureLogging(config, runInteractive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to configure logging: %v\n", err)
		os.Exit(1)
	}

	// 仅在守护进程模式下显示详细启动信息
	if !runInteractive {
		fmt.Printf("🚀 Tunnox Client Starting...\n")
		fmt.Printf("   Protocol: %s\n", config.Server.Protocol)
		fmt.Printf("   Server:   %s\n", config.Server.Address)
		if config.Anonymous {
			fmt.Printf("   Mode:     Anonymous (device: %s)\n", config.DeviceID)
		} else {
			fmt.Printf("   Mode:     Authenticated (client_id: %d)\n", config.ClientID)
		}
		if logFile != "" {
			fmt.Printf("   Logs:     %s\n", logFile)
		}
		fmt.Printf("\n")
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建客户端
	tunnoxClient := client.NewClient(ctx, config)

	// 根据运行模式决定连接策略
	if runInteractive {
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// 交互模式：可选连接，失败不退出
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

		// 尝试连接（如果有配置地址或需要自动连接）
		// 自动连接会在 Connect() 内部处理
		if err := tunnoxClient.Connect(); err != nil {
			// 连接失败，静默处理，用户可通过CLI命令重连
		}

		// 交互模式：尝试启动CLI
		tunnoxCLI, err := cli.NewCLI(ctx, tunnoxClient)
		if err != nil {
			// CLI初始化失败（通常是因为没有TTY），自动降级到daemon模式
			fmt.Fprintf(os.Stderr, "\n⚠️  CLI initialization failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "🔄 Auto-switching to daemon mode...\n")

			// 验证必须配置
			if config.Server.Address == "" {
				fmt.Fprintf(os.Stderr, "❌ Error: server address is required\n")
				fmt.Fprintf(os.Stderr, "💡 Please configure server address in config file or use -s flag\n")
				os.Exit(1)
			}

			// 如果还未连接，尝试连接
			if !tunnoxClient.IsConnected() {
				if err := connectWithRetry(tunnoxClient, 5); err != nil {
					fmt.Fprintf(os.Stderr, "❌ Failed to connect to server after retries: %v\n", err)
					os.Exit(1)
				}
				fmt.Println("✅ Connected to server successfully!")
			}

			fmt.Println("   Press Ctrl+C to stop")
			fmt.Println()

			// 启动自动重连监控
			go monitorConnectionAndReconnect(ctx, tunnoxClient)

			// 等待信号（daemon模式）
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			select {
			case sig := <-sigChan:
				utils.Infof("Client: received signal %v, shutting down...", sig)
			case <-ctx.Done():
				utils.Infof("Client: context cancelled, shutting down...")
			}
		} else {
			// CLI初始化成功，正常启动交互模式
			// 启动自动重连监控（交互模式也需要自动重连）
			go monitorConnectionAndReconnect(ctx, tunnoxClient)

			// 在goroutine中处理信号
			go func() {
				sigChan := make(chan os.Signal, 1)
				signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
				select {
				case sig := <-sigChan:
					utils.Infof("Client: received signal %v, shutting down...", sig)
					cancel()
					tunnoxCLI.Stop()
				case <-ctx.Done():
					tunnoxCLI.Stop()
				}
			}()

			// 启动CLI（阻塞）
			tunnoxCLI.Start()
		}

	} else {
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// 守护进程模式：必须连接成功，支持自动重连
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

		fmt.Println("🔄 Running in daemon mode...")

		// 验证必须配置
		if config.Server.Address == "" {
			fmt.Fprintf(os.Stderr, "❌ Error: server address is required in daemon mode\n")
			os.Exit(1)
		}

		// 连接到服务器（带重试）
		if err := connectWithRetry(tunnoxClient, 5); err != nil {
			fmt.Fprintf(os.Stderr, "❌ Failed to connect to server after retries: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ Connected to server successfully!")
		fmt.Println("   Press Ctrl+C to stop")
		fmt.Println()

		// 启动自动重连监控
		go monitorConnectionAndReconnect(ctx, tunnoxClient)

		// 等待信号
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		select {
		case sig := <-sigChan:
			utils.Infof("Client: received signal %v, shutting down...", sig)
		case <-ctx.Done():
			utils.Infof("Client: context cancelled, shutting down...")
		}
	}

	// 停止客户端
	fmt.Println("\n🛑 Shutting down client...")
	tunnoxClient.Stop()
	utils.Infof("Client: shutdown complete")
}

// loadOrCreateConfig 加载或创建配置
func loadOrCreateConfig(configFile, protocol, serverAddr string, clientID int64, deviceID, authToken string, anonymous bool, isCLIMode bool) (*client.ClientConfig, error) {
	// 使用配置管理器加载配置
	configManager := client.NewConfigManager()
	config, err := configManager.LoadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// 命令行参数覆盖配置文件
	if protocol != "" {
		config.Server.Protocol = normalizeProtocol(protocol)
	}
	if serverAddr != "" {
		config.Server.Address = serverAddr
	}
	if clientID > 0 {
		config.ClientID = clientID
		config.Anonymous = false
	}
	if deviceID != "" {
		config.DeviceID = deviceID
	}
	if authToken != "" {
		config.AuthToken = authToken
		config.Anonymous = false
	}
	if anonymous {
		config.Anonymous = true
	}

	// 检测是否需要自动连接（符合设计文档的三个条件）：
	// 1. 配置文件中没有指定服务器地址
	// 2. 命令行参数中没有指定服务器地址（-s 参数）
	// 3. 以cli的方式启动（runInteractive == true）
	needsAutoConnect := isCLIMode && config.Server.Address == "" && serverAddr == ""

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
		validProtocols := []string{"tcp", "websocket", "udp", "quic"}
		valid := false
		for _, p := range validProtocols {
			if config.Server.Protocol == p {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid protocol: %s (must be one of: tcp, websocket, udp, quic)", config.Server.Protocol)
		}
	}
	// 如果协议为空且地址也为空，说明是自动连接模式，协议会在自动连接时确定

	// 验证认证配置
	if !config.Anonymous {
		if config.ClientID == 0 {
			return fmt.Errorf("client_id is required for authenticated mode")
		}
	} else {
		if config.DeviceID == "" {
			config.DeviceID = "anonymous-device"
		}
	}

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
      -p <protocol>      Protocol: tcp/websocket/ws/udp/quic
      -s <address>       Server address (e.g., localhost:7001)
      -id <client_id>    Client ID for authenticated mode
      -token <token>     Auth token for authenticated mode
      -device <id>       Device ID for anonymous mode
      -anonymous         Use anonymous mode

    Mode:
      -interactive       Run in interactive mode with CLI (default)
      -daemon            Run in daemon mode (no CLI, for background service)

    Help:
      -h                 Show this help

EXAMPLES:
    # Interactive mode (default) - with CLI
    tunnox-client -p quic -s localhost:7003 -anonymous

    # Daemon mode - no CLI, runs in background
    tunnox-client -p quic -s localhost:7003 -anonymous -daemon

    # Use config file
    tunnox-client -config client-config.yaml

    # Quick start with QUIC (recommended)
    tunnox-client -p quic -s localhost:7003 -anonymous

    # Authenticated mode
    tunnox-client -p quic -s localhost:7003 -id 10000001 -token "your-jwt-token"

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
    - Anonymous mode is used if no client_id/token is provided`)
}

// configureLogging 配置日志输出
//
// 返回：日志文件路径（如果输出到文件）和可能的错误
// 注意：日志默认只输出到文件，不输出到console
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

	// 日志总是输出到文件，不输出到console
	// 如果有配置文件地址就使用，否则使用默认路径
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

// connectWithRetry 带重试的连接
func connectWithRetry(tunnoxClient *client.TunnoxClient, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			fmt.Printf("🔄 Retry %d/%d...\n", i, maxRetries)
			time.Sleep(time.Duration(i) * 2 * time.Second) // 指数退避
		}

		if err := tunnoxClient.Connect(); err != nil {
			if i == maxRetries-1 {
				return err
			}
			fmt.Printf("⚠️  Connection failed: %v\n", err)
			continue
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded")
}

// monitorConnectionAndReconnect 监控连接状态并自动重连
// 注意：此函数仅作为备用重连机制，主要重连由 readLoop 退出时触发
// 如果 readLoop 的重连机制正常工作，此函数通常不会触发
func monitorConnectionAndReconnect(ctx context.Context, tunnoxClient *client.TunnoxClient) {
	ticker := time.NewTicker(30 * time.Second) // ✅ 增加检查间隔，避免与 readLoop 重连冲突
	defer ticker.Stop()

	consecutiveFailures := 0
	maxFailures := 3

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 检查连接状态
			// ✅ 仅在连接断开且持续一段时间后才触发重连（给 readLoop 的重连机制时间）
			if !tunnoxClient.IsConnected() {
				consecutiveFailures++
				utils.Warnf("Connection lost (failure %d/%d), attempting to reconnect via monitor...",
					consecutiveFailures, maxFailures)

				// ✅ 使用 Reconnect() 方法，它内部已经有防重复重连机制
				if err := tunnoxClient.Reconnect(); err != nil {
					utils.Errorf("Reconnection failed: %v", err)

					if consecutiveFailures >= maxFailures {
						utils.Errorf("Max reconnection attempts reached, giving up")
						return
					}
				} else {
					utils.Infof("Reconnected successfully via monitor")
					consecutiveFailures = 0
				}
			} else {
				// 连接正常，重置失败计数
				if consecutiveFailures > 0 {
					consecutiveFailures = 0
				}
			}
		}
	}
}
