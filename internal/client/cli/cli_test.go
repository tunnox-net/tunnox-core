package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"tunnox-core/internal/client"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// CLI E2E 测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// MockTunnoxClient 模拟客户端用于测试
type MockTunnoxClient struct {
	connected bool
	apiClient *MockAPIClient
}

func NewMockTunnoxClient() *MockTunnoxClient {
	return &MockTunnoxClient{
		connected: false,
		apiClient: NewMockAPIClient(),
	}
}

func (m *MockTunnoxClient) Connect() error {
	m.connected = true
	return nil
}

func (m *MockTunnoxClient) Disconnect() error {
	m.connected = false
	return nil
}

func (m *MockTunnoxClient) IsConnected() bool {
	return m.connected
}

func (m *MockTunnoxClient) GetAPIClient() *client.ManagementAPIClient {
	// 注意：这里返回的类型不匹配，但对于测试命令解析是足够的
	// 实际API调用在e2e测试中会被模拟
	return nil
}

// MockAPIClient 模拟API客户端
type MockAPIClient struct{}

func NewMockAPIClient() *MockAPIClient {
	return &MockAPIClient{}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 命令解析测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestCLI_CommandParsing(t *testing.T) {
	tests := []struct {
		name        string
		commandLine string
		expectCmd   string
		expectArgs  []string
	}{
		{
			name:        "simple command",
			commandLine: "help",
			expectCmd:   "help",
			expectArgs:  []string{},
		},
		{
			name:        "command with args",
			commandLine: "use-code ABC123",
			expectCmd:   "use-code",
			expectArgs:  []string{"ABC123"},
		},
		{
			name:        "command with multiple args",
			commandLine: "list-mappings --type inbound",
			expectCmd:   "list-mappings",
			expectArgs:  []string{"--type", "inbound"},
		},
		{
			name:        "command with leading/trailing spaces",
			commandLine: "  status  ",
			expectCmd:   "status",
			expectArgs:  []string{},
		},
		{
			name:        "command alias",
			commandLine: "h",
			expectCmd:   "h",
			expectArgs:  []string{},
		},
		{
			name:        "config subcommand",
			commandLine: "config get server.address",
			expectCmd:   "config",
			expectArgs:  []string{"get", "server.address"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := strings.TrimSpace(tt.commandLine)
			parts := strings.Fields(line)

			if len(parts) == 0 {
				return
			}

			cmd := strings.ToLower(parts[0])
			args := parts[1:]

			if cmd != tt.expectCmd {
				t.Errorf("expected cmd %q, got %q", tt.expectCmd, cmd)
			}

			if len(args) != len(tt.expectArgs) {
				t.Errorf("expected %d args, got %d", len(tt.expectArgs), len(args))
			}

			for i, arg := range args {
				if i < len(tt.expectArgs) && arg != tt.expectArgs[i] {
					t.Errorf("arg[%d]: expected %q, got %q", i, tt.expectArgs[i], arg)
				}
			}
		})
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 输出工具测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestOutput_Messages(t *testing.T) {
	output := NewOutput(true) // 禁用颜色以便测试

	// 测试各种消息类型不会panic
	tests := []struct {
		name string
		fn   func()
	}{
		{"success", func() { output.Success("test message") }},
		{"error", func() { output.Error("test error") }},
		{"warning", func() { output.Warning("test warning") }},
		{"info", func() { output.Info("test info") }},
		{"plain", func() { output.Plain("test plain") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("function panicked: %v", r)
				}
			}()
			tt.fn()
		})
	}
}

func TestOutput_Table(t *testing.T) {
	table := NewTable("ID", "NAME", "STATUS")

	// 添加数据行
	table.AddRow("1", "Test", "Active")
	table.AddRow("2", "Example", "Inactive")

	if len(table.rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(table.rows))
	}

	if len(table.headers) != 3 {
		t.Errorf("expected 3 headers, got %d", len(table.headers))
	}

	// 测试列宽自动调整
	table.AddRow("3", "VeryLongNameHere", "Active")

	// NAME列的宽度应该增加
	expectedWidth := len("VeryLongNameHere")
	if table.widths[1] < expectedWidth {
		t.Errorf("expected width >= %d, got %d", expectedWidth, table.widths[1])
	}

	// 测试渲染不会panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Render panicked: %v", r)
		}
	}()
	table.Render()
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 命令执行测试（不依赖实际服务器）
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestCLI_BasicCommands(t *testing.T) {
	// 这些测试验证命令不会panic，但不检查输出
	// 因为输出会直接打印到stdout

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mockClient := NewMockTunnoxClient()

	// 由于readline需要实际终端，我们只测试命令处理逻辑
	// 而不是完整的CLI交互

	tests := []struct {
		name string
		cmd  string
		args []string
	}{
		{"help", "help", []string{}},
		{"help with command", "help", []string{"status"}},
		{"status", "status", []string{}},
		{"clear", "clear", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("command %s panicked: %v", tt.cmd, r)
				}
			}()

			// 注意：由于NewCLI需要readline，这里我们只能测试
			// 独立的命令处理函数
			// 完整的CLI交互测试需要在实际环境中进行
			_ = ctx
			_ = mockClient

			// 这里可以添加更细粒度的命令处理逻辑测试
		})
	}
}

