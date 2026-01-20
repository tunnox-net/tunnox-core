package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
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
	// 优先使用专用连接管理器，回退到连接池
	if s.tunnelConnMgr == nil && s.crossNodePool == nil {
		corelog.Errorf("CrossNode[%s]: neither TunnelConnectionManager nor CrossNodePool configured", req.TunnelID)
		return coreerrors.New(coreerrors.CodeUnavailable, "cross-node connection manager not configured")
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

	// 获取客户端地址和目标地址（用于连接标识）
	remoteAddr := ""
	if netConn != nil && netConn.RemoteAddr() != nil {
		remoteAddr = netConn.RemoteAddr().String()
	}
	target := ""
	if req.TargetHost != "" {
		target = fmt.Sprintf("%s:%d", req.TargetHost, req.TargetPort)
	}

	// 1. 创建专用跨节点连接（不使用连接池）
	var tcpConn *net.TCPConn
	var err error

	if s.tunnelConnMgr != nil {
		// 使用专用连接管理器（推荐）
		tcpConn, err = s.tunnelConnMgr.CreateDedicatedConnection(
			ctx,
			req.TunnelID,
			routingState.SourceNodeID,
			remoteAddr,
			target,
		)
		if err != nil {
			corelog.Errorf("CrossNode[%s]: failed to create dedicated connection: %v", req.TunnelID, err)
			return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to create dedicated connection")
		}
	} else {
		// 回退到连接池（兼容旧代码）
		crossConn, poolErr := s.crossNodePool.Get(ctx, routingState.SourceNodeID)
		if poolErr != nil {
			corelog.Errorf("CrossNode[%s]: failed to get cross-node connection: %v", req.TunnelID, poolErr)
			return coreerrors.Wrap(poolErr, coreerrors.CodeNetworkError, "failed to get cross-node connection")
		}
		tcpConn = crossConn.GetTCPConn()
		// 标记为 broken，防止归还到连接池
		crossConn.MarkBroken()
	}

	// 2. 发送 TargetTunnelReady 消息
	tunnelID, err := TunnelIDFromString(req.TunnelID)
	if err != nil {
		corelog.Errorf("CrossNode[%s]: invalid tunnel ID format: %v", req.TunnelID, err)
		s.closeCrossNodeConnection(req.TunnelID, tcpConn)
		return coreerrors.Wrap(err, coreerrors.CodeInvalidParam, "invalid tunnel ID format")
	}
	readyData := EncodeTargetReadyMessage(req.TunnelID, s.nodeID)
	if err := WriteFrame(tcpConn, tunnelID, FrameTypeTargetReady, readyData); err != nil {
		corelog.Errorf("CrossNode[%s]: failed to send target ready message: %v", req.TunnelID, err)
		s.closeCrossNodeConnection(req.TunnelID, tcpConn)
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send target ready message")
	}

	// 3. 启动数据转发（零拷贝）
	corelog.Infof("CrossNode[%s]: about to start runCrossNodeDataForward goroutine, connID=%s, netConn=%v",
		req.TunnelID, conn.ID, netConn != nil)
	go s.runCrossNodeDataForwardDedicated(req.TunnelID, conn, netConn, tcpConn)

	// 4. 返回特殊错误，让 readLoop 退出（连接已被跨节点转发接管）
	corelog.Infof("CrossNode[%s]: returning CodeTunnelModeSwitch to let readLoop exit", req.TunnelID)
	return coreerrors.New(coreerrors.CodeTunnelModeSwitch, "tunnel target connected via cross-node forwarding, switching to stream mode")
}

// closeCrossNodeConnection 关闭跨节点连接
func (s *SessionManager) closeCrossNodeConnection(tunnelID string, tcpConn *net.TCPConn) {
	if s.tunnelConnMgr != nil {
		s.tunnelConnMgr.CloseTunnel(tunnelID)
	} else if tcpConn != nil {
		tcpConn.Close()
	}
}

