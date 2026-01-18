// Package session DNS 解析请求处理
// 实现 Client -> Server -> TargetClient -> Server -> Client 的 DNS 解析转发
package session

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// ============================================================================
// DNS 解析管理器
// ============================================================================

// DNSResolveManager DNS 解析请求管理器
type DNSResolveManager struct {
	pendingRequests map[string]chan *packet.DNSResolveResponse
	mu              sync.RWMutex
}

// NewDNSResolveManager 创建 DNS 解析管理器
func NewDNSResolveManager() *DNSResolveManager {
	return &DNSResolveManager{
		pendingRequests: make(map[string]chan *packet.DNSResolveResponse),
	}
}

// RegisterRequest 注册等待响应的请求
func (m *DNSResolveManager) RegisterRequest(requestID string) chan *packet.DNSResolveResponse {
	ch := make(chan *packet.DNSResolveResponse, 1)

	m.mu.Lock()
	m.pendingRequests[requestID] = ch
	m.mu.Unlock()

	return ch
}

// UnregisterRequest 注销请求
func (m *DNSResolveManager) UnregisterRequest(requestID string) {
	m.mu.Lock()
	delete(m.pendingRequests, requestID)
	m.mu.Unlock()
}

// HandleResponse 处理 DNS 解析响应
func (m *DNSResolveManager) HandleResponse(requestID string, resp *packet.DNSResolveResponse) {
	m.mu.RLock()
	ch, exists := m.pendingRequests[requestID]
	m.mu.RUnlock()

	if !exists {
		corelog.Warnf("DNSResolveManager: no pending request for ID %s", requestID)
		return
	}

	select {
	case ch <- resp:
	default:
		corelog.Warnf("DNSResolveManager: response channel full for %s", requestID)
	}
}

// WaitForResponse 等待 DNS 解析响应
func (m *DNSResolveManager) WaitForResponse(ctx context.Context, requestID string, ch chan *packet.DNSResolveResponse, timeout time.Duration) (*packet.DNSResolveResponse, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case resp := <-ch:
		return resp, nil
	case <-timeoutCtx.Done():
		return nil, coreerrors.New(coreerrors.CodeTimeout, "DNS resolve timeout")
	}
}

// 全局 DNS 解析管理器
var (
	globalDNSResolveManager     *DNSResolveManager
	globalDNSResolveManagerOnce sync.Once
)

// getDNSResolveManager 获取全局 DNS 解析管理器
func getDNSResolveManager() *DNSResolveManager {
	globalDNSResolveManagerOnce.Do(func() {
		globalDNSResolveManager = NewDNSResolveManager()
	})
	return globalDNSResolveManager
}

// ============================================================================
// DNS 解析请求处理
// ============================================================================

// HandleDNSResolveRequest 处理 DNS 解析请求
// 由 listenClient（如移动端）发起，Server 转发到 targetClient
func (s *SessionManager) HandleDNSResolveRequest(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return coreerrors.New(coreerrors.CodeInvalidPacket, "command packet is nil")
	}

	cmd := connPacket.Packet.CommandPacket

	// 1. 解析 DNS 解析请求
	var req packet.DNSResolveRequest
	if err := json.Unmarshal([]byte(cmd.CommandBody), &req); err != nil {
		corelog.Errorf("DNSHandler: failed to parse request: %v", err)
		return s.sendDNSResolveError(connPacket, cmd.CommandId, "invalid request format")
	}

	corelog.Debugf("DNSHandler: received request - Domain=%s, QType=%d, TargetClientID=%d, CommandID=%s",
		req.Domain, req.QType, req.TargetClientID, cmd.CommandId)

	// 2. 获取目标客户端ID
	// 如果 TargetClientID 是 -1，需要从映射配置中获取默认目标客户端
	targetClientID := req.TargetClientID
	if targetClientID <= 0 {
		// 获取请求来源客户端的默认目标客户端
		sourceClientID := s.getClientIDFromConnection(connPacket.ConnectionID)
		if sourceClientID == 0 {
			corelog.Errorf("DNSHandler: cannot get source client ID from connection %s", connPacket.ConnectionID)
			return s.sendDNSResolveError(connPacket, cmd.CommandId, "unknown source client")
		}

		// 尝试获取默认目标客户端ID（从该客户端的 SOCKS5 映射中获取）
		targetClientID = s.getDefaultTargetClientID(sourceClientID)
		if targetClientID == 0 {
			corelog.Errorf("DNSHandler: no default target client for source client %d", sourceClientID)
			return s.sendDNSResolveError(connPacket, cmd.CommandId, "no target client configured")
		}
	}

	// 3. 获取目标客户端的控制连接
	targetConn := s.GetControlConnectionByClientID(targetClientID)
	if targetConn == nil || targetConn.Stream == nil {
		corelog.Errorf("DNSHandler: target client %d not connected", targetClientID)
		return s.sendDNSResolveError(connPacket, cmd.CommandId, "target client not connected")
	}

	// 4. 注册等待响应
	dnsMgr := getDNSResolveManager()
	waitCh := dnsMgr.RegisterRequest(cmd.CommandId)
	defer dnsMgr.UnregisterRequest(cmd.CommandId)

	// 5. 转发请求到目标客户端
	forwardPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.DNSResolve,
			CommandId:   cmd.CommandId,
			CommandBody: cmd.CommandBody,
		},
	}

	if _, err := targetConn.Stream.WritePacket(forwardPkt, true, 0); err != nil {
		corelog.Errorf("DNSHandler: failed to forward request to target client %d: %v", targetClientID, err)
		return s.sendDNSResolveError(connPacket, cmd.CommandId, "failed to forward request")
	}

	corelog.Debugf("DNSHandler: forwarded request to target client %d, waiting for response...", targetClientID)

	// 6. 等待响应（5 秒超时）
	resp, err := dnsMgr.WaitForResponse(s.Ctx(), cmd.CommandId, waitCh, 5*time.Second)
	if err != nil {
		corelog.Errorf("DNSHandler: timeout waiting for response from target client %d: %v", targetClientID, err)
		return s.sendDNSResolveError(connPacket, cmd.CommandId, "DNS resolve timeout")
	}

	corelog.Debugf("DNSHandler: received response from target client %d: success=%v, IPs=%v",
		targetClientID, resp.Success, resp.IPs)

	// 7. 将响应发送回请求客户端
	return s.sendDNSResolveResponse(connPacket, cmd.CommandId, resp)
}

