package cli

import (
	"strings"

	"github.com/chzyer/readline"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Tab补全逻辑 (P1.1)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CommandCompleter 命令补全器
type CommandCompleter struct {
	commands map[string][]string // 命令 -> 子命令/参数
}

// NewCommandCompleter 创建补全器
func NewCommandCompleter() *CommandCompleter {
	return &CommandCompleter{
		commands: make(map[string][]string),
	}
}

// RegisterCommand 注册命令及其参数
func (c *CommandCompleter) RegisterCommand(cmd string, params []string) {
	c.commands[cmd] = params
}

// BuildCompleter 构建readline补全器
func (c *CommandCompleter) BuildCompleter() *readline.PrefixCompleter {
	// 构建顶层命令列表
	items := make([]readline.PrefixCompleterInterface, 0)

	// 基础命令
	items = append(items,
		readline.PcItem("help"),
		readline.PcItem("exit"),
		readline.PcItem("quit"),
		readline.PcItem("clear"),
		readline.PcItem("status"),
		readline.PcItem("connect"),
		readline.PcItem("disconnect"),
	)

	// 连接码命令
	items = append(items,
		readline.PcItem("generate-code"),
		readline.PcItem("list-codes"),
	)

	// 隧道映射命令
	items = append(items,
		readline.PcItem("use-code"),
		readline.PcItem("list-mappings",
			readline.PcItem("--type",
				readline.PcItem("inbound"),
				readline.PcItem("outbound"),
			),
		),
		readline.PcItem("show-mapping"),
		readline.PcItem("delete-mapping"),
	)

	// 配置命令 (P1.3)
	items = append(items,
		readline.PcItem("config",
			readline.PcItem("list"),
			readline.PcItem("get"),
			readline.PcItem("set"),
			readline.PcItem("reset"),
			readline.PcItem("save"),
			readline.PcItem("reload"),
		),
	)

	return readline.NewPrefixCompleter(items...)
}

// FilterCommands 过滤匹配的命令
func FilterCommands(prefix string, commands []string) []string {
	prefix = strings.ToLower(prefix)
	matches := make([]string, 0)
	for _, cmd := range commands {
		if strings.HasPrefix(strings.ToLower(cmd), prefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}

// GetAllCommands 获取所有命令列表
func GetAllCommands() []string {
	return []string{
		"help", "h", "?",
		"exit", "quit", "q",
		"clear", "cls",
		"status", "st",
		"connect", "conn",
		"disconnect", "dc",
		"generate-code", "gen-code", "gen",
		"list-codes", "lsc",
		"use-code", "activate",
		"list-mappings", "lsm",
		"show-mapping", "show",
		"delete-mapping", "del", "rm",
		"config",
	}
}
