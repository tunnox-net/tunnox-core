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

// UDPSendBuffer 发送端分片缓冲区
type UDPSendBuffer struct {
	groupID        uint64
	originalData   []byte
	totalFragments uint16
	fragmentSize   int

	// 状态管理
	sentFragments  map[uint16]time.Time // 已发送的分片及发送时间
	ackedFragments map[uint16]bool      // 已确认的分片
	retryCount     map[uint16]int       // 重试次数
	sequenceNum    uint16               // 当前序列号
	mu             sync.RWMutex

	// 回调
	onComplete func([]byte) // 所有分片确认后的回调
	onTimeout  func()       // 超时回调
	onError    func(error)  // 错误回调

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	conn   net.PacketConn
	addr   net.Addr

	// 统计
	rtt            time.Duration // 往返时间
	lastACKTime    time.Time     // 最后收到 ACK 的时间
	completed      int32         // 是否已完成（原子操作）
}

// NewUDPSendBuffer 创建发送缓冲区
func NewUDPSendBuffer(ctx context.Context, conn net.PacketConn, addr net.Addr, data []byte, groupID uint64) *UDPSendBuffer {
	bufferCtx, cancel := context.WithCancel(ctx)
	fragmentSize, totalFragments := CalculateFragments(len(data))

	buf := &UDPSendBuffer{
		groupID:        groupID,
		originalData:   data,
		totalFragments: uint16(totalFragments),
		fragmentSize:   fragmentSize,
		sentFragments:  make(map[uint16]time.Time),
		ackedFragments: make(map[uint16]bool),
		retryCount:     make(map[uint16]int),
		sequenceNum:    0,
		ctx:            bufferCtx,
		cancel:         cancel,
		conn:           conn,
		addr:           addr,
		rtt:            UDPInitialRTT,
	}

	// 启动重传检查协程
	go buf.retransmissionLoop()

	return buf
}

// Send 发送所有分片
func (b *UDPSendBuffer) Send() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 如果只有一片，直接发送（不分片）
	if b.totalFragments == 1 {
		flags := FragmentFlags(0) // 不是分片
		if b.onComplete != nil {
			flags |= FlagNeedACK
		}

		pkt := &UDPFragmentPacket{
			Magic:          UDPFragmentMagic,
			Version:        UDPFragmentVersion,
			Flags:          flags,
			FragmentGroupID: b.groupID,
			FragmentIndex:  0,
			TotalFragments: 1,
			OriginalSize:   uint32(len(b.originalData)),
			FragmentSize:   uint16(len(b.originalData)),
			SequenceNum:    b.sequenceNum,
			Data:           b.originalData,
		}

		data, err := pkt.Marshal()
		if err != nil {
			return fmt.Errorf("failed to marshal packet: %w", err)
		}

		if _, err := b.conn.WriteTo(data, b.addr); err != nil {
			return fmt.Errorf("failed to send packet: %w", err)
		}

		b.sentFragments[0] = time.Now()
		b.sequenceNum++

		// 如果不需要 ACK，直接完成
		if flags&FlagNeedACK == 0 {
			atomic.StoreInt32(&b.completed, 1)
			if b.onComplete != nil {
				b.onComplete(b.originalData)
			}
		}

		return nil
	}

	// 分片发送
	for i := uint16(0); i < b.totalFragments; i++ {
		fragmentData := GetFragmentData(b.originalData, int(i), b.fragmentSize, int(b.totalFragments))
		if fragmentData == nil {
			return fmt.Errorf("failed to get fragment data for index %d", i)
		}

		flags := FlagIsFragment
		if i == 0 {
			flags |= FlagIsFirst
		}
		if i == b.totalFragments-1 {
			flags |= FlagIsLast
		}
		flags |= FlagNeedACK

		pkt := &UDPFragmentPacket{
			Magic:          UDPFragmentMagic,
			Version:        UDPFragmentVersion,
			Flags:          flags,
			FragmentGroupID: b.groupID,
			FragmentIndex:  i,
			TotalFragments: b.totalFragments,
			OriginalSize:   uint32(len(b.originalData)),
			FragmentSize:   uint16(len(fragmentData)),
			SequenceNum:    b.sequenceNum,
			Data:           fragmentData,
		}

		data, err := pkt.Marshal()
		if err != nil {
			return fmt.Errorf("failed to marshal fragment %d: %w", i, err)
		}

		if _, err := b.conn.WriteTo(data, b.addr); err != nil {
			return fmt.Errorf("failed to send fragment %d: %w", i, err)
		}

		b.sentFragments[i] = time.Now()
		b.sequenceNum++
	}

	utils.Debugf("UDP: sent %d fragments for groupID=%d", b.totalFragments, b.groupID)
	return nil
}

