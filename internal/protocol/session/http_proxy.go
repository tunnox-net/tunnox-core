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
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/httptypes"
)

// HTTPProxyManager HTTP 代理管理器
// 管理 HTTP 代理请求的发送和响应等待
type HTTPProxyManager struct {
	// 等待响应的请求
	pendingRequests map[string]chan *httptypes.HTTPProxyResponse
	pendingMu       sync.RWMutex

	// 默认超时
	defaultTimeout time.Duration
}

// NewHTTPProxyManager 创建 HTTP 代理管理器
func NewHTTPProxyManager() *HTTPProxyManager {
	return &HTTPProxyManager{
		pendingRequests: make(map[string]chan *httptypes.HTTPProxyResponse),
		defaultTimeout:  30 * time.Second,
	}
}

// RegisterPendingRequest 注册等待响应的请求
func (m *HTTPProxyManager) RegisterPendingRequest(requestID string) chan *httptypes.HTTPProxyResponse {
	ch := make(chan *httptypes.HTTPProxyResponse, 1)

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
func (m *HTTPProxyManager) HandleResponse(resp *httptypes.HTTPProxyResponse) {
	m.pendingMu.RLock()
	ch, exists := m.pendingRequests[resp.RequestID]
	m.pendingMu.RUnlock()

	if !exists {
		corelog.Warnf("HTTPProxyManager: no pending request for ID %s", resp.RequestID)
		return
	}

	select {
	case ch <- resp:
	default:
		corelog.Warnf("HTTPProxyManager: response channel full for request %s", resp.RequestID)
	}
}

// WaitForResponse 等待响应
func (m *HTTPProxyManager) WaitForResponse(
	ctx context.Context,
	requestID string,
	timeout time.Duration,
) (*httptypes.HTTPProxyResponse, error) {
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
// 支持跨节点转发：如果客户端在其他节点，会自动转发请求
func (s *SessionManager) SendHTTPProxyRequest(
	clientID int64,
	request *httptypes.HTTPProxyRequest,
) (*httptypes.HTTPProxyResponse, error) {
	// 1. 先尝试在本地节点查找控制连接
	conn := s.GetControlConnectionByClientID(clientID)
	if conn != nil && conn.Stream != nil {
		// 客户端在本地节点，直接发送
		return s.sendHTTPProxyRequestLocal(conn, request)
	}

	// 2. 客户端不在本地，尝试跨节点转发
	if s.connStateStore == nil || s.crossNodePool == nil {
		return nil, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not connected", clientID)
	}

	// 3. 查找客户端所在节点
	targetNodeID, _, err := s.connStateStore.FindClientNode(s.Ctx(), clientID)
	if err != nil {
		return nil, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not connected: %v", clientID, err)
	}

	// 4. 如果在本地节点但连接不存在，说明连接状态不一致
	if targetNodeID == s.nodeID {
		return nil, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not connected (state inconsistent)", clientID)
	}

	corelog.Infof("SessionManager: forwarding HTTP proxy request to node %s for client %d", targetNodeID, clientID)

	// 5. 跨节点转发
	return s.sendHTTPProxyRequestCrossNode(targetNodeID, clientID, request)
}

// sendHTTPProxyRequestLocal 在本地节点发送 HTTP 代理请求
func (s *SessionManager) sendHTTPProxyRequestLocal(
	conn *ControlConnection,
	request *httptypes.HTTPProxyRequest,
) (*httptypes.HTTPProxyResponse, error) {
	// 1. 序列化请求
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidRequest, "failed to marshal request")
	}

	// 2. 构建命令包
	cmdPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.HTTPProxyRequest,
			CommandId:   request.RequestID,
			CommandBody: string(reqBody),
		},
	}

	// 3. 计算超时
	timeout := time.Duration(request.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// 4. 获取代理管理器并注册等待
	proxyMgr := getHTTPProxyManager()

	// 5. 发送命令
	if _, err := conn.Stream.WritePacket(cmdPkt, true, 0); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send proxy request")
	}

	// 6. 等待响应
	resp, err := proxyMgr.WaitForResponse(s.Ctx(), request.RequestID, timeout)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// sendHTTPProxyRequestCrossNode 跨节点发送 HTTP 代理请求
