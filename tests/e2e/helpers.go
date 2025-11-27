package e2e

import (
	"testing"
)

// SetupE2EEnvironment 设置E2E测试环境
// 执行环境检查、清理旧资源、创建新环境
func SetupE2EEnvironment(t *testing.T, composeFile string) *DockerComposeEnv {
	t.Helper()
	
	// 1. 环境检查
	checker := NewEnvironmentChecker(t)
	if err := checker.CheckAll(); err != nil {
		t.Fatalf("Environment check failed: %v", err)
	}
	
	// 2. 清理旧环境
	cleaner := NewEnvironmentCleaner(t)
	if err := cleaner.CleanAll(); err != nil {
		t.Logf("Warning: Cleanup had errors: %v", err)
	}
	
	// 3. 创建新环境
	return NewDockerComposeEnv(t, composeFile)
}

// SkipIfShort 如果是短测试模式则跳过
func SkipIfShort(t *testing.T, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skipf("Skipping E2E test in short mode: %s", reason)
	}
}
