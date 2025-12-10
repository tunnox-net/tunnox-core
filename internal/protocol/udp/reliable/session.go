package reliable

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"tunnox-core/internal/core/dispose"
)

// Session represents a reliable UDP session
type Session struct {
	*dispose.ResourceBase

	// Connection info
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	sessionID  uint32
	streamID   uint32
	isClient   bool

	// State
	state      SessionState
	stateMu    sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	dispatcher *PacketDispatcher
	logger     *logrus.Logger

	// Sequence numbers
	sendSeq   uint32
	sendSeqMu sync.Mutex
	recvSeq   uint32
	recvSeqMu sync.Mutex

	// RTT and RTO
	srtt   float64
	rttvar float64
	rto    int
	rttMu  sync.Mutex

	// Modular controllers
	flowController       *FlowController
	congestionController *CongestionController
	bufferManager        *BufferManager

	// Data channels
	sendQueue chan []byte
	closeChan chan struct{}
	closeOnce sync.Once

	// Pipe for streaming data
	pipeReader *io.PipeReader
	pipeWriter *io.PipeWriter

	// Activity tracking for idle timeout
	lastActivity time.Time
	activityMu   sync.RWMutex

	// Statistics
	stats struct {
		packetsSent     uint64
		packetsReceived uint64
		packetsRetrans  uint64
		bytesReceived   uint64
		bytesSent       uint64
	}
	statsMu sync.RWMutex

	// Goroutine management
	wg sync.WaitGroup
}

// NewSession creates a new session with dispose support
func NewSession(conn *net.UDPConn, remoteAddr *net.UDPAddr, sessionID, streamID uint32, isClient bool, logger *logrus.Logger) *Session {
	pr, pw := io.Pipe()

	s := &Session{
		ResourceBase:         dispose.NewResourceBase("UDPSession"),
		conn:                 conn,
		remoteAddr:           remoteAddr,
		sessionID:            sessionID,
		streamID:             streamID,
		isClient:             isClient,
		state:                StateInit,
		logger:               logger,
		srtt:                 0,
		rttvar:               0,
		rto:                  InitialRTO,
		flowController:       NewFlowController(),
		congestionController: NewCongestionController(),
		bufferManager:        NewBufferManager(),
		sendQueue:            make(chan []byte, SendQueueSize),
		closeChan:            make(chan struct{}),
		pipeReader:           pr,
		pipeWriter:           pw,
		lastActivity:         time.Now(),
	}

	// Initialize with background context (will be replaced when dispatcher is set)
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Add cleanup handler
	s.AddCleanHandler(s.onClose)

	// Start background goroutines
	s.wg.Add(3)
	go s.retransmitLoop()
	go s.sendLoop()
	go s.keepAliveLoop()

	return s
}

// NewSessionWithContext creates a new session with parent context
func NewSessionWithContext(parentCtx context.Context, conn *net.UDPConn, remoteAddr *net.UDPAddr, sessionID, streamID uint32, isClient bool, logger *logrus.Logger) *Session {
	s := NewSession(conn, remoteAddr, sessionID, streamID, isClient, logger)
	s.Initialize(parentCtx)
	return s
}

// SetDispatcher sets the packet dispatcher
func (s *Session) SetDispatcher(d *PacketDispatcher) {
	s.dispatcher = d
}

// Connect initiates a connection (client side)
func (s *Session) Connect() error {
	if !s.isClient {
		return fmt.Errorf("Connect() can only be called on client sessions")
	}

	s.logger.Infof("Session: initiating connection to %s, session=%d", s.remoteAddr, s.sessionID)

	synPacket := &Packet{
		Header: &PacketHeader{
			Version:     Version,
			Type:        PacketTypeSYN,
			Flags:       FlagNone,
			SessionID:   s.sessionID,
			StreamID:    s.streamID,
			SequenceNum: s.getNextSendSeq(),
			AckNum:      0,
			WindowSize:  uint16(s.flowController.GetReceiveWindow()),
			PayloadLen:  0,
			Timestamp:   uint32(time.Now().UnixMilli()),
		},
	}

	if err := s.sendPacketDirect(synPacket); err != nil {
		return fmt.Errorf("failed to send SYN: %w", err)
	}

	s.setState(StateSynSent)

	// Wait for connection establishment
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("connection timeout")
		case <-ticker.C:
			if s.getState() == StateEstablished {
				s.logger.Infof("Session: connection established to %s", s.remoteAddr)
				return nil
			}
		case <-s.ctx.Done():
			return fmt.Errorf("session closed")
		}
	}
}

