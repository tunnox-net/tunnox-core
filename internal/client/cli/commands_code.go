package cli

import (
	"fmt"

	"tunnox-core/internal/client"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码管理命令（TargetClient）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cmdGenerateCode 生成连接码
func (c *CLI) cmdGenerateCode(args []string) {
	c.output.Header("🔑 Generate Connection Code")

	// 提示输入目标地址
	targetAddress, err := c.promptInput("Target Address (e.g., tcp://192.168.1.10:8080): ")
	if err != nil {
		return
	}
	if targetAddress == "" {
		c.output.Error("Target address cannot be empty")
		return
	}

	// 提示输入激活有效期
	activationTTLInput, err := c.promptInput("Activation TTL in minutes (default: 10): ")
	if err != nil {
		return
	}

	activationTTL := 10 * 60 // 默认10分钟
	if activationTTLInput != "" {
		minutes, err := ParseIntWithDefault(activationTTLInput, 10)
		if err != nil {
			c.output.Error("Invalid input: %v", err)
			return
		}
		activationTTL = minutes * 60
	}

	// 提示输入映射有效期
	mappingTTLInput, err := c.promptInput("Mapping TTL in days (default: 7): ")
	if err != nil {
		return
	}

	mappingTTL := 7 * 24 * 3600 // 默认7天
	if mappingTTLInput != "" {
		days, err := ParseIntWithDefault(mappingTTLInput, 7)
		if err != nil {
			c.output.Error("Invalid input: %v", err)
			return
		}
		mappingTTL = days * 24 * 3600
	}

	fmt.Println("")
	c.output.Info("Generating connection code...")

	// ✅ 通过指令通道发送命令
	resp, err := c.client.GenerateConnectionCode(&client.GenerateConnectionCodeRequest{
		TargetAddress: targetAddress,
		ActivationTTL: activationTTL,
		MappingTTL:    mappingTTL,
	})

	if err != nil {
		c.output.Error("Failed to generate code: %v", err)
		return
	}

	// 显示结果
	fmt.Println("")
	c.output.Success("Connection Code Generated!")
	c.output.Separator()
	c.output.KeyValue("Code", colorBold(resp.Code))
	c.output.KeyValue("Target", resp.TargetAddress)
	c.output.KeyValue("Expires At", resp.ExpiresAt)
	c.output.Separator()
	fmt.Println("")
	c.output.Info("Share this code with the ListenClient to create a tunnel mapping.")
	fmt.Println("")
}

// cmdListCodes 列出连接码
func (c *CLI) cmdListCodes(args []string) {
	c.output.Header("📋 Connection Codes")

	// ✅ 检查连接状态
	if !c.client.IsConnected() {
		c.output.Error("Not connected to server. Please connect first using 'connect' command.")
		return
	}

	// ✅ 通过指令通道发送命令
	resp, err := c.client.ListConnectionCodes()

	if err != nil {
		c.output.Error("Failed to list codes: %v", err)
		return
	}

	if len(resp.Codes) == 0 {
		c.output.Info("No connection codes found.")
		return
	}

	// 创建表格
	table := NewTable("CODE", "TARGET", "STATUS", "EXPIRES AT")

	for _, code := range resp.Codes {
		status := code.Status
		if code.Activated {
			status = colorSuccess("✅ " + status)
		}

		table.AddRow(
			Truncate(code.Code, 18),
			Truncate(code.TargetAddress, 35),
			status,
			FormatTime(code.ExpiresAt),
		)
	}

	table.Render()

	fmt.Println("")
	c.output.Info("Total: %d codes", resp.Total)
	fmt.Println("")
}
