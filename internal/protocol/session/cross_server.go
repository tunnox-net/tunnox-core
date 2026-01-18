package session

import (
	"context"
	"encoding/json"
	"time"

	"tunnox-core/internal/broker"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
)

// ============================================================================
// 跨服务器 Tunnel 处理
// ============================================================================

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
	TargetClientID int64  `json:"client_id"`
	SourceNodeID   string `json:"source_node_id"`
	Timestamp      int64  `json:"timestamp"`
	TargetHost     string `json:"target_host,omitempty"`
	TargetPort     int    `json:"target_port,omitempty"`
	TargetNetwork  string `json:"target_network,omitempty"`
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
	// 支持 "socks5" 和 "socks" 两种写法
	isSocks5 := mapping.Protocol == "socks5" || mapping.Protocol == "socks"
	if isSocks5 && msg.TargetHost != "" {
		targetHost = msg.TargetHost
		targetPort = msg.TargetPort
	}

	cmdBody := &packet.TunnelOpenRequestExtended{
		TunnelOpenRequest: packet.TunnelOpenRequest{
			TunnelID:      msg.TunnelID,
			MappingID:     msg.MappingID,
			SecretKey:     mapping.SecretKey,
			TargetHost:    targetHost,
			TargetPort:    targetPort,
			TargetNetwork: msg.TargetNetwork,
		},
		Protocol:          string(mapping.Protocol),
		EnableCompression: mapping.Config.EnableCompression,
		CompressionLevel:  mapping.Config.CompressionLevel,
		EnableEncryption:  mapping.Config.EnableEncryption,
		EncryptionMethod:  mapping.Config.EncryptionMethod,
		EncryptionKey:     mapping.Config.EncryptionKey,
		BandwidthLimit:    mapping.Config.BandwidthLimit,
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