// HandlePacket handles an incoming packet
func (s *Session) HandlePacket(packet *Packet) {
	// 移除高频日志，避免日志噪音
	// 如需调试，可以临时启用此日志
	// s.logger.Debugf("Session: handling %s packet, seq=%d, ack=%d",
	// 	packet.Header.Type, packet.Header.SequenceNum, packet.Header.AckNum)

	// Update statistics
	s.statsMu.Lock()
	s.stats.packetsReceived++
	s.stats.bytesReceived += uint64(packet.Header.PayloadLen)
	s.statsMu.Unlock()

	// Update flow controller with peer's window
	s.flowController.UpdateSendWindow(uint32(packet.Header.WindowSize))

	switch packet.Header.Type {
	case PacketTypeSYN:
		s.handleSYN(packet)
	case PacketTypeSYNACK:
		s.handleSYNACK(packet)
	case PacketTypeACK:
		s.handleACK(packet)
	case PacketTypeData:
		s.handleData(packet)
	case PacketTypeDataACK:
		s.handleDataACK(packet)
	case PacketTypeFIN:
		s.handleFIN(packet)
	case PacketTypeFINACK:
		s.handleFINACK(packet)
	case PacketTypeRST:
		s.handleRST(packet)
	default:
		s.logger.Warnf("Session: unknown packet type: %d", packet.Header.Type)
	}
}

// handleSYN handles SYN packet (server side)
func (s *Session) handleSYN(packet *Packet) {
	if s.isClient {
		s.logger.Warn("Session: client received SYN, ignoring")
		return
	}

	s.logger.Infof("Session: received SYN from %s", s.remoteAddr)

	synAckPacket := &Packet{
		Header: &PacketHeader{
			Version:     Version,
			Type:        PacketTypeSYNACK,
			Flags:       FlagNone,
			SessionID:   s.sessionID,
			StreamID:    s.streamID,
			SequenceNum: s.getNextSendSeq(),
			AckNum:      packet.Header.SequenceNum + 1,
			WindowSize:  uint16(s.flowController.GetReceiveWindow()),
			PayloadLen:  0,
			Timestamp:   uint32(time.Now().UnixMilli()),
		},
	}

	if err := s.sendPacketDirect(synAckPacket); err != nil {
		s.logger.Errorf("Session: failed to send SYN-ACK: %v", err)
		return
	}

	s.setRecvSeq(packet.Header.SequenceNum + 1)
	s.setState(StateSynReceived)
}

// handleSYNACK handles SYN-ACK packet (client side)
func (s *Session) handleSYNACK(packet *Packet) {
	if !s.isClient {
		s.logger.Warn("Session: server received SYN-ACK, ignoring")
		return
	}

	s.logger.Infof("Session: received SYN-ACK from %s", s.remoteAddr)

	ackPacket := &Packet{
		Header: &PacketHeader{
			Version:     Version,
			Type:        PacketTypeACK,
			Flags:       FlagNone,
			SessionID:   s.sessionID,
			StreamID:    s.streamID,
			SequenceNum: s.getNextSendSeq(),
			AckNum:      packet.Header.SequenceNum + 1,
			WindowSize:  uint16(s.flowController.GetReceiveWindow()),
			PayloadLen:  0,
			Timestamp:   uint32(time.Now().UnixMilli()),
		},
	}

	if err := s.sendPacketDirect(ackPacket); err != nil {
		s.logger.Errorf("Session: failed to send ACK: %v", err)
		return
	}

	s.updateRTT(packet.Header.Timestamp)
	s.bufferManager.MarkAcked(packet.Header.AckNum - 1)
	s.setRecvSeq(packet.Header.SequenceNum + 1)
	s.setState(StateEstablished)
}

