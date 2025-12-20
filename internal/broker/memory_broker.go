package broker

import (
	"context"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"
)

// MemoryBroker 内存消息代理（单节点，无持久化）
type MemoryBroker struct {
	*dispose.ServiceBase
	subscribers map[string][]chan *Message // topic -> []channel
	mu          sync.RWMutex
	nodeID      string
	closed      bool
}

// NewMemoryBroker 创建内存消息代理
func NewMemoryBroker(parentCtx context.Context, nodeID string) *MemoryBroker {
	broker := &MemoryBroker{
		ServiceBase: dispose.NewService("MemoryBroker", parentCtx),
		subscribers: make(map[string][]chan *Message),
		nodeID:      nodeID,
		closed:      false,
	}

	corelog.Infof("MemoryBroker initialized for node: %s", nodeID)
	return broker
}

// Publish 发布消息到指定主题
func (m *MemoryBroker) Publish(ctx context.Context, topic string, message []byte) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("broker is closed")
	}

	subscribers, exists := m.subscribers[topic]
	if !exists || len(subscribers) == 0 {
		// 没有订阅者，消息丢弃（符合 Pub/Sub 语义）
		corelog.Debugf("MemoryBroker: no subscribers for topic %s, message dropped", topic)
		return nil
	}

	msg := &Message{
		Topic:     topic,
		Payload:   message,
		Timestamp: time.Now(),
		NodeID:    m.nodeID,
	}

	// 向所有订阅者发送消息
	sentCount := 0
	for _, ch := range subscribers {
		select {
		case ch <- msg:
			sentCount++
		case <-ctx.Done():
			return ctx.Err()
		default:
			// 订阅者通道满，跳过（避免阻塞）
			corelog.Warnf("MemoryBroker: subscriber channel full for topic %s, skipping", topic)
		}
	}

	corelog.Debugf("MemoryBroker: published message to topic %s, sent to %d/%d subscribers",
		topic, sentCount, len(subscribers))

	return nil
}

// Subscribe 订阅主题，返回消息通道
func (m *MemoryBroker) Subscribe(ctx context.Context, topic string) (<-chan *Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, fmt.Errorf("broker is closed")
	}

	// 创建带缓冲的消息通道（避免阻塞）
	msgChan := make(chan *Message, 100)

	// 添加到订阅者列表
	m.subscribers[topic] = append(m.subscribers[topic], msgChan)

	corelog.Infof("MemoryBroker: new subscriber for topic %s (total: %d)",
		topic, len(m.subscribers[topic]))

	return msgChan, nil
}

// Unsubscribe 取消订阅
func (m *MemoryBroker) Unsubscribe(ctx context.Context, topic string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("broker is closed")
	}

	subscribers, exists := m.subscribers[topic]
	if !exists || len(subscribers) == 0 {
		return fmt.Errorf("no subscribers for topic: %s", topic)
	}

	// 关闭所有订阅者通道
	for _, ch := range subscribers {
		close(ch)
	}

	// 删除主题
	delete(m.subscribers, topic)

	corelog.Infof("MemoryBroker: unsubscribed from topic %s", topic)

	return nil
}

// Ping 检查内存消息代理状态（内存 broker 总是健康的）
func (m *MemoryBroker) Ping(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return fmt.Errorf("broker is closed")
	}

	return nil
}

// Close 关闭消息代理
func (m *MemoryBroker) Close() error {
	m.mu.Lock()

	if m.closed {
		m.mu.Unlock()
		return nil
	}

	m.closed = true

	// 关闭所有订阅者通道
	for topic, subscribers := range m.subscribers {
		for _, ch := range subscribers {
			close(ch)
		}
		corelog.Debugf("MemoryBroker: closed %d subscribers for topic %s", len(subscribers), topic)
	}

	// 清空订阅者
	m.subscribers = make(map[string][]chan *Message)
	m.mu.Unlock()

	corelog.Infof("MemoryBroker closed for node: %s", m.nodeID)

	// 调用基类 Close
	return m.ServiceBase.Close()
}

// GetSubscriberCount 获取订阅者数量（用于测试）
func (m *MemoryBroker) GetSubscriberCount(topic string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subscribers, exists := m.subscribers[topic]
	if !exists {
		return 0
	}
	return len(subscribers)
}