// HandleACK 处理 ACK 包
func (b *UDPSendBuffer) HandleACK(ack *UDPACKPacket) {
	if ack.FragmentGroupID != b.groupID {
		return // 不是这个分片组的 ACK
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// 更新 RTT（基于最后发送时间和当前时间）
	if len(b.sentFragments) > 0 {
		now := time.Now()
		if !b.lastACKTime.IsZero() {
			rtt := now.Sub(b.lastACKTime)
			if rtt < UDPMaxRTT {
				// 平滑 RTT（指数移动平均）
				b.rtt = time.Duration(float64(b.rtt)*0.875 + float64(rtt)*0.125)
			}
		}
		b.lastACKTime = now
	}

	// 处理位图 ACK（最多 16 个分片）
	receivedBits := ack.ReceivedBits
	for i := uint16(0); i < 16 && i < b.totalFragments; i++ {
		if receivedBits&(1<<i) != 0 {
			if !b.ackedFragments[i] {
				b.ackedFragments[i] = true
				delete(b.sentFragments, i) // 不再需要重传
				utils.Debugf("UDP: fragment %d/%d ACKed for groupID=%d", i, b.totalFragments, b.groupID)
			}
		}
	}

	// 检查是否全部确认
	if len(b.ackedFragments) == int(b.totalFragments) {
		atomic.StoreInt32(&b.completed, 1)
		b.cancel()
		if b.onComplete != nil {
			b.onComplete(b.originalData)
		}
		utils.Debugf("UDP: all fragments ACKed for groupID=%d", b.groupID)
	}
}

// retransmissionLoop 重传检查循环
func (b *UDPSendBuffer) retransmissionLoop() {
	ticker := time.NewTicker(UDPRetryTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			if atomic.LoadInt32(&b.completed) == 1 {
				return
			}
			b.checkAndRetransmit()
		}
	}
}

// checkAndRetransmit 检查并重传未确认的分片
func (b *UDPSendBuffer) checkAndRetransmit() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	timeout := b.rtt * 2
	if timeout < UDPRetryTimeout {
		timeout = UDPRetryTimeout
	}
	if timeout > UDPMaxRTT {
		timeout = UDPMaxRTT
	}

	// 检查需要重传的分片
	for index, sentTime := range b.sentFragments {
		if b.ackedFragments[index] {
			continue // 已确认，跳过
		}

		if now.Sub(sentTime) > timeout {
			retryCount := b.retryCount[index]
			if retryCount >= UDPMaxRetries {
				// 超过最大重试次数，报告错误
				utils.Errorf("UDP: fragment %d/%d failed after %d retries for groupID=%d",
					index, b.totalFragments, retryCount, b.groupID)
				if b.onError != nil {
					b.onError(fmt.Errorf("fragment %d retry limit exceeded", index))
				}
				delete(b.sentFragments, index)
				continue
			}

			// 重传
			b.retryCount[index] = retryCount + 1
			b.sentFragments[index] = now // 更新发送时间

			// 获取分片数据
			fragmentData := GetFragmentData(b.originalData, int(index), b.fragmentSize, int(b.totalFragments))
			if fragmentData == nil {
				continue
			}

			flags := FlagIsFragment | FlagNeedACK
			if index == 0 {
				flags |= FlagIsFirst
			}
			if index == b.totalFragments-1 {
				flags |= FlagIsLast
			}

			pkt := &UDPFragmentPacket{
				Magic:          UDPFragmentMagic,
				Version:        UDPFragmentVersion,
				Flags:          flags,
				FragmentGroupID: b.groupID,
				FragmentIndex:  index,
				TotalFragments: b.totalFragments,
				OriginalSize:   uint32(len(b.originalData)),
				FragmentSize:   uint16(len(fragmentData)),
				SequenceNum:    b.sequenceNum,
				Data:           fragmentData,
			}

			data, err := pkt.Marshal()
			if err != nil {
				utils.Errorf("UDP: failed to marshal retransmit packet: %v", err)
				continue
			}

			if _, err := b.conn.WriteTo(data, b.addr); err != nil {
				utils.Errorf("UDP: failed to retransmit fragment %d: %v", index, err)
				continue
			}

			b.sequenceNum++
			utils.Debugf("UDP: retransmitted fragment %d/%d (retry %d) for groupID=%d",
				index, b.totalFragments, retryCount+1, b.groupID)
		}
	}
}

// IsComplete 检查是否已完成
func (b *UDPSendBuffer) IsComplete() bool {
	return atomic.LoadInt32(&b.completed) == 1
}

// Close 关闭发送缓冲区
func (b *UDPSendBuffer) Close() {
	b.cancel()
}

