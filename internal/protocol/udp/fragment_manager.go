package udp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/utils"
)

// UDPFragmentManager 分片管理器（同时管理发送和接收）
type UDPFragmentManager struct {
	// 发送缓冲区
	sendBuffers map[uint64]*UDPSendBuffer
	sendMu      sync.RWMutex

	// 接收缓冲区
	receiveBuffers map[uint64]*UDPReceiveBuffer
	receiveMu      sync.RWMutex

	// 控制
	ctx    context.Context
	cancel context.CancelFunc

	// 连接信息
	conn net.PacketConn
	addr net.Addr

	// 序列号生成
	groupIDCounter uint64
	groupIDMu      sync.Mutex

	// 接收数据 channel
	reassembledData chan []byte
}

// NewUDPFragmentManager 创建分片管理器
func NewUDPFragmentManager(ctx context.Context, conn net.PacketConn, addr net.Addr) *UDPFragmentManager {
	managerCtx, cancel := context.WithCancel(ctx)

	manager := &UDPFragmentManager{
		sendBuffers:     make(map[uint64]*UDPSendBuffer),
		receiveBuffers:  make(map[uint64]*UDPReceiveBuffer),
		ctx:             managerCtx,
		cancel:          cancel,
		conn:            conn,
		addr:            addr,
		reassembledData: make(chan []byte, 100), // 缓冲 100 个重组数据包
	}

	// 启动清理协程
	go manager.cleanupLoop()

	return manager
}

// generateGroupID 生成分片组ID
func (m *UDPFragmentManager) generateGroupID() uint64 {
	m.groupIDMu.Lock()
	defer m.groupIDMu.Unlock()
	m.groupIDCounter++
	return m.groupIDCounter
}

// SendFragmented 分片发送数据
func (m *UDPFragmentManager) SendFragmented(data []byte, onComplete func([]byte), onError func(error)) error {
	// 检查是否需要分片
	if len(data) <= UDPFragmentThreshold {
		// 小包直接发送
		if _, err := m.conn.WriteTo(data, m.addr); err != nil {
			return fmt.Errorf("failed to send data: %w", err)
		}
		if onComplete != nil {
			onComplete(data)
		}
		return nil
	}

	// 生成分片组ID
	groupID := m.generateGroupID()

	// 创建发送缓冲区
	sendBuf := NewUDPSendBuffer(m.ctx, m.conn, m.addr, data, groupID)
	sendBuf.onComplete = onComplete
	sendBuf.onError = onError

	// 注册发送缓冲区
	m.sendMu.Lock()
	if len(m.sendBuffers) >= UDPMaxSendBuffers {
		// 清理最旧的缓冲区
		m.cleanupOldSendBuffersLocked()
		if len(m.sendBuffers) >= UDPMaxSendBuffers {
			m.sendMu.Unlock()
			return fmt.Errorf("too many send buffers: %d", len(m.sendBuffers))
		}
	}
	m.sendBuffers[groupID] = sendBuf
	m.sendMu.Unlock()

	// 发送所有分片
	if err := sendBuf.Send(); err != nil {
		m.sendMu.Lock()
		delete(m.sendBuffers, groupID)
		m.sendMu.Unlock()
		sendBuf.Close()
		return fmt.Errorf("failed to send fragments: %w", err)
	}

	return nil
}

// HandlePacket 处理接收到的数据包
func (m *UDPFragmentManager) HandlePacket(data []byte) error {
	// 检查是否为 ACK 包
	if IsACKPacket(data) {
		return m.handleACK(data)
	}

	// 检查是否为分片包
	if IsFragmentPacket(data) {
		return m.handleFragment(data)
	}

	// 普通数据包，直接发送到重组 channel
	select {
	case m.reassembledData <- data:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	default:
		utils.Warnf("UDP: reassembledData channel full, dropping packet")
		return nil
	}
}

// handleACK 处理 ACK 包
func (m *UDPFragmentManager) handleACK(data []byte) error {
	ack, err := UnmarshalACKPacket(data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal ACK: %w", err)
	}

	m.sendMu.RLock()
	sendBuf, exists := m.sendBuffers[ack.FragmentGroupID]
	m.sendMu.RUnlock()

	if exists {
		sendBuf.HandleACK(ack)

		// 如果已完成，清理缓冲区
		if sendBuf.IsComplete() {
			m.sendMu.Lock()
			delete(m.sendBuffers, ack.FragmentGroupID)
			m.sendMu.Unlock()
			sendBuf.Close()
		}
	} else {
		utils.Debugf("UDP: received ACK for unknown groupID=%d", ack.FragmentGroupID)
	}

	return nil
}

