// Package buffer 提供隧道数据缓冲功能
package buffer

import (
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	"tunnox-core/internal/packet"
)

// ============================================================================
// 接收端重组机制
// ============================================================================

const (
	// DefaultMaxOutOfOrder 默认最大乱序包数
	DefaultMaxOutOfOrder = 100
)

// ReceiveBuffer 隧道接收缓冲区
//
// 功能：
// 1. 缓冲乱序到达的数据包
// 2. 按序列号重组数据
// 3. 返回连续的数据块
type ReceiveBuffer struct {
	mu sync.RWMutex

	// 序列号管理
	nextExpected uint64 // 期望接收的下一个序列号

	// 乱序缓冲区
	buffer map[uint64]*BufferedPacket // seqNum -> buffered packet

	// 配置
	maxOutOfOrder int // 最大乱序包数

	// 统计信息
	totalReceived     uint64 // 总接收包数
	totalOutOfOrder   uint64 // 总乱序包数
	totalReordered    uint64 // 总重组包数
	currentBufferSize int    // 当前缓冲字节数
}

// NewReceiveBuffer 创建接收缓冲区
func NewReceiveBuffer() *ReceiveBuffer {
	return &ReceiveBuffer{
		nextExpected:  1, // 期望从1开始
		buffer:        make(map[uint64]*BufferedPacket),
		maxOutOfOrder: DefaultMaxOutOfOrder,
	}
}

// NewReceiveBufferWithConfig 使用自定义配置创建接收缓冲区
func NewReceiveBufferWithConfig(maxOutOfOrder int) *ReceiveBuffer {
	return &ReceiveBuffer{
		nextExpected:  1,
		buffer:        make(map[uint64]*BufferedPacket),
		maxOutOfOrder: maxOutOfOrder,
	}
}

// ============================================================================
// 接收和重组
// ============================================================================

// Receive 接收数据包并尝试重组
//
// 返回：
//   - 连续的数据块列表（按序列号排序）
//   - 错误（如果有）
//
// 行为：
//   - 如果包是期望的下一个，直接返回
//   - 如果包是未来的（乱序），缓冲起来
//   - 如果包已经接收过，丢弃
//   - 接收后尝试从缓冲区提取连续数据
func (b *ReceiveBuffer) Receive(pkt *packet.TransferPacket) ([][]byte, error) {
	if pkt == nil {
		return nil, coreerrors.New(coreerrors.CodeInvalidParam, "packet is nil")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	seqNum := pkt.SeqNum
	data := pkt.Payload

	b.totalReceived++

	// 情况1: 重复包（已经接收过）
	if seqNum < b.nextExpected {
		// 丢弃重复包
		return nil, nil
	}

	// 情况2: 期望的包（顺序到达）
	if seqNum == b.nextExpected {
		result := [][]byte{data}
		b.nextExpected++

		// 检查缓冲区中是否有后续连续包
		for {
			if bufferedPkt, exists := b.buffer[b.nextExpected]; exists {
				result = append(result, bufferedPkt.Data)
				delete(b.buffer, b.nextExpected)
				b.currentBufferSize -= len(bufferedPkt.Data)
				b.nextExpected++
				b.totalReordered++
			} else {
				break
			}
		}

		return result, nil
	}

	// 情况3: 未来的包（乱序到达）
	if seqNum > b.nextExpected {
		// 检查是否超过最大乱序限制
		if len(b.buffer) >= b.maxOutOfOrder {
			return nil, coreerrors.New(coreerrors.CodeQuotaExceeded, "too many out-of-order packets")
		}

		// 检查是否已缓冲（防止重复）
		if _, exists := b.buffer[seqNum]; exists {
			return nil, nil // 已缓冲，丢弃
		}

		// 缓冲乱序包
		b.buffer[seqNum] = &BufferedPacket{
			SeqNum: seqNum,
			Data:   data,
			SentAt: time.Now(),
		}
		b.currentBufferSize += len(data)
		b.totalOutOfOrder++

		return nil, nil // 等待期望的包到达
	}

	return nil, nil
}

// ============================================================================
// 状态查询
// ============================================================================

// GetNextExpected 获取期望的下一个序列号
func (b *ReceiveBuffer) GetNextExpected() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.nextExpected
}

// GetBufferedCount 获取缓冲区中的包数量
func (b *ReceiveBuffer) GetBufferedCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.buffer)
}

// GetBufferSize 获取当前缓冲区大小（字节）
func (b *ReceiveBuffer) GetBufferSize() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentBufferSize
}

// GetStats 获取统计信息
func (b *ReceiveBuffer) GetStats() map[string]uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return map[string]uint64{
		"total_received":     b.totalReceived,
		"total_out_of_order": b.totalOutOfOrder,
		"total_reordered":    b.totalReordered,
		"buffered_count":     uint64(len(b.buffer)),
		"buffer_size":        uint64(b.currentBufferSize),
		"next_expected":      b.nextExpected,
	}
}

// Reset 重置缓冲区
func (b *ReceiveBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer = make(map[uint64]*BufferedPacket)
	b.currentBufferSize = 0
	// 注意：不重置 nextExpected，保持连续性
}

// Clear 清空缓冲区并重置序列号
func (b *ReceiveBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer = make(map[uint64]*BufferedPacket)
	b.currentBufferSize = 0
	b.nextExpected = 1
	b.totalReceived = 0
	b.totalOutOfOrder = 0
	b.totalReordered = 0
}
