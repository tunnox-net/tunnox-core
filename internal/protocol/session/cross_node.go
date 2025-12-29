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
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/httptypes"
)

// ============================================================================
// 跨节点目标端连接处理
// ============================================================================

// handleCrossNodeTargetConnection 处理跨节点的目标端连接
// 当 TargetClient 连接到的节点与 Bridge 所在节点不同时调用
func (s *SessionManager) handleCrossNodeTargetConnection(
	req *packet.TunnelOpenRequest,
	conn *types.Connection,
	netConn net.Conn,
) error {
	// 1. 检查必要的组件
	if s.tunnelRouting == nil {
		corelog.Errorf("CrossNode[%s]: TunnelRoutingTable not configured", req.TunnelID)
		return coreerrors.New(coreerrors.CodeUnavailable, "TunnelRoutingTable not configured")
	}
	if s.crossNodePool == nil {
		corelog.Errorf("CrossNode[%s]: CrossNodePool not configured", req.TunnelID)
		return coreerrors.New(coreerrors.CodeUnavailable, "CrossNodePool not configured")
	}

	// 2. 设置超时上下文
	ctx, cancel := context.WithTimeout(s.Ctx(), 10*time.Second)
	defer cancel()

	// 3. 查询隧道路由信息
	routingState, err := s.lookupTunnelRouting(ctx, req.TunnelID)
	if err != nil {
		corelog.Errorf("CrossNode[%s]: failed to lookup routing: %v", req.TunnelID, err)
		return err
	}

	return s.processCrossNodeForward(ctx, req, conn, netConn, routingState)
}

// lookupTunnelRouting 查询隧道路由信息（带重试）
func (s *SessionManager) lookupTunnelRouting(ctx context.Context, tunnelID string) (*TunnelWaitingState, error) {
	var routingState *TunnelWaitingState
	var err error

	// 轮询 Redis 查找路由信息（解决时序问题）
	for range 100 { // 最多等待 10 秒（100 * 100ms）
		select {
		case <-ctx.Done():
			return nil, coreerrors.Wrap(ctx.Err(), coreerrors.CodeTimeout, "timeout waiting for tunnel routing")
		default:
		}

		routingState, err = s.tunnelRouting.LookupWaitingTunnel(ctx, tunnelID)
		if err == nil {
			return routingState, nil
		}

		if err != ErrTunnelNotFound && err != ErrTunnelExpired {
			return nil, coreerrors.Wrap(err, coreerrors.CodeStorageError, "failed to lookup tunnel routing")
		}

		// 路由信息不存在，等待一下再试
		time.Sleep(100 * time.Millisecond)
	}

	return nil, coreerrors.New(coreerrors.CodeNotFound, "tunnel routing not found after polling")
}

// processCrossNodeForward 处理跨节点转发
func (s *SessionManager) processCrossNodeForward(
	ctx context.Context,
	req *packet.TunnelOpenRequest,
	conn *types.Connection,
	netConn net.Conn,
	routingState *TunnelWaitingState,
) error {
	// 如果 Bridge 在当前节点，说明是时序问题，等待 Bridge 创建
	if routingState.SourceNodeID == s.nodeID {
		return s.handleLocalBridgeWait(req, conn, netConn)
	}

	// Bridge 在其他节点，需要跨节点转发
	return s.forwardToSourceNode(ctx, req, conn, netConn, routingState)
}

// handleLocalBridgeWait 等待本地 Bridge 创建
func (s *SessionManager) handleLocalBridgeWait(
	req *packet.TunnelOpenRequest,
	conn *types.Connection,
	netConn net.Conn,
) error {
	// 等待 Bridge 创建（最多等待 5 秒）
	for range 50 {
		time.Sleep(100 * time.Millisecond)

		s.bridgeLock.RLock()
		bridge, exists := s.tunnelBridges[req.TunnelID]
		s.bridgeLock.RUnlock()

		if exists {
			// Bridge 已创建，设置目标端连接
			clientID := extractClientID(conn.Stream, netConn)
			tunnelConn := CreateTunnelConnection(conn.ID, netConn, conn.Stream, clientID, req.MappingID, req.TunnelID)
			bridge.SetTargetConnection(tunnelConn)
			return nil
		}
	}

	return coreerrors.New(coreerrors.CodeTimeout, "bridge not created on source node after waiting")
}

