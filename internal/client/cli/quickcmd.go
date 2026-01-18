// Package cli 提供 Tunnox 客户端的快捷命令支持
// 实现 CLIENT_PRODUCT_DESIGN.md 中定义的命令行接口
//
// 文件拆分说明:
// - quickcmd.go       - 基础结构和命令路由
// - quickcmd_tunnel.go - 隧道命令 (http/tcp/udp/socks)
// - quickcmd_code.go   - 连接码命令 (generate/use/list/revoke)
// - quickcmd_system.go - 系统命令 (version/help/start/stop/status)
// - quickcmd_config.go - 配置命令 (init/show)
package cli

import (
	"context"
	"strings"

	"tunnox-core/internal/client"
)

// QuickCommandRunner 快捷命令执行器
type QuickCommandRunner struct {
	ctx            context.Context
	client         *client.TunnoxClient
	config         *client.ClientConfig
	configFilePath string // 配置文件路径（用于保存凭据）
	output         *Output
}

// NewQuickCommandRunner 创建快捷命令执行器
func NewQuickCommandRunner(ctx context.Context, cfg *client.ClientConfig) *QuickCommandRunner {
	return NewQuickCommandRunnerWithConfigPath(ctx, cfg, "")
}

// NewQuickCommandRunnerWithConfigPath 创建快捷命令执行器（带配置文件路径）
func NewQuickCommandRunnerWithConfigPath(ctx context.Context, cfg *client.ClientConfig, configFilePath string) *QuickCommandRunner {
	return &QuickCommandRunner{
		ctx:            ctx,
		config:         cfg,
		configFilePath: configFilePath,
		output:         NewOutput(false), // 启用颜色
	}
}

// Run 执行快捷命令
// 返回值: shouldContinue (是否应该继续运行传统流程), error
func (r *QuickCommandRunner) Run(args []string) (bool, error) {
	if len(args) == 0 {
		// 无参数 - 可以启动向导或交互式shell
		return true, nil
	}

	cmd := strings.ToLower(args[0])
	cmdArgs := args[1:]

	switch cmd {
	// 隧道命令 - 见 quickcmd_tunnel.go
	case "http":
		return r.runHTTPCommand(cmdArgs)
	case "tcp":
		return r.runTCPCommand(cmdArgs)
	case "udp":
		return r.runUDPCommand(cmdArgs)
	case "socks":
		return r.runSOCKSCommand(cmdArgs)

	// 连接码命令 - 见 quickcmd_code.go
	case "code":
		return r.runCodeCommand(cmdArgs)

	// 系统命令 - 见 quickcmd_system.go
	case "start":
		return r.runStartCommand(cmdArgs)
	case "stop":
		return r.runStopCommand(cmdArgs)
	case "status":
		return r.runStatusCommand(cmdArgs)
	case "version", "--version", "-v":
		return r.runVersionCommand()
	case "help", "--help", "-h":
		r.showQuickHelp()
		return false, nil

	// 配置命令 - 见 quickcmd_config.go
	case "config":
		return r.runConfigCommand(cmdArgs)

	case "shell":
		// 显式进入交互式 shell
		return true, nil

	default:
		// 未知命令，交给传统流程处理
		return true, nil
	}
}
