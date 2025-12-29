// Package cli 提供 Tunnox 客户端的系统命令（版本、帮助、守护进程）
package cli

import (
	"fmt"
	"os"
	"time"

	corelog "tunnox-core/internal/core/log"
)

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
