// Package cli 提供 Tunnox 客户端的快捷隧道命令
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"tunnox-core/internal/client"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 快捷隧道命令 (tunnox http/tcp/udp/socks)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// runHTTPCommand 执行 tunnox http <port> 命令
// 创建 HTTP 域名代理隧道，生成公网可访问的子域名
func (r *QuickCommandRunner) runHTTPCommand(args []string) (bool, error) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tunnox http <port|host:port> [options]\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		fmt.Fprintf(os.Stderr, "  --subdomain <name>   Specify custom subdomain (default: random)\n")
		fmt.Fprintf(os.Stderr, "  --ttl <days>         Mapping TTL in days (default: 7, 0 for no expiration)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  tunnox http 3000                      # Share localhost:3000 with random subdomain\n")
		fmt.Fprintf(os.Stderr, "  tunnox http 8080 --subdomain myapp    # Share with custom subdomain\n")
		fmt.Fprintf(os.Stderr, "  tunnox http 192.168.1.10:8080         # Share LAN device\n")
		return false, nil
	}

	targetAddress, err := r.parseTargetAddress(args[0], "http")
	if err != nil {
		return false, err
	}

	return r.createHTTPDomainAndWait(targetAddress, args[1:])
}

// createHTTPDomainAndWait 创建 HTTP 域名映射并等待
func (r *QuickCommandRunner) createHTTPDomainAndWait(targetAddress string, extraArgs []string) (bool, error) {
	// 解析额外参数
	mappingTTL := 7 * 24 * 3600 // 默认7天
	var customSubdomain string

	for i := 0; i < len(extraArgs); i++ {
		switch extraArgs[i] {
		case "--ttl":
			if i+1 < len(extraArgs) {
				days, err := strconv.Atoi(extraArgs[i+1])
				if err != nil {
					return false, coreerrors.Newf(coreerrors.CodeInvalidParam, "invalid --ttl value: %s", extraArgs[i+1])
				}
				if days == 0 {
					mappingTTL = 0
				} else {
					mappingTTL = days * 24 * 3600
				}
				i++
			}
		case "--subdomain", "-s":
			if i+1 < len(extraArgs) {
				customSubdomain = extraArgs[i+1]
				i++
			}
		}
	}

	// 连接到服务器
	if err := r.connectToServer(); err != nil {
		return false, err
	}
	defer r.client.Stop()

	// 获取可用的基础域名
	fmt.Fprintf(os.Stderr, "\n🔍 Fetching available domains...\n")
	corelog.Infof("QuickHTTP: calling GetBaseDomains, client connected=%v", r.client.IsConnected())
	baseDomainsResp, err := r.client.GetBaseDomains()
	corelog.Infof("QuickHTTP: GetBaseDomains returned, err=%v, resp=%+v", err, baseDomainsResp)
	if err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to get base domains")
	}

	if len(baseDomainsResp.BaseDomains) == 0 {
		return false, coreerrors.New(coreerrors.CodeInvalidState, "no base domains available, please contact administrator")
	}

	// 使用第一个可用的基础域名
	baseDomain := baseDomainsResp.BaseDomains[0].Domain

	var subdomain string
	var fullDomain string

	if customSubdomain != "" {
		// 使用用户指定的子域名
		subdomain = strings.ToLower(strings.TrimSpace(customSubdomain))
		fullDomain = subdomain + "." + baseDomain
		fmt.Fprintf(os.Stderr, "📝 Using custom subdomain: %s\n", fullDomain)

		// 检查子域名是否可用
		checkResp, err := r.client.CheckSubdomain(&client.CheckSubdomainRequest{
			Subdomain:  subdomain,
			BaseDomain: baseDomain,
		})
		if err != nil {
			return false, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to check subdomain availability")
		}
		if !checkResp.Available {
			return false, coreerrors.Newf(coreerrors.CodeAlreadyExists, "subdomain '%s' is not available", fullDomain)
		}
	} else {
		// 生成随机子域名
		fmt.Fprintf(os.Stderr, "🎲 Generating random subdomain...\n")
		corelog.Infof("QuickHTTP: calling GenSubdomain, baseDomain=%s", baseDomain)
		genResp, err := r.client.GenSubdomain(baseDomain)
		corelog.Infof("QuickHTTP: GenSubdomain returned, err=%v, resp=%+v", err, genResp)
		if err != nil {
			return false, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to generate subdomain")
		}
		subdomain = genResp.Subdomain
		_ = genResp.FullDomain
	}

	// 创建 HTTP 域名映射
	fmt.Fprintf(os.Stderr, "🔧 Creating HTTP tunnel...\n")
	corelog.Infof("QuickHTTP: calling CreateHTTPDomain, subdomain=%s, baseDomain=%s, target=%s", subdomain, baseDomain, targetAddress)
	createResp, err := r.client.CreateHTTPDomain(&client.CreateHTTPDomainRequest{
		TargetURL:  targetAddress,
		Subdomain:  subdomain,
		BaseDomain: baseDomain,
		MappingTTL: mappingTTL,
	})
	if err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to create HTTP domain mapping")
	}

	// 显示结果
	r.printHTTPDomainResult(createResp, targetAddress)

	// 等待 Ctrl+C
	r.waitForShutdown()

	// 清理：删除域名映射
	fmt.Fprintf(os.Stderr, "🗑️  Removing domain mapping...\n")
	if err := r.client.DeleteHTTPDomain(createResp.MappingID); err != nil {
		corelog.Warnf("Failed to delete domain mapping: %v", err)
	}

	return false, nil
}

