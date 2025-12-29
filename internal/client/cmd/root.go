// Package cmd 提供 Tunnox CLI 的命令框架
package cmd

import (
	"context"
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
	"tunnox-core/internal/version"

	"github.com/spf13/cobra"
)

// 全局标志
var (
	serverAddr  string
	transport   string
	configFile  string
	logFile     string
	interactive bool
	daemon      bool
)

// tunnoxClient 全局客户端实例
var tunnoxClient *client.TunnoxClient

// rootCmd 代表根命令
var rootCmd = &cobra.Command{
	Use:   "tunnox",
	Short: "Tunnox - Enterprise-grade port mapping and tunneling platform",
	Long: `Tunnox is an enterprise-grade port mapping and tunneling platform.
It supports TCP, WebSocket, KCP, and QUIC protocols for secure tunnel connections.

Quick Start:
  tunnox                    Start interactive wizard
  tunnox http 8080          Create HTTP tunnel for local port 8080
  tunnox tcp 3306           Create TCP tunnel for local port 3306
  tunnox code use <code>    Connect using a connection code`,
	Version: version.GetVersion(),
	Run:     runWizard,
}

// Execute 执行根命令
func Execute() {
	// 全局 panic recovery
	defer func() {
		if r := recover(); r != nil {
			corelog.Errorf("FATAL: main goroutine panic recovered: %v", r)
			corelog.Errorf("Stack trace:\n%s", string(debug.Stack()))
			fmt.Fprintf(os.Stderr, "\nPANIC: %v\n", r)
			fmt.Fprintf(os.Stderr, "Stack trace:\n%s\n", string(debug.Stack()))
			os.Exit(2)
		}
	}()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// 全局标志
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "", "Server address (e.g., localhost:7001)")
	rootCmd.PersistentFlags().StringVarP(&transport, "transport", "t", "", "Transport protocol: tcp/websocket/ws/kcp/quic")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Config file path")
	rootCmd.PersistentFlags().StringVar(&logFile, "log", "", "Log file path")
	rootCmd.PersistentFlags().BoolVarP(&interactive, "interactive", "i", false, "Start interactive CLI after command")
	rootCmd.PersistentFlags().BoolVarP(&daemon, "daemon", "d", false, "Run in daemon mode")

	// 添加子命令
	rootCmd.AddCommand(httpCmd)
	rootCmd.AddCommand(tcpCmd)
	rootCmd.AddCommand(udpCmd)
	rootCmd.AddCommand(socksCmd)
	rootCmd.AddCommand(codeCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
}

// runWizard 运行向导模式
func runWizard(cmd *cobra.Command, args []string) {
	fmt.Println()
	fmt.Println("Welcome to Tunnox!")
	fmt.Println("==================")
	fmt.Println()
	fmt.Println("What would you like to do?")
	fmt.Println()
	fmt.Println("  1. Create a tunnel (expose local service)")
	fmt.Println("  2. Connect using a code (access remote service)")
	fmt.Println("  3. Start interactive CLI")
	fmt.Println("  4. Show help")
	fmt.Println()

	options := []string{
		"Create a tunnel (expose local service)",
		"Connect using a code (access remote service)",
		"Start interactive CLI",
		"Show help",
	}

	choice, err := cli.PromptSelect("Select an option:", options)
	if err != nil || choice < 0 {
		return
	}

	switch choice {
	case 0:
		runTunnelWizard()
	case 1:
		runCodeWizard()
	case 2:
		runInteractiveCLI()
	case 3:
		cmd.Help()
	}
}

// runTunnelWizard 运行创建隧道向导
func runTunnelWizard() {
	fmt.Println()
	fmt.Println("Create a Tunnel")
	fmt.Println("---------------")
	fmt.Println()

	options := []string{"TCP", "HTTP", "UDP", "SOCKS5", "Back"}
	choice, err := cli.PromptSelect("Select tunnel type:", options)
	if err != nil || choice < 0 || choice >= len(options)-1 {
		return
	}

	tunnelType := strings.ToLower(options[choice])

	// 提示输入端口
	output := cli.NewOutput(false)
	output.Info("Enter the local port to expose:")

	// 根据隧道类型调用相应的命令
	switch tunnelType {
	case "tcp":
		tcpCmd.Run(tcpCmd, nil)
	case "http":
		httpCmd.Run(httpCmd, nil)
	case "udp":
		udpCmd.Run(udpCmd, nil)
	case "socks5":
		socksCmd.Run(socksCmd, nil)
	}
}

