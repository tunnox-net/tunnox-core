package udp

import (
	"net"
	"sync"
	"time"
	"tunnox-core/internal/utils"
)

// Receiver 接收端逻辑：分片收集、重组、ACK 生成
type Receiver struct {
	conn      *net.UDPConn
	remoteAddr *net.UDPAddr
	session   *SessionState
	sender    *Sender // 用于发送 ACK

	// 将完整逻辑包交给上层的回调
	onPacket func(payload []byte) error

	closeCh  chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
}

// NewReceiver 创建新的接收端
func NewReceiver(conn *net.UDPConn, remoteAddr *net.UDPAddr, session *SessionState, sender *Sender, onPacket func([]byte) error) *Receiver {
	return &Receiver{
		conn:      conn,
		remoteAddr: remoteAddr,
		session:   session,
		sender:    sender,
		onPacket:  onPacket,
		closeCh:   make(chan struct{}),
	}
}

// StartReadLoop 启动 UDP 读循环
func (r *Receiver) StartReadLoop() {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		buf := make([]byte, MaxUDPPayloadSize)
		for {
			select {
			case <-r.closeCh:
				return
			default:
			}

			// 设置读取超时，以便能响应 closeCh
			if err := r.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
				utils.Errorf("UDP receiver: failed to set read deadline: %v", err)
				return
			}

			n, remoteAddr, err := r.conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if !r.isClosed() {
					utils.Errorf("UDP receiver: read error: %v", err)
				}
				return
			}

			utils.Infof("UDP receiver: received datagram, size=%d, from=%s, sessionID=%d, expectedRemoteAddr=%v", 
				n, remoteAddr, r.session.Key.SessionID, r.remoteAddr)

			// 检查 remoteAddr 是否匹配（服务端需要过滤不同客户端的数据报）
			if r.remoteAddr != nil {
				if remoteAddr.IP.String() != r.remoteAddr.IP.String() || remoteAddr.Port != r.remoteAddr.Port {
					// 不匹配的 remoteAddr，丢弃（这是正常的，因为多个 Transport 共享 listener）
					utils.Debugf("UDP receiver: remoteAddr mismatch, expected %s, got %s, dropping", r.remoteAddr, remoteAddr)
					continue
				}
			}

			// 更新活跃时间
			r.session.UpdateLastActive()

			// 处理数据报
			r.handleDatagram(buf[:n], remoteAddr)
		}
	}()

	// 启动清理循环
	r.wg.Add(1)
	go r.cleanupLoop()
}

// HandleDatagram 处理单个 datagram（公开方法，用于 Accept 时传递第一个数据报）
func (r *Receiver) HandleDatagram(data []byte, remoteAddr *net.UDPAddr) {
	r.handleDatagram(data, remoteAddr)
}

// handleDatagram 处理单个 datagram（内部方法）
func (r *Receiver) handleDatagram(data []byte, remoteAddr *net.UDPAddr) {
	if len(data) < HeaderLength() {
		return
	}

	header, _, err := DecodeHeader(data)
	if err != nil {
		utils.Debugf("UDP receiver: failed to decode header: %v", err)
		return
	}

	// 校验 SessionID/StreamID（这是关键过滤条件，因为多个 Transport 共享 listener）
	if header.SessionID != r.session.Key.SessionID || header.StreamID != r.session.Key.StreamID {
		// SessionID 不匹配，这是正常的（多个 Transport 共享 listener，需要过滤）
		return
	}

	payload := data[HeaderLength():]

	// 若 Flags 带 ACK，则调用 Sender.HandleAck
	if header.Flags&FlagACK != 0 {
		if r.sender != nil {
			r.sender.HandleAck(header.AckSeq, header.WindowSize)
		}
	}

	// 处理数据分片
	if len(payload) > 0 {
		r.handleFragment(header, payload)
	}
}

