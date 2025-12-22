package session

import (
	"tunnox-core/internal/protocol/queue"
)

// 类型别名，保持向后兼容
// 新代码应直接使用 tunnox-core/internal/protocol/queue 包
type (
	PacketPriority = queue.PacketPriority
	PriorityPacket = queue.PriorityPacket
	PriorityQueue  = queue.PriorityQueue
)

// 常量别名
const (
	PriorityHeartbeat = queue.PriorityHeartbeat
	PriorityNormal    = queue.PriorityNormal
	PriorityCommand   = queue.PriorityCommand
)

// NewPriorityQueue 创建优先级队列（向后兼容）
func NewPriorityQueue(maxHeartbeats int) *PriorityQueue {
	return queue.NewPriorityQueue(maxHeartbeats)
}
