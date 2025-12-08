package server

import (
	"context"
	"encoding/json"
	"time"
	"tunnox-core/internal/broker"
	coreErrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"
)

// BridgeAdapter 适配器，通过MessageBroker实现跨服务器隧道转发
type BridgeAdapter struct {
	messageBroker broker.MessageBroker
	nodeID        string
	ctx           context.Context // context 用于接收退出信号
}

// NewBridgeAdapter 创建BridgeAdapter（不依赖BridgeManager，直接使用MessageBroker）
func NewBridgeAdapter(ctx context.Context, messageBroker broker.MessageBroker, nodeID string) *BridgeAdapter {
	if messageBroker == nil {
		utils.Warn("MessageBroker is nil in BridgeAdapter")
	}
	return &BridgeAdapter{
		messageBroker: messageBroker,
		nodeID:        nodeID,
		ctx:           ctx,
	}
}

// BroadcastTunnelOpen 广播隧道打开请求到其他节点
func (a *BridgeAdapter) BroadcastTunnelOpen(req *packet.TunnelOpenRequest, targetClientID int64) error {
	if a.messageBroker == nil {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "message broker not initialized")
	}

	// 构造广播消息
	message := broker.TunnelOpenMessage{
		ClientID:   targetClientID,
		TunnelID:   req.TunnelID,
		TargetHost: "", // 这些字段在TunnelOpen请求中没有，可能需要从其他地方获取
		TargetPort: 0,
		Timestamp:  time.Now().Unix(),
	}

	messageJSON, err := json.Marshal(&message)
	if err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypePermanent, "failed to marshal tunnel open message")
	}

	// ✅ 通过MessageBroker广播到所有节点
	// 使用 BridgeAdapter 的 context 作为父 context，确保能接收退出信号
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()

	if err := a.messageBroker.Publish(ctx, broker.TopicTunnelOpen, messageJSON); err != nil {
		return coreErrors.Wrap(err, coreErrors.ErrorTypeNetwork, "failed to publish tunnel open message")
	}

	return nil
}

// Subscribe 订阅消息主题
func (a *BridgeAdapter) Subscribe(ctx context.Context, topicPattern string) (<-chan *session.BroadcastMessage, error) {
	if a.messageBroker == nil {
		return nil, coreErrors.New(coreErrors.ErrorTypePermanent, "message broker not initialized")
	}

	
	msgChan := make(chan *session.BroadcastMessage, 100)
	
	// 启动订阅处理goroutine
	go func() {
		defer close(msgChan)
		
		// 🔥 FIX: 使用传入的topicPattern，而不是硬编码TopicTunnelOpen
		brokerChan, err := a.messageBroker.Subscribe(ctx, topicPattern)
		if err != nil {
			utils.LogErrorf(err, "BridgeAdapter: failed to subscribe to tunnel_open")
			return
		}
		
		
		for {
			select {
			case msg, ok := <-brokerChan:
				if !ok {
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
				return
			}
		}
	}()
	
	return msgChan, nil
}

// PublishMessage 发布消息到指定主题
func (a *BridgeAdapter) PublishMessage(ctx context.Context, topic string, payload []byte) error {
	if a.messageBroker == nil {
		return coreErrors.New(coreErrors.ErrorTypePermanent, "message broker not initialized")
	}

	if err := a.messageBroker.Publish(ctx, topic, payload); err != nil {
		return coreErrors.Wrapf(err, coreErrors.ErrorTypeNetwork, "failed to publish to topic %s", topic)
	}

	return nil
}

