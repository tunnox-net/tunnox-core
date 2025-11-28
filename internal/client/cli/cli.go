package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"tunnox-core/internal/client"
	"tunnox-core/internal/utils"

	"github.com/chzyer/readline"
	"github.com/mattn/go-isatty"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// CLI - Tunnox客户端交互式命令行界面
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CLI 交互式命令行接口
type CLI struct {
	client    *client.TunnoxClient
	ctx       context.Context
	readline  *readline.Instance
	output    *Output
	completer *CommandCompleter
	startTime time.Time
}

// NewCLI 创建CLI实例
func NewCLI(ctx context.Context, tunnoxClient *client.TunnoxClient) (*CLI, error) {
	// 检查stdin是否是TTY
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return nil, fmt.Errorf("stdin is not a terminal (TTY required for interactive CLI)\n" +
			"Please run directly in a terminal, not through pipe/redirect")
	}

	// 创建补全器
	completer := NewCommandCompleter()

	// 创建readline实例
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "\033[32mtunnox>\033[0m ", // 绿色提示符
		HistoryFile:     os.ExpandEnv("$HOME/.tunnox_history"),
		HistoryLimit:    500,
		AutoComplete:    completer.BuildCompleter(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		Stdin:           os.Stdin,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize readline: %w", err)
	}

	// 创建输出工具
	output := NewOutput(false) // 默认启用彩色

	cli := &CLI{
		client:    tunnoxClient,
		ctx:       ctx,
		readline:  rl,
		output:    output,
		completer: completer,
		startTime: time.Now(),
	}

	return cli, nil
}

// Start 启动交互式CLI
func (c *CLI) Start() {
	c.printWelcome()
	defer c.Stop()

	for {
		select {
		case <-c.ctx.Done():
			utils.Infof("CLI: context cancelled, shutting down")
			return
		default:
			line, err := c.readline.Readline()
			if err == readline.ErrInterrupt {
				// Ctrl+C
				if len(line) == 0 {
					c.output.Info("Use 'exit' or 'quit' to exit")
					continue
				}
			} else if err == io.EOF {
				// EOF - 用户按了Ctrl+D或stdin关闭
				c.output.Info("Received EOF, exiting...")
				return
			} else if err != nil {
				// 其他错误
				utils.Errorf("CLI: readline error: %v", err)
				c.output.Error("Failed to read input: %v", err)
				// 不要立即退出，尝试继续
				time.Sleep(100 * time.Millisecond)
				continue
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			c.executeCommand(line)
		}
	}
}

// Stop 停止CLI
func (c *CLI) Stop() {
	if c.readline != nil {
		c.readline.Close()
	}
}

// printWelcome 打印欢迎信息
func (c *CLI) printWelcome() {
	fmt.Println("")
	c.output.Header("🚀 Tunnox Client CLI")
	c.output.Plain("  Type 'help' to see available commands")
	c.output.Plain("  Type 'exit' or 'quit' to quit")
	c.output.Plain("  Press Tab for command completion")
	fmt.Println("")
}

// executeCommand 执行命令
func (c *CLI) executeCommand(commandLine string) {
	// 解析命令
	parts := strings.Fields(commandLine)
	if len(parts) == 0 {
		return
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	// 路由到具体命令处理器
	switch cmd {
	case "help", "h", "?":
		c.cmdHelp(args)
	case "exit", "quit", "q":
		c.cmdExit(args)
	case "clear", "cls":
		c.cmdClear(args)
	case "status", "st":
		c.cmdStatus(args)
	case "connect", "conn":
		c.cmdConnect(args)
	case "disconnect", "dc":
		c.cmdDisconnect(args)
	case "generate-code", "gen-code", "gen":
		c.cmdGenerateCode(args)
	case "list-codes", "lsc":
		c.cmdListCodes(args)
	case "use-code", "activate":
		c.cmdUseCode(args)
	case "list-mappings", "lsm":
		c.cmdListMappings(args)
	case "show-mapping", "show":
		c.cmdShowMapping(args)
	case "delete-mapping", "del", "rm":
		c.cmdDeleteMapping(args)
	case "config":
		c.cmdConfig(args)
	default:
		c.output.Error("Unknown command: %s", cmd)
		c.output.Info("Type 'help' to see available commands")
	}
}

// promptInput 提示用户输入
func (c *CLI) promptInput(prompt string) (string, error) {
	c.readline.SetPrompt(prompt)
	defer c.readline.SetPrompt("\033[32mtunnox>\033[0m ")

	line, err := c.readline.Readline()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptConfirm 提示用户确认
func (c *CLI) promptConfirm(prompt string) bool {
	input, err := c.promptInput(prompt + " (yes/no): ")
	if err != nil {
		return false
	}

	response := strings.ToLower(input)
	return response == "yes" || response == "y"
}
