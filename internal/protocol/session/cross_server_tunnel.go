package session

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"encoding/json"
	"time"

	"tunnox-core/internal/broker"
	"tunnox-core/internal/packet"
)

// TunnelOpenBroadcastMessage 跨服务器TunnelOpen广播消息
type TunnelOpenBroadcastMessage struct {
	Type           string `json:"type"`
	TunnelID       string `json:"tunnel_id"`
	MappingID      string `json:"mapping_id"`
	SecretKey      string `json:"secret_key"`
	TargetClientID int64  `json:"target_client_id"`
	SourceNodeID   string `json:"source_node_id"`
	Timestamp      int64  `json:"timestamp"`
}

// startTunnelOpenBroadcastSubscription 启动TunnelOpen广播订阅
func (s *SessionManager) startTunnelOpenBroadcastSubscription() {
	if s.bridgeManager == nil {
		corelog.Debugf("SessionManager: BridgeManager not configured, skipping tunnel broadcast subscription")
		return
	}

	corelog.Infof("SessionManager: starting TunnelOpen broadcast subscription for cross-server forwarding")

	// 订阅TunnelOpen广播
	msgChan, err := s.bridgeManager.Subscribe(s.Ctx(), broker.TopicTunnelOpen)
	if err != nil {
		corelog.Errorf("SessionManager: failed to subscribe to %s: %v", broker.TopicTunnelOpen, err)
		return
	}

	corelog.Infof("SessionManager: ✅ subscribed to %s for cross-server tunnel forwarding", broker.TopicTunnelOpen)

	// 启动消息处理循环
	go s.processTunnelOpenBroadcasts(msgChan)
}

// processTunnelOpenBroadcasts 处理TunnelOpen广播消息
func (s *SessionManager) processTunnelOpenBroadcasts(msgChan <-chan *BroadcastMessage) {
	corelog.Infof("SessionManager: TunnelOpen broadcast processor started")

	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				corelog.Infof("SessionManager: TunnelOpen broadcast channel closed")
				return
			}

			// 解析消息
			var broadcastMsg TunnelOpenBroadcastMessage
			if err := json.Unmarshal(msg.Payload, &broadcastMsg); err != nil {
				corelog.Errorf("SessionManager: failed to unmarshal TunnelOpen broadcast: %v", err)
				continue
			}

			// 处理广播
			s.handleTunnelOpenBroadcast(&broadcastMsg)

		case <-s.Ctx().Done():
			corelog.Infof("SessionManager: TunnelOpen broadcast processor stopped")
			return
		}
	}
}

// handleTunnelOpenBroadcast 处理收到的TunnelOpen广播
func (s *SessionManager) handleTunnelOpenBroadcast(msg *TunnelOpenBroadcastMessage) {
	corelog.Infof("SessionManager: received TunnelOpen broadcast for client %d, tunnel %s",
		msg.TargetClientID, msg.TunnelID)

	// 检查目标客户端是否在本地
	targetConn := s.GetControlConnectionByClientID(msg.TargetClientID)
	if targetConn == nil {
		corelog.Debugf("SessionManager: target client %d not on this node, ignoring broadcast",
			msg.TargetClientID)
		return
	}

	corelog.Infof("SessionManager: ✅ target client %d found locally, sending TunnelOpenRequest",
		msg.TargetClientID)

	// 获取映射配置
	if s.cloudControl == nil {
		corelog.Errorf("SessionManager: CloudControl not configured")
		return
	}

	// ✅ 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(msg.MappingID)
	if err != nil {
		corelog.Errorf("SessionManager: failed to get mapping %s: %v", msg.MappingID, err)
		return
	}

	// 构造TunnelOpenRequest命令
	cmdBody := map[string]interface{}{
		"tunnel_id":          msg.TunnelID,
		"mapping_id":         msg.MappingID,
		"secret_key":         mapping.SecretKey,
		"target_host":        mapping.TargetHost,
		"target_port":        mapping.TargetPort,
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
		corelog.Errorf("SessionManager: failed to marshal TunnelOpenRequest: %v", err)
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

	// 设置超时
	ctx, cancel := context.WithTimeout(s.Ctx(), 5*time.Second)
	defer cancel()

	// 异步发送（避免阻塞）
	go func() {
		select {
		case <-ctx.Done():
			corelog.Errorf("SessionManager: sending TunnelOpenRequest to client %d timed out",
				msg.TargetClientID)
			return
		default:
			if _, err := targetConn.Stream.WritePacket(pkt, true, 0); err != nil {
				corelog.Errorf("SessionManager: failed to send TunnelOpenRequest to client %d: %v",
					msg.TargetClientID, err)
				return
			}
			corelog.Infof("SessionManager: ✅ sent TunnelOpenRequest to client %d for tunnel %s (cross-server)",
				msg.TargetClientID, msg.TunnelID)
		}
	}()
}