func (s *SessionManager) sendHTTPProxyRequestCrossNode(
	targetNodeID string,
	clientID int64,
	request *httptypes.HTTPProxyRequest,
) (*httptypes.HTTPProxyResponse, error) {
	// 1. 获取到目标节点的连接
	crossConn, err := s.crossNodePool.Get(s.Ctx(), targetNodeID)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to get cross-node connection")
	}
	defer s.crossNodePool.Put(crossConn)

	// 2. 序列化请求
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidRequest, "failed to marshal request")
	}

	// 3. 构建跨节点 HTTP 代理消息
	proxyMsg := &HTTPProxyMessage{
		RequestID: request.RequestID,
		ClientID:  clientID,
		Request:   reqBody,
	}

	msgBody, err := json.Marshal(proxyMsg)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidRequest, "failed to marshal proxy message")
	}

	// 4. 发送 HTTP 代理请求帧
	tcpConn := crossConn.GetTCPConn()
	if tcpConn == nil {
		crossConn.MarkBroken()
		return nil, coreerrors.New(coreerrors.CodeNetworkError, "cross-node connection is nil")
	}

	// 使用空的 tunnelID（HTTP 代理不需要 tunnelID）
	var emptyTunnelID [16]byte
	if err := WriteFrame(tcpConn, emptyTunnelID, FrameTypeHTTPProxy, msgBody); err != nil {
		crossConn.MarkBroken()
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send HTTP proxy request")
	}

	// 5. 等待响应
	timeout := time.Duration(request.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// 设置读取超时
	tcpConn.SetReadDeadline(time.Now().Add(timeout))
	defer tcpConn.SetReadDeadline(time.Time{})

	// 6. 读取响应帧
	_, frameType, respData, err := ReadFrame(tcpConn)
	if err != nil {
		crossConn.MarkBroken()
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to read HTTP proxy response")
	}

	if frameType != FrameTypeHTTPResponse {
		crossConn.MarkBroken()
		return nil, coreerrors.Newf(coreerrors.CodeInvalidPacket, "unexpected frame type: %d", frameType)
	}

	// 7. 解析响应
	var respMsg HTTPProxyResponseMessage
	if err := json.Unmarshal(respData, &respMsg); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidPacket, "failed to unmarshal response message")
	}

	if respMsg.Error != "" {
		return nil, coreerrors.New(coreerrors.CodeInternal, respMsg.Error)
	}

	var resp httptypes.HTTPProxyResponse
	if err := json.Unmarshal(respMsg.Response, &resp); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidPacket, "failed to unmarshal HTTP response")
	}

	return &resp, nil
}

// HandleHTTPProxyResponse 处理 HTTP 代理响应（由命令处理器调用）
func (s *SessionManager) HandleHTTPProxyResponse(resp *httptypes.HTTPProxyResponse) {
	proxyMgr := getHTTPProxyManager()
	proxyMgr.HandleResponse(resp)
}

// ============================================================================
// Tunnel Mode Support for HTTP Proxy
// ============================================================================

// TunnelWaitManager 隧道等待管理器
// 管理等待建立的隧道连接
type TunnelWaitManager struct {
	pendingTunnels map[string]chan TunnelConnectionInterface
	mu             sync.RWMutex
}

// NewTunnelWaitManager 创建隧道等待管理器
func NewTunnelWaitManager() *TunnelWaitManager {
	return &TunnelWaitManager{
		pendingTunnels: make(map[string]chan TunnelConnectionInterface),
	}
}

// RegisterPendingTunnel 注册等待建立的隧道
func (m *TunnelWaitManager) RegisterPendingTunnel(tunnelID string) chan TunnelConnectionInterface {
	ch := make(chan TunnelConnectionInterface, 1)

	m.mu.Lock()
	m.pendingTunnels[tunnelID] = ch
	m.mu.Unlock()

	return ch
}