// forwardToSourceNode 转发到源节点
func (s *SessionManager) forwardToSourceNode(
	ctx context.Context,
	req *packet.TunnelOpenRequest,
	conn *types.Connection,
	netConn net.Conn,
	routingState *TunnelWaitingState,
) error {
	corelog.Infof("CrossNode[%s]: forwarding to sourceNode=%s", req.TunnelID, routingState.SourceNodeID)

	// 0. 先发送 TunnelOpenAck 给 Target 客户端
	s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  true,
	})

	// 1. 从连接池获取跨节点连接
	crossConn, err := s.crossNodePool.Get(ctx, routingState.SourceNodeID)
	if err != nil {
		corelog.Errorf("CrossNode[%s]: failed to get cross-node connection: %v", req.TunnelID, err)
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to get cross-node connection")
	}

	// 2. 发送 TargetTunnelReady 消息
	tunnelID, _ := TunnelIDFromString(req.TunnelID)
	readyData := EncodeTargetReadyMessage(req.TunnelID, s.nodeID)
	if err := WriteFrame(crossConn.GetTCPConn(), tunnelID, FrameTypeTargetReady, readyData); err != nil {
		corelog.Errorf("CrossNode[%s]: failed to send target ready message: %v", req.TunnelID, err)
		crossConn.MarkBroken()
		s.crossNodePool.CloseConn(crossConn)
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send target ready message")
	}

	// 3. 启动数据转发（零拷贝）
	go s.runCrossNodeDataForward(req.TunnelID, conn, netConn, crossConn)

	// 4. 返回特殊错误，让 readLoop 退出（连接已被跨节点转发接管）
	return fmt.Errorf("tunnel target connected via cross-node forwarding, switching to stream mode")
}

// runCrossNodeDataForward 运行跨节点数据转发（零拷贝）
func (s *SessionManager) runCrossNodeDataForward(
	tunnelID string,
	conn *types.Connection,
	netConn net.Conn,
	crossConn *CrossNodeConn,
) {
	// 确保数据转发完成后关闭本地连接
	defer func() {
		if netConn != nil {
			netConn.Close()
		}
		if conn != nil && conn.Stream != nil {
			conn.Stream.Close()
		}
	}()

	// 获取本地连接：优先使用 conn.Stream 的 GetReader()/GetWriter()
	var localConn io.ReadWriter
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		writer := conn.Stream.GetWriter()
		if reader != nil && writer != nil {
			localConn = &readWriterWrapper{reader: reader, writer: writer}
		}
	}

	// 如果 Stream 不可用，回退到 netConn
	if localConn == nil && netConn != nil {
		localConn = netConn
	}

	if localConn == nil {
		corelog.Errorf("CrossNodeDataForward[%s]: no valid localConn", tunnelID)
		return
	}

	// 解析 TunnelID
	tunnelIDBytes, err := TunnelIDFromString(tunnelID)
	if err != nil {
		corelog.Errorf("CrossNodeDataForward[%s]: invalid tunnel ID: %v", tunnelID, err)
		return
	}

	// 创建 FrameStream
	frameStream := NewFrameStreamWithTracker(crossConn, tunnelIDBytes, s)

	// 数据转发完成后：清理资源并归还连接
	defer func() {
		s.MarkTunnelClosed(tunnelID)
		if !frameStream.IsBroken() {
			crossConn.Release()
		} else {
			crossConn.Close()
		}
	}()

	// 双向数据转发
	done := make(chan struct{}, 2)
	var closeOnce sync.Once

	go func() {
		defer func() {
			closeOnce.Do(func() { _ = frameStream.Close() })
			done <- struct{}{}
		}()
		_, _ = io.Copy(frameStream, localConn)
	}()

	go func() {
		defer func() {
			closeOnce.Do(func() { _ = frameStream.Close() })
			done <- struct{}{}
		}()
		_, _ = io.Copy(localConn, frameStream)
	}()

	<-done
	<-done
}

// readWriterWrapper 包装 Reader 和 Writer
type readWriterWrapper struct {
	reader io.Reader
	writer io.Writer
}

func (w *readWriterWrapper) Read(p []byte) (n int, err error) {
	return w.reader.Read(p)
}

func (w *readWriterWrapper) Write(p []byte) (n int, err error) {
	return w.writer.Write(p)
}

// getNodeAddress 获取节点地址
func (s *SessionManager) getNodeAddress(nodeID string) (string, error) {
	if s.tunnelRouting != nil {
		addr, err := s.tunnelRouting.GetNodeAddress(nodeID)
		if err == nil && addr != "" {
			return addr, nil
		}
	}
	return fmt.Sprintf("%s:50052", nodeID), nil
}

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

	sourceForwarder := bridge.getSourceForwarder()
	if sourceForwarder == nil {
		corelog.Errorf("CrossNodeListener[%s]: sourceForwarder is nil", tunnelID)
		return
	}

	defer sourceForwarder.Close()

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

	done := make(chan struct{}, 2)
	var closeOnce sync.Once

	go func() {
		defer func() {
			closeOnce.Do(func() { _ = frameStream.Close() })
			done <- struct{}{}
		}()
		_, _ = io.Copy(frameStream, sourceForwarder)
	}()

	go func() {
		defer func() {
			closeOnce.Do(func() { _ = frameStream.Close() })
			done <- struct{}{}
		}()
		_, _ = io.Copy(sourceForwarder, frameStream)
	}()

	<-done
	<-done
}

// getBridgeIDs 获取所有 bridge ID（用于调试）
func (l *CrossNodeListener) getBridgeIDs() []string {
	l.sessionMgr.bridgeLock.RLock()
	defer l.sessionMgr.bridgeLock.RUnlock()
	ids := make([]string, 0, len(l.sessionMgr.tunnelBridges))
	for id := range l.sessionMgr.tunnelBridges {
		ids = append(ids, id)
	}
	return ids
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
