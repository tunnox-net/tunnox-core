package utils

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetDefaultClientLogPath 测试客户端默认日志路径
func TestGetDefaultClientLogPath(t *testing.T) {
	// 测试交互模式
	paths := GetDefaultClientLogPath(true)
	if len(paths) == 0 {
		t.Fatal("Expected at least one log path")
	}

	// 验证第一个路径包含 .tunnox
	if len(paths) > 0 {
		firstPath := paths[0]
		if !contains(firstPath, ".tunnox") {
			t.Errorf("Expected first path to contain .tunnox, got %s", firstPath)
		}
	}

	// 验证最后一个路径是 /tmp
	lastPath := paths[len(paths)-1]
	if lastPath != "/tmp/tunnox-client.log" {
		t.Errorf("Expected last path to be /tmp/tunnox-client.log, got %s", lastPath)
	}

	// 测试守护进程模式
	paths = GetDefaultClientLogPath(false)
	if len(paths) == 0 {
		t.Fatal("Expected at least one log path")
	}
}

// TestGetDefaultServerLogPath 测试服务端默认日志路径
func TestGetDefaultServerLogPath(t *testing.T) {
	path := GetDefaultServerLogPath()
	if path == "" {
		t.Fatal("Expected non-empty log path")
	}

	// 验证路径格式
	if !filepath.IsAbs(path) {
		// 如果不是绝对路径，至少应该是有效的相对路径
		if path != "/tmp/tunnox-server.log" {
			t.Logf("Warning: log path is not absolute: %s", path)
		}
	}
}

// TestResolveLogPath 测试日志路径解析
func TestResolveLogPath(t *testing.T) {
	// 创建临时目录用于测试
	tmpDir := t.TempDir()
	testLogFile := filepath.Join(tmpDir, "test.log")

	candidates := []string{
		testLogFile,
		"/tmp/tunnox-test.log",
	}

	path, err := ResolveLogPath(candidates)
	if err != nil {
		t.Fatalf("ResolveLogPath failed: %v", err)
	}

	if path == "" {
		t.Fatal("Expected non-empty resolved path")
	}

	// 验证路径可以写入
	if !canWriteToPath(path) {
		t.Errorf("Resolved path should be writable: %s", path)
	}
}

// TestResolveLogPath_AllFail 测试所有路径都失败的情况
func TestResolveLogPath_AllFail(t *testing.T) {
	// 使用无效的路径（需要 root 权限的路径，普通用户无法写入）
	candidates := []string{
		"/root/tunnox-test.log", // 通常需要 root 权限
		"/sys/tunnox-test.log",  // 系统目录，无法写入
	}

	// 这个测试可能会成功（如果以 root 运行）或失败（普通用户）
	// 我们只验证函数不会 panic
	path, err := ResolveLogPath(candidates)
	if err != nil {
		// 如果所有路径都失败，应该返回错误
		if path == "" {
			t.Logf("Expected error when all paths fail: %v", err)
		}
	}
}

// TestCanWriteToPath 测试路径写入检查
func TestCanWriteToPath(t *testing.T) {
	tmpDir := t.TempDir()
	testLogFile := filepath.Join(tmpDir, "test.log")

	// 测试可以写入的路径
	if !canWriteToPath(testLogFile) {
		t.Error("Expected path to be writable")
	}

	// 验证文件确实被创建
	if _, err := os.Stat(testLogFile); os.IsNotExist(err) {
		t.Error("Expected log file to be created")
	}
}

// contains 检查字符串是否包含子字符串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
