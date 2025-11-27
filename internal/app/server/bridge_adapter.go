package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/broker"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

// BridgeAdapter 适配器，通过MessageBroker实现跨服务器隧道转发
type BridgeAdapter struct {
	messageBroker broker.MessageBroker
	nodeID        string
}

// NewBridgeAdapter 创建BridgeAdapter（不依赖BridgeManager，直接使用MessageBroker）
func NewBridgeAdapter(messageBroker broker.MessageBroker, nodeID string) *BridgeAdapter {
	if messageBroker == nil {
		utils.Warn("MessageBroker is nil in BridgeAdapter")
	}
	return &BridgeAdapter{
		messageBroker: messageBroker,
		nodeID:        nodeID,
	}
}

// BroadcastTunnelOpen 广播隧道打开请求到其他节点
func (a *BridgeAdapter) BroadcastTunnelOpen(req *packet.TunnelOpenRequest, targetClientID int64) error {
	if a.messageBroker == nil {
		return fmt.Errorf("message broker not initialized")
	}

	// 构造广播消息
	message := map[string]interface{}{
		"type":             "tunnel_open",
		"tunnel_id":        req.TunnelID,
		"mapping_id":       req.MappingID,
		"secret_key":       req.SecretKey,
		"target_client_id": targetClientID,
		"source_node_id":   a.nodeID,
		"timestamp":        time.Now().Unix(),
	}

	messageJSON, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel open message: %w", err)
	}

	// ✅ 通过MessageBroker广播到所有节点
	// 使用统一的topic，所有节点都订阅这个topic
	topic := "tunnox.tunnel_open"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := a.messageBroker.Publish(ctx, topic, messageJSON); err != nil {
		return fmt.Errorf("failed to publish tunnel open message: %w", err)
	}

	utils.Infof("BridgeAdapter: ✅ broadcasted TunnelOpen for client %d, tunnel %s to topic %s", 
		targetClientID, req.TunnelID, topic)
	return nil
}

// Subscribe 订阅消息主题
func (a *BridgeAdapter) Subscribe(ctx context.Context, topicPattern string) (<-chan *session.BroadcastMessage, error) {
	if a.messageBroker == nil {
		return nil, fmt.Errorf("message broker not initialized")
	}

	// 订阅MessageBroker
	// 注意：需要将topicPattern转换为broker的topic格式
	// 例如: "tunnox.client.*.tunnel_open" 需要订阅所有 "tunnox.client.{id}.tunnel_open"
	
	// 由于当前broker可能不支持通配符订阅，我们订阅一个通用主题
	// 更好的方案是扩展broker支持pattern matching
	
	msgChan := make(chan *session.BroadcastMessage, 100)
	
	// 启动订阅处理goroutine
	go func() {
		defer close(msgChan)
		
		// 订阅通用的TunnelOpen主题
		brokerChan, err := a.messageBroker.Subscribe(ctx, "tunnox.tunnel_open")
		if err != nil {
			utils.Errorf("BridgeAdapter: failed to subscribe to tunnel_open: %v", err)
			return
		}
		
		utils.Infof("BridgeAdapter: ✅ subscribed to tunnox.tunnel_open for cross-server forwarding")
		
		for {
			select {
			case msg, ok := <-brokerChan:
				if !ok {
					utils.Infof("BridgeAdapter: subscription channel closed")
					return
				}
				
				// 转换为BroadcastMessage
				broadcastMsg := &session.BroadcastMessage{
					Topic:   msg.Topic,
					Payload: msg.Payload,
				}
				
				select {
				case msgChan <- broadcastMsg:
				case <-ctx.Done():
					return
				}
				
			case <-ctx.Done():
				utils.Infof("BridgeAdapter: subscription context cancelled")
				return
			}
		}
	}()
	
	return msgChan, nil
}

// PublishMessage 发布消息到指定主题
func (a *BridgeAdapter) PublishMessage(ctx context.Context, topic string, payload []byte) error {
	if a.messageBroker == nil {
		return fmt.Errorf("message broker not initialized")
	}

	if err := a.messageBroker.Publish(ctx, topic, payload); err != nil {
		return fmt.Errorf("failed to publish to topic %s: %w", topic, err)
	}

	utils.Debugf("BridgeAdapter: published message to topic %s (%d bytes)", topic, len(payload))
	return nil
}

