package cli

import (
	"fmt"
	"net/url"
	"strings"

	"tunnox-core/internal/client"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HTTP 域名注册命令
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// cmdRegisterDomain 注册 HTTP 域名映射
func (c *CLI) cmdRegisterDomain(args []string) {
	c.output.Header("🌐 Register HTTP Domain")

	if !c.client.IsConnected() {
		c.output.Error("Not connected to server. Please connect first using 'connect' command.")
		return
	}

	// 1. 获取可用的基础域名列表
	fmt.Println("")
	c.output.Info("Fetching available base domains...")

	baseDomainsResp, err := c.client.GetBaseDomains()
	if err != nil {
		c.output.Error("Failed to get base domains: %v", err)
		return
	}

	if len(baseDomainsResp.BaseDomains) == 0 {
		c.output.Error("No base domains available. Please contact administrator.")
		return
	}

	// 2. 让用户选择基础域名
	domainOptions := make([]string, 0, len(baseDomainsResp.BaseDomains)+1)
	for _, domain := range baseDomainsResp.BaseDomains {
		if domain.Description != "" {
			domainOptions = append(domainOptions, fmt.Sprintf("%s (%s)", domain.Domain, domain.Description))
		} else {
			domainOptions = append(domainOptions, domain.Domain)
		}
	}
	domainOptions = append(domainOptions, "Back")

	domainIndex, err := PromptSelect("Select Base Domain:", domainOptions)
	if err != nil || domainIndex < 0 {
		return
	}

	// If "Back" is selected
	if domainIndex == len(domainOptions)-1 {
		return
	}

	selectedBaseDomain := baseDomainsResp.BaseDomains[domainIndex].Domain
	fmt.Println("")

	// 3. 让用户选择生成随机子域名还是自定义
	subdomainOptions := []string{"Generate Random Subdomain", "Enter Custom Subdomain", "Back"}
	subdomainChoice, err := PromptSelect("Subdomain Option:", subdomainOptions)
	if err != nil || subdomainChoice < 0 {
		return
	}

	// If "Back" is selected
	if subdomainChoice == len(subdomainOptions)-1 {
		return
	}

	var subdomain string
	var fullDomain string

	if subdomainChoice == 0 {
		// 生成随机子域名
		fmt.Println("")
		c.output.Info("Generating random subdomain...")

		genResp, err := c.client.GenSubdomain(selectedBaseDomain)
		if err != nil {
			c.output.Error("Failed to generate subdomain: %v", err)
			return
		}

		subdomain = genResp.Subdomain
		fullDomain = genResp.FullDomain
		c.output.Success("Generated subdomain: %s", colorBold(fullDomain))

		// 询问是否接受
		fmt.Println("")
		if !c.promptConfirm(fmt.Sprintf("Accept subdomain '%s'?", fullDomain)) {
			// 用户不接受，让其输入自定义
			c.output.Info("You can enter a custom subdomain instead.")
			subdomainChoice = 1 // 切换到自定义输入模式
		}
	}

	if subdomainChoice == 1 {
		// 自定义子域名
		fmt.Println("")
		for {
			input, err := c.promptInput("Enter desired subdomain prefix: ")
			if err == ErrCancelled {
				return
			}
			if err != nil {
				return
			}

			// 验证子域名格式
			input = strings.TrimSpace(strings.ToLower(input))
			if input == "" {
				c.output.Error("Subdomain cannot be empty")
				continue
			}

			// 简单验证：只允许字母数字和短横线
			if !isValidSubdomain(input) {
				c.output.Error("Invalid subdomain format. Use only letters, numbers, and hyphens.")
				c.output.Info("Example: my-app, app123, test-service")
				continue
			}

			// 检查子域名是否可用
			c.output.Info("Checking availability...")
			checkResp, err := c.client.CheckSubdomain(&client.CheckSubdomainRequest{
				Subdomain:  input,
				BaseDomain: selectedBaseDomain,
			})
			if err != nil {
				c.output.Error("Failed to check subdomain: %v", err)
				continue
			}

			if !checkResp.Available {
				c.output.Error("Subdomain '%s' is not available. Please try another.", checkResp.FullDomain)
				continue
			}

			subdomain = input
			fullDomain = checkResp.FullDomain
			c.output.Success("Subdomain '%s' is available!", colorBold(fullDomain))
			break
		}
	}

	// 4. 输入本地目标地址
	fmt.Println("")
	var targetURL string
	for {
		input, err := c.promptInput("Enter local target URL (e.g., http://localhost:8080): ")
		if err == ErrCancelled {
			return
		}
		if err != nil {
			return
		}

		input = strings.TrimSpace(input)
		if input == "" {
			c.output.Error("Target URL cannot be empty")
			continue
		}

		// 验证 URL 格式
		parsed, err := url.Parse(input)
		if err != nil {
			c.output.Error("Invalid URL format: %v", err)
			c.output.Info("Example: http://localhost:8080, https://192.168.1.10:443")
			continue
		}

		// 必须是 http 或 https
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			c.output.Error("URL must use http:// or https:// scheme")
			continue
		}

		// 必须有主机
		if parsed.Host == "" {
			c.output.Error("URL must include host (e.g., localhost:8080)")
			continue
		}

		targetURL = input
		break
	}

	// 5. 可选：设置过期时间
	fmt.Println("")
	mappingTTLInput, err := c.promptInput("Mapping TTL in days (default: 7, 0 for no expiration): ")
	if err == ErrCancelled {
		return
	}

	mappingTTL := 7 * 24 * 3600 // 默认7天
	if mappingTTLInput != "" {
		days, err := ParseIntWithDefault(mappingTTLInput, 7)
		if err != nil {
			c.output.Error("Invalid input: %v", err)
			return
		}
		if days == 0 {
			mappingTTL = 0 // 无过期
		} else {
			mappingTTL = days * 24 * 3600
		}
	}

	// 6. 确认创建
	fmt.Println("")
	c.output.Header("📋 Domain Mapping Summary")
	c.output.Separator()
	c.output.KeyValue("Domain", colorBold(fullDomain))
	c.output.KeyValue("Target URL", targetURL)
	if mappingTTL > 0 {
		c.output.KeyValue("TTL", fmt.Sprintf("%d days", mappingTTL/86400))
	} else {
		c.output.KeyValue("TTL", "No expiration")
	}
	c.output.Separator()
	fmt.Println("")

	if !c.promptConfirm("Create this domain mapping?") {
		c.output.Info("Operation cancelled.")
		return
	}

	// 7. 创建域名映射
	fmt.Println("")
	c.output.Info("Creating domain mapping...")

	createResp, err := c.client.CreateHTTPDomain(&client.CreateHTTPDomainRequest{
		TargetURL:  targetURL,
		Subdomain:  subdomain,
		BaseDomain: selectedBaseDomain,
		MappingTTL: mappingTTL,
	})

	if err != nil {
		c.output.Error("Failed to create domain mapping: %v", err)
		return
	}

	// 显示结果
	fmt.Println("")
	c.output.Success("Domain Mapping Created!")
	c.output.Separator()
	c.output.KeyValue("Mapping ID", createResp.MappingID)
	c.output.KeyValue("Domain", colorBold(createResp.FullDomain))
	c.output.KeyValue("Target", createResp.TargetURL)
	if createResp.ExpiresAt != "" {
		c.output.KeyValue("Expires At", createResp.ExpiresAt)
	}
	c.output.Separator()
	fmt.Println("")
	c.output.Info("Your service is now accessible at: %s", colorBold("https://"+createResp.FullDomain))
	fmt.Println("")
}

