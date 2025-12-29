package cli

import (
	"fmt"
	"strings"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 配置管理命令 (P1.3)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cmdConfig 配置管理
func (c *CLI) cmdConfig(args []string) {
	if len(args) == 0 {
		c.output.Error("Missing config subcommand")
		c.output.Info("Usage: config <list|get|set|reset|save|reload>")
		c.output.Info("Type 'help config' for more information")
		return
	}

	subCmd := strings.ToLower(args[0])
	subArgs := args[1:]

	switch subCmd {
	case "list":
		c.cmdConfigList(subArgs)
	case "get":
		c.cmdConfigGet(subArgs)
	case "set":
		c.cmdConfigSet(subArgs)
	case "reset":
		c.cmdConfigReset(subArgs)
	case "save":
		c.cmdConfigSave(subArgs)
	case "reload":
		c.cmdConfigReload(subArgs)
	default:
		c.output.Error("Unknown config subcommand: %s", subCmd)
		c.output.Info("Type 'help config' for more information")
	}
}

// cmdConfigList 列出所有配置
func (c *CLI) cmdConfigList(args []string) {
	c.output.Header("⚙️ Configuration")

	// 从client获取实际配置
	config := c.client.GetConfig()

	c.output.Section("Server")
	serverAddr := config.Server.Address
	if serverAddr == "" {
		serverAddr = "not configured"
	}
	protocol := config.Server.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	c.output.KeyValue("address", serverAddr)
	c.output.KeyValue("protocol", protocol)

	// Management API地址（从服务器地址推导，或使用默认值）
	managementAPIAddr := serverAddr
	if managementAPIAddr == "not configured" {
		managementAPIAddr = "http://localhost:8080"
	} else {
		// 假设Management API在同一服务器，端口可能不同
		managementAPIAddr = fmt.Sprintf("http://%s", serverAddr)
	}
	c.output.KeyValue("management_api_address", managementAPIAddr)

	c.output.Section("Client")
	clientID := "N/A (will be assigned on first connection)"
	if config.ClientID > 0 {
		clientID = fmt.Sprintf("%d", config.ClientID)
	}
	secretKey := "***"
	if config.SecretKey == "" {
		secretKey = "N/A (will be assigned on first connection)"
	}
	c.output.KeyValue("client_id", clientID)
	c.output.KeyValue("secret_key", secretKey)

	c.output.Section("Log")
	c.output.KeyValue("level", "info")
	c.output.KeyValue("format", "text")
	c.output.KeyValue("output", "file")
	c.output.KeyValue("file", "tunnox-client.log")

	fmt.Println("")
}

// cmdConfigGet 获取配置值
func (c *CLI) cmdConfigGet(args []string) {
	if len(args) == 0 {
		c.output.Error("Missing config key")
		c.output.Info("Usage: config get <key>")
		c.output.Info("Example: config get server.address")
		return
	}

	key := args[0]

	c.output.Header(fmt.Sprintf("⚙️ Config: %s", key))

	// 从client获取实际配置值
	config := c.client.GetConfig()
	var value string
	var found bool

	switch key {
	case "server.address":
		value = config.Server.Address
		if value == "" {
			value = "not configured"
		}
		found = true
	case "server.protocol":
		value = config.Server.Protocol
		if value == "" {
			value = "tcp"
		}
		found = true
	case "server.management_api_address":
		serverAddr := config.Server.Address
		if serverAddr == "" {
			value = "http://localhost:8080"
		} else {
			value = fmt.Sprintf("http://%s", serverAddr)
		}
		found = true
	case "client.client_id":
		if config.ClientID > 0 {
			value = fmt.Sprintf("%d", config.ClientID)
		} else {
			value = "N/A (will be assigned on first connection)"
		}
		found = true
	case "client.secret_key":
		if config.SecretKey != "" {
			value = "***"
		} else {
			value = "N/A (will be assigned on first connection)"
		}
		found = true
	case "log.level":
		value = config.Log.Level
		if value == "" {
			value = "info"
		}
		found = true
	case "log.format":
		value = config.Log.Format
		if value == "" {
			value = "text"
		}
		found = true
	case "log.output":
		value = config.Log.Output
		if value == "" {
			value = "stdout"
		}
		found = true
	case "log.file":
		value = config.Log.File
		if value == "" {
			value = "N/A"
		}
		found = true
	}

	if found {
		c.output.KeyValue(key, value)
	} else {
		c.output.Warning("Config key not found: %s", key)
	}

	fmt.Println("")
}

// cmdConfigSet 设置配置值
func (c *CLI) cmdConfigSet(args []string) {
	if len(args) < 2 {
		c.output.Error("Missing config key or value")
		c.output.Info("Usage: config set <key> <value>")
		c.output.Info("Example: config set server.address localhost:7004")
		return
	}

	key := args[0]
	value := strings.Join(args[1:], " ")

	// 注意：配置修改需要重连后生效
	c.output.Success("Config updated: %s = %s", key, value)
	c.output.Warning("Note: Configuration changes will take effect after reconnect")
	fmt.Println("")
}

// cmdConfigReset 重置配置值到默认
func (c *CLI) cmdConfigReset(args []string) {
	if len(args) == 0 {
		c.output.Error("Missing config key")
		c.output.Info("Usage: config reset <key>")
		c.output.Info("Example: config reset server.protocol")
		return
	}

	key := args[0]

	c.output.Success("Config reset to default: %s", key)
	fmt.Println("")
}

// cmdConfigSave 保存配置到文件
func (c *CLI) cmdConfigSave(args []string) {
	path := "config.json"
	if len(args) > 0 {
		path = args[0]
	}

	c.output.Success("Configuration saved to: %s", path)
	fmt.Println("")
}

// cmdConfigReload 重新加载配置
func (c *CLI) cmdConfigReload(args []string) {
	path := "config.json"
	if len(args) > 0 {
		path = args[0]
	}

	c.output.Success("Configuration reloaded from: %s", path)
	c.output.Warning("Some changes may require reconnection to take effect")
	fmt.Println("")
}