// printHTTPDomainResult 打印 HTTP 域名结果
func (r *QuickCommandRunner) printHTTPDomainResult(resp *client.CreateHTTPDomainResponse, targetAddress string) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "✅ HTTP 隧道已创建!\n")
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "   公网地址:   \033[1mhttps://%s\033[0m\n", resp.FullDomain)
	fmt.Fprintf(os.Stderr, "   本地服务:   %s\n", targetAddress)
	if resp.ExpiresAt != "" {
		fmt.Fprintf(os.Stderr, "   过期时间:   %s\n", resp.ExpiresAt)
	}
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "   按 Ctrl+C 停止隧道\n")
	fmt.Fprintf(os.Stderr, "\n")
}

// runTCPCommand 执行 tunnox tcp <port> 命令
func (r *QuickCommandRunner) runTCPCommand(args []string) (bool, error) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tunnox tcp <port|host:port> [options]\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  tunnox tcp 22              # Share SSH service\n")
		fmt.Fprintf(os.Stderr, "  tunnox tcp 10.0.0.5:3306   # Share MySQL on LAN\n")
		return false, nil
	}

	targetAddress, err := r.parseTargetAddress(args[0], "tcp")
	if err != nil {
		return false, err
	}

	return r.generateCodeAndWait("tcp", targetAddress, args[1:])
}

// runUDPCommand 执行 tunnox udp <port> 命令
func (r *QuickCommandRunner) runUDPCommand(args []string) (bool, error) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: tunnox udp <port|host:port> [options]\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  tunnox udp 53              # Share DNS service\n")
		fmt.Fprintf(os.Stderr, "  tunnox udp 10.0.0.5:1194   # Share VPN on LAN\n")
		return false, nil
	}

	targetAddress, err := r.parseTargetAddress(args[0], "udp")
	if err != nil {
		return false, err
	}

	return r.generateCodeAndWait("udp", targetAddress, args[1:])
}

// runSOCKSCommand 执行 tunnox socks 命令
func (r *QuickCommandRunner) runSOCKSCommand(args []string) (bool, error) {
	// SOCKS5 不需要目标地址
	return r.generateCodeAndWait("socks5", "socks5://0.0.0.0:0", args)
}

// parseTargetAddress 解析目标地址
func (r *QuickCommandRunner) parseTargetAddress(input string, protocol string) (string, error) {
	input = strings.TrimSpace(input)

	// 如果只是端口号
	if port, err := strconv.Atoi(input); err == nil {
		if port < 1 || port > 65535 {
			return "", coreerrors.Newf(coreerrors.CodeInvalidParam, "port out of range: %d (must be 1-65535)", port)
		}
		return fmt.Sprintf("%s://localhost:%d", protocol, port), nil
	}

	// 如果是 host:port 格式
	if !strings.Contains(input, "://") {
		// 验证格式
		parts := strings.Split(input, ":")
		if len(parts) != 2 {
			return "", coreerrors.Newf(coreerrors.CodeInvalidParam, "invalid address format: %s (expected host:port)", input)
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return "", coreerrors.Newf(coreerrors.CodeInvalidParam, "invalid port: %s", parts[1])
		}
		if port < 1 || port > 65535 {
			return "", coreerrors.Newf(coreerrors.CodeInvalidParam, "port out of range: %d (must be 1-65535)", port)
		}
		return fmt.Sprintf("%s://%s", protocol, input), nil
	}

	// 已经包含协议前缀
	return input, nil
}