// UnregisterPendingTunnel 注销等待建立的隧道
func (m *TunnelWaitManager) UnregisterPendingTunnel(tunnelID string) {
	m.mu.Lock()
	delete(m.pendingTunnels, tunnelID)
	m.mu.Unlock()
}

// NotifyTunnelEstablished 通知隧道已建立
func (m *TunnelWaitManager) NotifyTunnelEstablished(tunnelID string, conn TunnelConnectionInterface) {
	m.mu.RLock()
	ch, exists := m.pendingTunnels[tunnelID]
	m.mu.RUnlock()

	if !exists {
		corelog.Warnf("TunnelWaitManager: no pending tunnel for ID %s", tunnelID)
		return
	}

	select {
	case ch <- conn:
	default:
		corelog.Warnf("TunnelWaitManager: tunnel channel full for %s", tunnelID)
	}
}

// 全局隧道等待管理器（懒加载）
var (
	globalTunnelWaitManager     *TunnelWaitManager
	globalTunnelWaitManagerOnce sync.Once
)

// getTunnelWaitManager 获取全局隧道等待管理器
func getTunnelWaitManager() *TunnelWaitManager {
	globalTunnelWaitManagerOnce.Do(func() {
		globalTunnelWaitManager = NewTunnelWaitManager()
	})
	return globalTunnelWaitManager
}

// RequestTunnelForHTTP 请求为 HTTP 代理创建隧道连接
// 用于处理大请求（文件上传、流式传输等）
func (s *SessionManager) RequestTunnelForHTTP(
	clientID int64,
	mappingID string,
	targetURL string,
	method string,
) (TunnelConnectionInterface, error) {
	// 1. 获取控制连接
	conn := s.GetControlConnectionByClientID(clientID)
	if conn == nil {
		return nil, coreerrors.Newf(coreerrors.CodeClientNotFound, "client %d not connected", clientID)
	}

	if conn.Stream == nil {
		return nil, coreerrors.Newf(coreerrors.CodeConnectionError, "client %d stream is nil", clientID)
	}

	// 2. 生成隧道ID
	tunnelID, err := s.idManager.GenerateTunnelID()
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to generate tunnel ID")
	}

	// 3. 构建隧道打开请求
	tunnelReq := &httptypes.HTTPTunnelRequest{
		TunnelID:  tunnelID,
		MappingID: mappingID,
		TargetURL: targetURL,
		Method:    method,
	}

	reqBody, err := json.Marshal(tunnelReq)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeInvalidRequest, "failed to marshal tunnel request")
	}

	// 4. 构建命令包
	cmdPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.TunnelOpenRequestCmd,
			CommandId:   tunnelID,
			CommandBody: string(reqBody),
		},
	}

	// 5. 注册等待隧道建立
	tunnelMgr := getTunnelWaitManager()
	waitCh := tunnelMgr.RegisterPendingTunnel(tunnelID)
	defer tunnelMgr.UnregisterPendingTunnel(tunnelID)

	corelog.Infof("SessionManager: requesting HTTP tunnel %s for client %d, url=%s",
		tunnelID, clientID, targetURL)

	// 6. 发送命令
	if _, err := conn.Stream.WritePacket(cmdPkt, true, 0); err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send tunnel request")
	}

	// 7. 等待隧道建立（30秒超时）
	timeout := 30 * time.Second
	select {
	case tunnelConn := <-waitCh:
		corelog.Infof("SessionManager: HTTP tunnel %s established for client %d", tunnelID, clientID)
		return tunnelConn, nil
	case <-time.After(timeout):
		return nil, coreerrors.New(coreerrors.CodeTimeout, "tunnel establishment timeout")
	case <-s.Ctx().Done():
		return nil, coreerrors.New(coreerrors.CodeTimeout, "context cancelled")
	}
}

// NotifyHTTPTunnelEstablished 通知 HTTP 隧道已建立
// 由 packet_handler_tunnel.go 在处理 TunnelOpen 包时调用
func (s *SessionManager) NotifyHTTPTunnelEstablished(tunnelID string, conn TunnelConnectionInterface) {
	tunnelMgr := getTunnelWaitManager()
	tunnelMgr.NotifyTunnelEstablished(tunnelID, conn)
}
