package broker

import (
	"context"
	"fmt"
)

// BrokerType 消息代理类型
type BrokerType string

const (
	BrokerTypeMemory BrokerType = "memory"
	BrokerTypeRedis  BrokerType = "redis"
	BrokerTypeNATS   BrokerType = "nats"
)

// BrokerConfig 消息代理配置
type BrokerConfig struct {
	Type   BrokerType // 类型：memory / redis / nats
	NodeID string     // 节点ID

	// Redis 配置
	Redis *RedisBrokerConfig

	// NATS 配置（未来扩展）
	NATS interface{}
}

// NewMessageBroker 创建消息代理
func NewMessageBroker(ctx context.Context, config *BrokerConfig) (MessageBroker, error) {
	if config == nil {
		return nil, fmt.Errorf("broker config is required")
	}

	switch config.Type {
	case BrokerTypeMemory:
		return NewMemoryBroker(ctx, config.NodeID), nil

	case BrokerTypeRedis:
		if config.Redis == nil {
			return nil, fmt.Errorf("redis config is required for redis broker")
		}
		return NewRedisBroker(ctx, config.Redis, config.NodeID)

	case BrokerTypeNATS:
		// 预留：可在此实现 NATS Broker
		return nil, fmt.Errorf("NATS broker not implemented yet")

	default:
		return nil, fmt.Errorf("unsupported broker type: %s", config.Type)
	}
}

// DefaultBrokerConfig 默认配置（单节点内存模式）
func DefaultBrokerConfig(nodeID string) *BrokerConfig {
	return &BrokerConfig{
		Type:   BrokerTypeMemory,
		NodeID: nodeID,
	}
}
