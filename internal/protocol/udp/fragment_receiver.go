package udp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"tunnox-core/internal/utils"
)

// UDPFragment 单个分片
type UDPFragment struct {
	Index    uint16
	Size     uint16
	Data     []byte
	Received time.Time
}

// UDPReceiveBuffer 接收端分片重组缓冲区
type UDPReceiveBuffer struct {
	groupID        uint64
	originalSize   uint32
	totalFragments uint16

	// 分片存储
	fragments     map[uint16]*UDPFragment // 按索引存储
	receivedBits  []bool                  // 位图，标记已接收
	receivedCount uint16

	// 状态
	createdTime    time.Time
	lastActiveTime time.Time
	reassembled    int32 // 是否已重组（原子操作）
	mu             sync.RWMutex

	// 回调
	onComplete chan []byte // 完成时发送重组数据
	onTimeout  func()      // 超时回调

	// 控制
	ctx    context.Context
	cancel context.CancelFunc

	// 用于发送 ACK
	conn net.PacketConn
	addr net.Addr
}

// NewUDPReceiveBuffer 创建接收缓冲区
func NewUDPReceiveBuffer(ctx context.Context, conn net.PacketConn, addr net.Addr, groupID uint64, originalSize uint32, totalFragments uint16) *UDPReceiveBuffer {
	bufferCtx, cancel := context.WithCancel(ctx)

	buf := &UDPReceiveBuffer{
		groupID:        groupID,
		originalSize:   originalSize,
		totalFragments: totalFragments,
		fragments:      make(map[uint16]*UDPFragment),
		receivedBits:   make([]bool, totalFragments),
		createdTime:    time.Now(),
		lastActiveTime: time.Now(),
		onComplete:     make(chan []byte, 1),
		ctx:            bufferCtx,
		cancel:         cancel,
		conn:           conn,
		addr:           addr,
	}

	// 启动超时检查协程
	go buf.timeoutLoop()

	return buf
}

// AddFragment 添加分片
func (b *UDPReceiveBuffer) AddFragment(pkt *UDPFragmentPacket) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 检查索引范围
	if pkt.FragmentIndex >= b.totalFragments {
		return fmt.Errorf("fragment index out of range: %d >= %d", pkt.FragmentIndex, b.totalFragments)
	}

	// 检查是否已存在
	if b.fragments[pkt.FragmentIndex] != nil {
		utils.Debugf("UDP: fragment %d/%d already exists for groupID=%d, ignoring duplicate",
			pkt.FragmentIndex, b.totalFragments, b.groupID)
		// 仍然发送 ACK（快速重传场景）
		go b.sendACK()
		return nil
	}

	// 验证大小
	if len(pkt.Data) != int(pkt.FragmentSize) {
		return fmt.Errorf("fragment size mismatch: expected %d, got %d", pkt.FragmentSize, len(pkt.Data))
	}

	// 添加分片
	b.fragments[pkt.FragmentIndex] = &UDPFragment{
		Index:    pkt.FragmentIndex,
		Size:     pkt.FragmentSize,
		Data:     pkt.Data,
		Received: time.Now(),
	}

	b.receivedBits[pkt.FragmentIndex] = true
	b.receivedCount++
	b.lastActiveTime = time.Now()

	utils.Debugf("UDP: added fragment %d/%d (size=%d, received=%d/%d) for groupID=%d",
		pkt.FragmentIndex, b.totalFragments, pkt.FragmentSize, b.receivedCount, b.totalFragments, b.groupID)

	// 发送 ACK
	go b.sendACK()

	// 检查是否完整
	if b.receivedCount == b.totalFragments {
		// 重组数据
		go b.reassemble()
	}

	return nil
}

