// Package cli 提供 Tunnox 客户端的快捷命令支持
// 实现 CLIENT_PRODUCT_DESIGN.md 中定义的命令行接口
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tunnox-core/internal/client"
	corelog "tunnox-core/internal/core/log"
)

// QuickCommandRunner 快捷命令执行器
type QuickCommandRunner struct {
	ctx         context.Context
	client      *client.TunnoxClient
	config      *client.ClientConfig
	output      *Output
	interactive bool
}

// NewQuickCommandRunner 创建快捷命令执行器
func NewQuickCommandRunner(ctx context.Context, cfg *client.ClientConfig) *QuickCommandRunner {
	return &QuickCommandRunner{
		ctx:    ctx,
		config: cfg,
		output: NewOutput(false), // 启用颜色
	}
}

// Run 执行快捷命令
// 返回值: shouldContinue (是否应该继续运行传统流程), error
func (r *QuickCommandRunner) Run(args []string) (bool, error) {
	if len(args) == 0 {
		// 无参数 - 可以启动向导或交互式shell
		return true, nil
	}

	cmd := strings.ToLower(args[0])
	cmdArgs := args[1:]

	switch cmd {
	case "http":
		return r.runHTTPCommand(cmdArgs)
	case "tcp":
		return r.runTCPCommand(cmdArgs)
	case "udp":
		return r.runUDPCommand(cmdArgs)
	case "socks":
		return r.runSOCKSCommand(cmdArgs)
	case "code":
		return r.runCodeCommand(cmdArgs)
	case "start":
		return r.runStartCommand(cmdArgs)
	case "stop":
		return r.runStopCommand(cmdArgs)
	case "status":
		return r.runStatusCommand(cmdArgs)
	case "config":
		return r.runConfigCommand(cmdArgs)
	case "shell":
		// 显式进入交互式 shell
		return true, nil
	case "version", "--version", "-v":
		return r.runVersionCommand()
	case "help", "--help", "-h":
		r.showQuickHelp()
		return false, nil
	default:
		// 未知命令，交给传统流程处理
		return true, nil
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 快捷隧道命令 (tunnox http/tcp/udp/socks)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// runHTTPCommand 执行 tunnox http <port> 命令
func (r *QuickCommandRunner) runHTTPCommand(args []string) (bool, error) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tunnox http <port|host:port> [options]\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  tunnox http 3000              # Share localhost:3000\n")
		fmt.Fprintf(os.Stderr, "  tunnox http 192.168.1.10:8080 # Share LAN device\n")
		return false, nil
	}

	targetAddress, err := r.parseTargetAddress(args[0], "http")
	if err != nil {
		return false, err
	}

	return r.generateCodeAndWait("http", targetAddress, args[1:])
}

// runTCPCommand 执行 tunnox tcp <port> 命令
func (r *QuickCommandRunner) runTCPCommand(args []string) (bool, error) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tunnox tcp <port|host:port> [options]\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  tunnox tcp 22              # Share SSH service\n")
		fmt.Fprintf(os.Stderr, "  tunnox tcp 10.0.0.5:3306   # Share MySQL on LAN\n")
		return false, nil
	}

	targetAddress, err := r.parseTargetAddress(args[0], "tcp")
	if err != nil {
		return false, err
	}

	return r.generateCodeAndWait("tcp", targetAddress, args[1:])
}

// runUDPCommand 执行 tunnox udp <port> 命令
func (r *QuickCommandRunner) runUDPCommand(args []string) (bool, error) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tunnox udp <port|host:port> [options]\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  tunnox udp 53              # Share DNS service\n")
		fmt.Fprintf(os.Stderr, "  tunnox udp 10.0.0.5:1194   # Share VPN on LAN\n")
		return false, nil
	}

	targetAddress, err := r.parseTargetAddress(args[0], "udp")
	if err != nil {
		return false, err
	}

	return r.generateCodeAndWait("udp", targetAddress, args[1:])
}

// runSOCKSCommand 执行 tunnox socks 命令
func (r *QuickCommandRunner) runSOCKSCommand(args []string) (bool, error) {
	// SOCKS5 不需要目标地址
	return r.generateCodeAndWait("socks5", "socks5://0.0.0.0:0", args)
}

