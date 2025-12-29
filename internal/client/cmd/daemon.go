package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tunnox-core/internal/client"
	corelog "tunnox-core/internal/core/log"

	"github.com/spf13/cobra"
)

// startCmd 启动守护进程
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Tunnox client daemon",
	Long: `Start the Tunnox client in daemon mode.

The client will run in the background, maintaining connections and
automatically reconnecting if disconnected.

Example:
  tunnox start                  # Start with default config
  tunnox start -c config.yaml   # Start with specific config
  tunnox start --server localhost:7001 --anonymous`,
	Run: runStart,
}

// stopCmd 停止守护进程
var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Tunnox client daemon",
	Long: `Stop a running Tunnox client daemon.

Example:
  tunnox stop`,
	Run: runStop,
}

// statusCmd 查看运行状态
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show client status",
	Long: `Show the current status of the Tunnox client.

Displays connection status, active mappings, and statistics.

Example:
  tunnox status`,
	Run: runStatus,
}

func runStart(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			corelog.Infof("Received signal, shutting down...")
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

	fmt.Printf("Tunnox Client Starting...\n")
	fmt.Printf("   Protocol:  %s\n", config.Server.Protocol)
	fmt.Printf("   Server:    %s://%s\n", config.Server.Protocol, config.Server.Address)
	fmt.Printf("   Client ID: %d\n", config.ClientID)
	fmt.Println()

	// 创建客户端
	tunnoxClient = client.NewClientWithCLIFlags(ctx, config, serverAddr != "", transport != "")

	// 带重试的连接
	fmt.Println("Running in daemon mode...")
	if err := connectWithRetry(tunnoxClient, 5); err != nil {
		if ctx.Err() == context.Canceled {
			fmt.Fprintf(os.Stderr, "\nConnection cancelled by user\n")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Failed to connect to server after retries: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected to server successfully!")
	fmt.Println("   Press Ctrl+C to stop")
	fmt.Println()

	// 启动自动重连监控
	go monitorConnectionAndReconnect(ctx, tunnoxClient)

	// 等待上下文取消
	<-ctx.Done()

	// 停止客户端
	fmt.Println("\nShutting down client...")
	tunnoxClient.Stop()
	corelog.Infof("Shutdown complete")
}

func runStop(cmd *cobra.Command, args []string) {
	fmt.Println("Stopping Tunnox client...")

	// TODO: 实现通过 PID 文件或 IPC 停止守护进程
	fmt.Println("Note: Manual stop via Ctrl+C or kill command")
	fmt.Println("      PID-based stop is not yet implemented")
}

func runStatus(cmd *cobra.Command, args []string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 确保连接
	if err := ensureConnected(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Not connected: %v\n", err)
		fmt.Fprintf(os.Stderr, "Use 'tunnox start' to start the client\n")
		os.Exit(1)
	}

	// 获取状态信息
	statusInfo := tunnoxClient.GetStatusInfo()
	config := tunnoxClient.GetConfig()

	fmt.Println()
	fmt.Println("Tunnox Client Status")
	fmt.Println("====================")
	fmt.Println()

	if tunnoxClient.IsConnected() {
		fmt.Printf("  Status:           Connected\n")
	} else {
		fmt.Printf("  Status:           Disconnected\n")
	}

	fmt.Printf("  Server:           %s://%s\n", config.Server.Protocol, config.Server.Address)
	fmt.Printf("  Client ID:        %d\n", config.ClientID)
	fmt.Printf("  Active Mappings:  %d\n", statusInfo.ActiveMappings)
	fmt.Printf("  Bytes Sent:       %s\n", formatBytes(statusInfo.TotalBytesSent))
	fmt.Printf("  Bytes Received:   %s\n", formatBytes(statusInfo.TotalBytesReceived))
	fmt.Println()
}

// connectWithRetry 带重试的连接
func connectWithRetry(tunnoxClient *client.TunnoxClient, maxRetries int) error {
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			fmt.Printf("Retry %d/%d...\n", i, maxRetries)
			time.Sleep(time.Duration(i) * 2 * time.Second)
		}

		if err := tunnoxClient.Connect(); err != nil {
			if i == maxRetries-1 {
				return err
			}
			fmt.Printf("Connection failed: %v\n", err)
			continue
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded")
}

// monitorConnectionAndReconnect 监控连接状态并自动重连
func monitorConnectionAndReconnect(ctx context.Context, tunnoxClient *client.TunnoxClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	consecutiveFailures := 0
	maxFailures := 3

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !tunnoxClient.IsConnected() {
				consecutiveFailures++
				corelog.Warnf("Connection lost (failure %d/%d), attempting to reconnect...",
					consecutiveFailures, maxFailures)

				if err := tunnoxClient.Reconnect(); err != nil {
					corelog.Errorf("Reconnection failed: %v", err)

					if consecutiveFailures >= maxFailures {
						corelog.Errorf("Max reconnection attempts reached, giving up")
						return
					}
				} else {
					corelog.Infof("Reconnected successfully")
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

// formatBytes 格式化字节数
func formatBytes(bytes int64) string {
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
