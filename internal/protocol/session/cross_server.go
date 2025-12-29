package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"tunnox-core/internal/broker"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ============================================================================
// 跨服务器 Tunnel 处理
// ============================================================================

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

	if s.tunnelRouting != nil && s.tunnelRouting.GetStorage() != nil {
		key := fmt.Sprintf("tunnox:cross_server_conn:%s", tunnelID)
		if err := s.tunnelRouting.GetStorage().Set(key, data, 30*time.Second); err != nil {
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

// ============================================================================
// TunnelOpen 广播订阅和处理
// ============================================================================

// TunnelOpenBroadcastMessage 跨服务器TunnelOpen广播消息
// 注意：字段名需要与 broker.TunnelOpenMessage 保持一致
type TunnelOpenBroadcastMessage struct {
	Type           string `json:"type"`
	TunnelID       string `json:"tunnel_id"`
	MappingID      string `json:"mapping_id"`
	SecretKey      string `json:"secret_key"`
	TargetClientID int64  `json:"client_id"` // 与 broker.TunnelOpenMessage.ClientID 对应
	SourceNodeID   string `json:"source_node_id"`
	Timestamp      int64  `json:"timestamp"`
	// SOCKS5 动态目标地址
	TargetHost string `json:"target_host,omitempty"`
	TargetPort int    `json:"target_port,omitempty"`
}

// startTunnelOpenBroadcastSubscription 启动TunnelOpen广播订阅
func (s *SessionManager) startTunnelOpenBroadcastSubscription() {
	if s.bridgeManager == nil {
		return
	}

	// 订阅TunnelOpen广播
	msgChan, err := s.bridgeManager.Subscribe(s.Ctx(), broker.TopicTunnelOpen)
	if err != nil {
		return
	}

	// 启动消息处理循环
	go s.processTunnelOpenBroadcasts(msgChan)
}

// processTunnelOpenBroadcasts 处理TunnelOpen广播消息
func (s *SessionManager) processTunnelOpenBroadcasts(msgChan <-chan *BroadcastMessage) {
	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				return
			}

			// 解析消息
			var broadcastMsg TunnelOpenBroadcastMessage
			if err := json.Unmarshal(msg.Payload, &broadcastMsg); err != nil {
				continue
			}

			// 处理广播
			s.handleTunnelOpenBroadcast(&broadcastMsg)

		case <-s.Ctx().Done():
			return
		}
	}
}

// handleTunnelOpenBroadcast 处理收到的TunnelOpen广播
func (s *SessionManager) handleTunnelOpenBroadcast(msg *TunnelOpenBroadcastMessage) {
	corelog.Infof("handleTunnelOpenBroadcast: received broadcast for tunnel %s, targetClientID=%d", msg.TunnelID, msg.TargetClientID)

	// 检查目标客户端是否在本地
	targetConn := s.GetControlConnectionByClientID(msg.TargetClientID)
	if targetConn == nil {
		corelog.Debugf("handleTunnelOpenBroadcast: target client %d not on this node", msg.TargetClientID)
		return
	}

	corelog.Infof("handleTunnelOpenBroadcast: found target client %d on this node, sending TunnelOpenRequest", msg.TargetClientID)

	// 获取映射配置
	if s.cloudControl == nil {
		return
	}

	// 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(msg.MappingID)
	if err != nil {
		return
	}

	// 构造TunnelOpenRequest命令
	// 对于 SOCKS5 协议，使用广播消息中的动态目标地址
	targetHost := mapping.TargetHost
	targetPort := mapping.TargetPort
	if mapping.Protocol == "socks5" && msg.TargetHost != "" {
		targetHost = msg.TargetHost
		targetPort = msg.TargetPort
	}

	cmdBody := map[string]interface{}{
		"tunnel_id":          msg.TunnelID,
		"mapping_id":         msg.MappingID,
		"secret_key":         mapping.SecretKey,
		"target_host":        targetHost,
		"target_port":        targetPort,
		"protocol":           string(mapping.Protocol),
		"enable_compression": mapping.Config.EnableCompression,
		"compression_level":  mapping.Config.CompressionLevel,
		"enable_encryption":  mapping.Config.EnableEncryption,
		"encryption_method":  mapping.Config.EncryptionMethod,
		"encryption_key":     mapping.Config.EncryptionKey,
		"bandwidth_limit":    mapping.Config.BandwidthLimit,
	}

	cmdBodyJSON, err := json.Marshal(cmdBody)
	if err != nil {
		return
	}

	// 通过控制连接发送命令
	cmd := &packet.CommandPacket{
		CommandType: packet.TunnelOpenRequestCmd,
		CommandBody: string(cmdBodyJSON),
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	// 异步发送（避免阻塞）
	go func() {
		// 设置超时（在 goroutine 内部创建 context，避免被外部 defer 取消）
		ctx, cancel := context.WithTimeout(s.Ctx(), 5*time.Second)
		defer cancel()

		// 使用 channel 来等待发送完成或超时
		done := make(chan error, 1)
		go func() {
			_, err := targetConn.Stream.WritePacket(pkt, true, 0)
			done <- err
		}()

		select {
		case <-ctx.Done():
			corelog.Warnf("handleTunnelOpenBroadcast: timeout sending TunnelOpenRequest to client %d", msg.TargetClientID)
		case err := <-done:
			if err != nil {
				corelog.Errorf("handleTunnelOpenBroadcast: failed to send TunnelOpenRequest to client %d: %v", msg.TargetClientID, err)
			} else {
				corelog.Infof("handleTunnelOpenBroadcast: sent TunnelOpenRequest to client %d for tunnel %s", msg.TargetClientID, msg.TunnelID)
			}
		}
	}()
}
