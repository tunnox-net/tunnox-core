package utils

import (
	"os"
	"path/filepath"
	"strings"
	coreErrors "tunnox-core/internal/core/errors"
)

// ExpandPath 展开路径，支持 ~ 和相对路径
// 例如：~/logs/app.log -> /home/user/logs/app.log
//      ./logs/app.log -> /current/dir/logs/app.log
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", coreErrors.New(coreErrors.ErrorTypePermanent, "path is empty")
	}

	// 展开 ~
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to get home directory")
		}
		path = filepath.Join(homeDir, path[2:])
	} else if path == "~" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to get home directory")
		}
		path = homeDir
	}

	// 转换为绝对路径（处理相对路径）
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to convert to absolute path")
		}
		path = absPath
	}

	return path, nil
}

