package session

import (
	"context"
	"encoding/json"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
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
		utils.Debugf("SessionManager: BridgeManager not configured, skipping tunnel broadcast subscription")
		return
	}

	utils.Infof("SessionManager: starting TunnelOpen broadcast subscription for cross-server forwarding")

	// 订阅TunnelOpen广播
	topic := "tunnox.tunnel_open"

	msgChan, err := s.bridgeManager.Subscribe(s.Ctx(), topic)
	if err != nil {
		utils.Errorf("SessionManager: failed to subscribe to %s: %v", topic, err)
		return
	}

	utils.Infof("SessionManager: ✅ subscribed to %s for cross-server tunnel forwarding", topic)

	// 启动消息处理循环
	go s.processTunnelOpenBroadcasts(msgChan)
}

// processTunnelOpenBroadcasts 处理TunnelOpen广播消息
func (s *SessionManager) processTunnelOpenBroadcasts(msgChan <-chan *BroadcastMessage) {
	utils.Infof("SessionManager: TunnelOpen broadcast processor started")

	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				utils.Infof("SessionManager: TunnelOpen broadcast channel closed")
				return
			}

			// 解析消息
			var broadcastMsg TunnelOpenBroadcastMessage
			if err := json.Unmarshal(msg.Payload, &broadcastMsg); err != nil {
				utils.Errorf("SessionManager: failed to unmarshal TunnelOpen broadcast: %v", err)
				continue
			}

			// 处理广播
			s.handleTunnelOpenBroadcast(&broadcastMsg)

		case <-s.Ctx().Done():
			utils.Infof("SessionManager: TunnelOpen broadcast processor stopped")
			return
		}
	}
}

// handleTunnelOpenBroadcast 处理收到的TunnelOpen广播
func (s *SessionManager) handleTunnelOpenBroadcast(msg *TunnelOpenBroadcastMessage) {
	utils.Infof("SessionManager: received TunnelOpen broadcast for client %d, tunnel %s", 
		msg.TargetClientID, msg.TunnelID)

	// 检查目标客户端是否在本地
	targetConn := s.GetControlConnectionByClientID(msg.TargetClientID)
	if targetConn == nil {
		utils.Debugf("SessionManager: target client %d not on this node, ignoring broadcast", 
			msg.TargetClientID)
		return
	}

	utils.Infof("SessionManager: ✅ target client %d found locally, sending TunnelOpenRequest", 
		msg.TargetClientID)

	// 获取映射配置
	if s.cloudControl == nil {
		utils.Errorf("SessionManager: CloudControl not configured")
		return
	}

	mappingInterface, err := s.cloudControl.GetPortMapping(msg.MappingID)
	if err != nil {
		utils.Errorf("SessionManager: failed to get mapping %s: %v", msg.MappingID, err)
		return
	}

	mapping, ok := mappingInterface.(interface {
		GetTargetHost() string
		GetTargetPort() int
		GetProtocol() string
		GetConfig() interface {
			GetEnableCompression() bool
			GetCompressionLevel() int
			GetEnableEncryption() bool
			GetEncryptionMethod() string
			GetEncryptionKey() string
			GetBandwidthLimit() int64
		}
	})
	if !ok {
		utils.Errorf("SessionManager: invalid mapping type for %s", msg.MappingID)
		return
	}

	// 构造TunnelOpenRequest命令
	cmdBody := map[string]interface{}{
		"tunnel_id":          msg.TunnelID,
		"mapping_id":         msg.MappingID,
		"secret_key":         msg.SecretKey,
		"target_host":        mapping.GetTargetHost(),
		"target_port":        mapping.GetTargetPort(),
		"protocol":           mapping.GetProtocol(),
		"enable_compression": mapping.GetConfig().GetEnableCompression(),
		"compression_level":  mapping.GetConfig().GetCompressionLevel(),
		"enable_encryption":  mapping.GetConfig().GetEnableEncryption(),
		"encryption_method":  mapping.GetConfig().GetEncryptionMethod(),
		"encryption_key":     mapping.GetConfig().GetEncryptionKey(),
		"bandwidth_limit":    mapping.GetConfig().GetBandwidthLimit(),
	}

	cmdBodyJSON, err := json.Marshal(cmdBody)
	if err != nil {
		utils.Errorf("SessionManager: failed to marshal TunnelOpenRequest: %v", err)
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
			utils.Errorf("SessionManager: sending TunnelOpenRequest to client %d timed out", 
				msg.TargetClientID)
			return
		default:
			if _, err := targetConn.Stream.WritePacket(pkt, false, 0); err != nil {
				utils.Errorf("SessionManager: failed to send TunnelOpenRequest to client %d: %v", 
					msg.TargetClientID, err)
				return
			}
			utils.Infof("SessionManager: ✅ sent TunnelOpenRequest to client %d for tunnel %s (cross-server)", 
				msg.TargetClientID, msg.TunnelID)
		}
	}()
}

