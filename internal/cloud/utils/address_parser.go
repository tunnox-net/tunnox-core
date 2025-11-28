package utils

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParseListenAddress 解析监听地址
//
// 格式：0.0.0.0:7788 或 127.0.0.1:9999
// 返回：主机地址、端口、错误
func ParseListenAddress(addr string) (string, int, error) {
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

	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port %d out of range [1, 65535]", port)
	}

	return host, port, nil
}

// ParseTargetAddress 解析目标地址
//
// 格式：tcp://10.51.22.69:3306 或 udp://192.168.1.1:53
// 返回：主机地址、端口、协议、错误
func ParseTargetAddress(addr string) (string, int, string, error) {
	if addr == "" {
		return "", 0, "", fmt.Errorf("target address is empty")
	}

	// 解析 URL 格式：tcp://host:port
	parsedURL, err := url.Parse(addr)
	if err != nil {
		// 如果不是 URL 格式，尝试直接解析为 host:port（默认 tcp）
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return "", 0, "", fmt.Errorf("invalid target address format %q: %w", addr, err)
		}

		portNum, err := strconv.Atoi(port)
		if err != nil {
			return "", 0, "", fmt.Errorf("invalid port in target address %q: %w", addr, err)
		}

		if portNum < 1 || portNum > 65535 {
			return "", 0, "", fmt.Errorf("port %d out of range [1, 65535]", portNum)
		}

		return host, portNum, "tcp", nil
	}

	// 从 URL 解析
	protocol := strings.ToLower(parsedURL.Scheme)
	if protocol == "" {
		protocol = "tcp" // 默认协议
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

	if port < 1 || port > 65535 {
		return "", 0, "", fmt.Errorf("port %d out of range [1, 65535]", port)
	}

	return host, port, protocol, nil
}
