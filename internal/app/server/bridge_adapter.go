package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"tunnox-core/internal/broker"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/protocol/session"
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
		corelog.Warn("MessageBroker is nil in BridgeAdapter")
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
		return fmt.Errorf("message broker not initialized")
	}

	// 构造广播消息（包含 SOCKS5 动态目标地址和 MappingID）
	message := broker.TunnelOpenMessage{
		ClientID:   targetClientID, // 目标客户端ID
		TunnelID:   req.TunnelID,
		MappingID:  req.MappingID,  // 映射ID
		TargetHost: req.TargetHost, // SOCKS5 动态目标地址
		TargetPort: req.TargetPort, // SOCKS5 动态目标端口
		Timestamp:  time.Now().Unix(),
	}

	messageJSON, err := json.Marshal(&message)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel open message: %w", err)
	}

	// ✅ 通过MessageBroker广播到所有节点
	// 使用 BridgeAdapter 的 context 作为父 context，确保能接收退出信号
	ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()

	if err := a.messageBroker.Publish(ctx, broker.TopicTunnelOpen, messageJSON); err != nil {
		return fmt.Errorf("failed to publish tunnel open message: %w", err)
	}

	return nil
}

// Subscribe 订阅消息主题
func (a *BridgeAdapter) Subscribe(ctx context.Context, topicPattern string) (<-chan *session.BroadcastMessage, error) {
	if a.messageBroker == nil {
		return nil, fmt.Errorf("message broker not initialized")
	}

	msgChan := make(chan *session.BroadcastMessage, 100)

	// 启动订阅处理goroutine
	go func() {
		defer close(msgChan)

		// 🔥 FIX: 使用传入的topicPattern，而不是硬编码TopicTunnelOpen
		brokerChan, err := a.messageBroker.Subscribe(ctx, topicPattern)
		if err != nil {
			corelog.Errorf("BridgeAdapter: failed to subscribe to tunnel_open: %v", err)
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
		return fmt.Errorf("message broker not initialized")
	}

	if err := a.messageBroker.Publish(ctx, topic, payload); err != nil {
		return fmt.Errorf("failed to publish to topic %s: %w", topic, err)
	}

	return nil
}

// GetNodeID 获取当前节点ID
func (a *BridgeAdapter) GetNodeID() string {
	return a.nodeID
}

// NotifyTunnelReady 广播隧道就绪通知
func (a *BridgeAdapter) NotifyTunnelReady(ctx context.Context, tunnelID, sourceNodeID string) error {
	if a.messageBroker == nil {
		return fmt.Errorf("message broker not initialized")
	}

	msg := broker.TunnelReadyMessage{
		TunnelID:     tunnelID,
		SourceNodeID: sourceNodeID,
		Timestamp:    time.Now().Unix(),
	}

	msgJSON, err := json.Marshal(&msg)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel ready message: %w", err)
	}

	if err := a.messageBroker.Publish(ctx, broker.TopicTunnelReady, msgJSON); err != nil {
		return fmt.Errorf("failed to publish tunnel ready message: %w", err)
	}

	corelog.Debugf("BridgeAdapter: notified tunnel ready - tunnelID=%s, sourceNodeID=%s", tunnelID, sourceNodeID)
	return nil
}

// WaitForTunnelReady 等待隧道就绪通知
// 通过订阅 TopicTunnelReady 主题，等待匹配的 tunnelID
func (a *BridgeAdapter) WaitForTunnelReady(ctx context.Context, tunnelID string) (string, error) {
	if a.messageBroker == nil {
		return "", fmt.Errorf("message broker not initialized")
	}

	// 订阅隧道就绪主题
	brokerChan, err := a.messageBroker.Subscribe(ctx, broker.TopicTunnelReady)
	if err != nil {
		return "", fmt.Errorf("failed to subscribe to tunnel ready: %w", err)
	}

	// 确保退出时取消订阅
	defer func() {
		if unsubErr := a.messageBroker.Unsubscribe(ctx, broker.TopicTunnelReady); unsubErr != nil {
			corelog.Warnf("BridgeAdapter: failed to unsubscribe from tunnel ready: %v", unsubErr)
		}
	}()

	corelog.Debugf("BridgeAdapter: waiting for tunnel ready - tunnelID=%s", tunnelID)

	for {
		select {
		case msg, ok := <-brokerChan:
			if !ok {
				return "", fmt.Errorf("tunnel ready channel closed")
			}

			// 解析消息
			var readyMsg broker.TunnelReadyMessage
			if err := json.Unmarshal(msg.Payload, &readyMsg); err != nil {
				corelog.Warnf("BridgeAdapter: failed to unmarshal tunnel ready message: %v", err)
				continue
			}

			// 检查是否是我们等待的隧道
			if readyMsg.TunnelID == tunnelID {
				corelog.Infof("BridgeAdapter: received tunnel ready - tunnelID=%s, sourceNodeID=%s",
					tunnelID, readyMsg.SourceNodeID)
				return readyMsg.SourceNodeID, nil
			}

			// 不是我们的隧道，继续等待
			corelog.Debugf("BridgeAdapter: received tunnel ready for different tunnel - got=%s, want=%s",
				readyMsg.TunnelID, tunnelID)

		case <-ctx.Done():
			return "", fmt.Errorf("timeout waiting for tunnel ready: %w", ctx.Err())
		}
	}
}
