package cli

import (
	"fmt"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 基础命令
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cmdHelp 显示帮助
func (c *CLI) cmdHelp(args []string) {
	if len(args) > 0 {
		c.showCommandHelp(args[0])
		return
	}

	c.output.Header("📖 Available Commands")

	fmt.Println("  General:")
	fmt.Println("    help, h, ?              Show this help message")
	fmt.Println("    connect, conn           Connect to server")
	fmt.Println("    disconnect, dc          Disconnect from server")
	fmt.Println("    status, st              Show client connection status")
	fmt.Println("    config                  Manage configuration")
	fmt.Println("    clear, cls              Clear screen")
	fmt.Println("    exit, quit, q           Exit CLI")
	fmt.Println("")
	fmt.Println("  Connection Code (TargetClient):")
	fmt.Println("    generate-code           Generate a connection code")
	fmt.Println("    list-codes              List all connection codes")
	fmt.Println("")
	fmt.Println("  Tunnel Mapping (ListenClient):")
	fmt.Println("    use-code <code>         Use a connection code to create mapping")
	fmt.Println("    list-mappings           List all tunnel mappings")
	fmt.Println("    show-mapping <id>       Show mapping details")
	fmt.Println("    delete-mapping <id>     Delete a mapping")
	fmt.Println("")
	c.output.Info("Type 'help <command>' for detailed help on a specific command")
	fmt.Println("")
}

// showCommandHelp 显示特定命令的帮助
func (c *CLI) showCommandHelp(cmd string) {
	c.output.Header(fmt.Sprintf("📖 Help: %s", cmd))

	switch cmd {
	case "generate-code", "gen":
		c.output.Plain("DESCRIPTION:")
		c.output.Plain("  Generates a one-time connection code that other clients can use")
		c.output.Plain("  to establish a tunnel mapping to this client.")
		fmt.Println("")
		c.output.Plain("USAGE:")
		c.output.Plain("  generate-code")
		fmt.Println("")
		c.output.Plain("INTERACTIVE:")
		c.output.Plain("  The command will prompt you for:")
		c.output.Plain("  - Target address (e.g., tcp://192.168.1.10:8080)")
		c.output.Plain("  - Activation TTL (how long the code is valid)")
		c.output.Plain("  - Mapping TTL (how long the mapping lasts)")

	case "use-code":
		c.output.Plain("DESCRIPTION:")
		c.output.Plain("  Activates a connection code to create a tunnel mapping.")
		fmt.Println("")
		c.output.Plain("USAGE:")
		c.output.Plain("  use-code <connection-code>")
		fmt.Println("")
		c.output.Plain("ARGUMENTS:")
		c.output.Plain("  <connection-code>    The code received from a TargetClient")

	case "list-mappings":
		c.output.Plain("DESCRIPTION:")
		c.output.Plain("  Lists all active tunnel mappings for this client.")
		fmt.Println("")
		c.output.Plain("USAGE:")
		c.output.Plain("  list-mappings [--type inbound|outbound]")
		fmt.Println("")
		c.output.Plain("OPTIONS:")
		c.output.Plain("  --type inbound     Show only inbound mappings (as TargetClient)")
		c.output.Plain("  --type outbound    Show only outbound mappings (as ListenClient)")

	case "config":
		c.output.Plain("DESCRIPTION:")
		c.output.Plain("  Manage client configuration.")
		fmt.Println("")
		c.output.Plain("USAGE:")
		c.output.Plain("  config list                 List all configuration")
		c.output.Plain("  config get <key>            Get a specific config value")
		c.output.Plain("  config set <key> <value>    Set a config value")
		c.output.Plain("  config reset <key>          Reset to default value")
		c.output.Plain("  config save [path]          Save config to file")
		c.output.Plain("  config reload [path]        Reload config from file")

	default:
		c.output.Warning("No detailed help available for '%s'", cmd)
		c.output.Info("Type 'help' to see all commands")
	}
	fmt.Println("")
}

// cmdExit 退出CLI
func (c *CLI) cmdExit(args []string) {
	uptime := time.Since(c.startTime)
	c.output.Success("Goodbye! (Uptime: %s)", FormatDuration(uptime))

	// 触发停止
	c.Stop()

	// 退出程序
	// 注意：这里不直接os.Exit，而是通过关闭readline来让主循环退出
}

// cmdClear 清屏
func (c *CLI) cmdClear(args []string) {
	// 使用ANSI转义序列清屏
	fmt.Print("\033[H\033[2J")
	c.printWelcome()
}

// cmdStatus 显示状态
func (c *CLI) cmdStatus(args []string) {
	c.output.Header("📊 Client Status")

	// 连接状态
	isConnected := c.client.IsConnected()
	if isConnected {
		c.output.KeyValue("Connection", colorSuccess("✅ Connected"))
	} else {
		c.output.KeyValue("Connection", colorError("❌ Disconnected"))
	}

	// TODO: 从client获取实际配置和状态
	c.output.KeyValue("Server", "localhost:7003")
	c.output.KeyValue("Protocol", "QUIC")
	c.output.KeyValue("Client ID", "10000001")
	c.output.KeyValue("Uptime", FormatDuration(time.Since(c.startTime)))

	fmt.Println("")
	c.output.KeyValue("Active Mappings", "2")
	c.output.KeyValue("Active Tunnels", "1")
	c.output.KeyValue("Bytes Sent", "1.2 MB")
	c.output.KeyValue("Bytes Received", "3.4 MB")

	fmt.Println("")
}

// cmdConnect 连接到服务器
func (c *CLI) cmdConnect(args []string) {
	// TODO: 支持指定服务器地址
	// if len(args) > 0 {
	//     c.client.SetServerAddress(args[0])
	// }

	c.output.Info("Connecting to server...")

	if err := c.client.Connect(); err != nil {
		c.output.Error("Connection failed: %v", err)
		return
	}

	c.output.Success("Connected successfully!")
}

// cmdDisconnect 断开与服务器的连接
func (c *CLI) cmdDisconnect(args []string) {
	if err := c.client.Disconnect(); err != nil {
		c.output.Warning("Disconnect warning: %v", err)
		return
	}

	c.output.Info("Disconnected from server")
}