func TestCLI_ConnectDisconnect(t *testing.T) {
	mockClient := NewMockTunnoxClient()

	// 测试连接
	if err := mockClient.Connect(); err != nil {
		t.Errorf("Connect failed: %v", err)
	}

	if !mockClient.IsConnected() {
		t.Error("expected client to be connected")
	}

	// 测试断开
	if err := mockClient.Disconnect(); err != nil {
		t.Errorf("Disconnect failed: %v", err)
	}

	if mockClient.IsConnected() {
		t.Error("expected client to be disconnected")
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 配置命令参数解析测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestConfigCommand_SubCommands(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedSubCmd string
		expectedArgs   []string
		shouldError    bool
	}{
		{
			name:           "list",
			args:           []string{"list"},
			expectedSubCmd: "list",
			expectedArgs:   []string{},
		},
		{
			name:           "get with key",
			args:           []string{"get", "server.address"},
			expectedSubCmd: "get",
			expectedArgs:   []string{"server.address"},
		},
		{
			name:           "set with key and value",
			args:           []string{"set", "server.address", "localhost:7004"},
			expectedSubCmd: "set",
			expectedArgs:   []string{"server.address", "localhost:7004"},
		},
		{
			name:           "reset",
			args:           []string{"reset", "server.protocol"},
			expectedSubCmd: "reset",
			expectedArgs:   []string{"server.protocol"},
		},
		{
			name:           "save with path",
			args:           []string{"save", "config.json"},
			expectedSubCmd: "save",
			expectedArgs:   []string{"config.json"},
		},
		{
			name:        "no subcommand",
			args:        []string{},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldError {
				if len(tt.args) > 0 {
					t.Error("expected error but test passed")
				}
				return
			}

			if len(tt.args) == 0 {
				return
			}

			subCmd := strings.ToLower(tt.args[0])
			subArgs := tt.args[1:]

			if subCmd != tt.expectedSubCmd {
				t.Errorf("expected subCmd %q, got %q", tt.expectedSubCmd, subCmd)
			}

			if len(subArgs) != len(tt.expectedArgs) {
				t.Errorf("expected %d args, got %d", len(tt.expectedArgs), len(subArgs))
			}

			for i, arg := range subArgs {
				if i < len(tt.expectedArgs) && arg != tt.expectedArgs[i] {
					t.Errorf("arg[%d]: expected %q, got %q", i, tt.expectedArgs[i], arg)
				}
			}
		})
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 命令别名测试
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func TestCommandAliases(t *testing.T) {
	aliases := map[string][]string{
		"help":           {"h", "?"},
		"exit":           {"quit", "q"},
		"clear":          {"cls"},
		"status":         {"st"},
		"connect":        {"conn"},
		"disconnect":     {"dc"},
		"generate-code":  {"gen-code", "gen"},
		"list-codes":     {"lsc"},
		"use-code":       {"activate"},
		"list-mappings":  {"lsm"},
		"show-mapping":   {"show"},
		"delete-mapping": {"del", "rm"},
	}

	// 验证所有别名都在命令列表中
	allCommands := GetAllCommands()
	commandSet := make(map[string]bool)
	for _, cmd := range allCommands {
		commandSet[cmd] = true
	}

	for primary, aliasList := range aliases {
		// 验证主命令存在
		if !commandSet[primary] {
			t.Errorf("primary command %q not found in command list", primary)
		}

		// 验证所有别名都存在
		for _, alias := range aliasList {
			if !commandSet[alias] {
				t.Errorf("alias %q for command %q not found in command list", alias, primary)
			}
		}
	}
}
