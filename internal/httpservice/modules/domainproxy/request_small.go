// Package domainproxy 提供 HTTP 域名代理功能
package domainproxy

import (
	"io"
	"net/http"

	"tunnox-core/internal/cloud/models"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/protocol/httptypes"

	"github.com/google/uuid"
)

// handleSmallRequest 处理小请求（命令模式）
func (m *DomainProxyModule) handleSmallRequest(w http.ResponseWriter, r *http.Request) {
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

	// 3. 构建代理请求
	proxyReq, err := m.buildProxyRequest(r, mapping)
	if err != nil {
		m.handleError(w, err)
		return
	}

	// 4. 发送代理请求
	proxyResp, err := m.deps.SessionMgr.SendHTTPProxyRequest(mapping.TargetClientID, proxyReq)
	if err != nil {
		m.handleError(w, err)
		return
	}

	// 5. 写入响应
	m.writeProxyResponse(w, proxyResp)
}

// buildProxyRequest 构建代理请求
func (m *DomainProxyModule) buildProxyRequest(r *http.Request, mapping *models.PortMapping) (*httptypes.HTTPProxyRequest, error) {
	// 读取请求体
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(r.Body, m.config.CommandModeThreshold))
		if err != nil {
			return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidRequest, "failed to read request body")
		}
	}

	// 构建目标 URL
	scheme := m.config.DefaultScheme
	if scheme == "" {
		scheme = "http"
	}
	targetURL := scheme + "://" + mapping.TargetHost + ":" + itoa(mapping.TargetPort) + r.URL.Path
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	// 提取请求头
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			// 跳过 hop-by-hop 头
			if isHopByHopHeader(key) {
				continue
			}
			headers[key] = values[0]
		}
	}

	// 添加 X-Forwarded 头
	headers["X-Forwarded-For"] = r.RemoteAddr
	headers["X-Forwarded-Host"] = r.Host
	headers["X-Forwarded-Proto"] = scheme

	return &httptypes.HTTPProxyRequest{
		RequestID: uuid.New().String(),
		Method:    r.Method,
		URL:       targetURL,
		Headers:   headers,
		Body:      body,
		Timeout:   int(m.config.RequestTimeout.Seconds()),
	}, nil
}

// writeProxyResponse 写入代理响应
func (m *DomainProxyModule) writeProxyResponse(w http.ResponseWriter, resp *httptypes.HTTPProxyResponse) {
	if resp == nil {
		http.Error(w, "Empty response from backend", http.StatusBadGateway)
		return
	}

	// 检查错误
	if resp.Error != "" {
		corelog.Warnf("DomainProxyModule: proxy error: %s", resp.Error)
		http.Error(w, resp.Error, http.StatusBadGateway)
		return
	}

	// 写入响应头
	for key, value := range resp.Headers {
		if !isHopByHopHeader(key) {
			w.Header().Set(key, value)
		}
	}

	// 写入状态码
	w.WriteHeader(resp.StatusCode)

	// 写入响应体
	if len(resp.Body) > 0 {
		w.Write(resp.Body)
	}
}
