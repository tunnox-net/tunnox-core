package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/httptypes"
)

// ============================================================================
// CrossNodeListener 跨节点连接监听器
// ============================================================================

// CrossNodeListener 跨节点连接监听器
// 在源节点上监听来自目标节点的连接
type CrossNodeListener struct {
	listener   net.Listener
	sessionMgr *SessionManager
	port       int
	running    bool
	mu         sync.Mutex
}

// NewCrossNodeListener 创建跨节点连接监听器
func NewCrossNodeListener(sessionMgr *SessionManager, port int) *CrossNodeListener {
	return &CrossNodeListener{
		sessionMgr: sessionMgr,
		port:       port,
	}
}

// Start 启动监听器
func (l *CrossNodeListener) Start(ctx context.Context) error {
	l.mu.Lock()
	if l.running {
		l.mu.Unlock()
		return nil
	}

	addr := fmt.Sprintf(":%d", l.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		l.mu.Unlock()
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to start cross-node listener")
	}

	l.listener = listener
	l.running = true
	l.mu.Unlock()

	go l.acceptLoop(ctx)
	return nil
}

// Stop 停止监听器
func (l *CrossNodeListener) Stop() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.running {
		return nil
	}

	l.running = false
	if l.listener != nil {
		return l.listener.Close()
	}
	return nil
}

// acceptLoop 接受连接循环
func (l *CrossNodeListener) acceptLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := l.listener.Accept()
		if err != nil {
			if !l.running {
				return
			}
			continue
		}

		go l.handleConnection(ctx, conn)
	}
}

// handleConnection 处理跨节点连接
func (l *CrossNodeListener) handleConnection(ctx context.Context, conn net.Conn) {
	// 注意：不在此处 defer conn.Close()
	// 对于 TargetReady 类型的连接，连接的生命周期由 runBridgeForward 管理
	// 对于其他类型（HTTP/DNS/Command），在处理完成后关闭
	shouldCloseConn := true
	defer func() {
		if shouldCloseConn {
			conn.Close()
		}
	}()

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		corelog.Warnf("CrossNodeListener: connection is not TCP, type=%T", conn)
		return
	}

	// 读取第一个帧，确定帧类型
	tunnelID, frameType, data, err := ReadFrame(tcpConn)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to read frame: %v", err)
		return
	}

	tunnelIDStr := TunnelIDToString(tunnelID)

	switch frameType {
	case FrameTypeTargetReady:
		// TargetReady 连接的生命周期由 runBridgeForward 管理，不在此处关闭
		shouldCloseConn = false
		l.handleTargetReady(ctx, tcpConn, tunnelIDStr, data)
	case FrameTypeHTTPProxy:
		l.handleHTTPProxy(ctx, tcpConn, data)
	case FrameTypeDNSQuery:
		l.handleDNSQuery(ctx, tcpConn, data)
	case FrameTypeCommand:
		l.handleCommand(ctx, tcpConn, data)
	default:
		corelog.Warnf("CrossNodeListener: unknown frame type %d", frameType)
	}
}

// handleTargetReady 处理 TargetTunnelReady 消息
func (l *CrossNodeListener) handleTargetReady(ctx context.Context, conn *net.TCPConn, tunnelIDStr string, data []byte) {
	fullTunnelID, targetNodeID, err := DecodeTargetReadyMessage(data)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to decode target ready message: %v", err)
		return
	}

	if fullTunnelID != "" {
		tunnelIDStr = fullTunnelID
	}

	corelog.Infof("CrossNodeListener: target ready, tunnelID=%s, targetNode=%s", tunnelIDStr, targetNodeID)

	// 查找对应的 Bridge
	l.sessionMgr.bridgeLock.RLock()
	bridge, exists := l.sessionMgr.tunnelBridges[tunnelIDStr]
	l.sessionMgr.bridgeLock.RUnlock()

	if !exists {
		corelog.Errorf("CrossNodeListener: bridge not found for tunnelID=%s", tunnelIDStr)
		return
	}

	// 创建 CrossNodeConn 并设置到 Bridge
	crossConn := NewCrossNodeConn(ctx, targetNodeID, conn, nil)
	bridge.SetCrossNodeConnection(crossConn)
	bridge.NotifyTargetReady()

	// 启动数据转发（零拷贝）
	l.runBridgeForward(tunnelIDStr, bridge, crossConn)
}

