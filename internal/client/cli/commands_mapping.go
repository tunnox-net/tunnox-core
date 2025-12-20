package cli

import (
	"fmt"
	"strconv"
	"strings"

	"tunnox-core/internal/client"
	cloudutils "tunnox-core/internal/cloud/utils"
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
	c.output.Header(fmt.Sprintf("Activating Connection Code: %s", code))

	// 提示输入本地监听地址
	var listenAddr string
	for {
		addr, err := c.promptInput("Local Listen Address (e.g., 127.0.0.1:8888): ")
		if err == ErrCancelled {
			// Ctrl+C 静默返回
			return
		}
		if err != nil {
			return
		}
		if addr == "" {
			c.output.Error("Listen address cannot be empty")
			c.output.Info("Valid format: host:port (e.g., 127.0.0.1:8888) or just port number")
			continue
		}

		// 验证地址格式（如果只输入了端口号，会在后续处理中补全）
		// 这里先检查是否包含冒号，如果没有，假设是端口号
		if !strings.Contains(addr, ":") {
			// 只输入了端口号，使用默认地址
			port, err := strconv.Atoi(addr)
			if err != nil || port < 1 || port > 65535 {
				c.output.Error("Invalid port number: %s", addr)
				c.output.Info("Port must be a number between 1 and 65535")
				continue
			}
			listenAddr = fmt.Sprintf("127.0.0.1:%d", port)
		} else {
			// 验证完整地址格式
			_, _, err := cloudutils.ParseListenAddress(addr)
			if err != nil {
				c.output.Error("Invalid listen address: %v", err)
				c.output.Info("Valid format: host:port (e.g., 127.0.0.1:8888)")
				continue
			}
			listenAddr = addr
		}

		// 地址有效，退出循环
		break
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

	header := "Tunnel Mappings"
	if direction != "" {
		header = fmt.Sprintf("Tunnel Mappings (%s)", direction)
	} else if mappingType != "" {
		header = fmt.Sprintf("Tunnel Mappings (%s)", mappingType)
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
	c.output.Header(fmt.Sprintf("Mapping Details: %s", mappingID))

	mapping, err := c.client.GetMapping(mappingID)
	if err != nil {
		c.output.Error("Failed to get mapping: %v", err)
		return
	}

	// 使用表格显示映射详情
	table := NewTable("PROPERTY", "VALUE")

	typeIcon := "📤"
	if mapping.Type == "inbound" {
		typeIcon = "📥"
	}

	table.AddRow("Mapping ID", mapping.MappingID)
	table.AddRow("Type", typeIcon+" "+mapping.Type)
	table.AddRow("Target Address", mapping.TargetAddress)
	table.AddRow("Listen Address", mapping.ListenAddress)
	table.AddRow("Status", mapping.Status)
	table.AddRow("Created At", FormatTime(mapping.CreatedAt))
	table.AddRow("Expires At", FormatTime(mapping.ExpiresAt))
	table.AddRow("Bytes Sent", FormatBytes(mapping.BytesSent))
	table.AddRow("Bytes Received", FormatBytes(mapping.BytesReceived))

	table.Render()
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
	c.output.Header(fmt.Sprintf("Delete Mapping: %s", mappingID))

	if !c.promptConfirm("Are you sure?") {
		// 静默返回，不显示警告
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