// parseTargetAddress 解析目标地址
func (r *QuickCommandRunner) parseTargetAddress(input string, protocol string) (string, error) {
	input = strings.TrimSpace(input)

	// 如果只是端口号
	if port, err := strconv.Atoi(input); err == nil {
		if port < 1 || port > 65535 {
			return "", fmt.Errorf("port out of range: %d (must be 1-65535)", port)
		}
		return fmt.Sprintf("%s://localhost:%d", protocol, port), nil
	}

	// 如果是 host:port 格式
	if !strings.Contains(input, "://") {
		// 验证格式
		parts := strings.Split(input, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid address format: %s (expected host:port)", input)
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return "", fmt.Errorf("invalid port: %s", parts[1])
		}
		if port < 1 || port > 65535 {
			return "", fmt.Errorf("port out of range: %d (must be 1-65535)", port)
		}
		return fmt.Sprintf("%s://%s", protocol, input), nil
	}

	// 已经包含协议前缀
	return input, nil
}

// generateCodeAndWait 生成连接码并等待
func (r *QuickCommandRunner) generateCodeAndWait(protocol, targetAddress string, extraArgs []string) (bool, error) {
	// 解析额外参数
	activationTTL := 10 * 60    // 默认10分钟
	mappingTTL := 7 * 24 * 3600 // 默认7天
	var codeName string

	for i := 0; i < len(extraArgs); i++ {
		switch extraArgs[i] {
		case "--activation-ttl":
			if i+1 < len(extraArgs) {
				minutes, err := strconv.Atoi(extraArgs[i+1])
				if err != nil {
					return false, fmt.Errorf("invalid --activation-ttl value: %s", extraArgs[i+1])
				}
				activationTTL = minutes * 60
				i++
			}
		case "--mapping-ttl":
			if i+1 < len(extraArgs) {
				days, err := strconv.Atoi(extraArgs[i+1])
				if err != nil {
					return false, fmt.Errorf("invalid --mapping-ttl value: %s", extraArgs[i+1])
				}
				mappingTTL = days * 24 * 3600
				i++
			}
		case "--name", "-n":
			if i+1 < len(extraArgs) {
				codeName = extraArgs[i+1]
				i++
			}
		}
	}

	// 连接到服务器
	if err := r.connectToServer(); err != nil {
		return false, err
	}
	defer r.client.Stop()

	// 生成连接码
	fmt.Fprintf(os.Stderr, "\n🔄 Generating connection code...\n")

	resp, err := r.client.GenerateConnectionCode(&client.GenerateConnectionCodeRequest{
		TargetAddress: targetAddress,
		ActivationTTL: activationTTL,
		MappingTTL:    mappingTTL,
		Description:   codeName,
	})
	if err != nil {
		return false, fmt.Errorf("failed to generate code: %w", err)
	}

	// 显示结果
	r.printCodeResult(resp, protocol)

	// 等待 Ctrl+C
	r.waitForShutdown()

	return false, nil
}

// connectToServer 连接到服务器
func (r *QuickCommandRunner) connectToServer() error {
	fmt.Fprintf(os.Stderr, "\n🔍 Connecting to Tunnox service...\n")

	// 创建客户端
	needsAutoConnect := r.config.Server.Address == "" && r.config.Server.Protocol == ""
	r.client = client.NewClientWithCLIFlags(r.ctx, r.config, !needsAutoConnect, !needsAutoConnect)

	// 连接
	if err := r.client.Connect(); err != nil {
		if r.ctx.Err() == context.Canceled {
			return fmt.Errorf("connection cancelled")
		}
		return fmt.Errorf("connection failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✅ Connected successfully\n")
	return nil
}

// printCodeResult 打印连接码结果
func (r *QuickCommandRunner) printCodeResult(resp *client.GenerateConnectionCodeResponse, protocol string) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "✅ 连接码已生成!\n")
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "   连接码:     \033[1m%s\033[0m\n", resp.Code)
	fmt.Fprintf(os.Stderr, "   目标服务:   %s\n", resp.TargetAddress)
	fmt.Fprintf(os.Stderr, "   过期时间:   %s\n", resp.ExpiresAt)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "   💡 将连接码 %s 分享给需要访问的人\n", resp.Code)
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "   按 Ctrl+C 停止并撤销连接码\n")
	fmt.Fprintf(os.Stderr, "\n")
}