// handleACK handles ACK packet (server side)
func (s *Session) handleACK(packet *Packet) {
	if s.isClient {
		s.logger.Warn("Session: client received ACK, ignoring")
		return
	}

	s.logger.Infof("Session: received ACK from %s, connection established", s.remoteAddr)

	s.updateRTT(packet.Header.Timestamp)
	s.bufferManager.MarkAcked(packet.Header.AckNum - 1)
	s.setRecvSeq(packet.Header.SequenceNum + 1)
	s.setState(StateEstablished)
}

// handleFIN handles FIN packet
func (s *Session) handleFIN(packet *Packet) {
	s.logger.Infof("Session: received FIN from %s", s.remoteAddr)

	finAckPacket := &Packet{
		Header: &PacketHeader{
			Version:     Version,
			Type:        PacketTypeFINACK,
			Flags:       FlagNone,
			SessionID:   s.sessionID,
			StreamID:    s.streamID,
			SequenceNum: s.getNextSendSeq(),
			AckNum:      packet.Header.SequenceNum + 1,
			WindowSize:  uint16(s.flowController.GetReceiveWindow()),
			PayloadLen:  0,
			Timestamp:   uint32(time.Now().UnixMilli()),
		},
	}

	s.sendPacketDirect(finAckPacket)
	s.setState(StateClosed)
	s.Close()
}

// handleFINACK handles FIN-ACK packet
func (s *Session) handleFINACK(packet *Packet) {
	s.logger.Infof("Session: received FIN-ACK from %s", s.remoteAddr)
	s.setState(StateClosed)
	s.Close()
}

// handleRST handles RST packet
func (s *Session) handleRST(packet *Packet) {
	s.logger.Warnf("Session: received RST from %s", s.remoteAddr)
	s.setState(StateClosed)
	s.Close()
}

// Read implements io.Reader
func (s *Session) Read(p []byte) (int, error) {
	return s.pipeReader.Read(p)
}

// Write implements io.Writer
func (s *Session) Write(p []byte) (int, error) {
	if s.getState() != StateEstablished {
		return 0, fmt.Errorf("session not established")
	}

	data := make([]byte, len(p))
	copy(data, p)

	select {
	case s.sendQueue <- data:
		return len(p), nil
	case <-s.closeChan:
		return 0, io.EOF
	case <-s.ctx.Done():
		return 0, io.EOF
	}
}

// Close implements io.Closer
func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.logger.Infof("Session: closing session %d, state=%v", s.sessionID, s.getState())

		// Send FIN if established
		if s.getState() == StateEstablished {
			finPacket := &Packet{
				Header: &PacketHeader{
					Version:     Version,
					Type:        PacketTypeFIN,
					Flags:       FlagNone,
					SessionID:   s.sessionID,
					StreamID:    s.streamID,
					SequenceNum: s.getNextSendSeq(),
					AckNum:      0,
					WindowSize:  uint16(s.flowController.GetReceiveWindow()),
					PayloadLen:  0,
					Timestamp:   uint32(time.Now().UnixMilli()),
				},
			}
			s.sendPacketDirect(finPacket)
			s.setState(StateFinWait)
		}

		// Close pipe writer
		if s.pipeWriter != nil {
			s.pipeWriter.Close()
		}

		// Close channels
		close(s.closeChan)
		s.cancel()

		// Unregister from dispatcher
		if s.dispatcher != nil {
			s.dispatcher.UnregisterSession(s)
		}

		// Wait for goroutines
		s.wg.Wait()

		s.logger.Infof("Session: session %d closed successfully", s.sessionID)
	})
	return err
}

// onClose is called when the resource is disposed
func (s *Session) onClose() error {
	return s.Close()
}

// GetRemoteAddr returns the remote address
func (s *Session) GetRemoteAddr() net.Addr {
	return s.remoteAddr
}

// GetSessionID returns the session ID
func (s *Session) GetSessionID() uint32 {
	return s.sessionID
}

// updateActivity updates the last activity timestamp
func (s *Session) updateActivity() {
	s.activityMu.Lock()
	s.lastActivity = time.Now()
	s.activityMu.Unlock()
}

// getLastActivity returns the last activity timestamp
func (s *Session) getLastActivity() time.Time {
	s.activityMu.RLock()
	defer s.activityMu.RUnlock()
	return s.lastActivity
}
