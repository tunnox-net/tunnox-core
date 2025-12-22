package session

import (
	"context"
	"tunnox-core/internal/bridge"
	"tunnox-core/internal/packet"
)

// BroadcastMessage 广播消息
type BroadcastMessage struct {
	Topic   string
	Payload []byte
}

// TunnelReadyNotification 隧道就绪通知
type TunnelReadyNotification struct {
	TunnelID     string `json:"tunnel_id"`
	SourceNodeID string `json:"source_node_id"`
}

// BridgeManager 接口（避免循环依赖）
type BridgeManager interface {
	// BroadcastTunnelOpen 广播隧道打开请求到其他节点
	BroadcastTunnelOpen(req *packet.TunnelOpenRequest, targetClientID int64) error

	// Subscribe 订阅消息主题（用于接收跨服务器广播）
	Subscribe(ctx context.Context, topicPattern string) (<-chan *BroadcastMessage, error)

	// PublishMessage 发布消息到指定主题
	PublishMessage(ctx context.Context, topic string, payload []byte) error

	// CreateCrossNodeSession 创建跨节点转发会话
	// 用于当 TargetClient 连接到的节点与 Bridge 所在节点不同时
	CreateCrossNodeSession(ctx context.Context, targetNodeID, targetNodeAddr string, metadata *bridge.SessionMetadata) (*bridge.ForwardSession, error)

	// GetNodeID 获取当前节点ID
	GetNodeID() string

	// NotifyTunnelReady 广播隧道就绪通知
	// 当 Bridge 创建完成后调用，通知其他节点可以进行跨节点转发
	NotifyTunnelReady(ctx context.Context, tunnelID, sourceNodeID string) error

	// WaitForTunnelReady 等待隧道就绪通知
	// 返回源节点ID，或者超时返回错误
	WaitForTunnelReady(ctx context.Context, tunnelID string) (sourceNodeID string, err error)
}
