package cli

import (
	"fmt"

	"tunnox-core/internal/client"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 隧道映射管理命令（ListenClient）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cmdUseCode 使用连接码
func (c *CLI) cmdUseCode(args []string) {
	if len(args) == 0 {
		c.output.Error("Missing connection code")
		c.output.Info("Usage: use-code <connection-code>")
		return
	}

	if !c.client.IsConnected() {
		c.output.Error("Not connected to server. Please connect first using 'connect' command.")
		return
	}

	code := args[0]
	c.output.Header(fmt.Sprintf("🔓 Activating Connection Code: %s", code))

	// 提示输入本地监听地址
	listenAddr, err := c.promptInput("Local Listen Address (e.g., 127.0.0.1:8888): ")
	if err != nil {
		return
	}
	if listenAddr == "" {
		c.output.Error("Listen address cannot be empty")
		return
	}

	fmt.Println("")
	c.output.Info("Activating connection code...")

	resp, err := c.client.ActivateConnectionCode(&client.ActivateConnectionCodeRequest{
		Code:          code,
		ListenAddress: listenAddr,
	})

	if err != nil {
		c.output.Error("Failed to activate code: %v", err)
		return
	}

	// 显示结果
	fmt.Println("")
	c.output.Success("Connection Code Activated!")
	c.output.Separator()
	c.output.KeyValue("Mapping ID", resp.MappingID)
	c.output.KeyValue("Target", resp.TargetAddress)
	c.output.KeyValue("Listen", resp.ListenAddress)
	c.output.KeyValue("Expires At", resp.ExpiresAt)
	c.output.Separator()
	fmt.Println("")
	c.output.Info("Tunnel mapping created! You can now connect to the local address.")
	fmt.Println("")
}

// cmdListMappings 列出隧道映射
func (c *CLI) cmdListMappings(args []string) {
	// 检查连接状态
	if !c.client.IsConnected() {
		c.output.Error("Not connected to server. Please connect first using 'connect' command.")
		return
	}

	// 解析参数
	var direction, mappingType string
	for i, arg := range args {
		if arg == "--type" && i+1 < len(args) {
			mappingType = args[i+1]
		}
		if arg == "--direction" && i+1 < len(args) {
			direction = args[i+1]
		}
	}

	header := "🔗 Tunnel Mappings"
	if direction != "" {
		header = fmt.Sprintf("🔗 Tunnel Mappings (%s)", direction)
	} else if mappingType != "" {
		header = fmt.Sprintf("🔗 Tunnel Mappings (%s)", mappingType)
	}
	c.output.Header(header)

	// 通过指令通道调用
	req := &client.ListMappingsRequest{
		Direction: direction,
		Type:      mappingType,
	}
	resp, err := c.client.ListMappings(req)

	if err != nil {
		c.output.Error("Failed to list mappings: %v", err)
		return
	}

	if len(resp.Mappings) == 0 {
		c.output.Info("No tunnel mappings found.")
		return
	}

	// 创建表格
	table := NewTable("MAPPING ID", "TYPE", "TARGET", "LISTEN", "STATUS", "BYTES")

	for _, mapping := range resp.Mappings {
		typeIcon := "📤"
		if mapping.Type == "inbound" {
			typeIcon = "📥"
		}

		bytesStr := formatBytes(mapping.BytesSent + mapping.BytesReceived)

		table.AddRow(
			Truncate(mapping.MappingID, 18),
			typeIcon+" "+mapping.Type,
			Truncate(mapping.TargetAddress, 30),
			Truncate(mapping.ListenAddress, 20),
			mapping.Status,
			bytesStr,
		)
	}

	table.Render()

	fmt.Println("")
	c.output.Info("Total: %d mappings", resp.Total)
	fmt.Println("")
}

// cmdShowMapping 显示映射详情
func (c *CLI) cmdShowMapping(args []string) {
	if len(args) == 0 {
		c.output.Error("Missing mapping ID")
		c.output.Info("Usage: show-mapping <mapping-id>")
		return
	}

	if !c.client.IsConnected() {
		c.output.Error("Not connected to server. Please connect first using 'connect' command.")
		return
	}

	mappingID := args[0]
	c.output.Header(fmt.Sprintf("📝 Mapping Details: %s", mappingID))

	mapping, err := c.client.GetMapping(mappingID)
	if err != nil {
		c.output.Error("Failed to get mapping: %v", err)
		return
	}

	c.output.KeyValue("Mapping ID", mapping.MappingID)
	c.output.KeyValue("Type", mapping.Type)
	c.output.KeyValue("Target Address", mapping.TargetAddress)
	c.output.KeyValue("Listen Address", mapping.ListenAddress)
	c.output.KeyValue("Status", mapping.Status)
	c.output.KeyValue("Created At", FormatTime(mapping.CreatedAt))
	c.output.KeyValue("Expires At", FormatTime(mapping.ExpiresAt))

	fmt.Println("")
	c.output.KeyValue("Bytes Sent", FormatBytes(mapping.BytesSent))
	c.output.KeyValue("Bytes Received", FormatBytes(mapping.BytesReceived))

	fmt.Println("")
}

// cmdDeleteMapping 删除映射
func (c *CLI) cmdDeleteMapping(args []string) {
	if len(args) == 0 {
		c.output.Error("Missing mapping ID")
		c.output.Info("Usage: delete-mapping <mapping-id>")
		return
	}

	if !c.client.IsConnected() {
		c.output.Error("Not connected to server. Please connect first using 'connect' command.")
		return
	}

	mappingID := args[0]
	c.output.Header(fmt.Sprintf("🗑️ Delete Mapping: %s", mappingID))

	if !c.promptConfirm("Are you sure?") {
		c.output.Warning("Cancelled")
		return
	}

	c.output.Info("Deleting mapping...")

	if err := c.client.DeleteMapping(mappingID); err != nil {
		c.output.Error("Failed to delete mapping: %v", err)
		return
	}

	c.output.Success("Mapping deleted successfully!")
	fmt.Println("")
}