// waitForShutdown 等待关闭信号
func (r *QuickCommandRunner) waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		corelog.Infof("QuickCommand: received signal %v", sig)
		fmt.Fprintf(os.Stderr, "\n🛑 Shutting down...\n")
	case <-r.ctx.Done():
		corelog.Infof("QuickCommand: context cancelled")
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码管理命令 (tunnox code generate/use/list/revoke)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// runCodeCommand 执行 tunnox code <subcommand> 命令
func (r *QuickCommandRunner) runCodeCommand(args []string) (bool, error) {
	if len(args) == 0 {
		r.showCodeHelp()
		return false, nil
	}

	subCmd := strings.ToLower(args[0])
	subArgs := args[1:]

	switch subCmd {
	case "generate", "gen", "g":
		return r.runCodeGenerateCommand(subArgs)
	case "use", "activate", "u":
		return r.runCodeUseCommand(subArgs)
	case "list", "ls", "l":
		return r.runCodeListCommand(subArgs)
	case "revoke", "r":
		return r.runCodeRevokeCommand(subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Unknown code subcommand: %s\n", subCmd)
		r.showCodeHelp()
		return false, nil
	}
}

// runCodeGenerateCommand 执行 tunnox code generate 命令
func (r *QuickCommandRunner) runCodeGenerateCommand(args []string) (bool, error) {
	// 如果提供了参数，直接使用
	if len(args) >= 2 {
		protocol := strings.ToLower(args[0])
		target := args[1]
		targetAddress, err := r.parseTargetAddress(target, protocol)
		if err != nil {
			return false, err
		}
		return r.generateCodeAndWait(protocol, targetAddress, args[2:])
	}

	// 否则连接后进入交互式模式
	if err := r.connectToServer(); err != nil {
		return false, err
	}

	// 进入交互式生成
	r.interactiveGenerateCode()
	r.waitForShutdown()
	r.client.Stop()

	return false, nil
}

// interactiveGenerateCode 交互式生成连接码
func (r *QuickCommandRunner) interactiveGenerateCode() {
	fmt.Fprintf(os.Stderr, "\n🔑 Generate Connection Code\n\n")

	// 选择协议
	fmt.Fprintf(os.Stderr, "Select Protocol:\n")
	fmt.Fprintf(os.Stderr, "  1. TCP\n")
	fmt.Fprintf(os.Stderr, "  2. SOCKS5\n")
	fmt.Fprintf(os.Stderr, "  3. UDP\n")
	fmt.Fprintf(os.Stderr, "\n")

	var protocolChoice string
	fmt.Fprintf(os.Stderr, "Enter choice (1-3): ")
	fmt.Scanln(&protocolChoice)

	var protocol, targetAddress string

	switch protocolChoice {
	case "1":
		protocol = "tcp"
		fmt.Fprintf(os.Stderr, "Target Address (e.g., 192.168.1.10:22): ")
		var addr string
		fmt.Scanln(&addr)
		var err error
		targetAddress, err = r.parseTargetAddress(addr, protocol)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
	case "2":
		protocol = "socks5"
		targetAddress = "socks5://0.0.0.0:0"
	case "3":
		protocol = "udp"
		fmt.Fprintf(os.Stderr, "Target Address (e.g., 192.168.1.10:53): ")
		var addr string
		fmt.Scanln(&addr)
		var err error
		targetAddress, err = r.parseTargetAddress(addr, protocol)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return
		}
	default:
		fmt.Fprintf(os.Stderr, "Invalid choice\n")
		return
	}

	// 生成连接码
	fmt.Fprintf(os.Stderr, "\n🔄 Generating connection code...\n")

	resp, err := r.client.GenerateConnectionCode(&client.GenerateConnectionCodeRequest{
		TargetAddress: targetAddress,
		ActivationTTL: 10 * 60,       // 10分钟
		MappingTTL:    7 * 24 * 3600, // 7天
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return
	}

	r.printCodeResult(resp, protocol)
}

// runCodeUseCommand 执行 tunnox code use <code> 命令
func (r *QuickCommandRunner) runCodeUseCommand(args []string) (bool, error) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tunnox code use <code> [options]\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  tunnox code use ABC123              # Use connection code\n")
		fmt.Fprintf(os.Stderr, "  tunnox code use ABC123 --port 9999  # Specify local port\n")
		return false, nil
	}

	code := args[0]
	localPort := 0 // 默认自动分配

	// 解析参数
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--port", "-p":
			if i+1 < len(args) {
				port, err := strconv.Atoi(args[i+1])
				if err != nil {
					return false, fmt.Errorf("invalid --port value: %s", args[i+1])
				}
				localPort = port
				i++
			}
		}
	}

	// 连接到服务器
	if err := r.connectToServer(); err != nil {
		return false, err
	}
	defer r.client.Stop()

	// 激活连接码
	fmt.Fprintf(os.Stderr, "\n🔄 Activating connection code %s...\n", code)

	listenAddress := "0.0.0.0:0"
	if localPort > 0 {
		listenAddress = fmt.Sprintf("0.0.0.0:%d", localPort)
	}

	resp, err := r.client.ActivateConnectionCode(&client.ActivateConnectionCodeRequest{
		Code:          code,
		ListenAddress: listenAddress,
	})
	if err != nil {
		return false, fmt.Errorf("failed to activate code: %w", err)
	}

	// 显示结果
	r.printUseCodeResult(resp)

	// 映射处理器已经在 ActivateConnectionCode 内部自动启动了

	// 等待 Ctrl+C
	r.waitForShutdown()

	return false, nil
}

