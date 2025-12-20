package client

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// validatePort 验证端口号是否在有效范围内
func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("port %d out of range [1, 65535]", port)
	}
	return nil
}

// parseListenAddress 解析监听地址 "127.0.0.1:8888" -> ("127.0.0.1", 8888, nil)
func parseListenAddress(addr string) (string, int, error) {
	if addr == "" {
		return "", 0, fmt.Errorf("listen address is empty")
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid listen address format %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port in listen address %q: %w", addr, err)
	}
	if err := validatePort(port); err != nil {
		return "", 0, err
	}
	return host, port, nil
}

// parseTargetAddress 解析目标地址 "tcp://10.51.22.69:3306" -> ("10.51.22.69", 3306, "tcp", nil)
func parseTargetAddress(addr string) (string, int, string, error) {
	if addr == "" {
		return "", 0, "", fmt.Errorf("target address is empty")
	}

	// 解析 URL 格式：tcp://host:port
	parsedURL, err := url.Parse(addr)
	if err != nil || parsedURL.Scheme == "" {
		// 如果不是URL格式，尝试直接解析为 host:port
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return "", 0, "", fmt.Errorf("invalid target address format %q: %w", addr, err)
		}
		portNum, err := strconv.Atoi(port)
		if err != nil {
			return "", 0, "", fmt.Errorf("invalid port in target address %q: %w", addr, err)
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
		return "", 0, "", fmt.Errorf("missing host in target address %q", addr)
	}
	portStr := parsedURL.Port()
	if portStr == "" {
		return "", 0, "", fmt.Errorf("missing port in target address %q", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, "", fmt.Errorf("invalid port in target address %q: %w", addr, err)
	}
	if err := validatePort(port); err != nil {
		return "", 0, "", err
	}
	return host, port, protocol, nil
}
