// Package domainproxy 提供 HTTP 域名代理功能
// 支持通过域名访问内网服务，无需端口映射
package domainproxy

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/dispose"
	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// DomainProxyModule 域名代理模块
type DomainProxyModule struct {
	*dispose.ServiceBase

	config   *httpservice.DomainProxyModuleConfig
	deps     *httpservice.ModuleDependencies
	registry *httpservice.DomainRegistry

	// HTTP 客户端（用于命令模式响应）
	httpClient *http.Client
}

// NewDomainProxyModule 创建域名代理模块
func NewDomainProxyModule(ctx context.Context, config *httpservice.DomainProxyModuleConfig) *DomainProxyModule {
	m := &DomainProxyModule{
		ServiceBase: dispose.NewService("DomainProxyModule", ctx),
		config:      config,
		httpClient: &http.Client{
			Timeout: config.RequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	return m
}

// Name 返回模块名称
func (m *DomainProxyModule) Name() string {
	return "DomainProxy"
}

// SetDependencies 注入依赖
func (m *DomainProxyModule) SetDependencies(deps *httpservice.ModuleDependencies) {
	m.deps = deps
	m.registry = deps.DomainRegistry
}

// RegisterRoutes 注册路由
// 域名代理使用默认路由（/*），根据 Host Header 进行路由
func (m *DomainProxyModule) RegisterRoutes(router *mux.Router) {
	// 域名代理作为默认路由，处理所有未匹配的请求
	// 注意：这个路由应该最后注册，优先级最低
	router.PathPrefix("/").Handler(m).Methods("GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD")

	corelog.Infof("DomainProxyModule: registered default route for domain proxy")
}

// Start 启动模块
func (m *DomainProxyModule) Start() error {
	corelog.Infof("DomainProxyModule: started with %d base domains", len(m.config.BaseDomains))
	return nil
}

// Stop 停止模块
func (m *DomainProxyModule) Stop() error {
	corelog.Infof("DomainProxyModule: stopped")
	return nil
}

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

	// TODO: 实现隧道模式
	// 目前先返回错误，后续实现
	corelog.Warnf("DomainProxyModule: large request not supported yet, host=%s, content-length=%d",
		r.Host, r.ContentLength)
	http.Error(w, "Large request not supported yet", http.StatusNotImplemented)
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

	// TODO: 实现 WebSocket 代理
	// 目前先返回错误，后续实现
	corelog.Warnf("DomainProxyModule: WebSocket proxy not supported yet, host=%s", r.Host)
	http.Error(w, "WebSocket proxy not supported yet", http.StatusNotImplemented)
}

// lookupMapping 查找域名映射
func (m *DomainProxyModule) lookupMapping(host string) (*models.PortMapping, error) {
	if m.registry == nil {
		return nil, httpservice.ErrDomainNotFound
	}

	mapping, found := m.registry.LookupByHost(host)
	if !found {
		corelog.Debugf("DomainProxyModule: domain not found: %s", host)
		return nil, httpservice.ErrDomainNotFound
	}

	// 检查映射状态
	if mapping.Status != models.MappingStatusActive {
		return nil, coreerrors.Newf(coreerrors.CodeUnavailable, "mapping is not active: %s", mapping.Status)
	}

	// 检查是否已撤销
	if mapping.IsRevoked {
		return nil, coreerrors.New(coreerrors.CodeForbidden, "mapping has been revoked")
	}

	// 检查是否已过期
	if mapping.ExpiresAt != nil && time.Now().After(*mapping.ExpiresAt) {
		return nil, coreerrors.New(coreerrors.CodeForbidden, "mapping has expired")
	}

	return mapping, nil
}

// buildProxyRequest 构建代理请求
func (m *DomainProxyModule) buildProxyRequest(r *http.Request, mapping *models.PortMapping) (*httpservice.HTTPProxyRequest, error) {
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

	return &httpservice.HTTPProxyRequest{
		RequestID: uuid.New().String(),
		Method:    r.Method,
		URL:       targetURL,
		Headers:   headers,
		Body:      body,
		Timeout:   int(m.config.RequestTimeout.Seconds()),
	}, nil
}

// writeProxyResponse 写入代理响应
func (m *DomainProxyModule) writeProxyResponse(w http.ResponseWriter, resp *httpservice.HTTPProxyResponse) {
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

// handleError 处理错误
func (m *DomainProxyModule) handleError(w http.ResponseWriter, err error) {
	var statusCode int
	var message string

	switch {
	case coreerrors.Is(err, httpservice.ErrDomainNotFound):
		statusCode = http.StatusNotFound
		message = "Domain not found"
	case coreerrors.Is(err, httpservice.ErrClientOffline):
		statusCode = http.StatusServiceUnavailable
		message = "Backend service unavailable"
	case coreerrors.Is(err, httpservice.ErrProxyTimeout):
		statusCode = http.StatusGatewayTimeout
		message = "Request timeout"
	case coreerrors.Is(err, httpservice.ErrBaseDomainNotAllow):
		statusCode = http.StatusForbidden
		message = "Domain not allowed"
	default:
		statusCode = http.StatusInternalServerError
		message = "Internal server error"
	}

	corelog.Warnf("DomainProxyModule: error: %v", err)
	http.Error(w, message, statusCode)
}

// isHopByHopHeader 检查是否为 hop-by-hop 头
func isHopByHopHeader(header string) bool {
	hopByHopHeaders := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
	}
	return hopByHopHeaders[header]
}

// itoa 整数转字符串
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}

	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	return string(result)
}
