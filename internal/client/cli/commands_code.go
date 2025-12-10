package cli

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"tunnox-core/internal/client"
	cloudutils "tunnox-core/internal/cloud/utils"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 连接码管理命令（TargetClient）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cmdGenerateCode 生成连接码
func (c *CLI) cmdGenerateCode(args []string) {
	c.output.Header("🔑 Generate Connection Code")

	// 1. 选择协议类型
	// 先输出一个空行，确保与readline prompt分开
	fmt.Println("")
	protocolOptions := []string{"TCP", "UDP", "SOCKS5", "Back"}
	protocolIndex, err := PromptSelect("Select Protocol:", protocolOptions)
	if err != nil || protocolIndex < 0 {
		// 静默返回，不显示警告
		return
	}
	
	// If "Back" is selected
	if protocolIndex == len(protocolOptions)-1 {
		return
	}
	
	fmt.Println("") // 选择后也输出空行

	selectedProtocol := strings.ToLower(protocolOptions[protocolIndex])
	var targetAddress string

	// 2. 根据协议类型决定是否需要输入地址
	if selectedProtocol == "socks5" {
		// SOCKS5 不需要目标地址
		targetAddress = "socks5://0.0.0.0:0"
		c.output.Info("SOCKS5 proxy selected (dynamic targets)")
	} else {
		// TCP/UDP 需要输入目标地址（只需输入 host:port，协议会自动添加）
		prompt := fmt.Sprintf("Target Address (e.g., 192.168.1.10:8080): ")
		
		for {
			addr, err := c.promptInput(prompt)
			if err == ErrCancelled {
				// Ctrl+C 静默返回
				return
			}
			if err != nil {
				return
			}
			
			if addr == "" {
				c.output.Error("Target address cannot be empty")
				c.output.Info("Valid format: host:port (e.g., 192.168.1.10:8080)")
				continue
			}

			// 清理地址，移除可能的控制字符
			addr = strings.TrimSpace(addr)
			
			// 如果用户输入了协议前缀，先移除它（因为我们已经在选择协议时确定了）
			if strings.Contains(addr, "://") {
				parts := strings.Split(addr, "://")
				if len(parts) == 2 {
					addr = strings.TrimSpace(parts[1]) // 只取地址部分，并清理
				}
			}

			// 先验证地址格式（host:port）
			if !strings.Contains(addr, ":") {
				c.output.Error("Invalid address format: missing port")
				c.output.Info("Valid format: host:port (e.g., 192.168.1.10:8080)")
				continue
			}

			// 验证 host:port 格式
			host, portStr, err := net.SplitHostPort(addr)
			if err != nil {
				c.output.Error("Invalid address format: %v", err)
				c.output.Info("Valid format: host:port (e.g., 192.168.1.10:8080)")
				continue
			}

			// 验证端口号
			port, err := strconv.Atoi(portStr)
			if err != nil {
				c.output.Error("Invalid port number: %s", portStr)
				c.output.Info("Port must be a number between 1 and 65535")
				continue
			}
			if port < 1 || port > 65535 {
				c.output.Error("Port out of range: %d (must be between 1 and 65535)", port)
				continue
			}

			// 验证主机地址不为空
			if host == "" {
				c.output.Error("Invalid address: host cannot be empty")
				c.output.Info("Valid format: host:port (e.g., 192.168.1.10:8080)")
				continue
			}

			// 自动添加协议前缀
			fullAddr := fmt.Sprintf("%s://%s:%d", selectedProtocol, host, port)

			// 校验地址格式
			_, _, protocol, err := cloudutils.ParseTargetAddress(fullAddr)
			if err != nil {
				c.output.Error("Invalid target address: %v", err)
				c.output.Info("Valid format: host:port (e.g., 192.168.1.10:8080)")
				continue
			}

			// 验证协议匹配（应该总是匹配，因为我们添加的）
			if protocol != selectedProtocol {
				c.output.Error("Protocol mismatch: expected %s, got %s", selectedProtocol, protocol)
				continue
			}

			targetAddress = fullAddr
			break
		}
	}

	// 提示输入激活有效期
	activationTTLInput, err := c.promptInput("Activation TTL in minutes (default: 10): ")
	if err == ErrCancelled {
		return
	}
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
	if err == ErrCancelled {
		return
	}
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
	c.output.KeyValue("Code Expires At", resp.ExpiresAt)
	c.output.KeyValue("Mapping Expires At", resp.MappingExpiresAt)
	c.output.Separator()
	fmt.Println("")
	c.output.Info("Share this code with the ListenClient to create a tunnel mapping.")
	c.output.Info("Code must be activated within %d minutes.", resp.ActivationTTLMinutes)
	c.output.Info("Once activated, the mapping will be valid for %d days.", resp.MappingTTLDays)
	fmt.Println("")
}

// cmdListCodes 列出连接码
func (c *CLI) cmdListCodes(args []string) {
	c.output.Header("Connection Codes")

	if !c.client.IsConnected() {
		c.output.Error("Not connected to server. Please connect first using 'connect' command.")
		return
	}

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
	table := NewTable("CODE", "TARGET", "STATUS", "ACTIVATED BY", "EXPIRES AT")

	for _, code := range resp.Codes {
		// 客户端再次过滤：跳过已过期的连接码（双重保险）
		if code.Status == "expired" && !code.Activated {
			continue
		}

		// 格式化状态 - 更清晰的状态显示
		var status string
		if code.Activated {
			// 已激活
			status = colorSuccess("activated")
		} else if code.Status == "available" || code.Status == "active" {
			// 未激活但可用
			status = "available"
		} else if code.Status == "revoked" {
			// 已撤销（未过期）
			status = colorWarning("revoked")
		} else {
			status = code.Status
		}

		// 格式化激活者信息
		activatedBy := "-"
		if code.Activated && code.ActivatedBy != nil {
			activatedBy = fmt.Sprintf("client-%d", *code.ActivatedBy)
		}

		table.AddRow(
			Truncate(code.Code, 18),
			Truncate(code.TargetAddress, 35),
			status,
			Truncate(activatedBy, 15),
			FormatTime(code.ExpiresAt),
		)
	}

	table.Render()

	fmt.Println("")
	c.output.Info("Total: %d codes", resp.Total)
	fmt.Println("")
}
