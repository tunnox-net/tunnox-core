package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
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
	defer conn.Close()

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
		l.handleTargetReady(ctx, tcpConn, tunnelIDStr, data)
	case FrameTypeHTTPProxy:
		l.handleHTTPProxy(ctx, tcpConn, data)
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
func (l *CrossNodeListener) runBridgeForward(tunnelID string, bridge *TunnelBridge, crossConn *CrossNodeConn) {
	defer bridge.Close()

	sourceForwarder := bridge.GetSourceForwarder()
	if sourceForwarder == nil {
		corelog.Errorf("CrossNodeListener[%s]: sourceForwarder is nil", tunnelID)
		return
	}

	tunnelIDBytes, err := TunnelIDFromString(tunnelID)
	if err != nil {
		corelog.Errorf("CrossNodeListener[%s]: invalid tunnel ID: %v", tunnelID, err)
		return
	}

	frameStream := NewFrameStreamWithTracker(crossConn, tunnelIDBytes, l.sessionMgr)

	defer func() {
		l.sessionMgr.MarkTunnelClosed(tunnelID)
		if !frameStream.IsBroken() {
			corelog.Debugf("CrossNodeListener[%s]: releasing connection to pool", tunnelID)
			crossConn.Release()
		} else {
			corelog.Warnf("CrossNodeListener[%s]: connection broken, closing", tunnelID)
			crossConn.Close()
		}
		bridge.ReleaseCrossNodeConnection()
	}()

	// 获取 Bridge 的流量计数器引用
	bytesSentPtr := bridge.GetBytesSentPtr()
	bytesReceivedPtr := bridge.GetBytesReceivedPtr()

	// 使用公共的双向转发逻辑（带流量统计）
	runBidirectionalForward(&BidirectionalForwardConfig{
		TunnelID:             tunnelID,
		LogPrefix:            "CrossNodeListener",
		LocalConn:            sourceForwarder,
		LocalConnCloser:      sourceForwarder,
		RemoteConn:           frameStream,
		BytesSentCounter:     bytesSentPtr,
		BytesReceivedCounter: bytesReceivedPtr,
	})
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