// runBridgeForward 运行 Bridge 数据转发
// runBridgeForward 运行 Bridge 数据转发
// 重要：使用简单的 io.Copy 直接传输数据，不使用 FrameStream
// 连接用完即关闭（避免复杂的生命周期管理问题）
func (l *CrossNodeListener) runBridgeForward(tunnelID string, bridge *TunnelBridge, crossConn *CrossNodeConn) {
	defer bridge.ReleaseCrossNodeConnection()

	// 关键：数据转发完成后关闭 Bridge，触发生命周期结束
	// 这样 bridge.Start() 会从 <-b.Ctx().Done() 返回
	defer func() {
		corelog.Infof("CrossNodeListener[%s]: closing bridge after data forward completion", tunnelID)
		bridge.Close()
		l.sessionMgr.MarkTunnelClosed(tunnelID)
	}()

	// 获取源端数据转发器
	sourceForwarder := bridge.GetSourceForwarder()
	if sourceForwarder == nil {
		corelog.Errorf("CrossNodeListener[%s]: sourceForwarder is nil", tunnelID)
		return
	}

	// 关键：确保数据转发完成后关闭源端连接
	// 这样 Listen 端客户端的 BidirectionalCopy 才能正确收到 EOF 并结束
	defer sourceForwarder.Close()

	// 获取跨节点 TCP 连接
	tcpConn := crossConn.GetTCPConn()
	if tcpConn == nil {
		corelog.Errorf("CrossNodeListener[%s]: tcpConn is nil", tunnelID)
		return
	}

	corelog.Infof("CrossNodeListener[%s]: starting data forward, sourceForwarder type=%T", tunnelID, sourceForwarder)

	// 双向数据转发（直接使用 io.Copy，不用 FrameStream）
	done := make(chan struct{}, 2)

	// 源端 -> 跨节点
	go func() {
		defer func() { done <- struct{}{} }()
		n, err := io.Copy(tcpConn, sourceForwarder)
		if err != nil && err != io.EOF {
			corelog.Debugf("CrossNodeListener[%s]: source->crossNode error: %v", tunnelID, err)
		}
		corelog.Infof("CrossNodeListener[%s]: source->crossNode finished, bytes=%d", tunnelID, n)
		// 关键：使用半关闭通知对端 EOF
		tcpConn.CloseWrite()
	}()

	// 跨节点 -> 源端
	go func() {
		defer func() { done <- struct{}{} }()
		n, err := io.Copy(sourceForwarder, tcpConn)
		if err != nil && err != io.EOF {
			corelog.Debugf("CrossNodeListener[%s]: crossNode->source error: %v", tunnelID, err)
		}
		corelog.Infof("CrossNodeListener[%s]: crossNode->source finished, bytes=%d", tunnelID, n)
		// 关键：对源端连接使用半关闭（如果支持）
		if closer, ok := sourceForwarder.(interface{ CloseWrite() error }); ok {
			closer.CloseWrite()
		} else if tcpSource, ok := sourceForwarder.(*net.TCPConn); ok {
			tcpSource.CloseWrite()
		}
	}()

	// 等待两个方向都完成
	<-done
	<-done
	corelog.Infof("CrossNodeListener[%s]: data forward completed", tunnelID)
}

