package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// DockerComposeEnv Docker Compose测试环境
type DockerComposeEnv struct {
	t           *testing.T
	composeFile string
	projectName string
	composePath string
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewDockerComposeEnv 创建Docker Compose测试环境
func NewDockerComposeEnv(t *testing.T, composeFile string) *DockerComposeEnv {
	ctx, cancel := context.WithCancel(context.Background())
	
	// 获取项目根目录
	wd, err := os.Getwd()
	require.NoError(t, err)
	
	// 查找tests/e2e目录
	composePath := filepath.Join(wd, composeFile)
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		// 尝试从项目根目录查找
		composePath = filepath.Join(wd, "tests", "e2e", composeFile)
	}
	
	// 生成唯一的项目名称（使用纳秒确保唯一性）
	projectName := fmt.Sprintf("tunnox-e2e-%d-%d", 
		time.Now().Unix(), 
		time.Now().UnixNano()%1000000)
	
	env := &DockerComposeEnv{
		t:           t,
		composeFile: composeFile,
		projectName: projectName,
		composePath: composePath,
		ctx:         ctx,
		cancel:      cancel,
	}
	
	// 环境检查
	checker := NewEnvironmentChecker(t)
	require.NoError(t, checker.CheckAll(), "Environment check failed")
	
	// 清理可能存在的残留
	env.forceCleanup()
	
	// 启动环境
	env.start()
	
	// 注册清理函数
	t.Cleanup(func() {
		env.Cleanup()
	})
	
	return env
}

// start 启动Docker Compose环境
func (e *DockerComposeEnv) start() {
	e.t.Logf("Starting Docker Compose environment: %s", e.composeFile)
	
	cmd := exec.CommandContext(e.ctx,
		"docker-compose",
		"-f", e.composePath,
		"-p", e.projectName,
		"up", "-d",
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Logf("Docker Compose output: %s", string(output))
		require.NoError(e.t, err, "Failed to start Docker Compose")
	}
	
	e.t.Logf("Docker Compose environment started")
}

// Cleanup 清理Docker Compose环境
func (e *DockerComposeEnv) Cleanup() {
	e.t.Logf("Cleaning up Docker Compose environment: %s", e.composeFile)
	
	// 取消context
	e.cancel()
	
	cmd := exec.Command(
		"docker-compose",
		"-f", e.composePath,
		"-p", e.projectName,
		"down", "-v",
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Logf("Docker Compose cleanup output: %s", string(output))
		e.t.Logf("Warning: Failed to cleanup Docker Compose: %v", err)
		return
	}
	
	e.t.Log("Docker Compose environment cleaned up")
}

// forceCleanup 强制清理环境（不输出错误）
func (e *DockerComposeEnv) forceCleanup() {
	cmd := exec.Command(
		"docker-compose",
		"-f", e.composePath,
		"-p", e.projectName,
		"down", "-v",
	)
	_ = cmd.Run()
}

// WaitForHealthy 等待服务健康
func (e *DockerComposeEnv) WaitForHealthy(serviceName string, timeout time.Duration) {
	e.t.Logf("Waiting for %s to be healthy (timeout: %v)", serviceName, timeout)
	
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	containerName := fmt.Sprintf("%s-%s-1", e.projectName, serviceName)
	
	for {
		select {
		case <-e.ctx.Done():
			e.t.Fatalf("Context cancelled while waiting for %s", serviceName)
		case <-ticker.C:
			if time.Now().After(deadline) {
				e.t.Fatalf("Timeout waiting for %s to be healthy", serviceName)
			}
			
			// 检查容器健康状态
			cmd := exec.Command("docker", "inspect", 
				"--format", "{{.State.Health.Status}}", 
				containerName)
			output, err := cmd.Output()
			if err != nil {
				continue
			}
			
			status := strings.TrimSpace(string(output))
			if status == "healthy" {
				e.t.Logf("✓ %s is healthy", serviceName)
				return
			}
		}
	}
}

// StopService 停止服务
func (e *DockerComposeEnv) StopService(serviceName string) {
	e.t.Logf("Stopping service: %s", serviceName)
	
	cmd := exec.Command(
		"docker-compose",
		"-f", e.composePath,
		"-p", e.projectName,
		"stop", serviceName,
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Logf("Stop service output: %s", string(output))
		require.NoError(e.t, err, "Failed to stop service")
	}
	
	e.t.Logf("✓ Service %s stopped", serviceName)
}

// StartService 启动服务
func (e *DockerComposeEnv) StartService(serviceName string) {
	e.t.Logf("Starting service: %s", serviceName)
	
	cmd := exec.Command(
		"docker-compose",
		"-f", e.composePath,
		"-p", e.projectName,
		"start", serviceName,
	)
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Logf("Start service output: %s", string(output))
		require.NoError(e.t, err, "Failed to start service")
	}
	
	e.t.Logf("✓ Service %s started", serviceName)
}

// GetLogs 获取服务日志
func (e *DockerComposeEnv) GetLogs(serviceName string) string {
	cmd := exec.Command(
		"docker-compose",
		"-f", e.composePath,
		"-p", e.projectName,
		"logs", "--tail=100", serviceName,
	)
	
	output, err := cmd.Output()
	if err != nil {
		e.t.Logf("Warning: Failed to get logs for %s: %v", serviceName, err)
		return ""
	}
	
	return string(output)
}

// GetAPIClient 获取API客户端
func (e *DockerComposeEnv) GetAPIClient(baseURL string) *E2EAPIClient {
	return NewE2EAPIClient(e.t, baseURL)
}

