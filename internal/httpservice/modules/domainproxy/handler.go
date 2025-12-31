// Package domainproxy 提供 HTTP 域名代理功能
package domainproxy

import (
	"net/http"
	"strings"
)

// ServeHTTP 实现 http.Handler 接口
func (m *DomainProxyModule) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. 检查是否为 WebSocket 升级请求
	if isWebSocketUpgrade(r) {
		m.handleUserWebSocket(w, r)
		return
	}

	// 2. 检查是否为大请求（需要隧道模式）
	if m.isLargeRequest(r) {
		m.handleLargeRequest(w, r)
		return
	}

	// 3. 小请求使用命令模式
	m.handleSmallRequest(w, r)
}

// isWebSocketUpgrade 检查是否为 WebSocket 升级请求
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// isLargeRequest 检查是否为大请求
func (m *DomainProxyModule) isLargeRequest(r *http.Request) bool {
	// 上传大文件
	if r.ContentLength > m.config.CommandModeThreshold {
		return true
	}
	// 未知大小的流式请求
	if r.ContentLength == -1 && r.Header.Get("Transfer-Encoding") == "chunked" {
		return true
	}
	return false
}
