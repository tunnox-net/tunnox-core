package udp

import (
	"net"
	"sync"
	"time"
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/utils"
)

// Sender 发送端逻辑：窗口、ACK、重传
type Sender struct {
	conn      *net.UDPConn
	remoteAddr *net.UDPAddr
	session   *SessionState
	cfg       *Config

	closeCh   chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
}

// Config 配置结构
type Config struct {
	SendWindowSize    uint16
	MaxRetransmit     int
	RetransmitTimeout time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		SendWindowSize:    DefaultSendWindowSize,
		MaxRetransmit:     DefaultMaxRetransmit,
		RetransmitTimeout: DefaultRetransmitTimeout,
	}
}

// NewSender 创建 Sender 并初始化 SessionState 窗口参数。
func NewSender(conn *net.UDPConn, remoteAddr *net.UDPAddr, session *SessionState, cfg *Config) *Sender {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	s := &Sender{
		conn:      conn,
		remoteAddr: remoteAddr,
		session:   session,
		cfg:       cfg,
		closeCh:   make(chan struct{}),
	}
	session.sendWindow = cfg.SendWindowSize
	session.maxWindow = cfg.SendWindowSize
	return s
}

// SendLogicalPacket 将一整个"逻辑包"发送出去。
func (s *Sender) SendLogicalPacket(payload []byte) error {
	utils.Infof("UDP sender: SendLogicalPacket called, payload size=%d, sessionID=%d, remoteAddr=%s, conn.RemoteAddr=%v", 
		len(payload), s.session.Key.SessionID, s.remoteAddr, s.conn.RemoteAddr())
	s.mu.Lock()
	defer s.mu.Unlock()

	s.session.sendMutex.Lock()

	// 检查窗口是否已满
	if len(s.session.inFlight) >= int(s.session.sendWindow) {
		s.session.sendMutex.Unlock()
		return errors.Newf(errors.ErrorTypeProtocol, "send window full: %d/%d", len(s.session.inFlight), s.session.sendWindow)
	}

	seq := s.session.nextSeq
	s.session.nextSeq++

	// 计算分片数量
	fragCount := (len(payload) + MaxDataPerDatagram - 1) / MaxDataPerDatagram
	if fragCount == 0 {
		fragCount = 1
	}

	// 注册 inFlight 状态
	state := &SendPacketState{
		Seq:       seq,
		Payload:   make([]byte, len(payload)),
		LastSend:  time.Now(),
		Retries:   0,
		FragCount: fragCount,
	}
	copy(state.Payload, payload)
	s.session.inFlight[seq] = state

	s.session.sendMutex.Unlock()

	// 发送所有分片
	if err := s.sendFragments(seq, payload, fragCount); err != nil {
		s.session.sendMutex.Lock()
		delete(s.session.inFlight, seq)
		s.session.sendMutex.Unlock()
		return errors.Wrap(err, errors.ErrorTypeNetwork, "failed to send fragments")
	}

	state.LastSend = time.Now()
	return nil
}

// sendFragments 发送一个逻辑包的所有分片
func (s *Sender) sendFragments(seq uint32, payload []byte, fragCount int) error {
	now := uint32(time.Now().UnixMilli())

	for fragSeq := uint16(0); fragSeq < uint16(fragCount); fragSeq++ {
		offset := int(fragSeq) * MaxDataPerDatagram
		end := offset + MaxDataPerDatagram
		if end > len(payload) {
			end = len(payload)
		}
		fragData := payload[offset:end]

		header := &TUTPHeader{
			Version:    TUTPVersion,
			Flags:      0,
			SessionID:  s.session.Key.SessionID,
			StreamID:   s.session.Key.StreamID,
			PacketSeq:  seq,
			FragSeq:    fragSeq,
			FragCount:  uint16(fragCount),
			AckSeq:     0,
			WindowSize: s.session.sendWindow,
			Reserved:   0,
			Timestamp:  now,
		}

		buf := make([]byte, HeaderLength()+len(fragData))
		if _, err := header.Encode(buf); err != nil {
			return errors.Wrap(err, errors.ErrorTypeProtocol, "failed to encode header")
		}
		copy(buf[HeaderLength():], fragData)

		// 对于已连接的 UDP socket（DialUDP），使用 Write() 而不是 WriteToUDP()
		// 对于未连接的 socket（ListenUDP），使用 WriteToUDP()
		// 检查 socket 是否已连接：如果 RemoteAddr() 不为 nil，说明已连接
		var err error
		if s.conn.RemoteAddr() != nil {
			// 已连接的 socket，使用 Write()
			_, err = s.conn.Write(buf)
			utils.Infof("UDP sender: using Write() for connected socket, sent fragment %d/%d of packet %d, size=%d", fragSeq+1, fragCount, seq, len(buf))
		} else {
			// 未连接的 socket，使用 WriteToUDP()
			_, err = s.conn.WriteToUDP(buf, s.remoteAddr)
			utils.Infof("UDP sender: using WriteToUDP() for unconnected socket, sent fragment %d/%d of packet %d to %s, size=%d", fragSeq+1, fragCount, seq, s.remoteAddr, len(buf))
		}
		if err != nil {
			return errors.Wrap(err, errors.ErrorTypeNetwork, "failed to write UDP")
		}
	}

	return nil
}

// HandleAck 处理从 Receiver 解析出的 AckSeq 和 WindowSize。
func (s *Sender) HandleAck(ackSeq uint32, windowSize uint16) {
	s.session.sendMutex.Lock()
	defer s.session.sendMutex.Unlock()

	// 移除 <= AckSeq 的 inFlight 状态
	for seq := range s.session.inFlight {
		if seq <= ackSeq {
			delete(s.session.inFlight, seq)
		}
	}

	// 更新 sendBase
	if ackSeq >= s.session.sendBase {
		s.session.sendBase = ackSeq + 1
	}

	// 更新窗口大小
	if windowSize > 0 {
		s.session.sendWindow = windowSize
		if s.session.sendWindow > s.session.maxWindow {
			s.session.sendWindow = s.session.maxWindow
		}
	}
}

// StartRetransmitLoop 启动重传检测循环，在独立 goroutine 运行。
func (s *Sender) StartRetransmitLoop() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.cfg.RetransmitTimeout / 2)
		defer ticker.Stop()

		for {
			select {
			case <-s.closeCh:
				return
			case <-ticker.C:
				s.checkAndRetransmit()
			}
		}
	}()
}

// checkAndRetransmit 检查并重传超时的包
func (s *Sender) checkAndRetransmit() {
	s.session.sendMutex.Lock()
	defer s.session.sendMutex.Unlock()

	now := time.Now()
	for seq, state := range s.session.inFlight {
		if now.Sub(state.LastSend) > s.cfg.RetransmitTimeout {
			if state.Retries >= s.cfg.MaxRetransmit {
				utils.Errorf("UDP sender: packet %d exceeded max retries, closing session", seq)
				// 通知上层关闭会话（通过关闭 closeCh）
				close(s.closeCh)
				return
			}

			// 重传
			fragCount := state.FragCount
			if err := s.sendFragments(seq, state.Payload, fragCount); err != nil {
				utils.Errorf("UDP sender: failed to retransmit packet %d: %v", seq, err)
				continue
			}

			state.Retries++
			state.LastSend = now
		}
	}
}

// Close 停止重传循环，释放资源。
func (s *Sender) Close() error {
	close(s.closeCh)
	s.wg.Wait()
	return nil
}

