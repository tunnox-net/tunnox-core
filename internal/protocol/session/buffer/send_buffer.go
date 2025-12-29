// Package buffer 提供隧道数据缓冲功能
package buffer

import (
	"errors"
	"sync"
	"time"
	"tunnox-core/internal/packet"
)

// ============================================================================
// 发送端缓冲机制
// ============================================================================

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
	SeqNum     uint64                 // 序列号
	Data       []byte                 // 数据内容
	SentAt     time.Time              // 发送时间
	RetryCount int                    // 重传次数
	Packet     *packet.TransferPacket // 原始包（用于重传）
}

// SendBuffer 隧道发送缓冲区
//
// 功能：
// 1. 缓冲未确认的数据包
// 2. 跟踪序列号和确认号
// 3. 支持数据重传
type SendBuffer struct {
	mu sync.RWMutex

	// 序列号管理
	nextSeq      uint64 // 下一个待分配的序列号
	confirmedSeq uint64 // 已确认的最大连续序列号

	// 缓冲区
	Buffer map[uint64]*BufferedPacket // seqNum -> buffered packet

	// 配置
	maxBufferSize      int           // 最大缓冲字节数
	maxBufferedPackets int           // 最大缓冲包数
	resendTimeout      time.Duration // 重传超时

	// 统计信息
	totalSent         uint64 // 总发送包数
	totalResent       uint64 // 总重传包数
	totalConfirmed    uint64 // 总确认包数
	CurrentBufferSize int    // 当前缓冲字节数
}

// NewSendBuffer 创建发送缓冲区
func NewSendBuffer() *SendBuffer {
	return &SendBuffer{
		nextSeq:            1, // 序列号从1开始
		confirmedSeq:       0,
		Buffer:             make(map[uint64]*BufferedPacket),
		maxBufferSize:      DefaultMaxBufferSize,
		maxBufferedPackets: DefaultMaxBufferedPackets,
		resendTimeout:      DefaultResendTimeout,
	}
}

// NewSendBufferWithConfig 使用自定义配置创建发送缓冲区
func NewSendBufferWithConfig(maxBufferSize, maxPackets int, resendTimeout time.Duration) *SendBuffer {
	return &SendBuffer{
		nextSeq:            1,
		confirmedSeq:       0,
		Buffer:             make(map[uint64]*BufferedPacket),
		maxBufferSize:      maxBufferSize,
		maxBufferedPackets: maxPackets,
		resendTimeout:      resendTimeout,
	}
}

// ============================================================================
// 发送和缓冲
// ============================================================================

