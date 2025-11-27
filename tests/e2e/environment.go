package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// EnvironmentChecker 环境检查器
type EnvironmentChecker struct {
	t *testing.T
}

// NewEnvironmentChecker 创建环境检查器
func NewEnvironmentChecker(t *testing.T) *EnvironmentChecker {
	return &EnvironmentChecker{t: t}
}

// CheckDocker 检查Docker是否安装并运行
func (e *EnvironmentChecker) CheckDocker() error {
	e.t.Log("Checking Docker availability...")
	
	// 检查docker命令是否存在
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker command not found: %w", err)
	}
	
	// 检查docker是否运行
	cmd := exec.Command("docker", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker is not running: %w\nOutput: %s", err, string(output))
	}
	
	e.t.Log("✓ Docker is available and running")
	return nil
}

// CheckDockerCompose 检查docker-compose是否安装
func (e *EnvironmentChecker) CheckDockerCompose() error {
	e.t.Log("Checking docker-compose availability...")
	
	if _, err := exec.LookPath("docker-compose"); err != nil {
		return fmt.Errorf("docker-compose command not found: %w", err)
	}
	
	cmd := exec.Command("docker-compose", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker-compose version check failed: %w", err)
	}
	
	version := strings.TrimSpace(string(output))
	e.t.Logf("✓ docker-compose is available: %s", version)
	return nil
}

// CheckPortAvailability 检查端口是否可用
func (e *EnvironmentChecker) CheckPortAvailability(ports []int) error {
	e.t.Logf("Checking port availability: %v", ports)
	
	for _, port := range ports {
		cmd := exec.Command("sh", "-c", 
			fmt.Sprintf("lsof -i :%d -sTCP:LISTEN -t", port))
		output, _ := cmd.CombinedOutput()
		
		if len(output) > 0 {
			pid := strings.TrimSpace(string(output))
			return fmt.Errorf("port %d is already in use (PID: %s)", port, pid)
		}
	}
	
	e.t.Logf("✓ All ports are available")
	return nil
}

// CheckAll 执行所有环境检查
func (e *EnvironmentChecker) CheckAll() error {
	checks := []func() error{
		e.CheckDocker,
		e.CheckDockerCompose,
	}
	
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	
	return nil
}

// EnvironmentCleaner 环境清理器
type EnvironmentCleaner struct {
	t *testing.T
}

// NewEnvironmentCleaner 创建环境清理器
func NewEnvironmentCleaner(t *testing.T) *EnvironmentCleaner {
	return &EnvironmentCleaner{t: t}
}

// CleanupDockerContainers 清理所有E2E测试相关的Docker容器
func (c *EnvironmentCleaner) CleanupDockerContainers() error {
	c.t.Log("Cleaning up Docker containers...")
	
	// 停止所有包含 "e2e" 或 "tunnox" 的容器
	cmd := exec.Command("sh", "-c", 
		`docker ps -a --filter "name=e2e" --filter "name=tunnox" -q`)
	output, err := cmd.Output()
	if err != nil {
		c.t.Logf("Warning: Failed to list containers: %v", err)
		return nil
	}
	
	containerIDs := strings.Fields(string(output))
	if len(containerIDs) == 0 {
		c.t.Log("✓ No containers to clean up")
		return nil
	}
	
	c.t.Logf("Found %d containers to clean up", len(containerIDs))
	
	// 停止容器
	stopCmd := append([]string{"stop"}, containerIDs...)
	if err := exec.Command("docker", stopCmd...).Run(); err != nil {
		c.t.Logf("Warning: Failed to stop containers: %v", err)
	}
	
	// 删除容器
	rmCmd := append([]string{"rm", "-f"}, containerIDs...)
	if err := exec.Command("docker", rmCmd...).Run(); err != nil {
		c.t.Logf("Warning: Failed to remove containers: %v", err)
	}
	
	c.t.Logf("✓ Cleaned up %d containers", len(containerIDs))
	return nil
}

// CleanupDockerNetworks 清理所有E2E测试相关的Docker网络
func (c *EnvironmentCleaner) CleanupDockerNetworks() error {
	c.t.Log("Cleaning up Docker networks...")
	
	cmd := exec.Command("docker", "network", "prune", "-f", 
		"--filter", "label=com.docker.compose.project")
	if err := cmd.Run(); err != nil {
		c.t.Logf("Warning: Failed to prune networks: %v", err)
		return nil
	}
	
	c.t.Log("✓ Cleaned up Docker networks")
	return nil
}

// CleanupDockerVolumes 清理未使用的Docker卷
func (c *EnvironmentCleaner) CleanupDockerVolumes() error {
	c.t.Log("Cleaning up Docker volumes...")
	
	cmd := exec.Command("docker", "volume", "prune", "-f")
	if err := cmd.Run(); err != nil {
		c.t.Logf("Warning: Failed to prune volumes: %v", err)
		return nil
	}
	
	c.t.Log("✓ Cleaned up Docker volumes")
	return nil
}

// CleanAll 执行完整清理
func (c *EnvironmentCleaner) CleanAll() error {
	cleanups := []func() error{
		c.CleanupDockerContainers,
		c.CleanupDockerNetworks,
		c.CleanupDockerVolumes,
	}
	
	var lastErr error
	for _, cleanup := range cleanups {
		if err := cleanup(); err != nil {
			lastErr = err
		}
	}
	
	return lastErr
}