// cmdListDomains 列出已注册的 HTTP 域名
func (c *CLI) cmdListDomains(args []string) {
	c.output.Header("🌐 HTTP Domain Mappings")

	if !c.client.IsConnected() {
		c.output.Error("Not connected to server. Please connect first using 'connect' command.")
		return
	}

	resp, err := c.client.ListHTTPDomains()
	if err != nil {
		c.output.Error("Failed to list domains: %v", err)
		return
	}

	if len(resp.Mappings) == 0 {
		c.output.Info("No HTTP domain mappings found.")
		c.output.Info("Use 'register-domain' to create a new domain mapping.")
		return
	}

	// 创建表格
	table := NewTable("ID", "DOMAIN", "TARGET", "STATUS", "EXPIRES")

	for _, mapping := range resp.Mappings {
		status := mapping.Status
		if status == "active" {
			status = colorSuccess("active")
		} else if status == "expired" {
			status = colorWarning("expired")
		}

		expiresAt := "-"
		if mapping.ExpiresAt != "" {
			expiresAt = FormatTime(mapping.ExpiresAt)
		}

		table.AddRow(
			Truncate(mapping.MappingID, 12),
			Truncate(mapping.FullDomain, 30),
			Truncate(mapping.TargetURL, 30),
			status,
			expiresAt,
		)
	}

	table.Render()

	fmt.Println("")
	c.output.Info("Total: %d domain mappings", resp.Total)
	fmt.Println("")
}

// cmdDeleteDomain 删除 HTTP 域名映射
func (c *CLI) cmdDeleteDomain(args []string) {
	c.output.Header("🗑️  Delete HTTP Domain Mapping")

	if !c.client.IsConnected() {
		c.output.Error("Not connected to server. Please connect first using 'connect' command.")
		return
	}

	var mappingID string

	if len(args) > 0 {
		mappingID = args[0]
	} else {
		// 提示用户输入
		input, err := c.promptInput("Enter Mapping ID to delete: ")
		if err == ErrCancelled {
			return
		}
		if err != nil {
			return
		}
		mappingID = strings.TrimSpace(input)
	}

	if mappingID == "" {
		c.output.Error("Mapping ID cannot be empty")
		return
	}

	// 确认删除
	if !c.promptConfirm(fmt.Sprintf("Delete domain mapping '%s'?", mappingID)) {
		c.output.Info("Operation cancelled.")
		return
	}

	// 执行删除
	err := c.client.DeleteHTTPDomain(mappingID)
	if err != nil {
		c.output.Error("Failed to delete domain mapping: %v", err)
		return
	}

	c.output.Success("Domain mapping '%s' deleted successfully!", mappingID)
}

// isValidSubdomain 验证子域名格式
func isValidSubdomain(s string) bool {
	if len(s) == 0 || len(s) > 63 {
		return false
	}

	// 不能以连字符开头或结尾
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}

	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}

	return true
}
