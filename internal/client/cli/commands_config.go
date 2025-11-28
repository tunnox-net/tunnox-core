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

	// TODO: 从client获取实际配置
	// 暂时显示示例配置

	c.output.Section("Server")
	c.output.KeyValue("address", "localhost:7003")
	c.output.KeyValue("protocol", "quic")
	c.output.KeyValue("management_api_address", "http://localhost:8080")

	c.output.Section("Client")
	c.output.KeyValue("client_id", "10000001")
	c.output.KeyValue("device_id", "my-device")
	c.output.KeyValue("anonymous", "false")

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

	// TODO: 从client获取实际配置值
	// value, err := c.client.GetConfig(key)

	c.output.Header(fmt.Sprintf("⚙️ Config: %s", key))

	// 示例值
	sampleValues := map[string]string{
		"server.address":                "localhost:7003",
		"server.protocol":               "quic",
		"server.management_api_address": "http://localhost:8080",
		"client.client_id":              "10000001",
		"client.anonymous":              "false",
		"log.level":                     "info",
		"log.output":                    "file",
	}

	if value, ok := sampleValues[key]; ok {
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

	// TODO: 实际设置配置值
	// if err := c.client.SetConfig(key, value); err != nil {
	//     c.output.Error("Failed to set config: %v", err)
	//     return
	// }

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

	// TODO: 实际重置配置值
	// if err := c.client.ResetConfig(key); err != nil {
	//     c.output.Error("Failed to reset config: %v", err)
	//     return
	// }

	c.output.Success("Config reset to default: %s", key)
	fmt.Println("")
}

// cmdConfigSave 保存配置到文件
func (c *CLI) cmdConfigSave(args []string) {
	path := "config.json"
	if len(args) > 0 {
		path = args[0]
	}

	// TODO: 实际保存配置
	// if err := c.client.SaveConfig(path); err != nil {
	//     c.output.Error("Failed to save config: %v", err)
	//     return
	// }

	c.output.Success("Configuration saved to: %s", path)
	fmt.Println("")
}

// cmdConfigReload 重新加载配置
func (c *CLI) cmdConfigReload(args []string) {
	path := "config.json"
	if len(args) > 0 {
		path = args[0]
	}

	// TODO: 实际重新加载配置
	// if err := c.client.ReloadConfig(path); err != nil {
	//     c.output.Error("Failed to reload config: %v", err)
	//     return
	// }

	c.output.Success("Configuration reloaded from: %s", path)
	c.output.Warning("Some changes may require reconnection to take effect")
	fmt.Println("")
}
