package cli

import (
	"reflect"
	"testing"
)

func TestFilterCommands(t *testing.T) {
	allCommands := []string{
		"help", "status", "connect", "disconnect",
		"generate-code", "list-codes", "list-mappings",
	}

	tests := []struct {
		name     string
		prefix   string
		expected []string
	}{
		{
			name:     "empty prefix",
			prefix:   "",
			expected: allCommands,
		},
		{
			name:     "single letter",
			prefix:   "h",
			expected: []string{"help"},
		},
		{
			name:     "partial match",
			prefix:   "list",
			expected: []string{"list-codes", "list-mappings"},
		},
		{
			name:     "exact match",
			prefix:   "status",
			expected: []string{"status"},
		},
		{
			name:     "no match",
			prefix:   "xyz",
			expected: []string{},
		},
		{
			name:     "case insensitive",
			prefix:   "HELP",
			expected: []string{"help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterCommands(tt.prefix, allCommands)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetAllCommands(t *testing.T) {
	commands := GetAllCommands()

	// 验证必须包含的命令
	requiredCommands := []string{
		"help", "exit", "status", "connect", "disconnect",
		"generate-code", "list-codes", "list-mappings",
		"config",
	}

	for _, required := range requiredCommands {
		found := false
		for _, cmd := range commands {
			if cmd == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing required command: %s", required)
		}
	}
}

func TestCommandCompleter(t *testing.T) {
	completer := NewCommandCompleter()

	// 测试注册命令
	completer.RegisterCommand("test", []string{"arg1", "arg2"})

	if len(completer.commands) != 1 {
		t.Errorf("expected 1 command, got %d", len(completer.commands))
	}

	params, ok := completer.commands["test"]
	if !ok {
		t.Error("command not registered")
	}

	expectedParams := []string{"arg1", "arg2"}
	if !reflect.DeepEqual(params, expectedParams) {
		t.Errorf("expected params %v, got %v", expectedParams, params)
	}
}

func TestBuildCompleter(t *testing.T) {
	completer := NewCommandCompleter()

	// 测试构建补全器
	prefixCompleter := completer.BuildCompleter()

	if prefixCompleter == nil {
		t.Error("BuildCompleter returned nil")
	}

	// 基本验证：确保返回了readline补全器
	// 实际补全功能由readline库实现，这里只测试构建过程
}
