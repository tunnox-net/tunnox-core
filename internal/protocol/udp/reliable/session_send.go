package reliable

import (
	"fmt"
	"sync"
	"time"
)

// 发送相关常量
const (
	// SendQueueSize 发送队列大小
	SendQueueSize = 100
	// SendWindowCheckInterval 发送窗口检查间隔
	SendWindowCheckInterval = 5 * time.Millisecond
)

// 数据包缓冲池，减少内存分配
var packetPool = sync.Pool{
	New: func() interface{} {
		return &Packet{
			Header: &PacketHeader{},
		}
	},
}

// getPacketFromPool 从池中获取数据包
func getPacketFromPool() *Packet {
	pkt := packetPool.Get().(*Packet)
	// 重置 header
	pkt.Header.Version = 0
	pkt.Header.Type = 0
	pkt.Header.Flags = 0
	pkt.Header.Reserved = 0
	pkt.Header.SessionID = 0
	pkt.Header.StreamID = 0
	pkt.Header.SequenceNum = 0
	pkt.Header.AckNum = 0
	pkt.Header.WindowSize = 0
	pkt.Header.PayloadLen = 0
	pkt.Header.Timestamp = 0
	pkt.Payload = nil
	return pkt
}

// putPacketToPool 将数据包放回池中
func putPacketToPool(pkt *Packet) {
	if pkt != nil {
		pkt.Payload = nil // 释放 payload 引用
		packetPool.Put(pkt)
	}
}

// sendPacketDirect sends a packet directly without buffering
func (s *Session) sendPacketDirect(packet *Packet) error {
	if s.dispatcher != nil {
		return s.dispatcher.Send(packet, s.remoteAddr)
	}

	// Fallback if no dispatcher
	data := EncodePacket(packet)

	var err error
	if s.conn.RemoteAddr() != nil {
		_, err = s.conn.Write(data)
	} else {
		_, err = s.conn.WriteToUDP(data, s.remoteAddr)
	}

	return err
}

// sendDataPacket sends a data packet and buffers it for retransmission
// Implements flow control and congestion control
func (s *Session) sendDataPacket(data []byte) error {
	offset := 0
	for offset < len(data) {
		// Wait for flow control and congestion control
		if err := s.waitForSendWindow(); err != nil {
			return err
		}

		// Calculate chunk size
		chunkSize := len(data) - offset
		if chunkSize > MaxPayloadSize {
			chunkSize = MaxPayloadSize
		}

		// Get chunk
		chunk := data[offset : offset+chunkSize]
		seq := s.getNextSendSeq()

		// Create packet
		packet := &Packet{
			Header: &PacketHeader{
				Version:     Version,
				Type:        PacketTypeData,
				Flags:       FlagNone,
				SessionID:   s.sessionID,
				StreamID:    s.streamID,
				SequenceNum: seq,
				AckNum:      0,
				WindowSize:  uint16(s.flowController.GetReceiveWindow()),
				PayloadLen:  uint16(len(chunk)),
				Timestamp:   uint32(time.Now().UnixMilli()),
			},
			Payload: chunk,
		}

		// Add to buffer manager
		if err := s.bufferManager.AddSendBuffer(seq, packet); err != nil {
			// 缓冲区满，先清理已确认的包
			s.bufferManager.CleanupAcked()
			// 移除高频日志，避免日志噪音
			// cleaned := s.bufferManager.CleanupAcked()
			// s.logger.Debugf("Session: send buffer full, cleaned %d acked packets, seq=%d", cleaned, seq)
			
			// 清理后重试
			if err := s.bufferManager.AddSendBuffer(seq, packet); err != nil {
				// 仍然满，等待一段时间让 ACK 到达
				// 移除高频日志，避免日志噪音（缓冲区满时等待是正常的流控行为）
				// s.logger.Warnf("Session: send buffer still full after cleanup, seq=%d", seq)
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		// Send
		if err := s.sendPacketDirect(packet); err != nil {
			return err
		}

		// Update statistics
		s.statsMu.Lock()
		s.stats.packetsSent++
		s.stats.bytesSent += uint64(len(chunk))
		s.statsMu.Unlock()

		offset += chunkSize
	}

	return nil
}

// waitForSendWindow waits until flow control and congestion control allow sending
func (s *Session) waitForSendWindow() error {
	ticker := time.NewTicker(SendWindowCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.closeChan:
			return fmt.Errorf("session closed")
		case <-s.ctx.Done():
			return fmt.Errorf("session context cancelled")
		default:
		}

		// Check congestion window
		cwnd := s.congestionController.GetCwnd()
		unackedCount := s.bufferManager.GetUnackedCount()

		// Check flow control window
		sendWindow := s.flowController.GetSendWindow()

		// Can send if both conditions are met
		if unackedCount < cwnd && sendWindow > 0 {
			return nil
		}

		select {
		case <-ticker.C:
			continue
		case <-s.closeChan:
			return fmt.Errorf("session closed")
		case <-s.ctx.Done():
			return fmt.Errorf("session context cancelled")
		}
	}
}

// sendDataACK sends an acknowledgment for a data packet
func (s *Session) sendDataACK(ackNum uint32) {
	recvWindow := s.flowController.GetReceiveWindow()
	windowSize := uint16(recvWindow)
	if recvWindow > 65535 {
		windowSize = 65535
	}

	ackPacket := &Packet{
		Header: &PacketHeader{
			Version:     Version,
			Type:        PacketTypeDataACK,
			Flags:       FlagNone,
			SessionID:   s.sessionID,
			StreamID:    s.streamID,
			SequenceNum: 0,
			AckNum:      ackNum,
			WindowSize:  windowSize,
			PayloadLen:  0,
			Timestamp:   uint32(time.Now().UnixMilli()),
		},
	}

	s.sendPacketDirect(ackPacket)
}

// sendLoop handles outgoing data from send queue
// Lifecycle: Managed by session context
// Cleanup: Triggered by session.Close() via closeChan or ctx.Done()
// Shutdown: Processes remaining queued data before exit
func (s *Session) sendLoop() {
	defer s.wg.Done()

	for {
		select {
		case data := <-s.sendQueue:
			// Update activity timestamp when sending data
			s.updateActivity()
			if err := s.sendDataPacket(data); err != nil {
				s.logger.Errorf("Session: failed to send data: %v", err)
			}
		case <-s.closeChan:
			return
		case <-s.ctx.Done():
			return
		}
	}
}

// retransmitLoop handles packet retransmission
// Lifecycle: Managed by session context
// Cleanup: Triggered by session.Close() via closeChan or ctx.Done()
// Shutdown: Waits for ticker cleanup before exit
func (s *Session) retransmitLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkRetransmissions()
		case <-s.closeChan:
			return
		case <-s.ctx.Done():
			return
		}
	}
}

