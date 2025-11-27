package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"tunnox-core/internal/client"
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
	help := flag.Bool("h", false, "show help")

	flag.Parse()

	// 显示帮助
	if *help {
		showHelp()
		os.Exit(0)
	}

	// 加载配置
	config, err := loadOrCreateConfig(*configFile, *protocol, *serverAddr, *clientID, *deviceID, *authToken, *anonymous)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 显示连接信息
	fmt.Printf("🚀 Tunnox Client Starting...\n")
	fmt.Printf("   Protocol: %s\n", config.Server.Protocol)
	fmt.Printf("   Server:   %s\n", config.Server.Address)
	if config.Anonymous {
		fmt.Printf("   Mode:     Anonymous (device: %s)\n", config.DeviceID)
	} else {
		fmt.Printf("   Mode:     Authenticated (client_id: %d)\n", config.ClientID)
	}
	fmt.Printf("\n")

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建客户端
	tunnoxClient := client.NewClient(ctx, config)

	// 连接到服务器
	if err := tunnoxClient.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Failed to connect to server: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Connected to server successfully!\n\n")

	// 连接成功后，客户端会自动从服务器获取映射配置

	// 等待终止信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		utils.Infof("Client: received signal %v, shutting down...", sig)
	case <-ctx.Done():
		utils.Infof("Client: context cancelled, shutting down...")
	}

	// 停止客户端
	tunnoxClient.Stop()
	utils.Infof("Client: shutdown complete")
}

// loadOrCreateConfig 加载或创建配置
func loadOrCreateConfig(configFile, protocol, serverAddr string, clientID int64, deviceID, authToken string, anonymous bool) (*client.ClientConfig, error) {
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

	// 验证配置
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// validateConfig 验证配置
func validateConfig(config *client.ClientConfig) error {
	if config.Server.Address == "" {
		return fmt.Errorf("server address is required")
	}
	if config.Server.Protocol == "" {
		config.Server.Protocol = "tcp"
	}

	// 规范化协议名称
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
    -config <file>     Path to config file (optional)
    -p <protocol>      Protocol: tcp/websocket/ws/udp/quic
    -s <address>       Server address (e.g., localhost:7001)
    -id <client_id>    Client ID for authenticated mode
    -token <token>     Auth token for authenticated mode
    -device <id>       Device ID for anonymous mode
    -anonymous         Use anonymous mode
    -h                 Show this help

EXAMPLES:
    # Use config file
    tunnox-client -config client-config.yaml

    # Quick start with TCP
    tunnox-client -p tcp -s localhost:7001 -anonymous

    # Quick start with WebSocket
    tunnox-client -p ws -s localhost:7000 -anonymous

    # Quick start with UDP
    tunnox-client -p udp -s localhost:7002 -anonymous

    # Quick start with QUIC
    tunnox-client -p quic -s localhost:7003 -anonymous

    # Authenticated mode
    tunnox-client -p tcp -s localhost:7001 -id 10000001 -token "your-jwt-token"

    # Override config file settings
    tunnox-client -config client.yaml -p websocket -s example.com:8080

NOTES:
    - Command line options override config file settings
    - If no config file is specified, uses client-config.yaml if it exists
    - Default protocol is tcp if not specified
    - Anonymous mode is used if no client_id/token is provided
`)
}
