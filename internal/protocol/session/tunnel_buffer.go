package session

import (
	"errors"
	"sync"
	"time"
	"tunnox-core/internal/packet"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 发送端缓冲机制
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// DefaultMaxBufferSize 默认最大缓冲大小（10MB）
	DefaultMaxBufferSize = 10 * 1024 * 1024

	// DefaultMaxBufferedPackets 默认最大缓冲包数
	DefaultMaxBufferedPackets = 1000

	// DefaultResendTimeout 默认重传超时（3秒）
	DefaultResendTimeout = 3 * time.Second
)

// BufferedPacket 缓冲的数据包
type BufferedPacket struct {
	SeqNum    uint64            // 序列号
	Data      []byte            // 数据内容
	SentAt    time.Time         // 发送时间
	RetryCount int              // 重传次数
	Packet    *packet.TransferPacket // 原始包（用于重传）
}

// TunnelSendBuffer 隧道发送缓冲区
//
// 功能：
// 1. 缓冲未确认的数据包
// 2. 跟踪序列号和确认号
// 3. 支持数据重传
type TunnelSendBuffer struct {
	mu sync.RWMutex

	// 序列号管理
	nextSeq      uint64 // 下一个待分配的序列号
	confirmedSeq uint64 // 已确认的最大连续序列号

	// 缓冲区
	buffer map[uint64]*BufferedPacket // seqNum -> buffered packet

	// 配置
	maxBufferSize     int           // 最大缓冲字节数
	maxBufferedPackets int          // 最大缓冲包数
	resendTimeout     time.Duration // 重传超时

	// 统计信息
	totalSent     uint64 // 总发送包数
	totalResent   uint64 // 总重传包数
	totalConfirmed uint64 // 总确认包数
	currentBufferSize int // 当前缓冲字节数
}

// NewTunnelSendBuffer 创建发送缓冲区
func NewTunnelSendBuffer() *TunnelSendBuffer {
	return &TunnelSendBuffer{
		nextSeq:            1, // 序列号从1开始
		confirmedSeq:       0,
		buffer:             make(map[uint64]*BufferedPacket),
		maxBufferSize:      DefaultMaxBufferSize,
		maxBufferedPackets: DefaultMaxBufferedPackets,
		resendTimeout:      DefaultResendTimeout,
	}
}

// NewTunnelSendBufferWithConfig 使用自定义配置创建发送缓冲区
func NewTunnelSendBufferWithConfig(maxBufferSize, maxPackets int, resendTimeout time.Duration) *TunnelSendBuffer {
	return &TunnelSendBuffer{
		nextSeq:            1,
		confirmedSeq:       0,
		buffer:             make(map[uint64]*BufferedPacket),
		maxBufferSize:      maxBufferSize,
		maxBufferedPackets: maxPackets,
		resendTimeout:      resendTimeout,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 发送和缓冲
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Send 发送数据并缓冲
//
// 返回：分配的序列号和可能的错误
func (b *TunnelSendBuffer) Send(data []byte, pkt *packet.TransferPacket) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 检查缓冲区是否已满
	if len(b.buffer) >= b.maxBufferedPackets {
		return 0, errors.New("send buffer full: too many packets")
	}

	if b.currentBufferSize+len(data) > b.maxBufferSize {
		return 0, errors.New("send buffer full: size limit exceeded")
	}

	// 分配序列号
	seqNum := b.nextSeq
	b.nextSeq++

	// 创建缓冲包
	bufferedPkt := &BufferedPacket{
		SeqNum:     seqNum,
		Data:       data,
		SentAt:     time.Now(),
		RetryCount: 0,
		Packet:     pkt,
	}

	// 添加到缓冲区
	b.buffer[seqNum] = bufferedPkt
	b.currentBufferSize += len(data)
	b.totalSent++

	return seqNum, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 确认和清理
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ConfirmUpTo 确认到指定序列号（不含）的所有数据包
//
// 例如：ConfirmUpTo(5) 表示确认 1, 2, 3, 4，期望接收 5
func (b *TunnelSendBuffer) ConfirmUpTo(ackNum uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 清理已确认的包
	for seqNum := b.confirmedSeq + 1; seqNum < ackNum; seqNum++ {
		if bufferedPkt, exists := b.buffer[seqNum]; exists {
			delete(b.buffer, seqNum)
			b.currentBufferSize -= len(bufferedPkt.Data)
			b.totalConfirmed++
		}
	}

	// 更新已确认序列号
	if ackNum > b.confirmedSeq {
		b.confirmedSeq = ackNum - 1
	}
}

// ConfirmPacket 确认单个数据包
func (b *TunnelSendBuffer) ConfirmPacket(seqNum uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if bufferedPkt, exists := b.buffer[seqNum]; exists {
		delete(b.buffer, seqNum)
		b.currentBufferSize -= len(bufferedPkt.Data)
		b.totalConfirmed++
	}

	// 如果是连续确认，更新 confirmedSeq
	if seqNum == b.confirmedSeq+1 {
		b.confirmedSeq = seqNum
		
		// 继续向前推进 confirmedSeq（如果后续包已确认）
		for {
			if _, exists := b.buffer[b.confirmedSeq+1]; !exists {
				b.confirmedSeq++
			} else {
				break
			}
		}
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 重传
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetUnconfirmedPackets 获取所有未确认的数据包（用于重传）
//
// 返回：需要重传的数据包列表
func (b *TunnelSendBuffer) GetUnconfirmedPackets() []*BufferedPacket {
	b.mu.RLock()
	defer b.mu.RUnlock()

	now := time.Now()
	unconfirmed := make([]*BufferedPacket, 0, len(b.buffer))

	for _, bufferedPkt := range b.buffer {
		// 检查是否超时需要重传
		if now.Sub(bufferedPkt.SentAt) >= b.resendTimeout {
			unconfirmed = append(unconfirmed, bufferedPkt)
		}
	}

	return unconfirmed
}

// MarkResent 标记数据包已重传
func (b *TunnelSendBuffer) MarkResent(seqNum uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if bufferedPkt, exists := b.buffer[seqNum]; exists {
		bufferedPkt.SentAt = time.Now()
		bufferedPkt.RetryCount++
		b.totalResent++
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 状态查询
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetNextSeq 获取下一个序列号
func (b *TunnelSendBuffer) GetNextSeq() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.nextSeq
}

// GetConfirmedSeq 获取已确认的序列号
func (b *TunnelSendBuffer) GetConfirmedSeq() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.confirmedSeq
}

// GetBufferedCount 获取缓冲区中的包数量
func (b *TunnelSendBuffer) GetBufferedCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.buffer)
}

// GetBufferSize 获取当前缓冲区大小（字节）
func (b *TunnelSendBuffer) GetBufferSize() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentBufferSize
}

// GetStats 获取统计信息
func (b *TunnelSendBuffer) GetStats() map[string]uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return map[string]uint64{
		"total_sent":      b.totalSent,
		"total_resent":    b.totalResent,
		"total_confirmed": b.totalConfirmed,
		"buffered_count":  uint64(len(b.buffer)),
		"buffer_size":     uint64(b.currentBufferSize),
		"next_seq":        b.nextSeq,
		"confirmed_seq":   b.confirmedSeq,
	}
}

