package e2e

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// SkipIfShort 如果是短测试则跳过
func SkipIfShort(t *testing.T, reason string) {
	t.Helper()
	if testing.Short() {
		t.Skipf("Skipping E2E test in short mode: %s", reason)
	}
}

// SetupE2EEnvironment 设置E2E测试环境
func SetupE2EEnvironment(t *testing.T, composeFile string) *DockerComposeEnv {
	t.Helper()
	env := NewDockerComposeEnv(t, composeFile)
	return env
}

// GetClientIDFromContainer 从Docker容器日志中提取ClientID
// containerSuffix: 容器名称后缀，如 "client-a", "client-b"
// 实际容器名称可能是 "tunnox-e2e-1764269935-281000-client-a"
func GetClientIDFromContainer(t *testing.T, containerSuffix string) (int64, error) {
	t.Helper()

	// 最多重试10次，每次等待1秒
	for attempt := 0; attempt < 10; attempt++ {
		// 获取所有容器，然后过滤包含后缀的容器
		listCmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}")
		var listOut bytes.Buffer
		listCmd.Stdout = &listOut

		if err := listCmd.Run(); err != nil {
			t.Logf("Attempt %d: failed to list containers: %v", attempt+1, err)
			if attempt < 9 {
				exec.Command("sleep", "1").Run()
			}
			continue
		}

		// 查找包含后缀的容器名
		var actualContainerName string
		for _, name := range strings.Split(listOut.String(), "\n") {
			if strings.Contains(name, containerSuffix) {
				actualContainerName = strings.TrimSpace(name)
				break
			}
		}

		if actualContainerName == "" {
			t.Logf("Attempt %d: no container found with suffix %s", attempt+1, containerSuffix)
			if attempt < 9 {
				exec.Command("sleep", "1").Run()
			}
			continue
		}

		t.Logf("Attempt %d: found container %s (suffix: %s)", attempt+1, actualContainerName, containerSuffix)

		// 执行docker logs命令查找ClientID
		cmd := exec.Command("docker", "logs", actualContainerName)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out

		if err := cmd.Run(); err != nil {
			t.Logf("Attempt %d: failed to get logs from %s: %v", attempt+1, actualContainerName, err)
			if attempt < 9 {
				exec.Command("sleep", "1").Run()
			}
			continue
		}

		logs := out.String()

		// 查找类似 "ClientID=12345678" 的行
		for _, line := range strings.Split(logs, "\n") {
			if strings.Contains(line, "ClientID=") && strings.Contains(line, "authenticated as anonymous client") {
				// 提取ClientID
				parts := strings.Split(line, "ClientID=")
				if len(parts) >= 2 {
					idPart := strings.Split(parts[1], ",")[0]
					var clientID int64
					_, err := fmt.Sscanf(idPart, "%d", &clientID)
					if err == nil {
						t.Logf("✅ Found ClientID=%d for container %s", clientID, actualContainerName)
						return clientID, nil
					}
				}
			}
		}

		t.Logf("Attempt %d: ClientID not yet found in %s logs, waiting...", attempt+1, actualContainerName)
		if attempt < 9 {
			exec.Command("sleep", "1").Run()
		}
	}

	return 0, fmt.Errorf("ClientID not found in logs for container with suffix %s after 10 attempts", containerSuffix)
}