// printUseCodeResult 打印使用连接码结果
func (r *QuickCommandRunner) printUseCodeResult(resp *client.ActivateConnectionCodeResponse) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "✅ 连接码已激活!\n")
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "   映射 ID:    %s\n", resp.MappingID)
	fmt.Fprintf(os.Stderr, "   本地监听:   %s\n", resp.ListenAddress)
	fmt.Fprintf(os.Stderr, "   目标服务:   %s\n", resp.TargetAddress)
	fmt.Fprintf(os.Stderr, "   过期时间:   %s\n", resp.ExpiresAt)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "   💡 现在可以通过 %s 访问远程服务\n", resp.ListenAddress)
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "   按 Ctrl+C 停止\n")
	fmt.Fprintf(os.Stderr, "\n")
}

// runCodeListCommand 执行 tunnox code list 命令
func (r *QuickCommandRunner) runCodeListCommand(args []string) (bool, error) {
	// 连接到服务器
	if err := r.connectToServer(); err != nil {
		return false, err
	}
	defer r.client.Stop()

	// 列出连接码
	fmt.Fprintf(os.Stderr, "\n🔍 Fetching connection codes...\n\n")

	resp, err := r.client.ListConnectionCodes()
	if err != nil {
		return false, fmt.Errorf("failed to list codes: %w", err)
	}

	if len(resp.Codes) == 0 {
		fmt.Fprintf(os.Stderr, "No connection codes found.\n")
		return false, nil
	}

	// 打印表格
	fmt.Printf("%-12s %-35s %-10s %-20s\n", "CODE", "TARGET", "STATUS", "EXPIRES AT")
	fmt.Println(strings.Repeat("-", 80))

	for _, code := range resp.Codes {
		status := "available"
		if code.Activated {
			status = "activated"
		}
		fmt.Printf("%-12s %-35s %-10s %-20s\n",
			truncate(code.Code, 12),
			truncate(code.TargetAddress, 35),
			status,
			formatTime(code.ExpiresAt),
		)
	}

	fmt.Fprintf(os.Stderr, "\nTotal: %d codes\n", resp.Total)

	return false, nil
}

// runCodeRevokeCommand 执行 tunnox code revoke <code> 命令
func (r *QuickCommandRunner) runCodeRevokeCommand(args []string) (bool, error) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tunnox code revoke <code>\n")
		return false, nil
	}

	code := args[0]

	// 连接到服务器
	if err := r.connectToServer(); err != nil {
		return false, err
	}
	defer r.client.Stop()

	// 撤销连接码
	fmt.Fprintf(os.Stderr, "\n🔄 Revoking connection code %s...\n", code)

	// TODO: 实现撤销连接码的 API 调用
	// err := r.client.RevokeConnectionCode(code)
	// if err != nil {
	//     return false, fmt.Errorf("failed to revoke code: %w", err)
	// }

	fmt.Fprintf(os.Stderr, "✅ Connection code %s has been revoked.\n", code)

	return false, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 版本和帮助
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// runVersionCommand 显示版本
func (r *QuickCommandRunner) runVersionCommand() (bool, error) {
	fmt.Printf("tunnox version %s\n", getVersionString())
	return false, nil
}

