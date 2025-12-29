package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"tunnox-core/internal/client"
	"tunnox-core/internal/client/cli"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/utils"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 快捷命令支持
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// isQuickCommand 检查是否是快捷命令
func isQuickCommand(arg string) bool {
	quickCommands := []string{
		// 快捷隧道命令
		"http", "tcp", "udp", "socks",
		// 连接码命令
		"code",
		// 守护进程命令
		"start", "stop", "status",
		// 配置命令
		"config",
		// 交互模式
		"shell",
		// 版本和帮助
		"version", "--version", "-v",
		"help", "--help",
	}
	arg = strings.ToLower(arg)
	for _, cmd := range quickCommands {
		if arg == cmd {
			return true
		}
	}
	return false
}

// runQuickCommand 执行快捷命令
func runQuickCommand(args []string) {
	// 创建上下文
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

	// 从配置文件加载配置（如果存在）
	configManager := client.NewConfigManager()
	config, err := configManager.LoadConfig("")
	if err != nil {
		// 配置加载失败，使用默认配置
		config = &client.ClientConfig{}
	}

	// 创建快捷命令执行器
	runner := cli.NewQuickCommandRunner(ctx, config)

	// 执行命令
	shouldContinue, err := runner.Run(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}

	// 如果需要继续传统流程（例如 shell 命令）
	if shouldContinue && len(args) > 0 && args[0] == "shell" {
		// 重新进入传统的交互式流程
		// 设置参数以模拟无参数启动
		os.Args = []string{os.Args[0]}
		// 递归调用 main 不太好，这里直接返回，让 shell 命令走传统流程
		runTraditionalInteractive(ctx, config)
		return
	}
}

// runTraditionalInteractive 运行传统交互式模式
func runTraditionalInteractive(ctx context.Context, config *client.ClientConfig) {
	// 配置日志（静默模式）
	logConfig := &utils.LogConfig{
		Level:  "info",
		Output: "file",
	}
	candidates := utils.GetDefaultClientLogPath(true)
	logFile, err := utils.ResolveLogPath(candidates)
	if err == nil {
		logConfig.File = logFile
	}
	utils.InitLogger(logConfig)

	// 创建客户端
	tunnoxClient := client.NewClient(ctx, config)

	// 连接
	fmt.Fprintf(os.Stderr, "\n🔍 Connecting to Tunnox service...\n")
	if err := tunnoxClient.Connect(); err != nil {
		if ctx.Err() == context.Canceled {
			fmt.Fprintf(os.Stderr, "\n⚠️  Connection cancelled\n")
			return
		}
		fmt.Fprintf(os.Stderr, "\n❌ Connection failed: %v\n", err)
		fmt.Fprintf(os.Stderr, "💡 Please check your network or specify server with -s flag\n")
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "✅ Connected successfully\n\n")

	// 启动 CLI
	tunnoxCLI, err := cli.NewCLI(ctx, tunnoxClient)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to initialize CLI: %v\n", err)
		os.Exit(1)
	}

	// 启动自动重连监控
	go monitorConnectionAndReconnect(ctx, tunnoxClient)

	// 启动 CLI（阻塞）
	tunnoxCLI.Start()

	// 停止客户端
	fmt.Println("\n🛑 Shutting down client...")
	tunnoxClient.Stop()

	// 检查是否被踢下线，设置相应的退出码
	// 退出码 2 表示被 DUPLICATE_LOGIN 踢下线
	if tunnoxClient.WasKicked() {
		corelog.Warnf("Client: exiting with code 2 (kicked by server)")
		os.Exit(2)
	}
}