// HandleDNSResolveResponse 处理来自 targetClient 的 DNS 解析响应
func (s *SessionManager) HandleDNSResolveResponse(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return coreerrors.New(coreerrors.CodeInvalidPacket, "command packet is nil")
	}

	cmd := connPacket.Packet.CommandPacket

	// 解析响应
	var resp packet.DNSResolveResponse
	if err := json.Unmarshal([]byte(cmd.CommandBody), &resp); err != nil {
		corelog.Errorf("DNSHandler: failed to parse response: %v", err)
		return err
	}

	corelog.Debugf("DNSHandler: received response for CommandID=%s: success=%v, IPs=%v",
		cmd.CommandId, resp.Success, resp.IPs)

	// 转发到 DNS 管理器
	dnsMgr := getDNSResolveManager()
	dnsMgr.HandleResponse(cmd.CommandId, &resp)

	return nil
}

// sendDNSResolveError 发送 DNS 解析错误响应
func (s *SessionManager) sendDNSResolveError(connPacket *types.StreamPacket, commandID, errMsg string) error {
	resp := &packet.DNSResolveResponse{
		Success: false,
		Error:   errMsg,
	}
	return s.sendDNSResolveResponse(connPacket, commandID, resp)
}

// sendDNSResolveResponse 发送 DNS 解析响应
func (s *SessionManager) sendDNSResolveResponse(connPacket *types.StreamPacket, commandID string, resp *packet.DNSResolveResponse) error {
	// 获取源客户端的控制连接
	sourceConn := s.clientRegistry.GetByConnID(connPacket.ConnectionID)
	if sourceConn == nil || sourceConn.Stream == nil {
		corelog.Errorf("DNSHandler: source connection not found: %s", connPacket.ConnectionID)
		return coreerrors.New(coreerrors.CodeConnectionError, "source connection not found")
	}

	// 序列化响应
	respBody, err := json.Marshal(resp)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal response")
	}

	// 构建响应包
	respPkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.DNSResolve,
			CommandId:   commandID,
			CommandBody: string(respBody),
		},
	}

	if _, err := sourceConn.Stream.WritePacket(respPkt, true, 0); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send response")
	}

	return nil
}

// getDefaultTargetClientID 获取默认目标客户端ID
// 从该客户端的 SOCKS5 映射中获取第一个活跃的目标客户端
func (s *SessionManager) getDefaultTargetClientID(sourceClientID int64) int64 {
	if s.cloudControl == nil {
		return 0
	}

	// 获取该客户端的所有映射
	mappings, err := s.cloudControl.GetClientPortMappings(sourceClientID)
	if err != nil {
		corelog.Warnf("DNSHandler: failed to get mappings for client %d: %v", sourceClientID, err)
		return 0
	}

	// 返回第一个活跃 SOCKS5 映射的目标客户端ID
	for _, mapping := range mappings {
		if mapping.Protocol == "socks" && mapping.Status == "active" && mapping.TargetClientID > 0 {
			return mapping.TargetClientID
		}
	}

	return 0
}