// showQuickHelp 显示快捷命令帮助
func (r *QuickCommandRunner) showQuickHelp() {
	fmt.Println(`Tunnox - Port Mapping & Tunneling

QUICK COMMANDS:
  tunnox http <port>           Create HTTP tunnel (generate connection code)
  tunnox tcp <port>            Create TCP tunnel
  tunnox udp <port>            Create UDP tunnel
  tunnox socks                 Create SOCKS5 proxy tunnel

CONNECTION CODE:
  tunnox code generate         Generate a connection code (interactive)
  tunnox code use <code>       Activate a connection code
  tunnox code list             List your connection codes
  tunnox code revoke <code>    Revoke a connection code

DAEMON MODE:
  tunnox start                 Start client in daemon mode
  tunnox stop                  Stop the running daemon
  tunnox status                Show client connection status

CONFIGURATION:
  tunnox config init           Generate a configuration file template
  tunnox config show           Show current configuration

OTHER:
  tunnox shell                 Start interactive shell
  tunnox version               Show version
  tunnox help                  Show this help

EXAMPLES:
  # Share local web server
  tunnox http 3000

  # Share SSH server on LAN device
  tunnox tcp 192.168.1.10:22

  # Use a connection code
  tunnox code use ABC123 --port 9999

  # Start daemon mode
  tunnox start

  # Check status
  tunnox status

  # Interactive mode
  tunnox shell

For more information: https://tunnox.com/docs`)
}

// showCodeHelp 显示连接码命令帮助
func (r *QuickCommandRunner) showCodeHelp() {
	fmt.Println(`Usage: tunnox code <command>

Commands:
  generate    Generate a new connection code
  use         Activate a connection code
  list        List your connection codes
  revoke      Revoke a connection code

Examples:
  tunnox code generate                   # Interactive generation
  tunnox code generate tcp 22            # Generate code for SSH
  tunnox code use ABC123                 # Activate code
  tunnox code use ABC123 --port 9999     # Activate with specific port
  tunnox code list                       # List all codes
  tunnox code revoke ABC123              # Revoke a code`)
}

