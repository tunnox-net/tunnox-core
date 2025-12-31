package broker

import (
	"context"

	coreerrors "tunnox-core/internal/core/errors"
)

// BrokerType 消息代理类型
type BrokerType string

const (
	BrokerTypeMemory BrokerType = "memory"
	BrokerTypeRedis  BrokerType = "redis"
	BrokerTypeNATS   BrokerType = "nats"
)

// NATSBrokerConfig NATS 消息代理配置（预留扩展）
type NATSBrokerConfig struct {
	// Servers NATS 服务器地址列表
	Servers []string `yaml:"servers" json:"servers"`
	// ClusterID NATS 集群 ID（用于 NATS Streaming）
	ClusterID string `yaml:"cluster_id" json:"cluster_id"`
	// ClientID 客户端 ID
	ClientID string `yaml:"client_id" json:"client_id"`
	// Token 认证令牌
	Token string `yaml:"token" json:"token"`
}

// BrokerConfig 消息代理配置
type BrokerConfig struct {
	Type   BrokerType // 类型：memory / redis / nats
	NodeID string     // 节点ID

	// Redis 配置
	Redis *RedisBrokerConfig

	// NATS 配置（预留扩展）
	NATS *NATSBrokerConfig
}

// NewMessageBroker 创建消息代理
func NewMessageBroker(ctx context.Context, config *BrokerConfig) (MessageBroker, error) {
	if config == nil {
		return nil, coreerrors.New(coreerrors.CodeNetworkError, "broker config is required")
	}

	switch config.Type {
	case BrokerTypeMemory:
		return NewMemoryBroker(ctx, config.NodeID), nil

	case BrokerTypeRedis:
		if config.Redis == nil {
			return nil, coreerrors.New(coreerrors.CodeNetworkError, "redis config is required for redis broker")
		}
		return NewRedisBroker(ctx, config.Redis, config.NodeID)

	case BrokerTypeNATS:
		// 预留：可在此实现 NATS Broker
		return nil, coreerrors.New(coreerrors.CodeNetworkError, "NATS broker not implemented yet")

	default:
		return nil, coreerrors.New(coreerrors.CodeNetworkError, "unsupported broker type: "+string(config.Type))
	}
}

// DefaultBrokerConfig 默认配置（单节点内存模式）
func DefaultBrokerConfig(nodeID string) *BrokerConfig {
	return &BrokerConfig{
		Type:   BrokerTypeMemory,
		NodeID: nodeID,
	}
}
