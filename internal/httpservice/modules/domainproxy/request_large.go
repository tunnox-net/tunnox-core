// Package domainproxy 提供 HTTP 域名代理功能
package domainproxy

import (
	"net/http"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"

	"github.com/gorilla/websocket"
)

// handleLargeRequest 处理大请求（隧道模式）
func (m *DomainProxyModule) handleLargeRequest(w http.ResponseWriter, r *http.Request) {
	// 1. 查找域名映射
	mapping, err := m.lookupMapping(r.Host)
	if err != nil {
		m.handleError(w, err)
		return
	}

	// 2. 检查客户端是否在线
	if m.deps.SessionMgr == nil {
		m.handleError(w, httpservice.ErrClientOffline)
		return
	}

	conn := m.deps.SessionMgr.GetControlConnectionInterface(mapping.TargetClientID)
	if conn == nil {
		m.handleError(w, httpservice.ErrClientOffline)
		return
	}

	// 3. 构建目标 URL
	scheme := m.config.DefaultScheme
	if scheme == "" {
		scheme = "http"
	}
	targetURL := scheme + "://" + mapping.TargetHost + ":" + itoa(mapping.TargetPort) + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// 4. 请求隧道连接
	corelog.Infof("DomainProxyModule: requesting tunnel for large request, host=%s, content-length=%d, url=%s",
		r.Host, r.ContentLength, targetURL)

	tunnelConn, err := m.deps.SessionMgr.RequestTunnelForHTTP(
		mapping.TargetClientID,
		mapping.ID,
		targetURL,
		r.Method,
	)
	if err != nil {
		corelog.Errorf("DomainProxyModule: failed to create tunnel: %v", err)
		m.handleError(w, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to create tunnel"))
		return
	}
	defer tunnelConn.Close()

	corelog.Debugf("DomainProxyModule: tunnel established, forwarding HTTP request")

	// 5. 写入 HTTP 请求行和头部到隧道
	if err := m.writeHTTPRequestToTunnel(tunnelConn, r, mapping); err != nil {
		corelog.Errorf("DomainProxyModule: failed to write request to tunnel: %v", err)
		m.handleError(w, err)
		return
	}

	// 6. 从隧道读取 HTTP 响应
	if err := m.readHTTPResponseFromTunnel(w, tunnelConn); err != nil {
		corelog.Errorf("DomainProxyModule: failed to read response from tunnel: %v", err)
		// Response may have already been partially written, so we can't call handleError
		return
	}

	corelog.Debugf("DomainProxyModule: tunnel request completed successfully")
}

// handleUserWebSocket 处理用户 WebSocket 请求
func (m *DomainProxyModule) handleUserWebSocket(w http.ResponseWriter, r *http.Request) {
	// 1. 查找域名映射
	mapping, err := m.lookupMapping(r.Host)
	if err != nil {
		m.handleError(w, err)
		return
	}

	// 2. 检查客户端是否在线
	if m.deps.SessionMgr == nil {
		m.handleError(w, httpservice.ErrClientOffline)
		return
	}

	conn := m.deps.SessionMgr.GetControlConnectionInterface(mapping.TargetClientID)
	if conn == nil {
		m.handleError(w, httpservice.ErrClientOffline)
		return
	}

	// 3. 构建目标 WebSocket URL
	scheme := "ws"
	if m.config.DefaultScheme == "https" {
		scheme = "wss"
	}
	targetURL := scheme + "://" + mapping.TargetHost + ":" + itoa(mapping.TargetPort) + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// 4. 请求隧道连接
	corelog.Infof("DomainProxyModule: requesting tunnel for WebSocket, host=%s, url=%s",
		r.Host, targetURL)

	tunnelConn, err := m.deps.SessionMgr.RequestTunnelForHTTP(
		mapping.TargetClientID,
		mapping.ID,
		targetURL,
		"WEBSOCKET",
	)
	if err != nil {
		corelog.Errorf("DomainProxyModule: failed to create WebSocket tunnel: %v", err)
		m.handleError(w, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to create WebSocket tunnel"))
		return
	}
	defer tunnelConn.Close()

	corelog.Debugf("DomainProxyModule: WebSocket tunnel established, upgrading connection")

	// 5. 升级用户连接为 WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for proxy
		},
	}

	userWS, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		corelog.Errorf("DomainProxyModule: failed to upgrade WebSocket: %v", err)
		return
	}
	defer userWS.Close()

	corelog.Infof("DomainProxyModule: WebSocket connection upgraded, starting bidirectional forwarding")

	// 6. 启动双向转发
	m.forwardWebSocket(userWS, tunnelConn)

	corelog.Debugf("DomainProxyModule: WebSocket proxy completed")
}
