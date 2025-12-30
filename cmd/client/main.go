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

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 快捷命令处理 (tunnox http/tcp/udp/socks/code)
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	if len(os.Args) > 1 && isQuickCommand(os.Args[1]) {
		runQuickCommand(os.Args[1:])
		return
	}

	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
	// 传统命令行参数处理
	// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

	// 解析命令行参数
	configFile := flag.String("config", "", "path to config file (optional)")
	protocol := flag.String("p", "", "protocol: tcp/websocket/ws/kcp/quic (overrides config)")
	serverAddr := flag.String("s", "", "server address (e.g., localhost:7001, overrides config)")
	clientID := flag.Int64("id", 0, "client ID (overrides config, auto-assigned on first connect)")
	secretKey := flag.String("key", "", "secret key (overrides config, auto-assigned on first connect)")
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
	config, err := loadOrCreateConfig(*configFile, *protocol, *serverAddr, *clientID, *secretKey, runInteractive)
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
		fmt.Printf("   ClientID: %d\n", config.ClientID)
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

	// 创建客户端（传递命令行参数信息和配置文件路径，用于保存凭据）
	serverAddressFromCLI := *serverAddr != ""
	serverProtocolFromCLI := *protocol != ""
	tunnoxClient := client.NewClientWithCLIFlags(ctx, config, serverAddressFromCLI, serverProtocolFromCLI, *configFile)

	// 根据运行模式决定连接策略
	if runInteractive {
		runInteractiveMode(ctx, cancel, tunnoxClient, config)
	} else {
		runDaemonMode(ctx, tunnoxClient, config)
	}

	// 停止客户端
	fmt.Println("\n🛑 Shutting down client...")
	tunnoxClient.Stop()
	corelog.Infof("Client: shutdown complete")

	// 检查是否被踢下线，设置相应的退出码
	// 退出码 2 表示被 DUPLICATE_LOGIN 踢下线
	if tunnoxClient.WasKicked() {
		corelog.Warnf("Client: exiting with code 2 (kicked by server)")
		os.Exit(2)
	}
}

// runInteractiveMode 运行交互模式
func runInteractiveMode(ctx context.Context, cancel context.CancelFunc, tunnoxClient *client.TunnoxClient, config *client.ClientConfig) {
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
}

// runDaemonMode 运行守护进程模式
func runDaemonMode(ctx context.Context, tunnoxClient *client.TunnoxClient, config *client.ClientConfig) {
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