// handleHTTPProxy 处理跨节点 HTTP 代理请求
func (l *CrossNodeListener) handleHTTPProxy(_ context.Context, conn *net.TCPConn, data []byte) {
	corelog.Infof("CrossNodeListener: handling HTTP proxy request, dataLen=%d", len(data))

	var proxyMsg HTTPProxyMessage
	if err := json.Unmarshal(data, &proxyMsg); err != nil {
		corelog.Errorf("CrossNodeListener: failed to unmarshal HTTP proxy message: %v", err)
		l.sendHTTPProxyError(conn, "", "failed to unmarshal request")
		return
	}

	corelog.Infof("CrossNodeListener: HTTP proxy request for client %d, requestID=%s",
		proxyMsg.ClientID, proxyMsg.RequestID)

	var request httptypes.HTTPProxyRequest
	if err := json.Unmarshal(proxyMsg.Request, &request); err != nil {
		corelog.Errorf("CrossNodeListener: failed to unmarshal HTTPProxyRequest: %v", err)
		l.sendHTTPProxyError(conn, proxyMsg.RequestID, "failed to unmarshal HTTP request")
		return
	}

	controlConn := l.sessionMgr.GetControlConnectionByClientID(proxyMsg.ClientID)
	if controlConn == nil {
		corelog.Errorf("CrossNodeListener: client %d not found on this node", proxyMsg.ClientID)
		l.sendHTTPProxyError(conn, proxyMsg.RequestID, fmt.Sprintf("client %d not connected", proxyMsg.ClientID))
		return
	}

	if controlConn.Stream == nil {
		corelog.Errorf("CrossNodeListener: client %d stream is nil", proxyMsg.ClientID)
		l.sendHTTPProxyError(conn, proxyMsg.RequestID, fmt.Sprintf("client %d stream is nil", proxyMsg.ClientID))
		return
	}

	resp, err := l.sessionMgr.sendHTTPProxyRequestLocal(controlConn, &request)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to send HTTP proxy request to client: %v", err)
		l.sendHTTPProxyError(conn, proxyMsg.RequestID, err.Error())
		return
	}

	l.sendHTTPProxyResponse(conn, proxyMsg.RequestID, resp)
}

// sendHTTPProxyError 发送 HTTP 代理错误响应
func (l *CrossNodeListener) sendHTTPProxyError(conn *net.TCPConn, requestID string, errMsg string) {
	respMsg := &HTTPProxyResponseMessage{
		RequestID: requestID,
		Error:     errMsg,
	}

	respData, err := json.Marshal(respMsg)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to marshal error response: %v", err)
		return
	}

	var emptyTunnelID [16]byte
	if err := WriteFrame(conn, emptyTunnelID, FrameTypeHTTPResponse, respData); err != nil {
		corelog.Errorf("CrossNodeListener: failed to send error response: %v", err)
	}
}

// handleDNSQuery 处理跨节点 DNS 查询请求
func (l *CrossNodeListener) handleDNSQuery(ctx context.Context, conn *net.TCPConn, data []byte) {
	var dnsMsg DNSQueryMessage
	if err := json.Unmarshal(data, &dnsMsg); err != nil {
		corelog.Errorf("CrossNodeListener: failed to unmarshal DNS query message: %v", err)
		l.sendDNSError(conn, "", "invalid request format")
		return
	}

	corelog.Infof("CrossNodeListener: handling DNS query for client %d, commandID=%s", dnsMsg.TargetClientID, dnsMsg.CommandID)

	// 查找目标客户端
	targetConn := l.sessionMgr.GetControlConnectionByClientID(dnsMsg.TargetClientID)
	if targetConn == nil || targetConn.Stream == nil {
		corelog.Errorf("CrossNodeListener: target client %d not found", dnsMsg.TargetClientID)
		l.sendDNSError(conn, dnsMsg.CommandID, "target client not connected")
		return
	}

	// 解析原始 DNS 请求
	var req packet.DNSQueryRequest
	if err := json.Unmarshal(dnsMsg.Request, &req); err != nil {
		corelog.Errorf("CrossNodeListener: failed to parse DNS request: %v", err)
		l.sendDNSError(conn, dnsMsg.CommandID, "invalid DNS request")
		return
	}

	// 注册等待响应
	queryMgr := getDNSQueryManager()
	waitCh := queryMgr.RegisterRequest(dnsMsg.CommandID)
	defer queryMgr.UnregisterRequest(dnsMsg.CommandID)

	// 转发到目标客户端
	forwardPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.DNSQuery,
			CommandId:   dnsMsg.CommandID,
			CommandBody: string(dnsMsg.Request),
		},
	}

	if _, err := targetConn.Stream.WritePacket(forwardPkt, true, 0); err != nil {
		corelog.Errorf("CrossNodeListener: failed to forward DNS request to client: %v", err)
		l.sendDNSError(conn, dnsMsg.CommandID, "failed to forward request")
		return
	}

	// 等待响应（5 秒超时）
	resp, err := queryMgr.WaitForResponse(ctx, dnsMsg.CommandID, waitCh, 5*time.Second)
	if err != nil {
		corelog.Errorf("CrossNodeListener: DNS query timeout: %v", err)
		l.sendDNSError(conn, dnsMsg.CommandID, "DNS query timeout")
		return
	}

	l.sendDNSResponse(conn, dnsMsg.CommandID, resp)
}

