package reliable

import (
	"math"
	"time"
)

// handleData handles data packet
func (s *Session) handleData(packet *Packet) {
	if s.getState() != StateEstablished {
		// 移除高频日志，避免日志噪音（连接建立/关闭过程中收到数据包是正常的）
		// s.logger.Warnf("Session: received data in non-established state, ignoring")
		return
	}

	// Update activity timestamp when receiving data
	s.updateActivity()

	seq := packet.Header.SequenceNum
	dataSize := uint32(len(packet.Payload))
	expectedSeq := s.getExpectedRecvSeq()

	if seq == expectedSeq {
		// In-order packet, deliver immediately
		s.flowController.OnDataReceived(dataSize)
		s.deliverData(packet.Payload)
		s.flowController.OnDataConsumed(dataSize)
		s.incrementRecvSeq()

		// Check for buffered packets that are now in order
		s.deliverBufferedData()
	} else if seq > expectedSeq {
		// Out-of-order packet, buffer it
		if err := s.bufferManager.AddRecvBuffer(seq, packet); err != nil {
			// 缓冲区满时，尝试清理陈旧的包（序列号间隔超过 1000 的旧包）
			cleaned := s.bufferManager.CleanupStaleRecvPackets(expectedSeq, 1000)
			if cleaned > 0 {
				s.logger.Infof("Session: cleaned %d stale recv packets to make room", cleaned)
				// 清理后重试
				if err := s.bufferManager.AddRecvBuffer(seq, packet); err == nil {
					return // 成功添加
				}
			}
			
			// 仍然满，记录警告
			_, recvBufSize := s.bufferManager.GetStats()
			s.logger.Warnf("Session: recv buffer full (%d/%d), dropping packet seq=%d (expected=%d)",
				recvBufSize, MaxRecvBufSize, seq, expectedSeq)
		}
		// 移除高频日志，避免日志噪音
		// 如需调试，可以临时启用此日志
		// s.logger.Debugf("Session: buffered out-of-order packet seq=%d (expected=%d)", seq, expectedSeq)
	} else {
		// Duplicate or old packet, ignore
		// 移除高频日志，避免日志噪音
		// s.logger.Debugf("Session: ignoring old/duplicate packet seq=%d (expected=%d)", seq, expectedSeq)
	}

	// Send ACK
	s.sendDataACK(packet.Header.SequenceNum)
}

// handleDataACK handles data acknowledgment
func (s *Session) handleDataACK(packet *Packet) {
	ackNum := packet.Header.AckNum
	entry := s.bufferManager.GetSendBuffer(ackNum)

	if entry != nil && !entry.Acked {
		// Calculate RTT
		rtt := time.Since(entry.Timestamp).Milliseconds()
		payloadLen := len(entry.Packet.Payload)

		// Mark as acked
		s.bufferManager.MarkAcked(ackNum)

		// Update RTT and RTO
		s.updateRTTValue(int(rtt))

		// Update congestion controller
		s.congestionController.OnAck(ackNum, payloadLen)

		// 移除高频日志，避免日志噪音
		// s.logger.Debugf("Session: ACK received for seq=%d, RTT=%dms, peerWindow=%d",
		// 	ackNum, rtt, packet.Header.WindowSize)
	} else {
		// Duplicate ACK - notify congestion controller
		s.congestionController.OnAck(ackNum, 0)
	}

	// Clean up acknowledged packets
	s.cleanupAckedPackets()
}

// deliverData delivers received data to the application
func (s *Session) deliverData(data []byte) {
	// Check if session is closing before writing to pipe
	if s.getState() == StateClosed {
		return
	}

	_, err := s.pipeWriter.Write(data)
	if err != nil {
		// Only log if it's not a closed pipe error (which is expected during shutdown)
		if err.Error() != "io: read/write on closed pipe" {
			s.logger.Errorf("Session: failed to write to pipe: %v", err)
		}
	}
}

// deliverBufferedData delivers any buffered in-order packets
func (s *Session) deliverBufferedData() {
	for {
		expectedSeq := s.getExpectedRecvSeq()
		entry := s.bufferManager.GetRecvBuffer(expectedSeq)

		if entry == nil {
			break
		}

		// Remove from buffer
		s.bufferManager.RemoveRecvBuffer(expectedSeq)

		// Update flow controller
		dataSize := uint32(len(entry.Packet.Payload))
		s.flowController.OnDataReceived(dataSize)

		// Deliver data
		s.deliverData(entry.Packet.Payload)

		// Mark as consumed
		s.flowController.OnDataConsumed(dataSize)

		// Increment expected sequence
		s.incrementRecvSeq()
	}
}

// cleanupAckedPackets removes acknowledged packets from buffer
func (s *Session) cleanupAckedPackets() {
	// 立即清理所有已确认的包
	s.bufferManager.CleanupAcked()
}

// updateRTT updates RTT measurement from timestamp
func (s *Session) updateRTT(timestamp uint32) {
	now := uint32(time.Now().UnixMilli())
	if now > timestamp {
		rtt := int(now - timestamp)
		s.updateRTTValue(rtt)
	}
}

// updateRTTValue updates RTT and RTO using Jacobson's algorithm
func (s *Session) updateRTTValue(rtt int) {
	s.rttMu.Lock()
	defer s.rttMu.Unlock()

	if s.srtt == 0 {
		// First measurement
		s.srtt = float64(rtt)
		s.rttvar = float64(rtt) / 2
	} else {
		// Exponential weighted moving average
		diff := math.Abs(float64(rtt) - s.srtt)
		s.rttvar = (1-RTTBeta)*s.rttvar + RTTBeta*diff
		s.srtt = (1-RTTAlpha)*s.srtt + RTTAlpha*float64(rtt)
	}

	// Calculate RTO (Karn's algorithm)
	rto := int(s.srtt + 4*s.rttvar)
	if rto < MinRTO {
		rto = MinRTO
	}
	if rto > MaxRTO {
		rto = MaxRTO
	}
	s.rto = rto

	// 移除高频日志，避免日志噪音
	// s.logger.Debugf("Session: RTT updated - srtt=%.2fms, rttvar=%.2fms, rto=%dms",
	// 	s.srtt, s.rttvar, s.rto)
}

// GetStats returns session statistics
func (s *Session) GetStats() (sent, received, retrans, bytesRx, bytesTx uint64) {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	return s.stats.packetsSent, s.stats.packetsReceived, s.stats.packetsRetrans,
		s.stats.bytesReceived, s.stats.bytesSent
}