// generateCodeAndWait 生成连接码并等待
func (r *QuickCommandRunner) generateCodeAndWait(protocol, targetAddress string, extraArgs []string) (bool, error) {
	// 解析额外参数
	activationTTL := 10 * 60    // 默认10分钟
	mappingTTL := 7 * 24 * 3600 // 默认7天
	var codeName string

	for i := 0; i < len(extraArgs); i++ {
		switch extraArgs[i] {
		case "--activation-ttl":
			if i+1 < len(extraArgs) {
				minutes, err := strconv.Atoi(extraArgs[i+1])
				if err != nil {
					return false, coreerrors.Newf(coreerrors.CodeInvalidParam, "invalid --activation-ttl value: %s", extraArgs[i+1])
				}
				activationTTL = minutes * 60
				i++
			}
		case "--mapping-ttl":
			if i+1 < len(extraArgs) {
				days, err := strconv.Atoi(extraArgs[i+1])
				if err != nil {
					return false, coreerrors.Newf(coreerrors.CodeInvalidParam, "invalid --mapping-ttl value: %s", extraArgs[i+1])
				}
				mappingTTL = days * 24 * 3600
				i++
			}
		case "--name", "-n":
			if i+1 < len(extraArgs) {
				codeName = extraArgs[i+1]
				i++
			}
		}
	}

	// 连接到服务器
	if err := r.connectToServer(); err != nil {
		return false, err
	}
	defer r.client.Stop()

	// 生成连接码
	fmt.Fprintf(os.Stderr, "\n🔄 Generating connection code...\n")

	resp, err := r.client.GenerateConnectionCode(&client.GenerateConnectionCodeRequest{
		TargetAddress: targetAddress,
		ActivationTTL: activationTTL,
		MappingTTL:    mappingTTL,
		Description:   codeName,
	})
	if err != nil {
		return false, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to generate code")
	}

	// 显示结果
	r.printCodeResult(resp, protocol)

	// 等待 Ctrl+C
	r.waitForShutdown()

	return false, nil
}

// connectToServer 连接到服务器
func (r *QuickCommandRunner) connectToServer() error {
	fmt.Fprintf(os.Stderr, "\n🔍 Connecting to Tunnox service...\n")

	// 创建客户端（传递配置文件路径，用于保存凭据）
	needsAutoConnect := r.config.Server.Address == "" && r.config.Server.Protocol == ""
	r.client = client.NewClientWithCLIFlags(r.ctx, r.config, !needsAutoConnect, !needsAutoConnect, r.configFilePath)

	// 连接
	if err := r.client.Connect(); err != nil {
		if r.ctx.Err() == context.Canceled {
			return coreerrors.New(coreerrors.CodeCancelled, "connection cancelled")
		}
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "connection failed")
	}

	fmt.Fprintf(os.Stderr, "✅ Connected successfully\n")
	return nil
}

// printCodeResult 打印连接码结果
func (r *QuickCommandRunner) printCodeResult(resp *client.GenerateConnectionCodeResponse, protocol string) {
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "✅ 连接码已生成!\n")
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "   连接码:     \033[1m%s\033[0m\n", resp.Code)
	fmt.Fprintf(os.Stderr, "   目标服务:   %s\n", resp.TargetAddress)
	fmt.Fprintf(os.Stderr, "   过期时间:   %s\n", resp.ExpiresAt)
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "   💡 将连接码 %s 分享给需要访问的人\n", resp.Code)
	fmt.Fprintf(os.Stderr, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "   按 Ctrl+C 停止并撤销连接码\n")
	fmt.Fprintf(os.Stderr, "\n")
}

// waitForShutdown 等待关闭信号
func (r *QuickCommandRunner) waitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		corelog.Infof("QuickCommand: received signal %v", sig)
		fmt.Fprintf(os.Stderr, "\n🛑 Shutting down...\n")
	case <-r.ctx.Done():
		corelog.Infof("QuickCommand: context cancelled")
	}
}
