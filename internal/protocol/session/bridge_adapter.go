package session

import (
	"context"
	"tunnox-core/internal/packet"
)

// BroadcastMessage 广播消息
type BroadcastMessage struct {
	Topic   string
	Payload []byte
}

// BridgeManager 接口（避免循环依赖）
type BridgeManager interface {
	// BroadcastTunnelOpen 广播隧道打开请求到其他节点
	BroadcastTunnelOpen(req *packet.TunnelOpenRequest, targetClientID int64) error
	
	// Subscribe 订阅消息主题（用于接收跨服务器广播）
	Subscribe(ctx context.Context, topicPattern string) (<-chan *BroadcastMessage, error)
}