// runCrossNodeDataForwardDedicated 运行跨节点数据转发（专用连接模型）
// 使用简单的 io.Copy 直接传输数据
// 连接生命周期与隧道绑定，隧道结束时自动关闭
func (s *SessionManager) runCrossNodeDataForwardDedicated(
	tunnelID string,
	conn *types.Connection,
	netConn net.Conn,
	tcpConn *net.TCPConn,
) {
	corelog.Infof("CrossNodeDataForward[%s]: goroutine started (dedicated mode), connID=%s, netConn=%v",
		tunnelID, conn.ID, netConn != nil)

	// 数据转发完成后：关闭专用连接
	defer func() {
		// 通过 TunnelConnectionManager 关闭（如果使用）
		if s.tunnelConnMgr != nil {
			s.tunnelConnMgr.CloseTunnel(tunnelID)
		} else if tcpConn != nil {
			tcpConn.Close()
		}
		s.MarkTunnelClosed(tunnelID)
		corelog.Infof("CrossNodeDataForward[%s]: cleanup completed (dedicated mode)", tunnelID)
	}()

	// 确保数据转发完成后关闭本地连接
	defer func() {
		if netConn != nil {
			netConn.Close()
		}
		if conn != nil && conn.Stream != nil {
			conn.Stream.Close()
		}
	}()

	// 获取本地连接
	var localConn io.ReadWriter
	var localNetConn net.Conn
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		writer := conn.Stream.GetWriter()
		if reader != nil && writer != nil {
			localConn = &readWriterWrapper{reader: reader, writer: writer}
		}
	}

	if localConn == nil && netConn != nil {
		localConn = netConn
		localNetConn = netConn
		corelog.Warnf("CrossNodeDataForward[%s]: falling back to netConn as localConn", tunnelID)
	}

	if localConn == nil {
		corelog.Errorf("CrossNodeDataForward[%s]: no valid localConn", tunnelID)
		return
	}

	if tcpConn == nil {
		corelog.Errorf("CrossNodeDataForward[%s]: tcpConn is nil", tunnelID)
		return
	}

	corelog.Infof("CrossNodeDataForward[%s]: starting data forward (dedicated mode)", tunnelID)

	// 双向数据转发
	done := make(chan struct{}, 2)

	// 本地 -> 跨节点
	go func() {
		defer func() { done <- struct{}{} }()
		n, err := io.Copy(tcpConn, localConn)
		if err != nil && err != io.EOF {
			corelog.Debugf("CrossNodeDataForward[%s]: local->crossNode error: %v", tunnelID, err)
		}
		corelog.Infof("CrossNodeDataForward[%s]: local->crossNode finished, bytes=%d", tunnelID, n)
		tcpConn.CloseWrite()
		// 更新活动时间
		if s.tunnelConnMgr != nil {
			s.tunnelConnMgr.UpdateActivity(tunnelID)
		}
	}()

	// 跨节点 -> 本地
	go func() {
		defer func() { done <- struct{}{} }()
		n, err := io.Copy(localConn, tcpConn)
		if err != nil && err != io.EOF {
			corelog.Debugf("CrossNodeDataForward[%s]: crossNode->local error: %v", tunnelID, err)
		}
		corelog.Infof("CrossNodeDataForward[%s]: crossNode->local finished, bytes=%d", tunnelID, n)
		if localNetConn != nil {
			if tcpLocal, ok := localNetConn.(*net.TCPConn); ok {
				tcpLocal.CloseWrite()
			}
		}
		// 更新活动时间
		if s.tunnelConnMgr != nil {
			s.tunnelConnMgr.UpdateActivity(tunnelID)
		}
	}()

	// 等待两个方向都完成
	<-done
	<-done
	corelog.Infof("CrossNodeDataForward[%s]: data forward completed (dedicated mode)", tunnelID)
}

// runCrossNodeDataForward 运行跨节点数据转发（连接池模型 - 兼容旧代码）
// Deprecated: 推荐使用 runCrossNodeDataForwardDedicated
func (s *SessionManager) runCrossNodeDataForward(
	tunnelID string,
	conn *types.Connection,
	netConn net.Conn,
	crossConn *CrossNodeConn,
) {
	corelog.Infof("CrossNodeDataForward[%s]: goroutine started (pool mode), connID=%s, netConn=%v",
		tunnelID, conn.ID, netConn != nil)

	defer func() {
		if crossConn != nil && s.crossNodePool != nil {
			s.crossNodePool.CloseConn(crossConn)
		}
		s.MarkTunnelClosed(tunnelID)
		corelog.Infof("CrossNodeDataForward[%s]: cleanup completed (pool mode)", tunnelID)
	}()

	defer func() {
		if netConn != nil {
			netConn.Close()
		}
		if conn != nil && conn.Stream != nil {
			conn.Stream.Close()
		}
	}()

	var localConn io.ReadWriter
	var localNetConn net.Conn
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		writer := conn.Stream.GetWriter()
		if reader != nil && writer != nil {
			localConn = &readWriterWrapper{reader: reader, writer: writer}
		}
	}

	if localConn == nil && netConn != nil {
		localConn = netConn
		localNetConn = netConn
	}

	if localConn == nil {
		corelog.Errorf("CrossNodeDataForward[%s]: no valid localConn", tunnelID)
		return
	}

	tcpConn := crossConn.GetTCPConn()
	if tcpConn == nil {
		corelog.Errorf("CrossNodeDataForward[%s]: tcpConn is nil", tunnelID)
		return
	}

	done := make(chan struct{}, 2)

	go func() {
		defer func() { done <- struct{}{} }()
		n, err := io.Copy(tcpConn, localConn)
		if err != nil && err != io.EOF {
			corelog.Debugf("CrossNodeDataForward[%s]: local->crossNode error: %v", tunnelID, err)
		}
		corelog.Infof("CrossNodeDataForward[%s]: local->crossNode finished, bytes=%d", tunnelID, n)
		tcpConn.CloseWrite()
	}()

	go func() {
		defer func() { done <- struct{}{} }()
		n, err := io.Copy(localConn, tcpConn)
		if err != nil && err != io.EOF {
			corelog.Debugf("CrossNodeDataForward[%s]: crossNode->local error: %v", tunnelID, err)
		}
		corelog.Infof("CrossNodeDataForward[%s]: crossNode->local finished, bytes=%d", tunnelID, n)
		if localNetConn != nil {
			if tcpLocal, ok := localNetConn.(*net.TCPConn); ok {
				tcpLocal.CloseWrite()
			}
		}
	}()

	<-done
	<-done
	corelog.Infof("CrossNodeDataForward[%s]: data forward completed (pool mode)", tunnelID)
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

// multiCloserWithStream 组合 net.Conn 和 stream.PackageStreamer
type multiCloserWithStream struct {
	netConn net.Conn
	stream  stream.PackageStreamer
}

func (m *multiCloserWithStream) Close() error {
	if m.stream != nil {
		m.stream.Close()
	}
	if m.netConn != nil {
		return m.netConn.Close()
	}
	return nil
}
