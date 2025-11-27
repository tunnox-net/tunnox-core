package session

import (
	"context"
	"encoding/json"
	"time"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
)

// ConfigPushMessage 配置推送广播消息
type ConfigPushMessage struct {
	ClientID   int64  `json:"client_id"`
	ConfigBody string `json:"config_body"`
	Timestamp  int64  `json:"timestamp"`
}

// startConfigPushBroadcastSubscription 启动配置推送广播订阅
func (s *SessionManager) startConfigPushBroadcastSubscription() {
	if s.bridgeManager == nil {
		utils.Debugf("SessionManager: BridgeManager not configured, skipping config push subscription")
		return
	}

	utils.Infof("SessionManager: starting ConfigPush broadcast subscription for cross-node config delivery")

	// 订阅配置推送广播主题
	topic := "config.push"
	msgChan, err := s.bridgeManager.Subscribe(s.Ctx(), topic)
	if err != nil {
		utils.Errorf("SessionManager: failed to subscribe to %s: %v", topic, err)
		return
	}

	utils.Infof("SessionManager: ✅ subscribed to %s for cross-node config push", topic)

	// 启动消息处理循环
	go s.processConfigPushBroadcasts(msgChan)
}

// processConfigPushBroadcasts 处理配置推送广播消息
func (s *SessionManager) processConfigPushBroadcasts(msgChan <-chan *BroadcastMessage) {
	utils.Infof("SessionManager: config push broadcast processor started")

	for {
		select {
		case <-s.Ctx().Done():
			utils.Infof("SessionManager: config push broadcast processor stopped")
			return

		case msg, ok := <-msgChan:
			if !ok {
				utils.Warnf("SessionManager: config push broadcast channel closed")
				return
			}

			// 解析消息
			var pushMsg ConfigPushMessage
			if err := json.Unmarshal(msg.Payload, &pushMsg); err != nil {
				utils.Errorf("SessionManager: failed to unmarshal config push message: %v", err)
				continue
			}

			// 处理配置推送
			s.handleConfigPushBroadcast(&pushMsg)
		}
	}
}

// handleConfigPushBroadcast 处理配置推送广播
func (s *SessionManager) handleConfigPushBroadcast(msg *ConfigPushMessage) {
	utils.Infof("SessionManager: received config push broadcast for client %d", msg.ClientID)

	// 检查目标客户端是否在本节点
	targetConn := s.GetControlConnectionByClientID(msg.ClientID)
	if targetConn == nil {
		utils.Debugf("SessionManager: client %d not on this node, ignoring broadcast", msg.ClientID)
		return
	}

	utils.Infof("SessionManager: ✅ client %d found locally, pushing config", msg.ClientID)

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
			utils.Errorf("SessionManager: config push to client %d timed out", msg.ClientID)
		default:
			if _, err := targetConn.Stream.WritePacket(pkt, false, 0); err != nil {
				utils.Errorf("SessionManager: failed to push config to client %d: %v", msg.ClientID, err)
			} else {
				utils.Infof("SessionManager: ✅ config pushed successfully to client %d via broadcast", msg.ClientID)
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
	message := ConfigPushMessage{
		ClientID:   clientID,
		ConfigBody: configBody,
		Timestamp:  time.Now().Unix(),
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// 通过BridgeManager发布到集群
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := s.bridgeManager.PublishMessage(ctx, "config.push", messageBytes); err != nil {
		return err
	}

	utils.Infof("SessionManager: ✅ config push broadcast sent for client %d", clientID)
	return nil
}

