package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"tunnox-core/internal/client"
	"tunnox-core/internal/client/cli"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils"
)

func main() {
	// 🔥 全局 panic recovery - 捕获并记录所有未处理的 panic
	defer func() {
		if r := recover(); r != nil {
			// 尝试记录到日志（如果日志已初始化）
			corelog.Errorf("FATAL: main goroutine panic recovered: %v", r)
			corelog.Errorf("Stack trace:\n%s", string(debug.Stack()))

			// 同时输出到 stderr 以确保可见
			fmt.Fprintf(os.Stderr, "\n❌ PANIC: %v\n", r)
			fmt.Fprintf(os.Stderr, "Stack trace:\n%s\n", string(debug.Stack()))
			os.Exit(2)
		}
	}()

	// 解析命令行参数
	configFile := flag.String("config", "", "path to config file (optional)")
	protocol := flag.String("p", "", "protocol: tcp/websocket/ws/kcp/quic (overrides config)")
	serverAddr := flag.String("s", "", "server address (e.g., localhost:7001, overrides config)")
	clientID := flag.Int64("id", 0, "client ID (overrides config)")
	deviceID := flag.String("device", "", "device ID for anonymous mode (overrides config)")
	authToken := flag.String("token", "", "auth token (overrides config)")
	anonymous := flag.Bool("anonymous", false, "use anonymous mode (overrides config)")
	logFile := flag.String("log", "", "log file path (overrides config file)")
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

	// 配置日志输出（如果指定了日志文件路径，覆盖配置）
	if *logFile != "" {
		expandedPath, err := utils.ExpandPath(*logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to expand log file path %q: %v\n", *logFile, err)
			os.Exit(1)
		}
		config.Log.File = expandedPath
		// 确保日志目录存在
		logDir := filepath.Dir(expandedPath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log directory %q: %v\n", logDir, err)
			os.Exit(1)
		}
	}

	logFilePath, err := configureLogging(config, runInteractive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to configure logging: %v\n", err)
		os.Exit(1)
	}

	// 仅在守护进程模式下显示详细启动信息
	if !runInteractive {
		fmt.Printf("🚀 Tunnox Client Starting...\n")
		fmt.Printf("   Protocol: %s\n", config.Server.Protocol)
		// 智能显示服务器地址（避免重复协议前缀）
		serverDisplay := config.Server.Address
		if config.Server.Protocol != "" && !strings.Contains(serverDisplay, "://") {
			// 只有当地址不包含协议时才添加
			serverDisplay = fmt.Sprintf("%s://%s", config.Server.Protocol, serverDisplay)
		}
		fmt.Printf("   Server:   %s\n", serverDisplay)
		if config.Anonymous {
			fmt.Printf("   Mode:     Anonymous (device: %s)\n", config.DeviceID)
		} else {
			fmt.Printf("   Mode:     Authenticated (client_id: %d)\n", config.ClientID)
		}
		if logFilePath != "" {
			fmt.Printf("   Logs:     %s\n", logFilePath)
		}
		fmt.Printf("\n")
	}

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 在连接之前就设置信号处理，使 Ctrl+C 能够中断连接过程
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			// 用户按下Ctrl+C，取消连接
			cancel()
		case <-ctx.Done():
		}
	}()

	// 创建客户端（传递命令行参数信息）
	serverAddressFromCLI := *serverAddr != ""
	serverProtocolFromCLI := *protocol != ""
	tunnoxClient := client.NewClientWithCLIFlags(ctx, config, serverAddressFromCLI, serverProtocolFromCLI)

	// 根据运行模式决定连接策略
	if runInteractive {
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
		// 交互模式：必须连接成功才能进入CLI
		// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

		// 尝试连接
		needsAutoConnect := config.Server.Address == "" && config.Server.Protocol == ""
		if needsAutoConnect {
			// 自动连接模式
			fmt.Fprintf(os.Stderr, "\n🔍 Connecting to Tunnox service...\n")
		} else {
			// 指定服务器连接 - 智能显示地址
			serverDisplay := config.Server.Address
			if strings.Contains(serverDisplay, "://") {
				// 地址已包含协议，直接显示
				fmt.Fprintf(os.Stderr, "\n🔗 Connecting to %s...\n", serverDisplay)
			} else {
				// 地址不包含协议，添加协议前缀
				fmt.Fprintf(os.Stderr, "\n🔗 Connecting to %s://%s...\n", config.Server.Protocol, serverDisplay)
			}
		}

		if err := tunnoxClient.Connect(); err != nil {
			// 检查是否是因为 context 取消导致的错误
			if ctx.Err() == context.Canceled {
				fmt.Fprintf(os.Stderr, "\n⚠️  Connection cancelled\n")
				os.Exit(0)
			}
			// 连接失败，CLI模式下直接退出
			fmt.Fprintf(os.Stderr, "\n❌ Connection failed\n")
			fmt.Fprintf(os.Stderr, "💡 Please check your network or specify server with -s flag\n")
			os.Exit(1)
		}

		// 连接成功，启动CLI
		fmt.Fprintf(os.Stderr, "✅ Connected successfully\n\n")

		// 启动CLI
		corelog.Infof("Client: initializing CLI...")
		tunnoxCLI, err := cli.NewCLI(ctx, tunnoxClient)
		if err != nil {
			corelog.Errorf("Client: CLI initialization failed: %v", err)
			fmt.Fprintf(os.Stderr, "❌ Failed to initialize CLI: %v\n", err)
			os.Exit(1)
		}

		// 启动自动重连监控（交互模式也需要自动重连）
		go monitorConnectionAndReconnect(ctx, tunnoxClient)

		// 在goroutine中处理信号
		go func() {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			select {
			case sig := <-sigChan:
				corelog.Infof("Client: received signal %v, shutting down...", sig)
				cancel()
				tunnoxCLI.Stop()
			case <-ctx.Done():
				tunnoxCLI.Stop()
			}
		}()

		// 启动CLI（阻塞）
		corelog.Infof("Client: calling CLI.Start()...")
		tunnoxCLI.Start()
		corelog.Infof("Client: CLI.Start() returned")

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
			// 检查是否是因为 context 取消导致的错误
			if ctx.Err() == context.Canceled {
				fmt.Fprintf(os.Stderr, "\n⚠️  Connection cancelled by user\n")
				os.Exit(0)
			}
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
			corelog.Infof("Client: received signal %v, shutting down...", sig)
		case <-ctx.Done():
			corelog.Infof("Client: context cancelled, shutting down...")
		}
	}

	// 停止客户端
	fmt.Println("\n🛑 Shutting down client...")
	tunnoxClient.Stop()
	corelog.Infof("Client: shutdown complete")
}

// loadOrCreateConfig 加载或创建配置
func loadOrCreateConfig(configFile, protocol, serverAddr string, clientID int64, deviceID, authToken string, anonymous bool, isCLIMode bool) (*client.ClientConfig, error) {
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
      -p <protocol>      Protocol: tcp/websocket/ws/kcp/quic
      -s <address>       Server address (e.g., localhost:7001)
      -id <client_id>    Client ID for authenticated mode
      -token <token>     Auth token for authenticated mode
      -device <id>       Device ID for anonymous mode
      -anonymous         Use anonymous mode
      -log <file>        Log file path (overrides config file)

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
		// Daemon模式：同时写文件和输出到控制台
		logConfig.Output = "both"
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
				corelog.Warnf("Connection lost (failure %d/%d), attempting to reconnect via monitor...",
					consecutiveFailures, maxFailures)

				// ✅ 使用 Reconnect() 方法，它内部已经有防重复重连机制
				if err := tunnoxClient.Reconnect(); err != nil {
					corelog.Errorf("Reconnection failed: %v", err)

					if consecutiveFailures >= maxFailures {
						corelog.Errorf("Max reconnection attempts reached, giving up")
						return
					}
				} else {
					corelog.Infof("Reconnected successfully via monitor")
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
