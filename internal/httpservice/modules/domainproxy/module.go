// Package domainproxy 提供 HTTP 域名代理功能
// 支持通过域名访问内网服务，无需端口映射
package domainproxy

import (
	"context"
	"fmt"
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
	"github.com/gorilla/websocket"
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

// ============================================================================
// Tunnel Mode HTTP Request/Response Forwarding
// ============================================================================

// writeHTTPRequestToTunnel 写入 HTTP 请求到隧道
func (m *DomainProxyModule) writeHTTPRequestToTunnel(
	tunnelConn httpservice.TunnelConnectionInterface,
	r *http.Request,
	mapping *models.PortMapping,
) error {
	// 1. 写入请求行
	requestLine := r.Method + " " + r.URL.RequestURI() + " HTTP/1.1\r\n"
	if _, err := tunnelConn.Write([]byte(requestLine)); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write request line")
	}

	// 2. 写入请求头
	// 添加/修改必要的头部
	headers := r.Header.Clone()

	// 设置 Host 头
	headers.Set("Host", mapping.TargetHost+":"+itoa(mapping.TargetPort))

	// 添加 X-Forwarded 头
	scheme := m.config.DefaultScheme
	if scheme == "" {
		scheme = "http"
	}
	headers.Set("X-Forwarded-For", r.RemoteAddr)
	headers.Set("X-Forwarded-Host", r.Host)
	headers.Set("X-Forwarded-Proto", scheme)

	// 移除 hop-by-hop 头
	headers.Del("Connection")
	headers.Del("Keep-Alive")
	headers.Del("Proxy-Authenticate")
	headers.Del("Proxy-Authorization")
	headers.Del("Te")
	headers.Del("Trailers")
	headers.Del("Transfer-Encoding")
	headers.Del("Upgrade")

	// 写入所有头部
	for key, values := range headers {
		for _, value := range values {
			headerLine := key + ": " + value + "\r\n"
			if _, err := tunnelConn.Write([]byte(headerLine)); err != nil {
				return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write header")
			}
		}
	}

	// 3. 写入空行（头部结束标记）
	if _, err := tunnelConn.Write([]byte("\r\n")); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to write header end")
	}

	// 4. 复制请求体
	if r.Body != nil {
		defer r.Body.Close()

		buf := make([]byte, 32*1024) // 32KB buffer
		for {
			n, err := r.Body.Read(buf)
			if n > 0 {
				if _, writeErr := tunnelConn.Write(buf[:n]); writeErr != nil {
					return coreerrors.Wrap(writeErr, coreerrors.CodeNetworkError, "failed to write request body")
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read request body")
			}
		}
	}

	corelog.Debugf("DomainProxyModule: HTTP request written to tunnel successfully")
	return nil
}

// readHTTPResponseFromTunnel 从隧道读取 HTTP 响应
func (m *DomainProxyModule) readHTTPResponseFromTunnel(
	w http.ResponseWriter,
	tunnelConn httpservice.TunnelConnectionInterface,
) error {
	// 使用 bufio.Reader 读取 HTTP 响应
	reader := &tunnelReader{conn: tunnelConn}
	bufReader := io.Reader(reader)

	// 读取响应（使用简单的状态机解析）
	// 1. 读取状态行
	statusLine, err := m.readLine(bufReader)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read status line")
	}

	// 解析状态码
	var statusCode int
	if _, err := fmt.Sscanf(statusLine, "HTTP/1.%d %d", new(int), &statusCode); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInvalidRequest, "invalid status line")
	}

	// 2. 读取响应头
	headers := make(http.Header)
	for {
		line, err := m.readLine(bufReader)
		if err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read header")
		}

		// 空行表示头部结束
		if line == "" {
			break
		}

		// 解析头部
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if !isHopByHopHeader(key) {
				headers.Add(key, value)
			}
		}
	}

	// 3. 写入响应头到客户端
	for key, values := range headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(statusCode)

	// 4. 复制响应体
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := bufReader.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return coreerrors.Wrap(writeErr, coreerrors.CodeNetworkError, "failed to write response body")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read response body")
		}
	}

	corelog.Debugf("DomainProxyModule: HTTP response read from tunnel successfully")
	return nil
}

