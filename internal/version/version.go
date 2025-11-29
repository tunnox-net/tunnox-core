package version

import (
	"os"
	"strings"
)

var (
	// Version 版本号，从 VERSION 文件读取，构建时可通过 -ldflags 覆盖
	// 默认值 "dev"，构建时会通过 -ldflags 注入实际版本号
	Version = "dev"

	// BuildTime 构建时间，通过 -ldflags 注入
	BuildTime = ""

	// GitCommit Git 提交哈希，通过 -ldflags 注入
	GitCommit = ""
)

func init() {
	// 如果版本号仍然是默认值 "dev"，尝试从 VERSION 文件读取
	if Version == "dev" {
		Version = readVersionFromFile()
	}
}

// readVersionFromFile 从 VERSION 文件读取版本号
func readVersionFromFile() string {
	// 尝试从 VERSION 文件读取
	data, err := os.ReadFile("VERSION")
	if err != nil {
		// 如果文件不存在，尝试从当前目录的父目录查找
		data, err = os.ReadFile("../VERSION")
		if err != nil {
			return "dev"
		}
	}

	version := strings.TrimSpace(string(data))
	if version == "" {
		return "dev"
	}

	// 移除可能的 'v' 前缀
	version = strings.TrimPrefix(version, "v")
	return version
}

// GetVersion 获取完整版本信息
func GetVersion() string {
	version := "v" + Version
	if BuildTime != "" {
		version += " (built " + BuildTime + ")"
	}
	if GitCommit != "" {
		version += " commit " + GitCommit[:8]
	}
	return version
}

// GetShortVersion 获取简短版本号
func GetShortVersion() string {
	return "v" + Version
}

