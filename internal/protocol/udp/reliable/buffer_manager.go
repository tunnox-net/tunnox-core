package reliable

import (
	"sync"
	"time"

	coreErrors "tunnox-core/internal/core/errors"
)

const (
	// MaxSendBufSize 最多缓存的未确认包数
	// 参考 httppoll 的 MaxFragmentGroups (5000) 和 tunnel buffer 的配置
	// 10000 个包 * 1444 字节 ≈ 14.4MB 的在途数据
	// 这个值足够支持高吞吐量传输和高延迟网络
	MaxSendBufSize = 10000

	// MaxRecvBufSize 最多缓存的乱序包数
	// 增加接收缓冲区以应对网络抖动和乱序包
	// 参考 httppoll 的配置，设置为 10000
	MaxRecvBufSize = 10000

	// AggressiveCleanupThreshold 激进清理阈值
	// 当缓冲区使用率达到 80% 时，触发激进清理
	AggressiveCleanupThreshold = 8000 // 80% of MaxRecvBufSize
)

var (
	// ErrBufferFull 缓冲区已满错误
	ErrBufferFull = coreErrors.New(coreErrors.ErrorTypeTemporary, "buffer full")
)

// BufferEntry 缓冲区条目
type BufferEntry struct {
	Packet     *Packet
	Timestamp  time.Time
	RetryCount int
	Acked      bool
}

// BufferManager 缓冲区管理器
// 管理发送和接收缓冲区，防止无限增长
type BufferManager struct {
	sendBuf map[uint32]*BufferEntry
	recvBuf map[uint32]*BufferEntry
	mu      sync.RWMutex

	maxSendSize int
	maxRecvSize int
}

// NewBufferManager 创建缓冲区管理器
func NewBufferManager() *BufferManager {
	return &BufferManager{
		sendBuf:     make(map[uint32]*BufferEntry),
		recvBuf:     make(map[uint32]*BufferEntry),
		maxSendSize: MaxSendBufSize,
		maxRecvSize: MaxRecvBufSize,
	}
}

// AddSendBuffer 添加到发送缓冲区
func (bm *BufferManager) AddSendBuffer(seq uint32, packet *Packet) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if len(bm.sendBuf) >= bm.maxSendSize {
		return ErrBufferFull
	}

	bm.sendBuf[seq] = &BufferEntry{
		Packet:     packet,
		Timestamp:  time.Now(),
		RetryCount: 0,
		Acked:      false,
	}
	return nil
}

// GetSendBuffer 获取发送缓冲区条目
func (bm *BufferManager) GetSendBuffer(seq uint32) *BufferEntry {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.sendBuf[seq]
}

// RemoveSendBuffer 从发送缓冲区移除
func (bm *BufferManager) RemoveSendBuffer(seq uint32) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	delete(bm.sendBuf, seq)
}

// MarkAcked 标记为已确认
func (bm *BufferManager) MarkAcked(seq uint32) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if entry, exists := bm.sendBuf[seq]; exists {
		entry.Acked = true
	}
}

// GetUnackedPackets 获取所有未确认的包
func (bm *BufferManager) GetUnackedPackets() []*BufferEntry {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	var unacked []*BufferEntry
	for _, entry := range bm.sendBuf {
		if !entry.Acked {
			unacked = append(unacked, entry)
		}
	}
	return unacked
}

// AddRecvBuffer 添加到接收缓冲区
func (bm *BufferManager) AddRecvBuffer(seq uint32, packet *Packet) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	// 如果缓冲区满，返回错误（不自动清理，因为乱序包需要等待后续包）
	if len(bm.recvBuf) >= bm.maxRecvSize {
		return ErrBufferFull
	}

	bm.recvBuf[seq] = &BufferEntry{
		Packet:    packet,
		Timestamp: time.Now(),
	}
	return nil
}

// GetRecvBuffer 获取接收缓冲区条目
func (bm *BufferManager) GetRecvBuffer(seq uint32) *BufferEntry {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.recvBuf[seq]
}

// RemoveRecvBuffer 从接收缓冲区移除
func (bm *BufferManager) RemoveRecvBuffer(seq uint32) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	delete(bm.recvBuf, seq)
}

