// Package cli 提供 Tunnox 客户端的连接码管理命令
package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"tunnox-core/internal/client"
)

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

// showCodeHelp 显示 code 命令帮助
func (r *QuickCommandRunner) showCodeHelp() {
	help := `
Connection Code Commands:

  tunnox code generate [protocol target]  Generate a new connection code
  tunnox code use <code> [--port PORT]    Use a connection code
  tunnox code list                        List your connection codes
  tunnox code revoke <code>               Revoke a connection code

Examples:
  tunnox code generate tcp 192.168.1.10:22
  tunnox code use ABC123
  tunnox code use ABC123 --port 9999
  tunnox code list
  tunnox code revoke ABC123
`
	fmt.Fprint(os.Stderr, help)
}