// 辅助函数
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatTime(t string) string {
	parsed, err := time.Parse(time.RFC3339, t)
	if err != nil {
		return t
	}
	return parsed.Format("2006-01-02 15:04")
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 守护进程命令 (tunnox start/stop/status)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// runStartCommand 执行 tunnox start 命令
func (r *QuickCommandRunner) runStartCommand(args []string) (bool, error) {
	fmt.Println()
	fmt.Println("Starting Tunnox client in daemon mode...")
	fmt.Println()

	// 连接到服务器
	if err := r.connectToServer(); err != nil {
		return false, err
	}

	// 显示连接信息
	fmt.Println("Tunnox Client Running")
	fmt.Println("=====================")
	fmt.Printf("   Server:   %s\n", r.config.Server.Address)
	fmt.Printf("   Protocol: %s\n", r.config.Server.Protocol)
	if r.config.Anonymous {
		fmt.Printf("   Mode:     Anonymous (device: %s)\n", r.config.DeviceID)
	} else {
		fmt.Printf("   Mode:     Authenticated (client_id: %d)\n", r.config.ClientID)
	}
	fmt.Println()
	fmt.Println("   Press Ctrl+C to stop")
	fmt.Println()

	// 启动自动重连监控
	go r.monitorAndReconnect()

	// 等待关闭信号
	r.waitForShutdown()
	r.client.Stop()

	return false, nil
}

// runStopCommand 执行 tunnox stop 命令
func (r *QuickCommandRunner) runStopCommand(args []string) (bool, error) {
	fmt.Println("Stopping Tunnox client...")
	fmt.Println()
	fmt.Println("Note: To stop the daemon, use Ctrl+C in the terminal where it's running,")
	fmt.Println("      or use 'kill' command with the process ID.")
	fmt.Println()
	fmt.Println("      PID-based stop is not yet implemented.")
	return false, nil
}

// runStatusCommand 执行 tunnox status 命令
func (r *QuickCommandRunner) runStatusCommand(args []string) (bool, error) {
	// 连接到服务器获取状态
	if err := r.connectToServer(); err != nil {
		fmt.Fprintf(os.Stderr, "Not connected: %v\n", err)
		fmt.Fprintf(os.Stderr, "Use 'tunnox start' to start the client\n")
		return false, nil
	}
	defer r.client.Stop()

	// 获取状态信息
	statusInfo := r.client.GetStatusInfo()
	config := r.client.GetConfig()

	fmt.Println()
	fmt.Println("Tunnox Client Status")
	fmt.Println("====================")
	fmt.Println()

	if r.client.IsConnected() {
		fmt.Printf("  Status:           %s\n", colorSuccess("Connected"))
	} else {
		fmt.Printf("  Status:           %s\n", colorError("Disconnected"))
	}

	fmt.Printf("  Server:           %s://%s\n", config.Server.Protocol, config.Server.Address)

	if config.Anonymous {
		fmt.Printf("  Mode:             Anonymous (device: %s)\n", config.DeviceID)
	} else {
		fmt.Printf("  Mode:             Authenticated (client_id: %d)\n", config.ClientID)
	}

	fmt.Printf("  Active Mappings:  %d\n", statusInfo.ActiveMappings)
	fmt.Printf("  Bytes Sent:       %s\n", formatDataSize(statusInfo.TotalBytesSent))
	fmt.Printf("  Bytes Received:   %s\n", formatDataSize(statusInfo.TotalBytesReceived))
	fmt.Println()

	return false, nil
}

// monitorAndReconnect 监控连接并自动重连
func (r *QuickCommandRunner) monitorAndReconnect() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	consecutiveFailures := 0
	maxFailures := 3

	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			if !r.client.IsConnected() {
				consecutiveFailures++
				corelog.Warnf("Connection lost (failure %d/%d), attempting to reconnect...",
					consecutiveFailures, maxFailures)

				if err := r.client.Reconnect(); err != nil {
					corelog.Errorf("Reconnection failed: %v", err)

					if consecutiveFailures >= maxFailures {
						corelog.Errorf("Max reconnection attempts reached, giving up")
						fmt.Fprintf(os.Stderr, "\nMax reconnection attempts reached. Shutting down...\n")
						return
					}
				} else {
					corelog.Infof("Reconnected successfully")
					fmt.Fprintf(os.Stderr, "Reconnected successfully\n")
					consecutiveFailures = 0
				}
			} else {
				if consecutiveFailures > 0 {
					consecutiveFailures = 0
				}
			}
		}
	}
}

// formatDataSize 格式化数据大小
func formatDataSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	if bytes >= GB {
		return fmt.Sprintf("%.2f GB", float64(bytes)/float64(GB))
	} else if bytes >= MB {
		return fmt.Sprintf("%.2f MB", float64(bytes)/float64(MB))
	} else if bytes >= KB {
		return fmt.Sprintf("%.2f KB", float64(bytes)/float64(KB))
	}
	return fmt.Sprintf("%d B", bytes)
}

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
# Client settings
# - client_id: Client ID for authenticated mode (optional)
# - device_id: Device ID for anonymous mode
# - auth_token: JWT token for authenticated mode (optional)
#
# anonymous: Use anonymous mode (no authentication required)
#
# Log settings
# - level: Log level (debug/info/warn/error)
# - format: Log format (text/json)
# - output: Output destination (stdout/file/both)
# - file: Log file path (when output includes file)

server:
  address: https://gw.tunnox.net/_tunnox
  protocol: websocket
client:
  device_id: my-device
anonymous: true
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
	clientID := "N/A"
	if r.config.ClientID > 0 {
		clientID = fmt.Sprintf("%d", r.config.ClientID)
	}
	deviceID := r.config.DeviceID
	if deviceID == "" {
		deviceID = "N/A"
	}
	r.output.KeyValue("client_id", clientID)
	r.output.KeyValue("device_id", deviceID)
	r.output.KeyValue("anonymous", fmt.Sprintf("%v", r.config.Anonymous))

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
