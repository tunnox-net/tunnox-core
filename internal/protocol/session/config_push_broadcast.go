package session

import (
corelog "tunnox-core/internal/core/log"
	"context"
	"encoding/json"
	"time"
	
	"tunnox-core/internal/broker"
	"tunnox-core/internal/packet"
)

// startConfigPushBroadcastSubscription 启动配置推送广播订阅
func (s *SessionManager) startConfigPushBroadcastSubscription() {
	if s.bridgeManager == nil {
		corelog.Debugf("SessionManager: BridgeManager not configured, skipping config push subscription")
		return
	}

	corelog.Infof("SessionManager: starting ConfigPush broadcast subscription for cross-node config delivery")

	// 订阅配置推送广播主题
	msgChan, err := s.bridgeManager.Subscribe(s.Ctx(), broker.TopicConfigPush)
	if err != nil {
		corelog.Errorf("SessionManager: failed to subscribe to %s: %v", broker.TopicConfigPush, err)
		return
	}

	corelog.Infof("SessionManager: ✅ subscribed to %s for cross-node config push", broker.TopicConfigPush)

	// 启动消息处理循环
	go s.processConfigPushBroadcasts(msgChan)
}

// processConfigPushBroadcasts 处理配置推送广播消息
func (s *SessionManager) processConfigPushBroadcasts(msgChan <-chan *BroadcastMessage) {
	corelog.Infof("SessionManager: config push broadcast processor started")

	for {
		select {
		case <-s.Ctx().Done():
			corelog.Infof("SessionManager: config push broadcast processor stopped")
			return

		case msg, ok := <-msgChan:
			if !ok {
				corelog.Warnf("SessionManager: config push broadcast channel closed")
				return
			}

			// 解析消息
			var pushMsg broker.ConfigPushMessage
			if err := json.Unmarshal(msg.Payload, &pushMsg); err != nil {
				corelog.Errorf("SessionManager: failed to unmarshal config push message: %v", err)
				continue
			}

			// 处理配置推送
			s.handleConfigPushBroadcast(&pushMsg)
		}
	}
}

// handleConfigPushBroadcast 处理配置推送广播
func (s *SessionManager) handleConfigPushBroadcast(msg *broker.ConfigPushMessage) {
	corelog.Infof("SessionManager: received config push broadcast for client %d", msg.ClientID)

	// 检查目标客户端是否在本节点
	targetConn := s.GetControlConnectionByClientID(msg.ClientID)
	corelog.Infof("📨 SessionManager[%s]: Received ConfigPush broadcast for client %d", s.nodeID, msg.ClientID)
	corelog.Infof("🔍 SessionManager[%s]: Checking if client %d is on this node...", s.nodeID, msg.ClientID)
	
	if targetConn == nil {
		corelog.Infof("⏭️  SessionManager[%s]: client %d not on this node, ignoring broadcast", s.nodeID, msg.ClientID)
		return
	}

	corelog.Infof("✅ SessionManager[%s]: client %d FOUND locally! Pushing config...", s.nodeID, msg.ClientID)

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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		select {
		case <-ctx.Done():
			corelog.Errorf("SessionManager: config push to client %d timed out", msg.ClientID)
		default:
			if _, err := targetConn.Stream.WritePacket(pkt, true, 0); err != nil {
				corelog.Errorf("SessionManager: failed to push config to client %d: %v", msg.ClientID, err)
			} else {
				corelog.Infof("SessionManager: ✅ config pushed successfully to client %d via broadcast", msg.ClientID)
			}
		}
	}()
}

// BroadcastConfigPush 广播配置推送到集群（供API层调用）
func (s *SessionManager) BroadcastConfigPush(clientID int64, configBody string) error {
	corelog.Infof("🌐 SessionManager[%s]: BroadcastConfigPush CALLED for client %d", s.nodeID, clientID)
	
	if s.bridgeManager == nil {
		corelog.Warnf("⚠️  SessionManager[%s]: BridgeManager is nil, cannot broadcast (single node mode?)", s.nodeID)
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
		corelog.Errorf("❌ SessionManager[%s]: failed to marshal message: %v", s.nodeID, err)
		return err
	}

	// 通过BridgeManager发布到集群
	// 使用 SessionManager 的 context 作为父 context，确保能接收退出信号
	ctx, cancel := context.WithTimeout(s.Ctx(), 3*time.Second)
	defer cancel()

	corelog.Infof("🌐 SessionManager[%s]: Publishing to topic %s...", s.nodeID, broker.TopicConfigPush)
	if err := s.bridgeManager.PublishMessage(ctx, broker.TopicConfigPush, messageBytes); err != nil {
		corelog.Errorf("❌ SessionManager[%s]: Publish failed: %v", s.nodeID, err)
		return err
	}

	corelog.Infof("✅ SessionManager[%s]: config push broadcast sent for client %d to topic %s", s.nodeID, clientID, broker.TopicConfigPush)
	return nil
}

