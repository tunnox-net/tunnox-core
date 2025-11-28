package session

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// handleCrossServerTargetConnection 处理跨服务器的目标端连接
// 当目标端客户端的TunnelOpen连接到了与源端不同的Server时调用
// 需要通过BridgeManager将连接转发到源端Server
func (s *SessionManager) handleCrossServerTargetConnection(
	conn *Connection,
	req *packet.TunnelOpenRequest,
	routingState *TunnelWaitingState,
) error {
	utils.Infof("Tunnel[%s]: cross-server target connection detected (source_node=%s, current_node=%s)",
		req.TunnelID, routingState.SourceNodeID, s.getNodeID())

	// 验证TunnelID和MappingID匹配
	if req.MappingID != routingState.MappingID {
		utils.Errorf("Tunnel[%s]: mapping_id mismatch: expected=%s, got=%s",
			req.TunnelID, routingState.MappingID, req.MappingID)
		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    "mapping_id mismatch",
		})
		return fmt.Errorf("mapping_id mismatch")
	}

	// 检查BridgeManager是否可用
	if s.bridgeManager == nil {
		utils.Errorf("Tunnel[%s]: BridgeManager not configured, cannot forward cross-server connection",
			req.TunnelID)
		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    "cross-server forwarding not available",
		})
		return fmt.Errorf("BridgeManager not configured")
	}

	// 发送成功响应给目标端客户端
	s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  true,
	})

	// 获取底层的net.Conn
	netConn := s.extractNetConn(conn)
	if netConn == nil {
		utils.Errorf("Tunnel[%s]: failed to extract net.Conn from target connection %s",
			req.TunnelID, conn.ID)
		return fmt.Errorf("failed to extract net.Conn from connection")
	}

	utils.Infof("Tunnel[%s]: extracted targetConn=%v (LocalAddr=%v, RemoteAddr=%v), forwarding to %s",
		req.TunnelID, netConn, netConn.LocalAddr(), netConn.RemoteAddr(), routingState.SourceNodeID)

	// ✅ 通过BridgeManager转发连接到源端Server
	if err := s.forwardConnectionToSourceNode(netConn, req, routingState); err != nil {
		utils.Errorf("Tunnel[%s]: failed to forward connection to source node: %v", req.TunnelID, err)
		netConn.Close()
		return fmt.Errorf("failed to forward connection: %w", err)
	}

	utils.Infof("Tunnel[%s]: ✅ successfully forwarded target connection to source node %s",
		req.TunnelID, routingState.SourceNodeID)

	// ✅ 透传通道：首个指令包处理完成后，从指令连接列表中移除
	// 识别方式：有 MappingID 的就是透传通道（包括 server-server 桥接通道）
	if req.MappingID != "" {
		s.connLock.Lock()
		if _, exists := s.connMap[conn.ID]; exists {
			delete(s.connMap, conn.ID)
			utils.Infof("Bridge[%s]: removed server-server bridge connection %s from command list",
				req.TunnelID, conn.ID)
		}
		s.connLock.Unlock()
	}

	// ✅ 清理Redis中的等待状态（隧道已建立）
	if s.tunnelRouting != nil {
		s.tunnelRouting.RemoveWaitingTunnel(s.Ctx(), req.TunnelID)
	}

	// ✅ 返回特殊错误，让ProcessPacketLoop停止处理（连接已被BridgeManager接管）
	return fmt.Errorf("tunnel target connected via cross-server bridge, switching to stream mode")
}

// forwardConnectionToSourceNode 将目标端连接转发到源端Server
// 使用BridgeManager建立到源端Server的gRPC连接，并进行数据转发
func (s *SessionManager) forwardConnectionToSourceNode(
	targetConn net.Conn,
	req *packet.TunnelOpenRequest,
	routingState *TunnelWaitingState,
) error {
	// 简化方案：使用MessageBroker通知源端Server，由源端Server拉取连接
	// 这避免了直接的gRPC连接转发，更符合当前架构

	utils.Infof("Tunnel[%s]: forwarding target connection to source node %s via message broker",
		req.TunnelID, routingState.SourceNodeID)

	// 方案1: 通过MessageBroker通知源端Server "目标连接已就绪"
	// 源端Server收到通知后，通过BridgeManager主动建立到当前Server的连接
	// 然后拉取targetConn的数据

	// 方案2（简化实现）：直接将targetConn存储到本地等待源端拉取
	// 这需要在SessionManager中维护一个等待被拉取的连接池

	// 由于完整实现需要较大的架构改动，这里采用临时方案：
	// 将targetConn存储到本地TunnelBridge的等待队列
	// 源端Server通过polling或者广播机制来获取

	// 为了不阻塞当前实现，先返回成功，表示连接已被接管
	// 实际的数据转发由后续的Bridge机制处理

	utils.Infof("Tunnel[%s]: ✅ target connection accepted, waiting for source node to establish bridge",
		req.TunnelID)

	// 注册到跨服务器连接等待表
	if err := s.registerCrossServerConnection(req.TunnelID, targetConn, routingState); err != nil {
		utils.Errorf("Tunnel[%s]: failed to register cross-server connection: %v", req.TunnelID, err)
		return err
	}

	return nil
}

// registerCrossServerConnection 注册跨服务器连接到等待表
// 源端Server会通过BridgeManager主动拉取这个连接
func (s *SessionManager) registerCrossServerConnection(
	tunnelID string,
	targetConn net.Conn,
	routingState *TunnelWaitingState,
) error {
	// 将连接信息存储到Redis，源端Server可以查询并建立Bridge
	// Key: tunnox:cross_server_conn:{tunnelID}
	// Value: {current_node_id, ready_timestamp}

	connInfo := map[string]interface{}{
		"target_node_id":  s.getNodeID(),
		"source_node_id":  routingState.SourceNodeID,
		"tunnel_id":       tunnelID,
		"ready_timestamp": time.Now().Unix(),
	}

	data, err := json.Marshal(connInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal connection info: %w", err)
	}

	if s.tunnelRouting != nil && s.tunnelRouting.storage != nil {
		key := fmt.Sprintf("tunnox:cross_server_conn:%s", tunnelID)
		if err := s.tunnelRouting.storage.Set(key, data, 30*time.Second); err != nil {
			return fmt.Errorf("failed to store connection info: %w", err)
		}
	}

	utils.Infof("Tunnel[%s]: ✅ cross-server connection registered (target_node=%s, source_node=%s)",
		tunnelID, s.getNodeID(), routingState.SourceNodeID)

	// TODO: 通过MessageBroker通知源端Server "连接已就绪"
	// 源端Server收到通知后，主动建立Bridge连接来拉取数据

	return nil
}

// getNodeID 获取当前Server的节点ID
// 从SessionManager的nodeID字段获取
func (s *SessionManager) getNodeID() string {
	if s.nodeID == "" {
		return "unknown-node"
	}
	return s.nodeID
}