// GetOrderedRecvPackets 获取有序的接收包
// 从 startSeq 开始，返回连续的包
func (bm *BufferManager) GetOrderedRecvPackets(startSeq uint32) []*Packet {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	var packets []*Packet
	seq := startSeq

	for {
		entry, exists := bm.recvBuf[seq]
		if !exists {
			break
		}
		packets = append(packets, entry.Packet)
		seq++
	}

	return packets
}

// Cleanup 清理缓冲区条目
// 立即清理已确认的发送缓冲区，清理超时的接收缓冲区
func (bm *BufferManager) Cleanup(timeout time.Duration) int {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	now := time.Now()
	cleaned := 0

	// 立即清理已确认的发送缓冲区
	for seq, entry := range bm.sendBuf {
		if entry.Acked {
			delete(bm.sendBuf, seq)
			cleaned++
		}
	}

	// 清理超时的接收缓冲区（只清理真正超时的，默认超时时间应该足够长）
	for seq, entry := range bm.recvBuf {
		if now.Sub(entry.Timestamp) > timeout {
			delete(bm.recvBuf, seq)
			cleaned++
		}
	}

	return cleaned
}

// CleanupStaleRecvPackets 清理陈旧的接收包
// 只清理那些序列号远小于当前期望序列号的包（说明中间的包已经丢失很久了）
// expectedSeq: 当前期望的序列号
// maxGap: 允许的最大序列号间隔（超过此间隔的旧包会被清理）
func (bm *BufferManager) CleanupStaleRecvPackets(expectedSeq uint32, maxGap uint32) int {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	cleaned := 0
	
	// 清理序列号远小于期望序列号的包
	// 这些包说明中间有很多包丢失，不太可能再收到了
	for seq := range bm.recvBuf {
		// 处理序列号回绕的情况
		var gap uint32
		if seq > expectedSeq {
			// 可能是序列号回绕，或者是未来的包
			// 如果差距很大，说明是旧包（回绕前的）
			gap = (^uint32(0) - seq) + expectedSeq + 1
		} else {
			gap = expectedSeq - seq
		}
		
		// 如果间隔超过 maxGap，说明这个包太旧了
		if gap > maxGap {
			delete(bm.recvBuf, seq)
			cleaned++
		}
	}

	return cleaned
}

// CleanupAcked 立即清理所有已确认的发送缓冲区
func (bm *BufferManager) CleanupAcked() int {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	cleaned := 0
	for seq, entry := range bm.sendBuf {
		if entry.Acked {
			delete(bm.sendBuf, seq)
			cleaned++
		}
	}
	return cleaned
}

// GetStats 获取统计信息
func (bm *BufferManager) GetStats() (sendBufSize, recvBufSize int) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	return len(bm.sendBuf), len(bm.recvBuf)
}

// ForEachUnacked 遍历所有未确认的包
// callback 返回 false 时停止遍历
func (bm *BufferManager) ForEachUnacked(callback func(seq uint32, entry *BufferEntry) bool) {
	bm.mu.RLock()
	// 收集需要处理的 seq
	var seqs []uint32
	for seq, entry := range bm.sendBuf {
		if !entry.Acked {
			seqs = append(seqs, seq)
		}
	}
	bm.mu.RUnlock()

	// 在锁外处理，避免死锁
	for _, seq := range seqs {
		bm.mu.RLock()
		entry, exists := bm.sendBuf[seq]
		bm.mu.RUnlock()

		if !exists || entry.Acked {
			continue
		}

		if !callback(seq, entry) {
			break
		}
	}
}

// UpdateRetransmit 更新重传信息
func (bm *BufferManager) UpdateRetransmit(seq uint32, timestamp time.Time) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if entry, exists := bm.sendBuf[seq]; exists {
		entry.Timestamp = timestamp
		entry.RetryCount++
	}
}

// GetUnackedCount 获取未确认包的数量
func (bm *BufferManager) GetUnackedCount() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	count := 0
	for _, entry := range bm.sendBuf {
		if !entry.Acked {
			count++
		}
	}
	return count
}
