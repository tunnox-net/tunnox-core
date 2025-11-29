package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetDefaultClientLogPath 获取客户端默认日志路径列表（按优先级排序）
func GetDefaultClientLogPath(interactive bool) []string {
	var paths []string

	// 交互模式：优先使用用户目录
	if interactive {
		if homeDir, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(homeDir, ".tunnox", "logs", "client.log"))
		}
		// 备选：当前工作目录
		if workDir, err := os.Getwd(); err == nil {
			paths = append(paths, filepath.Join(workDir, "logs", "client.log"))
		}
	} else {
		// 守护进程模式：优先用户目录，然后工作目录
		if homeDir, err := os.UserHomeDir(); err == nil {
			paths = append(paths, filepath.Join(homeDir, ".tunnox", "logs", "client.log"))
		}
		if workDir, err := os.Getwd(); err == nil {
			paths = append(paths, filepath.Join(workDir, "logs", "client.log"))
		}
	}

	// 最后备选：临时目录
	paths = append(paths, "/tmp/tunnox-client.log")

	return paths
}

// GetDefaultServerLogPath 获取服务端默认日志路径（按优先级尝试）
func GetDefaultServerLogPath() string {
	candidates := []string{}

	// 1. 相对于可执行文件的路径
	if execPath, err := os.Executable(); err == nil {
		execDir := filepath.Dir(execPath)
		candidates = append(candidates, filepath.Join(execDir, "logs", "server.log"))
	}

	// 2. 当前工作目录
	if workDir, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(workDir, "logs", "server.log"))
	}

	// 3. 用户目录
	if homeDir, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(homeDir, ".tunnox", "logs", "server.log"))
	}

	// 4. 最后备选：临时目录
	candidates = append(candidates, "/tmp/tunnox-server.log")

	// 尝试解析路径（会自动检查可写性）
	if path, err := ResolveLogPath(candidates); err == nil {
		return path
	}

	// 如果所有路径都失败，返回最后一个（至少尝试）
	return candidates[len(candidates)-1]
}

// ResolveLogPath 解析并验证日志路径，返回第一个可用的路径
func ResolveLogPath(candidates []string) (string, error) {
	for _, candidate := range candidates {
		// 展开路径（支持 ~ 和相对路径）
		expandedPath, err := ExpandPath(candidate)
		if err != nil {
			continue
		}

		// 检查是否可以写入
		if canWriteToPath(expandedPath) {
			return expandedPath, nil
		}
	}

	// 所有路径都失败，返回最后一个路径（至少尝试创建）
	if len(candidates) > 0 {
		lastPath, err := ExpandPath(candidates[len(candidates)-1])
		if err != nil {
			return "", fmt.Errorf("failed to resolve any log path: %w", err)
		}
		return lastPath, nil
	}

	return "", fmt.Errorf("no log path candidates provided")
}

// canWriteToPath 检查是否可以写入指定路径（包括目录创建）
func canWriteToPath(path string) bool {
	// 确保目录存在
	logDir := filepath.Dir(path)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return false
	}

	// 尝试打开文件（创建或追加）
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return false
	}
	_ = file.Close()

	return true
}

