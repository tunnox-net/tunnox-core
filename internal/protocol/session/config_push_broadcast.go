package session

import (
	"context"
	"encoding/json"
	"time"

	"tunnox-core/internal/broker"
	"tunnox-core/internal/packet"
)

// startConfigPushBroadcastSubscription 启动配置推送广播订阅
func (s *SessionManager) startConfigPushBroadcastSubscription() {
	if s.bridgeManager == nil {
		return
	}

	// 订阅配置推送广播主题
	msgChan, err := s.bridgeManager.Subscribe(s.Ctx(), broker.TopicConfigPush)
	if err != nil {
		return
	}

	// 启动消息处理循环
	go s.processConfigPushBroadcasts(msgChan)
}

// processConfigPushBroadcasts 处理配置推送广播消息
func (s *SessionManager) processConfigPushBroadcasts(msgChan <-chan *BroadcastMessage) {
	for {
		select {
		case <-s.Ctx().Done():
			return

		case msg, ok := <-msgChan:
			if !ok {
				return
			}

			// 解析消息
			var pushMsg broker.ConfigPushMessage
			if err := json.Unmarshal(msg.Payload, &pushMsg); err != nil {
				continue
			}

			// 处理配置推送
			s.handleConfigPushBroadcast(&pushMsg)
		}
	}
}

// handleConfigPushBroadcast 处理配置推送广播
func (s *SessionManager) handleConfigPushBroadcast(msg *broker.ConfigPushMessage) {
	// 检查目标客户端是否在本节点
	targetConn := s.GetControlConnectionByClientID(msg.ClientID)
	if targetConn == nil {
		return
	}

	// 构造ConfigSet命令
	cmd := &packet.CommandPacket{
		CommandType: packet.ConfigSet,
		CommandBody: msg.ConfigBody,
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	// 推送配置
	go func() {
		select {
		case <-s.Ctx().Done():
			return
		default:
			// 使用带超时的 context，避免阻塞过久
			ctx, cancel := context.WithTimeout(s.Ctx(), 5*time.Second)
			defer cancel()

			select {
			case <-ctx.Done():
			default:
				targetConn.Stream.WritePacket(pkt, true, 0)
			}
		}
	}()
}

// BroadcastConfigPush 广播配置推送到集群（供API层调用）
func (s *SessionManager) BroadcastConfigPush(clientID int64, configBody string) error {
	if s.bridgeManager == nil {
		return nil // 单节点模式，不需要广播
	}

	// 构造配置推送消息
	message := broker.ConfigPushMessage{
		ClientID:   clientID,
		ConfigBody: configBody,
		Timestamp:  time.Now().Unix(),
	}

	messageBytes, err := json.Marshal(&message)
	if err != nil {
		return err
	}

	// 通过BridgeManager发布到集群
	ctx, cancel := context.WithTimeout(s.Ctx(), 3*time.Second)
	defer cancel()

	return s.bridgeManager.PublishMessage(ctx, broker.TopicConfigPush, messageBytes)
}
