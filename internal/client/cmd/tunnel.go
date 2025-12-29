package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"tunnox-core/internal/client"
	"tunnox-core/internal/client/cli"

	"github.com/spf13/cobra"
)

// httpCmd 创建 HTTP 隧道
var httpCmd = &cobra.Command{
	Use:   "http [port]",
	Short: "Create an HTTP tunnel for local port",
	Long: `Create an HTTP tunnel that generates a connection code.
Share this code with others to allow them to access your local HTTP service.

Example:
  tunnox http 8080              # Expose local port 8080 via HTTP tunnel
  tunnox http 3000 --server localhost:7001`,
	Args: cobra.MaximumNArgs(1),
	Run:  runHTTPTunnel,
}

// tcpCmd 创建 TCP 隧道
var tcpCmd = &cobra.Command{
	Use:   "tcp [port]",
	Short: "Create a TCP tunnel for local port",
	Long: `Create a TCP tunnel that generates a connection code.
Share this code with others to allow them to access your local TCP service.

Example:
  tunnox tcp 3306               # Expose MySQL on port 3306
  tunnox tcp 22                 # Expose SSH on port 22
  tunnox tcp 5432 --server localhost:7001`,
	Args: cobra.MaximumNArgs(1),
	Run:  runTCPTunnel,
}

// udpCmd 创建 UDP 隧道
var udpCmd = &cobra.Command{
	Use:   "udp [port]",
	Short: "Create a UDP tunnel for local port",
	Long: `Create a UDP tunnel that generates a connection code.
Share this code with others to allow them to access your local UDP service.

Example:
  tunnox udp 53                 # Expose DNS on port 53
  tunnox udp 1194               # Expose OpenVPN on port 1194`,
	Args: cobra.MaximumNArgs(1),
	Run:  runUDPTunnel,
}

// socksCmd 创建 SOCKS5 代理隧道
var socksCmd = &cobra.Command{
	Use:   "socks",
	Short: "Create a SOCKS5 proxy tunnel",
	Long: `Create a SOCKS5 proxy tunnel that generates a connection code.
Share this code with others to allow them to use your network as a proxy.

Example:
  tunnox socks                  # Create SOCKS5 proxy tunnel`,
	Run: runSOCKSTunnel,
}

func runHTTPTunnel(cmd *cobra.Command, args []string) {
	runTunnel("http", args)
}

func runTCPTunnel(cmd *cobra.Command, args []string) {
	runTunnel("tcp", args)
}

func runUDPTunnel(cmd *cobra.Command, args []string) {
	runTunnel("udp", args)
}

func runSOCKSTunnel(cmd *cobra.Command, args []string) {
	runTunnel("socks5", args)
}

// runTunnel 运行隧道创建流程
func runTunnel(tunnelType string, args []string) {
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

	output := cli.NewOutput(false)

	// 获取端口
	var port int
	var err error

	if len(args) > 0 {
		port, err = strconv.Atoi(args[0])
		if err != nil || port < 1 || port > 65535 {
			output.Error("Invalid port: %s (must be between 1 and 65535)", args[0])
			os.Exit(1)
		}
	} else if tunnelType != "socks5" {
		// 交互式输入端口
		fmt.Println()
		output.Header(fmt.Sprintf("Create %s Tunnel", tunnelType))

		for {
			input, err := promptInput("Enter local port to expose: ")
			if err != nil {
				return
			}

			port, err = strconv.Atoi(input)
			if err != nil || port < 1 || port > 65535 {
				output.Error("Invalid port: %s (must be between 1 and 65535)", input)
				continue
			}
			break
		}
	}

	// 确保连接
	if err := ensureConnected(ctx); err != nil {
		output.Error("Connection failed: %v", err)
		os.Exit(1)
	}

	// 构建目标地址
	var targetAddress string
	if tunnelType == "socks5" {
		targetAddress = "socks5://0.0.0.0:0"
	} else {
		targetAddress = fmt.Sprintf("%s://127.0.0.1:%d", tunnelType, port)
	}

	// 生成连接码
	fmt.Println()
	output.Info("Generating connection code...")

	resp, err := tunnoxClient.GenerateConnectionCode(&client.GenerateConnectionCodeRequest{
		TargetAddress: targetAddress,
		ActivationTTL: 10 * 60,       // 10 分钟
		MappingTTL:    7 * 24 * 3600, // 7 天
	})

	if err != nil {
		output.Error("Failed to generate code: %v", err)
		os.Exit(1)
	}

	// 显示结果
	fmt.Println()
	output.Success("Connection Code Generated!")
	output.Separator()
	output.KeyValue("Code", cli.ColorBold(resp.Code))
	output.KeyValue("Target", resp.TargetAddress)
	output.KeyValue("Expires At", resp.ExpiresAt)
	output.Separator()
	fmt.Println()
	output.Info("Share this code with others to allow them to connect.")
	output.Info("They can use: tunnox code use %s", resp.Code)
	fmt.Println()

	// 如果指定了交互模式，进入 CLI
	if interactive {
		runInteractiveCLI()
	}
}

// promptInput 简化版输入提示
func promptInput(prompt string) (string, error) {
	fmt.Print(prompt)
	var input string
	_, err := fmt.Scanln(&input)
	return input, err
}