// Reset 重置缓冲区（用于重连等场景）
func (b *TunnelSendBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer = make(map[uint64]*BufferedPacket)
	b.currentBufferSize = 0
	// 注意：不重置序列号，保持连续性
}

// Clear 清空缓冲区并重置序列号
func (b *TunnelSendBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer = make(map[uint64]*BufferedPacket)
	b.currentBufferSize = 0
	b.nextSeq = 1
	b.confirmedSeq = 0
	b.totalSent = 0
	b.totalResent = 0
	b.totalConfirmed = 0
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 接收端重组机制
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// DefaultMaxOutOfOrder 默认最大乱序包数
	DefaultMaxOutOfOrder = 100
)

// TunnelReceiveBuffer 隧道接收缓冲区
//
// 功能：
// 1. 缓冲乱序到达的数据包
// 2. 按序列号重组数据
// 3. 返回连续的数据块
type TunnelReceiveBuffer struct {
	mu sync.RWMutex

	// 序列号管理
	nextExpected uint64 // 期望接收的下一个序列号

	// 乱序缓冲区
	buffer map[uint64]*BufferedPacket // seqNum -> buffered packet

	// 配置
	maxOutOfOrder int // 最大乱序包数

	// 统计信息
	totalReceived    uint64 // 总接收包数
	totalOutOfOrder  uint64 // 总乱序包数
	totalReordered   uint64 // 总重组包数
	currentBufferSize int   // 当前缓冲字节数
}

// NewTunnelReceiveBuffer 创建接收缓冲区
func NewTunnelReceiveBuffer() *TunnelReceiveBuffer {
	return &TunnelReceiveBuffer{
		nextExpected:  1, // 期望从1开始
		buffer:        make(map[uint64]*BufferedPacket),
		maxOutOfOrder: DefaultMaxOutOfOrder,
	}
}

// NewTunnelReceiveBufferWithConfig 使用自定义配置创建接收缓冲区
func NewTunnelReceiveBufferWithConfig(maxOutOfOrder int) *TunnelReceiveBuffer {
	return &TunnelReceiveBuffer{
		nextExpected:  1,
		buffer:        make(map[uint64]*BufferedPacket),
		maxOutOfOrder: maxOutOfOrder,
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 接收和重组
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

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
func (b *TunnelReceiveBuffer) Receive(pkt *packet.TransferPacket) ([][]byte, error) {
	if pkt == nil {
		return nil, errors.New("packet is nil")
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
			return nil, errors.New("too many out-of-order packets")
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

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// 状态查询
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetNextExpected 获取期望的下一个序列号
func (b *TunnelReceiveBuffer) GetNextExpected() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.nextExpected
}

// GetBufferedCount 获取缓冲区中的包数量
func (b *TunnelReceiveBuffer) GetBufferedCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.buffer)
}

// GetBufferSize 获取当前缓冲区大小（字节）
func (b *TunnelReceiveBuffer) GetBufferSize() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.currentBufferSize
}

// GetStats 获取统计信息
func (b *TunnelReceiveBuffer) GetStats() map[string]uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return map[string]uint64{
		"total_received":   b.totalReceived,
		"total_out_of_order": b.totalOutOfOrder,
		"total_reordered":  b.totalReordered,
		"buffered_count":   uint64(len(b.buffer)),
		"buffer_size":      uint64(b.currentBufferSize),
		"next_expected":    b.nextExpected,
	}
}

// Reset 重置缓冲区
func (b *TunnelReceiveBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer = make(map[uint64]*BufferedPacket)
	b.currentBufferSize = 0
	// 注意：不重置 nextExpected，保持连续性
}

// Clear 清空缓冲区并重置序列号
func (b *TunnelReceiveBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.buffer = make(map[uint64]*BufferedPacket)
	b.currentBufferSize = 0
	b.nextExpected = 1
	b.totalReceived = 0
	b.totalOutOfOrder = 0
	b.totalReordered = 0
}


