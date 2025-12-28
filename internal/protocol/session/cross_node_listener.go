package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/protocol/httptypes"
)

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
	// 解析消息 - 从消息体获取完整的 tunnelID（帧头中的可能被截断）
	fullTunnelID, targetNodeID, err := DecodeTargetReadyMessage(data)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to decode target ready message: %v", err)
		return
	}

	// 使用消息体中的完整 tunnelID
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

	// 通知 Bridge target 已就绪
	bridge.NotifyTargetReady()

	// 启动数据转发（零拷贝）
	l.runBridgeForward(tunnelIDStr, bridge, crossConn)
}

// runBridgeForward 运行 Bridge 数据转发
// 🔥 重构：使用 FrameStream 封装帧协议，实现连接复用
func (l *CrossNodeListener) runBridgeForward(tunnelID string, bridge *TunnelBridge, crossConn *CrossNodeConn) {
	// 🔧 关键修复：数据转发完成后关闭 Bridge，触发生命周期结束
	// 这样 bridge.Start() 会从 <-b.Ctx().Done() 返回，runBridgeLifecycle 会从 map 中删除 bridge
	// 防止高并发场景下 bridge 泄漏导致后续请求因 tunnelID 重复而失败
	defer bridge.Close()

	// 获取源端数据转发器（支持所有协议）
	sourceForwarder := bridge.getSourceForwarder()
	if sourceForwarder == nil {
		corelog.Errorf("CrossNodeListener[%s]: sourceForwarder is nil, bridge.sourceConn=%v, bridge.sourceStream=%v",
			tunnelID, bridge.sourceConn != nil, bridge.sourceStream != nil)
		return
	}

	// 🔧 关键修复：确保数据转发完成后关闭源端连接
	// 这样 Listen 端客户端的 BidirectionalCopy 才能正确收到 EOF 并结束
	defer sourceForwarder.Close()

	// 解析 TunnelID
	tunnelIDBytes, err := TunnelIDFromString(tunnelID)
	if err != nil {
		corelog.Errorf("CrossNodeListener[%s]: invalid tunnel ID: %v", tunnelID, err)
		return
	}

	// 🔥 创建 FrameStream（封装帧协议，传入 SessionManager 用于状态跟踪）
	frameStream := NewFrameStreamWithTracker(crossConn, tunnelIDBytes, l.sessionMgr)

	// 🔥 数据转发完成后：清理资源并归还连接
	defer func() {
		// 标记 tunnel 为已关闭状态（用于过滤残留帧）
		l.sessionMgr.MarkTunnelClosed(tunnelID)

		// 归还连接到池
		if !frameStream.IsBroken() {
			corelog.Debugf("CrossNodeListener[%s]: releasing connection to pool", tunnelID)
			crossConn.Release()
		} else {
			corelog.Warnf("CrossNodeListener[%s]: connection broken, closing", tunnelID)
			crossConn.Close()
		}

		// 清理 bridge 对连接的引用
		bridge.ReleaseCrossNodeConnection()
	}()

	// 双向数据转发（使用 FrameStream，自动处理帧协议）
	done := make(chan struct{}, 2)
	var closeOnce sync.Once
	var bytesSent, bytesRecv int64

	// 源端 -> 跨节点
	go func() {
		defer func() {
			closeOnce.Do(func() { _ = frameStream.Close() })
			done <- struct{}{}
		}()
		bytesSent, _ = io.Copy(frameStream, sourceForwarder)
	}()

	// 跨节点 -> 源端
	go func() {
		defer func() {
			closeOnce.Do(func() { _ = frameStream.Close() })
			done <- struct{}{}
		}()
		bytesRecv, _ = io.Copy(sourceForwarder, frameStream)
	}()

	// 等待两个方向都完成
	<-done
	<-done
	corelog.Infof("CrossNodeListener[%s]: forward completed, sent=%d, recv=%d", tunnelID, bytesSent, bytesRecv)
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

	// 1. 解析 HTTP 代理消息
	var proxyMsg HTTPProxyMessage
	if err := json.Unmarshal(data, &proxyMsg); err != nil {
		corelog.Errorf("CrossNodeListener: failed to unmarshal HTTP proxy message: %v", err)
		l.sendHTTPProxyError(conn, "", "failed to unmarshal request")
		return
	}

	corelog.Infof("CrossNodeListener: HTTP proxy request for client %d, requestID=%s",
		proxyMsg.ClientID, proxyMsg.RequestID)

	// 2. 解析 HTTPProxyRequest
	var request httptypes.HTTPProxyRequest
	if err := json.Unmarshal(proxyMsg.Request, &request); err != nil {
		corelog.Errorf("CrossNodeListener: failed to unmarshal HTTPProxyRequest: %v", err)
		l.sendHTTPProxyError(conn, proxyMsg.RequestID, "failed to unmarshal HTTP request")
		return
	}

	// 3. 在本地节点查找客户端连接
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

	// 4. 发送 HTTP 代理请求到客户端
	resp, err := l.sessionMgr.sendHTTPProxyRequestLocal(controlConn, &request)
	if err != nil {
		corelog.Errorf("CrossNodeListener: failed to send HTTP proxy request to client: %v", err)
		l.sendHTTPProxyError(conn, proxyMsg.RequestID, err.Error())
		return
	}

	// 5. 发送响应回源节点
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
	// 序列化 HTTPProxyResponse
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
