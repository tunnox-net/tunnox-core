package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	corelog "tunnox-core/internal/core/log"

	"github.com/redis/go-redis/v9"
)

// RedisBrokerConfig Redis Broker 配置
type RedisBrokerConfig struct {
	Addrs       []string // Redis 地址列表
	Password    string   // 密码
	DB          int      // 数据库编号
	ClusterMode bool     // 是否集群模式
	PoolSize    int      // 连接池大小
}

// RedisBroker Redis 消息代理（基于 Pub/Sub）
type RedisBroker struct {
	*dispose.ServiceBase
	client      redis.UniversalClient // 支持单机和集群
	pubsub      *redis.PubSub
	subscribers map[string]chan *Message // topic -> channel
	mu          sync.RWMutex
	nodeID      string
	closed      bool
}

// NewRedisBroker 创建 Redis 消息代理
func NewRedisBroker(parentCtx context.Context, config *RedisBrokerConfig, nodeID string) (*RedisBroker, error) {
	if config == nil {
		return nil, fmt.Errorf("redis broker config is required")
	}

	// 设置默认值
	if config.PoolSize <= 0 {
		config.PoolSize = 100
	}

	// 创建 Redis 客户端（支持集群和单机）
	var client redis.UniversalClient
	if config.ClusterMode {
		client = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    config.Addrs,
			Password: config.Password,
			PoolSize: config.PoolSize,
		})
	} else {
		addr := "localhost:6379"
		if len(config.Addrs) > 0 {
			addr = config.Addrs[0]
		}
		client = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: config.Password,
			DB:       config.DB,
			PoolSize: config.PoolSize,
		})
	}

	// 测试连接
	pingCtx, pingCancel := context.WithTimeout(parentCtx, 5*time.Second)
	defer pingCancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	broker := &RedisBroker{
		ServiceBase: dispose.NewService("RedisBroker", parentCtx),
		client:      client,
		subscribers: make(map[string]chan *Message),
		nodeID:      nodeID,
		closed:      false,
	}

	corelog.Infof("RedisBroker initialized for node: %s (cluster_mode: %v)", nodeID, config.ClusterMode)
	return broker, nil
}

// Publish 发布消息到指定主题
func (r *RedisBroker) Publish(ctx context.Context, topic string, message []byte) error {
	if r.closed {
		return fmt.Errorf("broker is closed")
	}

	// 构造完整消息（包含元数据）
	msg := &Message{
		Topic:     topic,
		Payload:   message,
		Timestamp: time.Now(),
		NodeID:    r.nodeID,
	}

	// 序列化消息
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// 发布到 Redis（添加前缀避免冲突）
	channel := fmt.Sprintf("tunnox:%s", topic)
	if err := r.client.Publish(ctx, channel, data).Err(); err != nil {
		corelog.Errorf("RedisBroker: failed to publish to %s: %v", topic, err)
		return fmt.Errorf("failed to publish to Redis: %w", err)
	}

	corelog.Debugf("RedisBroker: published message to topic %s", topic)
	return nil
}

// Subscribe 订阅主题，返回消息通道
func (r *RedisBroker) Subscribe(ctx context.Context, topic string) (<-chan *Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, fmt.Errorf("broker is closed")
	}

	// 检查是否已订阅该主题
	if _, exists := r.subscribers[topic]; exists {
		return nil, fmt.Errorf("already subscribed to topic: %s", topic)
	}

	// 创建消息通道
	msgChan := make(chan *Message, 100)
	r.subscribers[topic] = msgChan

	// 订阅 Redis 频道（首次订阅时创建 PubSub）
	if r.pubsub == nil {
		r.pubsub = r.client.Subscribe(r.Ctx())
	}

	// 订阅 Redis 频道
	channel := fmt.Sprintf("tunnox:%s", topic)
	if err := r.pubsub.Subscribe(r.Ctx(), channel); err != nil {
		delete(r.subscribers, topic)
		close(msgChan)
		return nil, fmt.Errorf("failed to subscribe to Redis: %w", err)
	}

	// 首次订阅时启动接收循环
	if len(r.subscribers) == 1 {
		go r.receiveLoop()
	}

	corelog.Infof("RedisBroker: subscribed to topic %s (total topics: %d)", topic, len(r.subscribers))
	return msgChan, nil
}