// sendDNSError 发送 DNS 错误响应
func (l *CrossNodeListener) sendDNSError(conn *net.TCPConn, commandID string, errMsg string) {
	respMsg := &DNSQueryResponseMessage{
		CommandID: commandID,
		Error:     errMsg,
	}

	respData, err := json.Marshal(respMsg)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to marshal DNS error response: %v", err)
		return
	}

	var emptyTunnelID [16]byte
	if err := WriteFrame(conn, emptyTunnelID, FrameTypeDNSResponse, respData); err != nil {
		corelog.Errorf("CrossNodeListener: failed to send DNS error response: %v", err)
	}
}

// sendDNSResponse 发送 DNS 响应
func (l *CrossNodeListener) sendDNSResponse(conn *net.TCPConn, commandID string, resp *packet.DNSQueryResponse) {
	respBody, err := json.Marshal(resp)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to marshal DNS response: %v", err)
		l.sendDNSError(conn, commandID, "failed to marshal response")
		return
	}

	respMsg := &DNSQueryResponseMessage{
		CommandID: commandID,
		Response:  respBody,
	}

	respData, err := json.Marshal(respMsg)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to marshal DNS response message: %v", err)
		l.sendDNSError(conn, commandID, "failed to marshal response message")
		return
	}

	var emptyTunnelID [16]byte
	if err := WriteFrame(conn, emptyTunnelID, FrameTypeDNSResponse, respData); err != nil {
		corelog.Errorf("CrossNodeListener: failed to send DNS response: %v", err)
	} else {
		corelog.Infof("CrossNodeListener: sent DNS response for commandID=%s", commandID)
	}
}

// sendHTTPProxyResponse 发送 HTTP 代理响应
func (l *CrossNodeListener) sendHTTPProxyResponse(conn *net.TCPConn, requestID string, resp *httptypes.HTTPProxyResponse) {
	respBody, err := json.Marshal(resp)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to marshal HTTP response: %v", err)
		l.sendHTTPProxyError(conn, requestID, "failed to marshal response")
		return
	}

	respMsg := &HTTPProxyResponseMessage{
		RequestID: requestID,
		Response:  respBody,
	}

	respData, err := json.Marshal(respMsg)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to marshal response message: %v", err)
		l.sendHTTPProxyError(conn, requestID, "failed to marshal response message")
		return
	}

	var emptyTunnelID [16]byte
	if err := WriteFrame(conn, emptyTunnelID, FrameTypeHTTPResponse, respData); err != nil {
		corelog.Errorf("CrossNodeListener: failed to send HTTP response: %v", err)
	} else {
		corelog.Infof("CrossNodeListener: sent HTTP proxy response for requestID=%s", requestID)
	}
}

// ============================================================================
// 通用命令处理（统一跨节点命令转发）
// ============================================================================

