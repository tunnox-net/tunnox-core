package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	coreerrors "tunnox-core/internal/core/errors"
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
		return
	}

	// 读取第一个帧，确定隧道 ID
	tunnelID, frameType, data, err := ReadFrame(tcpConn)
	if err != nil {
		return
	}

	tunnelIDStr := TunnelIDToString(tunnelID)

	switch frameType {
	case FrameTypeTargetReady:
		l.handleTargetReady(ctx, tcpConn, tunnelIDStr, data)
	}
}

// handleTargetReady 处理 TargetTunnelReady 消息
func (l *CrossNodeListener) handleTargetReady(ctx context.Context, conn *net.TCPConn, tunnelIDStr string, data []byte) {
	// 解析消息 - 从消息体获取完整的 tunnelID（帧头中的可能被截断）
	fullTunnelID, targetNodeID, err := DecodeTargetReadyMessage(data)
	if err != nil {
		return
	}

	// 使用消息体中的完整 tunnelID
	if fullTunnelID != "" {
		tunnelIDStr = fullTunnelID
	}

	// 查找对应的 Bridge
	l.sessionMgr.bridgeLock.RLock()
	bridge, exists := l.sessionMgr.tunnelBridges[tunnelIDStr]
	l.sessionMgr.bridgeLock.RUnlock()

	if !exists {
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
func (l *CrossNodeListener) runBridgeForward(tunnelID string, bridge *TunnelBridge, crossConn *CrossNodeConn) {
	defer bridge.ReleaseCrossNodeConnection()

	// 获取源端数据转发器（支持所有协议）
	sourceForwarder := bridge.getSourceForwarder()
	if sourceForwarder == nil {
		return
	}

	// 获取跨节点 TCP 连接
	tcpConn := crossConn.GetTCPConn()
	if tcpConn == nil {
		return
	}

	// 双向数据转发
	errChan := make(chan error, 2)

	// 源端 -> 跨节点
	go func() {
		_, err := io.Copy(tcpConn, sourceForwarder)
		errChan <- err
	}()

	// 跨节点 -> 源端
	go func() {
		_, err := io.Copy(sourceForwarder, tcpConn)
		errChan <- err
	}()

	// 等待任一方向完成
	<-errChan
}