// Send 发送数据并缓冲
//
// 返回：分配的序列号和可能的错误
func (b *SendBuffer) Send(data []byte, pkt *packet.TransferPacket) (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 检查缓冲区是否已满
	if len(b.Buffer) >= b.maxBufferedPackets {
		return 0, errors.New("send buffer full: too many packets")
	}

	if b.CurrentBufferSize+len(data) > b.maxBufferSize {
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
	b.Buffer[seqNum] = bufferedPkt
	b.CurrentBufferSize += len(data)
	b.totalSent++

	return seqNum, nil
}

// ============================================================================
// 确认和清理
// ============================================================================

// ConfirmUpTo 确认到指定序列号（不含）的所有数据包
//
// 例如：ConfirmUpTo(5) 表示确认 1, 2, 3, 4，期望接收 5
func (b *SendBuffer) ConfirmUpTo(ackNum uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 清理已确认的包
	for seqNum := b.confirmedSeq + 1; seqNum < ackNum; seqNum++ {
		if bufferedPkt, exists := b.Buffer[seqNum]; exists {
			delete(b.Buffer, seqNum)
			b.CurrentBufferSize -= len(bufferedPkt.Data)
			b.totalConfirmed++
		}
	}

	// 更新已确认序列号
	if ackNum > b.confirmedSeq {
		b.confirmedSeq = ackNum - 1
	}
}

// ConfirmPacket 确认单个数据包
func (b *SendBuffer) ConfirmPacket(seqNum uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if bufferedPkt, exists := b.Buffer[seqNum]; exists {
		delete(b.Buffer, seqNum)
		b.CurrentBufferSize -= len(bufferedPkt.Data)
		b.totalConfirmed++
	}

	// 如果是连续确认，更新 confirmedSeq
	if seqNum == b.confirmedSeq+1 {
		b.confirmedSeq = seqNum

		// 继续向前推进 confirmedSeq（如果后续包已确认）
		for {
			if _, exists := b.Buffer[b.confirmedSeq+1]; !exists {
				b.confirmedSeq++
			} else {
				break
			}
		}
	}
}

// ============================================================================
// 重传
// ============================================================================

// GetUnconfirmedPackets 获取所有未确认的数据包（用于重传）
//
// 返回：需要重传的数据包列表
func (b *SendBuffer) GetUnconfirmedPackets() []*BufferedPacket {
	b.mu.RLock()
	defer b.mu.RUnlock()

	now := time.Now()
	unconfirmed := make([]*BufferedPacket, 0, len(b.Buffer))

	for _, bufferedPkt := range b.Buffer {
		// 检查是否超时需要重传
		if now.Sub(bufferedPkt.SentAt) >= b.resendTimeout {
			unconfirmed = append(unconfirmed, bufferedPkt)
		}
	}

	return unconfirmed
}

// MarkResent 标记数据包已重传
func (b *SendBuffer) MarkResent(seqNum uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if bufferedPkt, exists := b.Buffer[seqNum]; exists {
		bufferedPkt.SentAt = time.Now()
		bufferedPkt.RetryCount++
		b.totalResent++
	}
}

// ============================================================================
// 状态查询
// ============================================================================

// GetNextSeq 获取下一个序列号
func (b *SendBuffer) GetNextSeq() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.nextSeq
}

// GetConfirmedSeq 获取已确认的序列号
func (b *SendBuffer) GetConfirmedSeq() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.confirmedSeq
}

// GetBufferedCount 获取缓冲区中的包数量
func (b *SendBuffer) GetBufferedCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.Buffer)
}

// GetBufferSize 获取当前缓冲区大小（字节）
func (b *SendBuffer) GetBufferSize() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.CurrentBufferSize
}

// GetStats 获取统计信息
func (b *SendBuffer) GetStats() map[string]uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return map[string]uint64{
		"total_sent":      b.totalSent,
		"total_resent":    b.totalResent,
		"total_confirmed": b.totalConfirmed,
		"buffered_count":  uint64(len(b.Buffer)),
		"buffer_size":     uint64(b.CurrentBufferSize),
		"next_seq":        b.nextSeq,
		"confirmed_seq":   b.confirmedSeq,
	}
}

// Reset 重置缓冲区（用于重连等场景）
func (b *SendBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.Buffer = make(map[uint64]*BufferedPacket)
	b.CurrentBufferSize = 0
	// 注意：不重置序列号，保持连续性
}

// Clear 清空缓冲区并重置序列号
func (b *SendBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.Buffer = make(map[uint64]*BufferedPacket)
	b.CurrentBufferSize = 0
	b.nextSeq = 1
	b.confirmedSeq = 0
	b.totalSent = 0
	b.totalResent = 0
	b.totalConfirmed = 0
}

// Lock 获取写锁（用于状态捕获）
func (b *SendBuffer) Lock() {
	b.mu.Lock()
}

// Unlock 释放写锁
func (b *SendBuffer) Unlock() {
	b.mu.Unlock()
}

// RLock 获取读锁（用于状态捕获）
func (b *SendBuffer) RLock() {
	b.mu.RLock()
}

// RUnlock 释放读锁
func (b *SendBuffer) RUnlock() {
	b.mu.RUnlock()
}