// ============================================================================
// DNS 原始报文查询处理（DNSQuery）
// 通过控制通道转发原始 DNS 报文，避免 UDP 隧道的不稳定性
// ============================================================================

// DNSQueryManager DNS 原始报文查询管理器
type DNSQueryManager struct {
	pendingRequests map[string]chan *packet.DNSQueryResponse
	mu              sync.RWMutex
}

// NewDNSQueryManager 创建 DNS 查询管理器
func NewDNSQueryManager() *DNSQueryManager {
	return &DNSQueryManager{
		pendingRequests: make(map[string]chan *packet.DNSQueryResponse),
	}
}

// RegisterRequest 注册等待响应的请求
func (m *DNSQueryManager) RegisterRequest(queryID string) chan *packet.DNSQueryResponse {
	ch := make(chan *packet.DNSQueryResponse, 1)

	m.mu.Lock()
	m.pendingRequests[queryID] = ch
	m.mu.Unlock()

	return ch
}

// UnregisterRequest 注销请求
func (m *DNSQueryManager) UnregisterRequest(queryID string) {
	m.mu.Lock()
	delete(m.pendingRequests, queryID)
	m.mu.Unlock()
}

// HandleResponse 处理 DNS 查询响应
func (m *DNSQueryManager) HandleResponse(queryID string, resp *packet.DNSQueryResponse) {
	m.mu.RLock()
	ch, exists := m.pendingRequests[queryID]
	m.mu.RUnlock()

	if !exists {
		corelog.Warnf("DNSQueryManager: no pending request for QueryID %s", queryID)
		return
	}

	select {
	case ch <- resp:
	default:
		corelog.Warnf("DNSQueryManager: response channel full for %s", queryID)
	}
}

// WaitForResponse 等待 DNS 查询响应
func (m *DNSQueryManager) WaitForResponse(ctx context.Context, queryID string, ch chan *packet.DNSQueryResponse, timeout time.Duration) (*packet.DNSQueryResponse, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case resp := <-ch:
		return resp, nil
	case <-timeoutCtx.Done():
		return nil, coreerrors.New(coreerrors.CodeTimeout, "DNS query timeout")
	}
}

// 全局 DNS 查询管理器
var (
	globalDNSQueryManager     *DNSQueryManager
	globalDNSQueryManagerOnce sync.Once
)

// getDNSQueryManager 获取全局 DNS 查询管理器
func getDNSQueryManager() *DNSQueryManager {
	globalDNSQueryManagerOnce.Do(func() {
		globalDNSQueryManager = NewDNSQueryManager()
	})
	return globalDNSQueryManager
}

