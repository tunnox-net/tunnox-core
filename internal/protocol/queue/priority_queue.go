// Package queue 提供协议层使用的数据结构
// 该包无外部依赖，可被 client 和 server 共用
package queue

import (
	"sync"
	"tunnox-core/internal/packet"
)

// PacketPriority 数据包优先级
type PacketPriority int

const (
	PriorityHeartbeat PacketPriority = iota // 心跳包：最低优先级
	PriorityNormal                          // 普通数据包：正常优先级
	PriorityCommand                         // 命令包：高优先级
)

// PriorityPacket 带优先级的数据包
type PriorityPacket struct {
	Data     []byte
	Priority PacketPriority
}

// PriorityQueue 优先级队列（非阻塞，线程安全）
type PriorityQueue struct {
	mu            sync.Mutex
	commandPkts   []PriorityPacket // 高优先级：命令包
	normalPkts    []PriorityPacket // 正常优先级：普通数据包
	heartbeatPkts []PriorityPacket // 低优先级：心跳包
	maxHeartbeats int              // 心跳包最大缓存数量（超过则丢弃）
}

// NewPriorityQueue 创建优先级队列
func NewPriorityQueue(maxHeartbeats int) *PriorityQueue {
	if maxHeartbeats <= 0 {
		maxHeartbeats = 3 // 默认最多缓存3个心跳包
	}
	return &PriorityQueue{
		commandPkts:   make([]PriorityPacket, 0),
		normalPkts:    make([]PriorityPacket, 0),
		heartbeatPkts: make([]PriorityPacket, 0),
		maxHeartbeats: maxHeartbeats,
	}
}

// Push 添加数据包到队列（非阻塞）
func (q *PriorityQueue) Push(data []byte) {
	if len(data) == 0 {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// 判断数据包类型和优先级
	priority := q.determinePriority(data)
	pkt := PriorityPacket{
		Data:     data,
		Priority: priority,
	}

	switch priority {
	case PriorityCommand:
		// 命令包：直接添加到命令队列
		q.commandPkts = append(q.commandPkts, pkt)
	case PriorityNormal:
		// 普通数据包：添加到普通队列
		q.normalPkts = append(q.normalPkts, pkt)
	case PriorityHeartbeat:
		// 心跳包：如果已有太多心跳包，丢弃旧的
		if len(q.heartbeatPkts) >= q.maxHeartbeats {
			// 丢弃最旧的心跳包
			q.heartbeatPkts = q.heartbeatPkts[1:]
		}
		q.heartbeatPkts = append(q.heartbeatPkts, pkt)
	}
}

// Pop 从队列中取出最高优先级的数据包（非阻塞）
func (q *PriorityQueue) Pop() ([]byte, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// 优先级顺序：命令包 > 普通数据包 > 心跳包
	if len(q.commandPkts) > 0 {
		pkt := q.commandPkts[0]
		q.commandPkts = q.commandPkts[1:]
		return pkt.Data, true
	}

	if len(q.normalPkts) > 0 {
		pkt := q.normalPkts[0]
		q.normalPkts = q.normalPkts[1:]
		return pkt.Data, true
	}

	if len(q.heartbeatPkts) > 0 {
		pkt := q.heartbeatPkts[0]
		q.heartbeatPkts = q.heartbeatPkts[1:]
		return pkt.Data, true
	}

	return nil, false
}

// Len 返回队列总长度
func (q *PriorityQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.commandPkts) + len(q.normalPkts) + len(q.heartbeatPkts)
}

// determinePriority 确定数据包优先级
func (q *PriorityQueue) determinePriority(data []byte) PacketPriority {
	if len(data) == 0 {
		return PriorityHeartbeat
	}

	// 检查是否是心跳包
	packetType := packet.Type(data[0])
	if packetType.IsHeartbeat() {
		return PriorityHeartbeat
	}

	// 检查是否是命令包（JsonCommand）
	baseType := packetType & 0x3F
	if baseType == packet.JsonCommand {
		return PriorityCommand
	}

	// 其他数据包为普通优先级
	return PriorityNormal
}