// handleFragment 处理分片
func (r *Receiver) handleFragment(header *TUTPHeader, payload []byte) {
	r.session.recvMutex.Lock()
	defer r.session.recvMutex.Unlock()

	key := FragmentGroupKey{
		SessionID: header.SessionID,
		StreamID:  header.StreamID,
		PacketSeq: header.PacketSeq,
	}

	// 获取或创建分片组
	group, exists := r.session.fragments[key]
	if !exists {
		// 估算原始大小（最后一个分片可能不完整）
		estimatedSize := (int(header.FragCount)-1)*MaxDataPerDatagram + len(payload)
		group = NewFragmentGroup(key, int(header.FragCount), estimatedSize)
		r.session.fragments[key] = group
	}

	// 添加分片
	if err := group.AddFragment(int(header.FragSeq), payload); err != nil {
		utils.Debugf("UDP receiver: failed to add fragment: %v", err)
		return
	}

	// 如果完整，尝试重组和交付
	if group.IsComplete() {
		// 重新计算实际大小
		actualSize := 0
		for _, frag := range group.Fragments {
			actualSize += len(frag)
		}
		group.OriginalSize = actualSize

		payload, err := group.Reassemble()
		if err != nil {
			utils.Errorf("UDP receiver: failed to reassemble: %v", err)
			delete(r.session.fragments, key)
			return
		}

		// 按序交付（reliable-ordered 模式）
		if header.PacketSeq == r.session.recvBase+1 {
			// 立即交付
			if r.onPacket != nil {
				if err := r.onPacket(payload); err != nil {
					utils.Errorf("UDP receiver: onPacket callback failed: %v", err)
				}
			}
			r.session.recvBase++

			// 检查是否有后续已完成的包可以交付
			r.deliverPendingPackets()
		} else if header.PacketSeq > r.session.recvBase+1 {
			// 标记为已完成但待交付，等待前面的包到齐
			// 这里可以维护一个待交付队列，简化实现先不做
		} else {
			// PacketSeq <= recvBase，可能是重复包，忽略
		}

		// 删除已处理的分片组
		delete(r.session.fragments, key)

		// 发送 ACK
		r.sendAck(header.PacketSeq)
	}
}

// deliverPendingPackets 交付待交付的包
func (r *Receiver) deliverPendingPackets() {
	for {
		nextSeq := r.session.recvBase + 1
		key := FragmentGroupKey{
			SessionID: r.session.Key.SessionID,
			StreamID:  r.session.Key.StreamID,
			PacketSeq: nextSeq,
		}

		group, exists := r.session.fragments[key]
		if !exists || !group.IsComplete() {
			break
		}

		payload, err := group.Reassemble()
		if err != nil {
			utils.Errorf("UDP receiver: failed to reassemble pending packet %d: %v", nextSeq, err)
			delete(r.session.fragments, key)
			break
		}

		if r.onPacket != nil {
			if err := r.onPacket(payload); err != nil {
				utils.Errorf("UDP receiver: onPacket callback failed for packet %d: %v", nextSeq, err)
			}
		}
		r.session.recvBase++
		delete(r.session.fragments, key)
		r.sendAck(nextSeq)
	}
}

// sendAck 发送 ACK
func (r *Receiver) sendAck(ackSeq uint32) {
	if r.conn == nil || r.remoteAddr == nil {
		return
	}

	header := &TUTPHeader{
		Version:    TUTPVersion,
		Flags:      FlagACK,
		SessionID:  r.session.Key.SessionID,
		StreamID:   r.session.Key.StreamID,
		PacketSeq:  0,
		FragSeq:    0,
		FragCount:  1,
		AckSeq:     ackSeq,
		WindowSize: uint16(DefaultRecvWindowSize - r.session.GetFragmentGroupCount()),
		Reserved:   0,
		Timestamp:  uint32(time.Now().UnixMilli()),
	}

	buf := make([]byte, HeaderLength())
	if _, err := header.Encode(buf); err != nil {
		utils.Errorf("UDP receiver: failed to encode ACK header: %v", err)
		return
	}

	// 对于已连接的 UDP socket（DialUDP），使用 Write() 而不是 WriteToUDP()
	// 对于未连接的 socket（ListenUDP），使用 WriteToUDP()
	if r.conn.RemoteAddr() != nil {
		// 已连接的 socket，使用 Write()
		if _, err := r.conn.Write(buf); err != nil {
			utils.Errorf("UDP receiver: failed to send ACK: %v", err)
		}
	} else {
		// 未连接的 socket，使用 WriteToUDP()
		if _, err := r.conn.WriteToUDP(buf, r.remoteAddr); err != nil {
			utils.Errorf("UDP receiver: failed to send ACK: %v", err)
		}
	}
}

// cleanupLoop 清理过期的分片组
func (r *Receiver) cleanupLoop() {
	defer r.wg.Done()
	ticker := time.NewTicker(DefaultFragmentGroupTTL / 2)
	defer ticker.Stop()

	for {
		select {
		case <-r.closeCh:
			return
		case <-ticker.C:
			r.cleanupExpiredFragments()
		}
	}
}

// cleanupExpiredFragments 清理过期的分片组
func (r *Receiver) cleanupExpiredFragments() {
	r.session.recvMutex.Lock()
	defer r.session.recvMutex.Unlock()

	now := time.Now()
	for key, group := range r.session.fragments {
		if now.Sub(group.LastAccessTime) > DefaultFragmentGroupTTL {
			delete(r.session.fragments, key)
		}
	}
}

// isClosed 检查是否已关闭
func (r *Receiver) isClosed() bool {
	select {
	case <-r.closeCh:
		return true
	default:
		return false
	}
}

// Close 停止读循环，释放资源。
func (r *Receiver) Close() error {
	close(r.closeCh)
	r.wg.Wait()
	return nil
}

