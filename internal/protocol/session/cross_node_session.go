package session

import (
	"context"
	"io"
	"net"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// ============================================================================
// 跨节点目标端连接处理 - SessionManager 相关方法
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

// 指数退避参数
const (
	pollInitialInterval = 50 * time.Millisecond  // 起始间隔
	pollMaxInterval     = 200 * time.Millisecond // 最大间隔
	pollBackoffFactor   = 2                      // 退避因子
)

// lookupTunnelRouting 查询隧道路由信息（带指数退避重试）
func (s *SessionManager) lookupTunnelRouting(ctx context.Context, tunnelID string) (*TunnelWaitingState, error) {
	var routingState *TunnelWaitingState
	var err error

	// 轮询 Redis 查找路由信息（解决时序问题）
	// 使用指数退避：50ms -> 100ms -> 200ms -> 200ms...
	interval := pollInitialInterval
	for {
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

		// 路由信息不存在，等待一下再试（指数退避）
		time.Sleep(interval)
		interval *= pollBackoffFactor
		if interval > pollMaxInterval {
			interval = pollMaxInterval
		}
	}
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

// handleLocalBridgeWait 等待本地 Bridge 创建（使用指数退避）
func (s *SessionManager) handleLocalBridgeWait(
	req *packet.TunnelOpenRequest,
	conn *types.Connection,
	netConn net.Conn,
) error {
	// 等待 Bridge 创建（最多等待 5 秒）
	// 使用指数退避：50ms -> 100ms -> 200ms -> 200ms...
	timeout := time.After(5 * time.Second)
	interval := pollInitialInterval

	for {
		select {
		case <-timeout:
			return coreerrors.New(coreerrors.CodeTimeout, "bridge not created on source node after waiting")
		default:
		}

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

		// 等待后重试（指数退避）
		time.Sleep(interval)
		interval *= pollBackoffFactor
		if interval > pollMaxInterval {
			interval = pollMaxInterval
		}
	}
}

// forwardToSourceNode 转发到源节点
func (s *SessionManager) forwardToSourceNode(
	ctx context.Context,
	req *packet.TunnelOpenRequest,
	conn *types.Connection,
	netConn net.Conn,
	routingState *TunnelWaitingState,
) error {
	corelog.Infof("CrossNode[%s]: currentNode=%s -> sourceNode=%s (reason: bridge on remote)",
		req.TunnelID, s.nodeID, routingState.SourceNodeID)

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
	tunnelID, err := TunnelIDFromString(req.TunnelID)
	if err != nil {
		corelog.Errorf("CrossNode[%s]: invalid tunnel ID format: %v", req.TunnelID, err)
		crossConn.Release()
		return coreerrors.Wrap(err, coreerrors.CodeInvalidParam, "invalid tunnel ID format")
	}
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
	return coreerrors.New(coreerrors.CodeTunnelModeSwitch, "tunnel target connected via cross-node forwarding, switching to stream mode")
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

	// 使用公共的双向转发逻辑
	runBidirectionalForward(&BidirectionalForwardConfig{
		TunnelID:   tunnelID,
		LogPrefix:  "CrossNodeDataForward",
		LocalConn:  localConn,
		RemoteConn: frameStream,
	})
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
	return nodeID + ":50052", nil
}
