package session

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/packet"
)

// handleCrossServerTargetConnection 处理跨服务器的目标端连接
// 当目标端客户端的TunnelOpen连接到了与源端不同的Server时调用
// 需要通过BridgeManager将连接转发到源端Server
func (s *SessionManager) handleCrossServerTargetConnection(
	conn *Connection,
	req *packet.TunnelOpenRequest,
	routingState *TunnelWaitingState,
) error {
	// 验证TunnelID和MappingID匹配
	if req.MappingID != routingState.MappingID {
		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    "mapping_id mismatch",
		})
		return fmt.Errorf("mapping_id mismatch")
	}

	// 检查BridgeManager是否可用
	if s.bridgeManager == nil {
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
		return fmt.Errorf("failed to extract net.Conn from connection")
	}

	// 通过BridgeManager转发连接到源端Server
	if err := s.forwardConnectionToSourceNode(netConn, req, routingState); err != nil {
		netConn.Close()
		return fmt.Errorf("failed to forward connection: %w", err)
	}

	// 透传通道：首个指令包处理完成后，从指令连接列表中移除
	// 识别方式：有 MappingID 的就是透传通道（包括 server-server 桥接通道）
	if req.MappingID != "" {
		s.connLock.Lock()
		if _, exists := s.connMap[conn.ID]; exists {
			delete(s.connMap, conn.ID)
		}
		s.connLock.Unlock()
	}

	// 清理Redis中的等待状态（隧道已建立）
	if s.tunnelRouting != nil {
		s.tunnelRouting.RemoveWaitingTunnel(s.Ctx(), req.TunnelID)
	}

	// 返回特殊错误，让ProcessPacketLoop停止处理（连接已被BridgeManager接管）
	return fmt.Errorf("tunnel target connected via cross-server bridge, switching to stream mode")
}

// forwardConnectionToSourceNode 将目标端连接转发到源端Server
// 使用BridgeManager建立到源端Server的gRPC连接，并进行数据转发
func (s *SessionManager) forwardConnectionToSourceNode(
	targetConn net.Conn,
	req *packet.TunnelOpenRequest,
	routingState *TunnelWaitingState,
) error {
	// 注册到跨服务器连接等待表
	if err := s.registerCrossServerConnection(req.TunnelID, targetConn, routingState); err != nil {
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

	// 通过MessageBroker通知源端Server "连接已就绪"
	if s.bridgeManager != nil {
		msg := &broker.TunnelOpenMessage{
			TunnelID:   tunnelID,
			TargetHost: s.getNodeID(),
			Timestamp:  time.Now().Unix(),
		}
		msgData, err := json.Marshal(msg)
		if err == nil {
			s.bridgeManager.PublishMessage(s.Ctx(), broker.TopicTunnelOpen, msgData)
		}
	}

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
