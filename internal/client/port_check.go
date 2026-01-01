// Package client 端口检查工具
package client

import (
	"fmt"
	"net"
	"strings"

	coreerrors "tunnox-core/internal/core/errors"
)

// CheckPortAvailable 检查指定端口是否可用
// 通过尝试监听端口来判断，如果能监听则端口可用，立即释放
func CheckPortAvailable(host string, port int) error {
	if port <= 0 || port > 65535 {
		return coreerrors.Newf(coreerrors.CodeInvalidParam, "invalid port number: %d", port)
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// 检查是否是端口被占用的错误
		if strings.Contains(err.Error(), "address already in use") ||
			strings.Contains(err.Error(), "bind: address already in use") {
			return coreerrors.Newf(coreerrors.CodePortConflict,
				"port %d is already in use", port)
		}
		return coreerrors.Wrapf(err, coreerrors.CodeNetworkError,
			"failed to check port %d availability", port)
	}

	// 端口可用，立即释放
	listener.Close()
	return nil
}

// CanBindPort 检查监听地址是否可绑定
// listenAddr 格式: "host:port" 或 ":port"
func CanBindPort(listenAddr string) error {
	if listenAddr == "" {
		return coreerrors.New(coreerrors.CodeInvalidParam, "listen address is empty")
	}

	// 直接使用 net.SplitHostPort 解析，避免 parseListenAddress 对端口 0 的限制
	host, portStr, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return coreerrors.Wrapf(err, coreerrors.CodeInvalidParam,
			"invalid listen address format: %s", listenAddr)
	}

	// 解析端口
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return coreerrors.Wrapf(err, coreerrors.CodeInvalidParam,
			"invalid port in listen address: %s", listenAddr)
	}

	// 端口为 0 表示自动分配，不需要检查
	if port == 0 {
		return nil
	}

	return CheckPortAvailable(host, port)
}