// checkRetransmissions checks for packets that need retransmission
func (s *Session) checkRetransmissions() {
	s.rttMu.Lock()
	rto := time.Duration(s.rto) * time.Millisecond
	s.rttMu.Unlock()

	now := time.Now()
	
	// 使用 BufferManager 的方法遍历未确认的包
	s.bufferManager.ForEachUnacked(func(seq uint32, entry *BufferEntry) bool {
		// Check if timeout
		if now.Sub(entry.Timestamp) < rto {
			return true // continue
		}

		// Check max retries
		if entry.RetryCount >= MaxRetries {
			s.logger.Errorf("Session: packet %d exceeded max retries, closing session %d", seq, s.sessionID)
			go s.Close()
			return false // stop iteration
		}

		// Retransmit
		// 移除高频日志，避免日志噪音（重传是正常的网络行为）
		// 如需调试，可以临时启用此日志
		// s.logger.Warnf("Session: retransmitting packet seq=%d (retry %d/%d)",
		// 	seq, entry.RetryCount+1, MaxRetries)

		entry.Packet.Header.Flags |= FlagRetransmission
		entry.Packet.Header.Timestamp = uint32(time.Now().UnixMilli())

		if err := s.sendPacketDirect(entry.Packet); err != nil {
			s.logger.Errorf("Session: failed to retransmit: %v", err)
			return true // continue
		}

		// Update entry in buffer manager
		s.bufferManager.UpdateRetransmit(seq, now)

		// Update statistics
		s.statsMu.Lock()
		s.stats.packetsRetrans++
		s.statsMu.Unlock()

		// Congestion event on first retry
		if entry.RetryCount == 0 {
			s.congestionController.OnTimeout()
		}

		return true // continue
	})
}

// keepAliveLoop sends periodic keep-alive packets and checks for idle timeout
// Lifecycle: Managed by session context
// Cleanup: Triggered by session.Close() via closeChan or ctx.Done()
// Shutdown: Waits for ticker cleanup before exit
func (s *Session) keepAliveLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(KeepAliveInterval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.getState() != StateEstablished {
				continue
			}

			// Check for idle timeout
			idleTime := time.Since(s.getLastActivity())
			idleTimeoutDuration := time.Duration(SessionIdleTimeout) * time.Millisecond

			if idleTime > idleTimeoutDuration {
				s.logger.Warnf("Session: idle timeout (%.1f minutes), closing session %d",
					idleTime.Minutes(), s.sessionID)
				go s.Close()
				return
			}

			// Send keep-alive packet
			s.sendDataACK(0)

		case <-s.closeChan:
			return
		case <-s.ctx.Done():
			return
		}
	}
}
