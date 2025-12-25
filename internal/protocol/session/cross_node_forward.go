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
)

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
	corelog.Infof("CrossNode[%s]: forwardToSourceNode called, sourceNodeID=%s", req.TunnelID, routingState.SourceNodeID)

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
	corelog.Infof("CrossNode[%s]: got cross-node connection to %s", req.TunnelID, routingState.SourceNodeID)

	// 2. 发送 TargetTunnelReady 消息
	tunnelID, _ := TunnelIDFromString(req.TunnelID)
	readyData := EncodeTargetReadyMessage(req.TunnelID, s.nodeID)
	corelog.Infof("CrossNode[%s]: sending TargetReady message, tunnelID=%v, dataLen=%d", req.TunnelID, tunnelID, len(readyData))
	if err := WriteFrame(crossConn.GetTCPConn(), tunnelID, FrameTypeTargetReady, readyData); err != nil {
		corelog.Errorf("CrossNode[%s]: failed to send target ready message: %v", req.TunnelID, err)
		crossConn.MarkBroken()
		s.crossNodePool.CloseConn(crossConn)
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to send target ready message")
	}
	corelog.Infof("CrossNode[%s]: TargetReady message sent successfully", req.TunnelID)

	// 3. 启动数据转发（零拷贝）
	go s.runCrossNodeDataForward(req.TunnelID, conn, netConn, crossConn)

	// 4. 返回特殊错误，让 readLoop 退出（连接已被跨节点转发接管）
	return fmt.Errorf("tunnel target connected via cross-node forwarding, switching to stream mode")
}

// runCrossNodeDataForward 运行跨节点数据转发（零拷贝）
// 重要：这个函数在 Target 节点上运行，负责在 Target 客户端的隧道连接和跨节点连接之间转发数据
// 数据流：Target Client ←→ [本函数] ←→ CrossNodeConn ←→ Source 节点
//
// 关键点：必须使用 conn.Stream 的 GetReader()/GetWriter()，而不是原始 netConn
// 因为 Target 客户端通过 tunnelStream 读写数据（带协议层），我们需要在同一层对接
//
// 🔧 修复：使用半关闭语义避免高并发时连接过早关闭
func (s *SessionManager) runCrossNodeDataForward(
	tunnelID string,
	conn *types.Connection,
	netConn net.Conn,
	crossConn *CrossNodeConn,
) {
	defer func() {
		if crossConn != nil {
			// 重要：数据转发完成后，连接已经被使用（CloseWrite），
			// 不能归还到连接池，必须直接关闭
			crossConn.MarkBroken()
			crossConn.Close()
		}
	}()

	// 🔧 关键修复：确保数据转发完成后关闭本地连接
	// 这样 Target 客户端的 BidirectionalCopy 才能正确收到 EOF 并结束
	defer func() {
		if netConn != nil {
			netConn.Close()
		}
		if conn != nil && conn.Stream != nil {
			conn.Stream.Close()
		}
	}()

	// 获取本地连接
	// 重要：优先使用 conn.Stream 的 GetReader()/GetWriter()
	// 这样才能和 Target 客户端的 tunnelStream 正确对接
	var localConn io.ReadWriter
	var localNetConn net.Conn // 用于半关闭
	if conn != nil && conn.Stream != nil {
		reader := conn.Stream.GetReader()
		writer := conn.Stream.GetWriter()
		if reader != nil && writer != nil {
			localConn = &readWriterWrapper{reader: reader, writer: writer}
		}
	}

	// 如果 Stream 不可用，回退到 netConn（但这可能导致协议层不匹配）
	if localConn == nil && netConn != nil {
		localConn = netConn
		localNetConn = netConn
		corelog.Warnf("CrossNodeDataForward[%s]: falling back to netConn as localConn (may cause protocol mismatch)", tunnelID)
	}

	if localConn == nil {
		corelog.Errorf("CrossNodeDataForward[%s]: no valid localConn", tunnelID)
		return
	}

	// 获取跨节点 TCP 连接（用于零拷贝）
	tcpConn := crossConn.GetTCPConn()
	if tcpConn == nil {
		corelog.Errorf("CrossNodeDataForward[%s]: tcpConn is nil", tunnelID)
		return
	}

	// 双向数据转发
	done := make(chan struct{}, 2)

	// 本地 -> 跨节点
	go func() {
		defer func() { done <- struct{}{} }()
		n, err := io.Copy(tcpConn, localConn)
		if err != nil && err != io.EOF {
			corelog.Debugf("CrossNodeDataForward[%s]: local->crossNode error: %v", tunnelID, err)
		}
		corelog.Debugf("CrossNodeDataForward[%s]: local->crossNode finished, bytes=%d", tunnelID, n)
		// 🔧 关键：使用半关闭通知对端 EOF
		tcpConn.CloseWrite()
	}()

	// 跨节点 -> 本地
	go func() {
		defer func() { done <- struct{}{} }()
		n, err := io.Copy(localConn, tcpConn)
		if err != nil && err != io.EOF {
			corelog.Debugf("CrossNodeDataForward[%s]: crossNode->local error: %v", tunnelID, err)
		}
		corelog.Debugf("CrossNodeDataForward[%s]: crossNode->local finished, bytes=%d", tunnelID, n)
		// 🔧 关键：对本地连接使用半关闭（如果支持）
		if localNetConn != nil {
			if tcpLocal, ok := localNetConn.(*net.TCPConn); ok {
				tcpLocal.CloseWrite()
			}
		}
	}()

	// 等待两个方向都完成
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
	// 尝试从 TunnelRoutingTable 获取节点地址
	if s.tunnelRouting != nil {
		addr, err := s.tunnelRouting.GetNodeAddress(nodeID)
		if err == nil && addr != "" {
			return addr, nil
		}
	}

	// 默认使用节点 ID 作为主机名，端口为 50052
	return fmt.Sprintf("%s:50052", nodeID), nil
}