// receiveLoop 接收 Redis 消息循环
func (r *RedisBroker) receiveLoop() {
	corelog.Infof("RedisBroker: receive loop started")

	for {
		select {
		case <-r.Ctx().Done():
			corelog.Infof("RedisBroker: receive loop stopped")
			return
		default:
			// 接收消息
			msg, err := r.pubsub.ReceiveMessage(r.Ctx())
			if err != nil {
				if r.closed {
					return
				}
				corelog.Errorf("RedisBroker: failed to receive message: %v", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// 解析消息
			var message Message
			if err := json.Unmarshal([]byte(msg.Payload), &message); err != nil {
				corelog.Errorf("RedisBroker: failed to unmarshal message: %v", err)
				continue
			}

			// 移除 tunnox: 前缀获取原始 topic
			topic := message.Topic

			// 分发到订阅者
			r.mu.RLock()
			ch, exists := r.subscribers[topic]
			r.mu.RUnlock()

			if exists {
				select {
				case ch <- &message:
					corelog.Debugf("RedisBroker: delivered message to topic %s", topic)
				case <-r.Ctx().Done():
					return
				default:
					corelog.Warnf("RedisBroker: subscriber channel full for topic %s, dropping message", topic)
				}
			}
		}
	}
}

// Unsubscribe 取消订阅
func (r *RedisBroker) Unsubscribe(ctx context.Context, topic string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return fmt.Errorf("broker is closed")
	}

	ch, exists := r.subscribers[topic]
	if !exists {
		return fmt.Errorf("not subscribed to topic: %s", topic)
	}

	// 取消 Redis 订阅
	channel := fmt.Sprintf("tunnox:%s", topic)
	if r.pubsub != nil {
		if err := r.pubsub.Unsubscribe(ctx, channel); err != nil {
			corelog.Warnf("RedisBroker: failed to unsubscribe from Redis: %v", err)
		}
	}

	// 关闭消息通道
	close(ch)
	delete(r.subscribers, topic)

	corelog.Infof("RedisBroker: unsubscribed from topic %s", topic)
	return nil
}

// Ping 检查 Redis 连接
func (r *RedisBroker) Ping(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.closed {
		return fmt.Errorf("broker is closed")
	}

	if r.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	return r.client.Ping(ctx).Err()
}

// Close 关闭消息代理
func (r *RedisBroker) Close() error {
	r.mu.Lock()

	if r.closed {
		r.mu.Unlock()
		return nil
	}

	r.closed = true

	// 关闭 PubSub
	if r.pubsub != nil {
		if err := r.pubsub.Close(); err != nil {
			corelog.Warnf("RedisBroker: failed to close pubsub: %v", err)
		}
	}

	// 关闭所有订阅者通道
	for topic, ch := range r.subscribers {
		close(ch)
		corelog.Debugf("RedisBroker: closed subscriber for topic %s", topic)
	}
	r.subscribers = make(map[string]chan *Message)

	// 关闭 Redis 客户端
	if err := r.client.Close(); err != nil {
		corelog.Warnf("RedisBroker: failed to close Redis client: %v", err)
	}

	r.mu.Unlock()

	corelog.Infof("RedisBroker closed for node: %s", r.nodeID)

	// 调用基类 Close
	return r.ServiceBase.Close()
}

// GetSubscriberCount 获取订阅者数量（用于测试）
func (r *RedisBroker) GetSubscriberCount(topic string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, exists := r.subscribers[topic]; !exists {
		return 0
	}
	return 1 // Redis模式下每个topic只有一个本地channel
}