// HandleDNSQueryRequest 处理 DNS 原始报文查询请求
// 由 listenClient 发起，Server 转发到 targetClient 执行实际的 DNS 查询
func (s *SessionManager) HandleDNSQueryRequest(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return coreerrors.New(coreerrors.CodeInvalidPacket, "command packet is nil")
	}

	cmd := connPacket.Packet.CommandPacket

	// 1. 解析 DNS 查询请求
	var req packet.DNSQueryRequest
	if err := json.Unmarshal([]byte(cmd.CommandBody), &req); err != nil {
		corelog.Errorf("DNSQueryHandler: failed to parse request: %v", err)
		return s.sendDNSQueryError(connPacket, cmd.CommandId, req.QueryID, "invalid request format")
	}

	corelog.Debugf("DNSQueryHandler: received request - QueryID=%s, TargetClientID=%d, DNSServer=%s, QueryLen=%d",
		req.QueryID, req.TargetClientID, req.DNSServer, len(req.RawQuery))

	// 2. 获取目标客户端ID
	targetClientID := req.TargetClientID
	if targetClientID <= 0 {
		// 获取请求来源客户端的默认目标客户端
		sourceClientID := s.getClientIDFromConnection(connPacket.ConnectionID)
		if sourceClientID == 0 {
			corelog.Errorf("DNSQueryHandler: cannot get source client ID from connection %s", connPacket.ConnectionID)
			return s.sendDNSQueryError(connPacket, cmd.CommandId, req.QueryID, "unknown source client")
		}

		// 尝试获取默认目标客户端ID
		targetClientID = s.getDefaultTargetClientID(sourceClientID)
		if targetClientID == 0 {
			corelog.Errorf("DNSQueryHandler: no default target client for source client %d", sourceClientID)
			return s.sendDNSQueryError(connPacket, cmd.CommandId, req.QueryID, "no target client configured")
		}
	}

	// 3. 获取目标客户端的控制连接
	targetConn := s.GetControlConnectionByClientID(targetClientID)
	if targetConn == nil || targetConn.Stream == nil {
		corelog.Errorf("DNSQueryHandler: target client %d not connected", targetClientID)
		return s.sendDNSQueryError(connPacket, cmd.CommandId, req.QueryID, "target client not connected")
	}

	// 4. 注册等待响应
	queryMgr := getDNSQueryManager()
	waitCh := queryMgr.RegisterRequest(cmd.CommandId)
	defer queryMgr.UnregisterRequest(cmd.CommandId)

	// 5. 转发请求到目标客户端
	forwardPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.DNSQuery,
			CommandId:   cmd.CommandId,
			CommandBody: cmd.CommandBody,
		},
	}

	if _, err := targetConn.Stream.WritePacket(forwardPkt, true, 0); err != nil {
		corelog.Errorf("DNSQueryHandler: failed to forward request to target client %d: %v", targetClientID, err)
		return s.sendDNSQueryError(connPacket, cmd.CommandId, req.QueryID, "failed to forward request")
	}

	corelog.Debugf("DNSQueryHandler: forwarded request to target client %d, waiting for response...", targetClientID)

	// 6. 等待响应（5 秒超时）
	resp, err := queryMgr.WaitForResponse(s.Ctx(), cmd.CommandId, waitCh, 5*time.Second)
	if err != nil {
		corelog.Errorf("DNSQueryHandler: timeout waiting for response from target client %d: %v", targetClientID, err)
		return s.sendDNSQueryError(connPacket, cmd.CommandId, req.QueryID, "DNS query timeout")
	}

	corelog.Debugf("DNSQueryHandler: received response from target client %d: success=%v, responseLen=%d",
		targetClientID, resp.Success, len(resp.RawAnswer))

	// 7. 将响应发送回请求客户端
	return s.sendDNSQueryResponse(connPacket, cmd.CommandId, resp)
}

// HandleDNSQueryResponse 处理来自 targetClient 的 DNS 查询响应
func (s *SessionManager) HandleDNSQueryResponse(connPacket *types.StreamPacket) error {
	if connPacket.Packet.CommandPacket == nil {
		return coreerrors.New(coreerrors.CodeInvalidPacket, "command packet is nil")
	}

	cmd := connPacket.Packet.CommandPacket

	// 解析响应
	var resp packet.DNSQueryResponse
	if err := json.Unmarshal([]byte(cmd.CommandBody), &resp); err != nil {
		corelog.Errorf("DNSQueryHandler: failed to parse response: %v", err)
		return err
	}

	corelog.Debugf("DNSQueryHandler: received response for CommandID=%s: success=%v, queryID=%s",
		cmd.CommandId, resp.Success, resp.QueryID)

	// 转发到 DNS 查询管理器
	queryMgr := getDNSQueryManager()
	queryMgr.HandleResponse(cmd.CommandId, &resp)

	return nil
}

// sendDNSQueryError 发送 DNS 查询错误响应
func (s *SessionManager) sendDNSQueryError(connPacket *types.StreamPacket, commandID, queryID, errMsg string) error {
	resp := &packet.DNSQueryResponse{
		QueryID: queryID,
		Success: false,
		Error:   errMsg,
	}
	return s.sendDNSQueryResponse(connPacket, commandID, resp)
}

// sendDNSQueryResponse 发送 DNS 查询响应
func (s *SessionManager) sendDNSQueryResponse(connPacket *types.StreamPacket, commandID string, resp *packet.DNSQueryResponse) error {
	// 获取源客户端的控制连接
	sourceConn := s.clientRegistry.GetByConnID(connPacket.ConnectionID)
	if sourceConn == nil || sourceConn.Stream == nil {
		corelog.Errorf("DNSQueryHandler: source connection not found: %s", connPacket.ConnectionID)
		return coreerrors.New(coreerrors.CodeConnectionError, "source connection not found")
	}

	// 序列化响应
	respBody, err := json.Marshal(resp)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeInternal, "failed to marshal response")
	}

	// 构建响应包
	respPkt := &packet.TransferPacket{
		PacketType: packet.CommandResp,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.DNSQuery,
			CommandId:   commandID,
			CommandBody: string(respBody),
		},
	}

	if _, err := sourceConn.Stream.WritePacket(respPkt, true, 0); err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send response")
	}

	return nil
}