// readLine 从 reader 读取一行（以 \r\n 结尾）
func (m *DomainProxyModule) readLine(reader io.Reader) (string, error) {
	var line []byte
	buf := make([]byte, 1)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			return "", err
		}
		if n == 0 {
			continue
		}

		line = append(line, buf[0])

		// 检查是否为 \r\n 结尾
		if len(line) >= 2 && line[len(line)-2] == '\r' && line[len(line)-1] == '\n' {
			return string(line[:len(line)-2]), nil
		}
	}
}

// tunnelReader 包装 TunnelConnectionInterface 为 io.Reader
type tunnelReader struct {
	conn httpservice.TunnelConnectionInterface
}

func (r *tunnelReader) Read(p []byte) (int, error) {
	return r.conn.Read(p)
}

// ============================================================================
// WebSocket Proxy Forwarding
// ============================================================================

// forwardWebSocket 双向转发 WebSocket 数据
func (m *DomainProxyModule) forwardWebSocket(userWS *websocket.Conn, tunnelConn httpservice.TunnelConnectionInterface) {
	// 创建错误通道
	errChan := make(chan error, 2)

	// 用户 → 隧道
	go m.forwardUserToTunnel(userWS, tunnelConn, errChan)

	// 隧道 → 用户
	go m.forwardTunnelToUser(tunnelConn, userWS, errChan)

	// 等待任一方向出错或关闭
	err := <-errChan
	if err != nil && err != io.EOF {
		corelog.Debugf("DomainProxyModule: WebSocket forwarding stopped: %v", err)
	}

	// 关闭连接
	userWS.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second))
}

// forwardUserToTunnel 转发用户 WebSocket 消息到隧道
func (m *DomainProxyModule) forwardUserToTunnel(userWS *websocket.Conn, tunnelConn httpservice.TunnelConnectionInterface, errChan chan error) {
	defer func() {
		if r := recover(); r != nil {
			corelog.Errorf("DomainProxyModule: panic in forwardUserToTunnel: %v", r)
			errChan <- fmt.Errorf("panic: %v", r)
		}
	}()

	for {
		// 读取 WebSocket 消息
		messageType, data, err := userWS.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				corelog.Debugf("DomainProxyModule: user WebSocket closed normally")
				errChan <- io.EOF
			} else {
				corelog.Errorf("DomainProxyModule: failed to read from user WebSocket: %v", err)
				errChan <- err
			}
			return
		}

		// 写入隧道（格式：1字节类型 + 数据）
		frame := make([]byte, 1+len(data))
		frame[0] = byte(messageType)
		copy(frame[1:], data)

		if _, err := tunnelConn.Write(frame); err != nil {
			corelog.Errorf("DomainProxyModule: failed to write to tunnel: %v", err)
			errChan <- err
			return
		}

		corelog.Debugf("DomainProxyModule: forwarded %d bytes from user to tunnel (type=%d)", len(data), messageType)
	}
}

// forwardTunnelToUser 转发隧道消息到用户 WebSocket
func (m *DomainProxyModule) forwardTunnelToUser(tunnelConn httpservice.TunnelConnectionInterface, userWS *websocket.Conn, errChan chan error) {
	defer func() {
		if r := recover(); r != nil {
			corelog.Errorf("DomainProxyModule: panic in forwardTunnelToUser: %v", r)
			errChan <- fmt.Errorf("panic: %v", r)
		}
	}()

	buf := make([]byte, 32*1024) // 32KB buffer

	for {
		// 读取隧道数据（格式：1字节类型 + 数据）
		n, err := tunnelConn.Read(buf)
		if err != nil {
			if err == io.EOF {
				corelog.Debugf("DomainProxyModule: tunnel closed")
				errChan <- io.EOF
			} else {
				corelog.Errorf("DomainProxyModule: failed to read from tunnel: %v", err)
				errChan <- err
			}
			return
		}

		if n < 1 {
			continue
		}

		// 解析消息类型和数据
		messageType := int(buf[0])
		data := buf[1:n]

		// 写入用户 WebSocket
		if err := userWS.WriteMessage(messageType, data); err != nil {
			corelog.Errorf("DomainProxyModule: failed to write to user WebSocket: %v", err)
			errChan <- err
			return
		}

		corelog.Debugf("DomainProxyModule: forwarded %d bytes from tunnel to user (type=%d)", len(data), messageType)
	}
}
