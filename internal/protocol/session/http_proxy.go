// Package session HTTP 代理功能扩展
// 实现 Server 端到 Client 端的 HTTP 代理请求转发
package session

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/httpservice"
	"tunnox-core/internal/packet"
)

// HTTPProxyManager HTTP 代理管理器
// 管理 HTTP 代理请求的发送和响应等待
type HTTPProxyManager struct {
	// 等待响应的请求
	pendingRequests map[string]chan *httpservice.HTTPProxyResponse
	pendingMu       sync.RWMutex

	// 默认超时
	defaultTimeout time.Duration
}

// NewHTTPProxyManager 创建 HTTP 代理管理器
func NewHTTPProxyManager() *HTTPProxyManager {
	return &HTTPProxyManager{
		pendingRequests: make(map[string]chan *httpservice.HTTPProxyResponse),
		defaultTimeout:  30 * time.Second,
	}
}

// RegisterPendingRequest 注册等待响应的请求
func (m *HTTPProxyManager) RegisterPendingRequest(requestID string) chan *httpservice.HTTPProxyResponse {
	ch := make(chan *httpservice.HTTPProxyResponse, 1)

	m.pendingMu.Lock()
	m.pendingRequests[requestID] = ch
	m.pendingMu.Unlock()

	return ch
}

// UnregisterPendingRequest 注销等待响应的请求
func (m *HTTPProxyManager) UnregisterPendingRequest(requestID string) {
	m.pendingMu.Lock()
	delete(m.pendingRequests, requestID)
	m.pendingMu.Unlock()
}

// HandleResponse 处理 HTTP 代理响应
func (m *HTTPProxyManager) HandleResponse(resp *httpservice.HTTPProxyResponse) {
	m.pendingMu.RLock()
	ch, exists := m.pendingRequests[resp.RequestID]
	m.pendingMu.RUnlock()

	if !exists {
		corelog.Warnf("HTTPProxyManager: no pending request for ID %s", resp.RequestID)
		return
	}

	select {
	case ch <- resp:
		corelog.Debugf("HTTPProxyManager: response delivered for request %s", resp.RequestID)
	default:
		corelog.Warnf("HTTPProxyManager: response channel full for request %s", resp.RequestID)
	}
}

// WaitForResponse 等待响应
func (m *HTTPProxyManager) WaitForResponse(
	ctx context.Context,
	requestID string,
	timeout time.Duration,
) (*httpservice.HTTPProxyResponse, error) {
	ch := m.RegisterPendingRequest(requestID)
	defer m.UnregisterPendingRequest(requestID)

	if timeout == 0 {
		timeout = m.defaultTimeout
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		return nil, coreerrors.New(coreerrors.CodeTimeout, "HTTP proxy request timeout")
	case <-ctx.Done():
		return nil, coreerrors.New(coreerrors.CodeTimeout, "context cancelled")
	}
}

// ============================================================================
// SessionManager HTTP 代理扩展
// ============================================================================

// httpProxyManager HTTP 代理管理器（懒加载）
var (
	globalHTTPProxyManager     *HTTPProxyManager
	globalHTTPProxyManagerOnce sync.Once
)

// getHTTPProxyManager 获取全局 HTTP 代理管理器
func getHTTPProxyManager() *HTTPProxyManager {
	globalHTTPProxyManagerOnce.Do(func() {
		globalHTTPProxyManager = NewHTTPProxyManager()
	})
	return globalHTTPProxyManager
}

// SendHTTPProxyRequest 发送 HTTP 代理请求到 Client
func (s *SessionManager) SendHTTPProxyRequest(
	clientID int64,
	request *httpservice.HTTPProxyRequest,
) (*httpservice.HTTPProxyResponse, error) {
	// 1. 获取控制连接
	conn := s.GetControlConnectionByClientID(clientID)
	if conn == nil {
		return nil, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not connected", clientID)
	}

	if conn.Stream == nil {
		return nil, coreerrors.Newf(coreerrors.CodeConnectionError, "client %d stream is nil", clientID)
	}

	// 2. 序列化请求
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidRequest, "failed to marshal request")
	}

	// 3. 构建命令包
	cmdPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.HTTPProxyRequest,
			CommandId:   request.RequestID,
			CommandBody: string(reqBody),
		},
	}

	// 4. 计算超时
	timeout := time.Duration(request.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// 5. 获取代理管理器并注册等待
	proxyMgr := getHTTPProxyManager()

	corelog.Debugf("SessionManager: sending HTTP proxy request %s to client %d, url=%s",
		request.RequestID, clientID, request.URL)

	// 6. 发送命令
	if _, err := conn.Stream.WritePacket(cmdPkt, true, 0); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send proxy request")
	}

	// 7. 等待响应
	resp, err := proxyMgr.WaitForResponse(s.Ctx(), request.RequestID, timeout)
	if err != nil {
		return nil, err
	}

	corelog.Debugf("SessionManager: received HTTP proxy response for request %s, status=%d",
		request.RequestID, resp.StatusCode)

	return resp, nil
}

// HandleHTTPProxyResponse 处理 HTTP 代理响应（由命令处理器调用）
func (s *SessionManager) HandleHTTPProxyResponse(resp *httpservice.HTTPProxyResponse) {
	proxyMgr := getHTTPProxyManager()
	proxyMgr.HandleResponse(resp)
}
