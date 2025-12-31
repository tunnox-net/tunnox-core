package client

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	coreerrors "tunnox-core/internal/core/errors"
)

// validatePort 验证端口号是否在有效范围内
func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return coreerrors.Newf(coreerrors.CodeInvalidParam, "port %d out of range [1, 65535]", port)
	}
	return nil
}

// parseListenAddress 解析监听地址 "127.0.0.1:8888" -> ("127.0.0.1", 8888, nil)
func parseListenAddress(addr string) (string, int, error) {
	if addr == "" {
		return "", 0, coreerrors.New(coreerrors.CodeInvalidParam, "listen address is empty")
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, coreerrors.Wrapf(err, coreerrors.CodeInvalidParam, "invalid listen address format %q", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, coreerrors.Wrapf(err, coreerrors.CodeInvalidParam, "invalid port in listen address %q", addr)
	}
	if err := validatePort(port); err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// parseTargetAddress 解析目标地址 "tcp://10.51.22.69:3306" -> ("10.51.22.69", 3306, "tcp", nil)
// 特殊格式：socks5://0.0.0.0:0 表示 SOCKS5 代理模式（动态目标）
func parseTargetAddress(addr string) (string, int, string, error) {
	if addr == "" {
		return "", 0, "", coreerrors.New(coreerrors.CodeInvalidParam, "target address is empty")
	}

	// 解析 URL 格式：tcp://host:port
	parsedURL, err := url.Parse(addr)
	if err != nil || parsedURL.Scheme == "" {
		// 如果不是URL格式，尝试直接解析为 host:port
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return "", 0, "", coreerrors.Wrapf(err, coreerrors.CodeInvalidParam, "invalid target address format %q", addr)
		}
		portNum, err := strconv.Atoi(port)
		if err != nil {
			return "", 0, "", coreerrors.Wrapf(err, coreerrors.CodeInvalidParam, "invalid port in target address %q", addr)
		}
		if err := validatePort(portNum); err != nil {
			return "", 0, "", err
		}
		return host, portNum, "tcp", nil // 默认协议为tcp
	}

	// 从 URL 解析
	protocol := strings.ToLower(parsedURL.Scheme)
	if protocol == "" {
		protocol = "tcp"
	}
	host := parsedURL.Hostname()
	if host == "" {
		return "", 0, "", coreerrors.Newf(coreerrors.CodeInvalidParam, "missing host in target address %q", addr)
	}
	portStr := parsedURL.Port()
	if portStr == "" {
		return "", 0, "", coreerrors.Newf(coreerrors.CodeInvalidParam, "missing port in target address %q", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, "", coreerrors.Wrapf(err, coreerrors.CodeInvalidParam, "invalid port in target address %q", addr)
	}

	// SOCKS5 协议特殊处理：socks5://0.0.0.0:0 是有效的（表示动态目标代理模式）
	if protocol == "socks5" {
		// 对于 SOCKS5，端口 0 是允许的（表示动态目标）
		if port != 0 {
			if err := validatePort(port); err != nil {
				return "", 0, "", err
			}
		}
	} else {
		if err := validatePort(port); err != nil {
			return "", 0, "", err
		}
	}

	return host, port, protocol, nil
}
