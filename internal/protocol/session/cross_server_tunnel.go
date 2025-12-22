package session

import (
	"context"
	"encoding/json"
	"time"

	"tunnox-core/internal/broker"
	"tunnox-core/internal/packet"
)

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
	// 检查目标客户端是否在本地
	targetConn := s.GetControlConnectionByClientID(msg.TargetClientID)
	if targetConn == nil {
		return
	}

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
		case <-done:
		}
	}()
}