// runCodeWizard 运行连接码向导
func runCodeWizard() {
	fmt.Println()
	fmt.Println("Connect Using a Code")
	fmt.Println("--------------------")
	fmt.Println()

	output := cli.NewOutput(false)
	output.Info("Enter the connection code:")

	// 调用 code use 命令
	codeUseCmd.Run(codeUseCmd, nil)
}

// runInteractiveCLI 运行交互式 CLI
func runInteractiveCLI() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	// 加载配置
	config, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 配置日志
	if err := configureLogging(config); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to configure logging: %v\n", err)
		os.Exit(1)
	}

	// 创建客户端
	tunnoxClient = client.NewClientWithCLIFlags(ctx, config, serverAddr != "", transport != "")

	// 连接到服务器
	fmt.Fprintf(os.Stderr, "\nConnecting to Tunnox service...\n")
	if err := tunnoxClient.Connect(); err != nil {
		if ctx.Err() == context.Canceled {
			fmt.Fprintf(os.Stderr, "\nConnection cancelled\n")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "\nConnection failed\n")
		fmt.Fprintf(os.Stderr, "Please check your network or specify server with --server flag\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Connected successfully\n\n")

	// 启动 CLI
	tunnoxCLI, err := cli.NewCLI(ctx, tunnoxClient)
	if err != nil {
		corelog.Errorf("CLI initialization failed: %v", err)
		fmt.Fprintf(os.Stderr, "Failed to initialize CLI: %v\n", err)
		os.Exit(1)
	}

	// 启动 CLI（阻塞）
	tunnoxCLI.Start()

	// 停止客户端
	fmt.Println("\nShutting down client...")
	tunnoxClient.Stop()
}

// loadConfig 加载配置
func loadConfig() (*client.ClientConfig, error) {
	configManager := client.NewConfigManager()
	config, err := configManager.LoadConfig(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// 命令行参数覆盖配置文件
	if transport != "" {
		config.Server.Protocol = normalizeProtocol(transport)
	}
	if serverAddr != "" {
		config.Server.Address = serverAddr
	}

	// 设置默认值
	if config.Server.Address == "" {
		config.Server.Address = "https://gw.tunnox.net/_tunnox"
		config.Server.Protocol = "websocket"
	}
	if config.Server.Protocol == "" {
		config.Server.Protocol = "websocket"
	}

	// 验证配置
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// validateConfig 验证配置
func validateConfig(config *client.ClientConfig) error {
	if config.Server.Protocol != "" {
		config.Server.Protocol = normalizeProtocol(config.Server.Protocol)
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

	// ClientID 和 SecretKey 可以为空，首次连接时由服务端分配
	return nil
}

// normalizeProtocol 规范化协议名称
func normalizeProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "ws" {
		return "websocket"
	}
	return protocol
}

// configureLogging 配置日志
func configureLogging(config *client.ClientConfig) error {
	logConfig := &client.LogConfig{
		Level:  "info",
		Format: "text",
		Output: "file",
	}

	if config.Log.Level != "" {
		logConfig.Level = config.Log.Level
	}
	if config.Log.Format != "" {
		logConfig.Format = config.Log.Format
	}

	// 确定日志文件路径
	logFilePath := config.Log.File
	if logFile != "" {
		logFilePath = logFile
	}
	if logFilePath == "" {
		candidates := utils.GetDefaultClientLogPath(true)
		var err error
		logFilePath, err = utils.ResolveLogPath(candidates)
		if err != nil {
			return fmt.Errorf("failed to resolve log path: %w", err)
		}
	} else {
		expandedPath, err := utils.ExpandPath(logFilePath)
		if err != nil {
			return fmt.Errorf("failed to expand log file path %q: %w", logFilePath, err)
		}
		logFilePath = expandedPath

		logDir := filepath.Dir(logFilePath)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return fmt.Errorf("failed to create log directory %q: %w", logDir, err)
		}
	}

	logConfig.File = logFilePath

	if err := utils.InitLogger((*utils.LogConfig)(logConfig)); err != nil {
		return err
	}

	return nil
}

// ensureConnected 确保客户端已连接
func ensureConnected(ctx context.Context) error {
	if tunnoxClient != nil && tunnoxClient.IsConnected() {
		return nil
	}

	config, err := loadConfig()
	if err != nil {
		return err
	}

	if err := configureLogging(config); err != nil {
		return err
	}

	tunnoxClient = client.NewClientWithCLIFlags(ctx, config, serverAddr != "", transport != "")

	fmt.Fprintf(os.Stderr, "Connecting to server...\n")
	if err := tunnoxClient.Connect(); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Connected successfully\n")
	return nil
}