// handleCommand 处理通用跨节点命令
// 这是统一的跨节点命令处理入口，支持任何类型的命令
func (l *CrossNodeListener) handleCommand(ctx context.Context, conn *net.TCPConn, data []byte) {
	var cmdMsg CommandMessage
	if err := json.Unmarshal(data, &cmdMsg); err != nil {
		corelog.Errorf("CrossNodeListener: failed to parse command message: %v", err)
		return
	}

	corelog.Infof("CrossNodeListener: handling command type=%d for client %d, commandID=%s",
		cmdMsg.CommandType, cmdMsg.TargetClientID, cmdMsg.CommandID)

	// 查找目标客户端的 control 连接
	targetConn := l.sessionMgr.GetControlConnectionByClientID(cmdMsg.TargetClientID)
	if targetConn == nil || targetConn.Stream == nil {
		corelog.Errorf("CrossNodeListener: target client %d not found", cmdMsg.TargetClientID)
		l.sendCommandError(conn, cmdMsg.CommandID, cmdMsg.CommandType, "target client not connected")
		return
	}

	// 注册等待响应
	waitCh := l.sessionMgr.commandResponseMgr.Register(cmdMsg.CommandID)
	defer l.sessionMgr.commandResponseMgr.Unregister(cmdMsg.CommandID)

	// 构建并转发命令到目标客户端
	forwardPkt := &packet.TransferPacket{
		PacketType: packet.JsonCommand,
		CommandPacket: &packet.CommandPacket{
			CommandType: packet.CommandType(cmdMsg.CommandType),
			CommandId:   cmdMsg.CommandID,
			CommandBody: string(cmdMsg.Payload),
		},
	}

	if _, err := targetConn.Stream.WritePacket(forwardPkt, true, 0); err != nil {
		corelog.Errorf("CrossNodeListener: failed to forward command to client: %v", err)
		l.sendCommandError(conn, cmdMsg.CommandID, cmdMsg.CommandType, "failed to forward command")
		return
	}

	// 等待响应（使用上下文的超时，默认 30 秒）
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := l.sessionMgr.commandResponseMgr.Wait(timeoutCtx, cmdMsg.CommandID, waitCh, 30*time.Second)
	if err != nil {
		corelog.Errorf("CrossNodeListener: command timeout: %v", err)
		l.sendCommandError(conn, cmdMsg.CommandID, cmdMsg.CommandType, "command timeout")
		return
	}

	l.sendCommandResponse(conn, cmdMsg.CommandID, cmdMsg.CommandType, resp)
}

// sendCommandError 发送命令错误响应
func (l *CrossNodeListener) sendCommandError(conn *net.TCPConn, commandID string, commandType byte, errMsg string) {
	respMsg := &CommandResponseMessage{
		CommandID:   commandID,
		CommandType: commandType,
		Success:     false,
		Error:       errMsg,
	}

	respData, err := json.Marshal(respMsg)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to marshal command error response: %v", err)
		return
	}

	var emptyTunnelID [16]byte
	if err := WriteFrame(conn, emptyTunnelID, FrameTypeCommandResponse, respData); err != nil {
		corelog.Errorf("CrossNodeListener: failed to send command error response: %v", err)
	}
}

// sendCommandResponse 发送命令响应
func (l *CrossNodeListener) sendCommandResponse(conn *net.TCPConn, commandID string, commandType byte, resp *packet.CommandPacket) {
	respMsg := &CommandResponseMessage{
		CommandID:   commandID,
		CommandType: commandType,
		Success:     true,
		Payload:     []byte(resp.CommandBody),
	}

	respData, err := json.Marshal(respMsg)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to marshal command response: %v", err)
		l.sendCommandError(conn, commandID, commandType, "failed to marshal response")
		return
	}

	var emptyTunnelID [16]byte
	if err := WriteFrame(conn, emptyTunnelID, FrameTypeCommandResponse, respData); err != nil {
		corelog.Errorf("CrossNodeListener: failed to send command response: %v", err)
	} else {
		corelog.Infof("CrossNodeListener: sent command response for commandID=%s", commandID)
	}
}