// handleFragment 处理分片包
func (m *UDPFragmentManager) handleFragment(data []byte) error {
	pkt, err := UnmarshalFragmentPacket(data)
	if err != nil {
		return fmt.Errorf("failed to unmarshal fragment: %w", err)
	}

	m.receiveMu.Lock()

	// 查找或创建接收缓冲区
	receiveBuf, exists := m.receiveBuffers[pkt.FragmentGroupID]
	if !exists {
		// 检查缓冲区数量限制
		if len(m.receiveBuffers) >= UDPMaxReceiveBuffers {
			// 清理过期的缓冲区
			m.cleanupExpiredReceiveBuffersLocked()
			if len(m.receiveBuffers) >= UDPMaxReceiveBuffers {
				m.receiveMu.Unlock()
				return fmt.Errorf("too many receive buffers: %d", len(m.receiveBuffers))
			}
		}

		// 创建新缓冲区
		receiveBuf = NewUDPReceiveBuffer(m.ctx, m.conn, m.addr, pkt.FragmentGroupID, pkt.OriginalSize, pkt.TotalFragments)
		receiveBuf.onTimeout = func() {
			m.receiveMu.Lock()
			delete(m.receiveBuffers, pkt.FragmentGroupID)
			m.receiveMu.Unlock()
		}

		// 启动数据读取协程
		go func() {
			reassembledData, err := receiveBuf.GetReassembledData(UDPBufferTimeout)
			if err != nil {
				utils.Debugf("UDP: failed to get reassembled data: %v", err)
				return
			}

			// 发送到重组 channel
			select {
			case m.reassembledData <- reassembledData:
				// 成功
			case <-time.After(100 * time.Millisecond):
				utils.Warnf("UDP: reassembledData channel full, dropping reassembled data")
			}

			// 清理缓冲区
			m.receiveMu.Lock()
			delete(m.receiveBuffers, pkt.FragmentGroupID)
			m.receiveMu.Unlock()
			receiveBuf.Close()
		}()

		m.receiveBuffers[pkt.FragmentGroupID] = receiveBuf
	}

	m.receiveMu.Unlock()

	// 添加分片
	return receiveBuf.AddFragment(pkt)
}

// ReadReassembledData 读取重组后的数据
func (m *UDPFragmentManager) ReadReassembledData(timeout time.Duration) ([]byte, error) {
	select {
	case data := <-m.reassembledData:
		return data, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for reassembled data")
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	}
}

// cleanupOldSendBuffersLocked 清理旧的发送缓冲区（需要持有锁）
func (m *UDPFragmentManager) cleanupOldSendBuffersLocked() {
	now := time.Now()
	var oldestGroupID uint64
	var oldestTime time.Time

	for groupID, sendBuf := range m.sendBuffers {
		if sendBuf.IsComplete() {
			delete(m.sendBuffers, groupID)
			sendBuf.Close()
			continue
		}

		// 查找最旧的未完成缓冲区
		sendBuf.mu.RLock()
		if len(sendBuf.sentFragments) > 0 {
			for _, sentTime := range sendBuf.sentFragments {
				if oldestTime.IsZero() || sentTime.Before(oldestTime) {
					oldestTime = sentTime
					oldestGroupID = groupID
				}
			}
		}
		sendBuf.mu.RUnlock()
	}

	// 如果找到最旧的且已超时，删除它
	if oldestGroupID != 0 && !oldestTime.IsZero() && now.Sub(oldestTime) > UDPBufferTimeout {
		if sendBuf, exists := m.sendBuffers[oldestGroupID]; exists {
			delete(m.sendBuffers, oldestGroupID)
			sendBuf.Close()
			utils.Debugf("UDP: cleaned up old send buffer, groupID=%d", oldestGroupID)
		}
	}
}

// cleanupExpiredReceiveBuffersLocked 清理过期的接收缓冲区（需要持有锁）
func (m *UDPFragmentManager) cleanupExpiredReceiveBuffersLocked() {
	for groupID, receiveBuf := range m.receiveBuffers {
		if receiveBuf.IsExpired() {
			delete(m.receiveBuffers, groupID)
			receiveBuf.Close()
			utils.Debugf("UDP: cleaned up expired receive buffer, groupID=%d", groupID)
		}
	}
}

// cleanupLoop 定期清理
func (m *UDPFragmentManager) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// 清理发送缓冲区
			m.sendMu.Lock()
			m.cleanupOldSendBuffersLocked()
			m.sendMu.Unlock()

			// 清理接收缓冲区
			m.receiveMu.Lock()
			m.cleanupExpiredReceiveBuffersLocked()
			m.receiveMu.Unlock()
		}
	}
}

// Close 关闭管理器
func (m *UDPFragmentManager) Close() {
	m.cancel()

	// 清理所有缓冲区
	m.sendMu.Lock()
	for _, sendBuf := range m.sendBuffers {
		sendBuf.Close()
	}
	m.sendBuffers = make(map[uint64]*UDPSendBuffer)
	m.sendMu.Unlock()

	m.receiveMu.Lock()
	for _, receiveBuf := range m.receiveBuffers {
		receiveBuf.Close()
	}
	m.receiveBuffers = make(map[uint64]*UDPReceiveBuffer)
	m.receiveMu.Unlock()
}