// reassemble 重组数据（在独立的 goroutine 中执行，避免阻塞）
func (b *UDPReceiveBuffer) reassemble() {
	// 使用原子操作确保只重组一次
	if !atomic.CompareAndSwapInt32(&b.reassembled, 0, 1) {
		return // 已经重组过
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	// 再次检查完整性（双重检查）
	if b.receivedCount != b.totalFragments {
		atomic.StoreInt32(&b.reassembled, 0) // 重置，允许再次尝试
		return
	}

	// 按索引顺序拼接
	result := make([]byte, 0, b.originalSize)
	for i := uint16(0); i < b.totalFragments; i++ {
		fragment, exists := b.fragments[i]
		if !exists {
			atomic.StoreInt32(&b.reassembled, 0)
			utils.Errorf("UDP: fragment %d is missing for groupID=%d", i, b.groupID)
			return
		}
		result = append(result, fragment.Data...)
	}

	// 验证总大小
	if len(result) != int(b.originalSize) {
		atomic.StoreInt32(&b.reassembled, 0)
		utils.Errorf("UDP: reassembled size mismatch: expected %d, got %d for groupID=%d",
			b.originalSize, len(result), b.groupID)
		return
	}

	utils.Debugf("UDP: reassembled %d bytes from %d fragments for groupID=%d",
		len(result), b.totalFragments, b.groupID)

	// 发送到完成 channel（非阻塞）
	select {
	case b.onComplete <- result:
		// 成功发送
	case <-time.After(100 * time.Millisecond):
		utils.Warnf("UDP: onComplete channel full for groupID=%d", b.groupID)
	}

	b.cancel() // 取消超时检查
}

// sendACK 发送 ACK 包
func (b *UDPReceiveBuffer) sendACK() {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// 构建位图（最多 16 个分片）
	var receivedBits uint16
	lastReceivedIndex := uint16(0)

	for i := uint16(0); i < b.totalFragments && i < 16; i++ {
		if b.receivedBits[i] {
			receivedBits |= 1 << i
			lastReceivedIndex = i
		}
	}

	// 对于超过 16 个分片的情况，只标记最后接收的索引
	// 发送端可以根据 LastReceivedIndex 判断进度

	ack := &UDPACKPacket{
		Magic:            UDPACKMagic,
		Version:          UDPFragmentVersion,
		Flags:            0,
		FragmentGroupID: b.groupID,
		ReceivedBits:     receivedBits,
		LastReceivedIndex: lastReceivedIndex,
	}

	ackData, err := ack.Marshal()
	if err != nil {
		utils.Errorf("UDP: failed to marshal ACK: %v", err)
		return
	}

	if _, err := b.conn.WriteTo(ackData, b.addr); err != nil {
		utils.Debugf("UDP: failed to send ACK: %v", err)
		return
	}

	utils.Debugf("UDP: sent ACK for groupID=%d, receivedBits=0x%04X, lastIndex=%d",
		b.groupID, receivedBits, lastReceivedIndex)
}

// IsComplete 检查是否完整
func (b *UDPReceiveBuffer) IsComplete() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.receivedCount == b.totalFragments
}

// GetReassembledData 获取重组后的数据（阻塞直到完成或超时）
func (b *UDPReceiveBuffer) GetReassembledData(timeout time.Duration) ([]byte, error) {
	select {
	case data := <-b.onComplete:
		return data, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for reassembly")
	case <-b.ctx.Done():
		return nil, fmt.Errorf("buffer cancelled")
	}
}

// Close 关闭接收缓冲区
func (b *UDPReceiveBuffer) Close() {
	b.cancel()
}

// timeoutLoop 超时检查循环
func (b *UDPReceiveBuffer) timeoutLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			if atomic.LoadInt32(&b.reassembled) == 1 {
				return // 已重组，退出
			}
			if b.IsExpired() {
				utils.Warnf("UDP: receive buffer expired for groupID=%d", b.groupID)
				if b.onTimeout != nil {
					b.onTimeout()
				}
				b.cancel()
				return
			}
		}
	}
}

// IsExpired 检查是否过期
func (b *UDPReceiveBuffer) IsExpired() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return time.Since(b.lastActiveTime) > UDPBufferTimeout
}

