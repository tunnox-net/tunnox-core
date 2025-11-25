package broker

import (
	"context"
	"time"
)

// MessageBroker 消息代理接口（抽象 MQ 能力）
type MessageBroker interface {
	// Publish 发布消息到指定主题
	Publish(ctx context.Context, topic string, message []byte) error

	// Subscribe 订阅主题，返回消息通道
	Subscribe(ctx context.Context, topic string) (<-chan *Message, error)

	// Unsubscribe 取消订阅
	Unsubscribe(ctx context.Context, topic string) error

	// Close 关闭连接
	Close() error
}

// Message 消息结构
type Message struct {
	Topic     string    // 消息主题
	Payload   []byte    // 消息内容
	Timestamp time.Time // 消息时间戳
	NodeID    string    // 发布者节点ID
}

// Topic 常量定义
const (
	TopicClientOnline   = "client.online"    // 客户端上线
	TopicClientOffline  = "client.offline"   // 客户端下线
	TopicConfigUpdate   = "config.update"    // 配置更新
	TopicMappingCreated = "mapping.created"  // 映射创建
	TopicMappingDeleted = "mapping.deleted"  // 映射删除
	TopicBridgeRequest  = "bridge.request"   // 桥接请求
	TopicBridgeResponse = "bridge.response"  // 桥接响应
	TopicNodeHeartbeat  = "node.heartbeat"   // 节点心跳
	TopicNodeShutdown   = "node.shutdown"    // 节点下线
)

