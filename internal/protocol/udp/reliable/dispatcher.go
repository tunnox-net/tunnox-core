package reliable

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// PacketDispatcher centralizes UDP packet reading and dispatches to sessions
// This solves the problem of multiple receivers competing for the same socket
type PacketDispatcher struct {
	conn   *net.UDPConn
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *logrus.Logger

	// Session management
	sessions   map[SessionKey]*Session
	sessionsMu sync.RWMutex

	// Pending connections (server side)
	pendingConns chan *Session

	// Stop management (ensure idempotent Stop)
	stopOnce sync.Once

	// Statistics
	stats struct {
		packetsReceived uint64
		packetsDropped  uint64
		bytesReceived   uint64
	}
	statsMu sync.RWMutex
}

// NewPacketDispatcher creates a new packet dispatcher
func NewPacketDispatcher(conn *net.UDPConn, logger *logrus.Logger) *PacketDispatcher {
	ctx, cancel := context.WithCancel(context.Background())

	d := &PacketDispatcher{
		conn:         conn,
		ctx:          ctx,
		cancel:       cancel,
		logger:       logger,
		sessions:     make(map[SessionKey]*Session),
		pendingConns: make(chan *Session, 100),
	}

	return d
}

// Start starts the dispatcher's read loop
func (d *PacketDispatcher) Start() {
	d.wg.Add(1)
	go d.readLoop()
	d.logger.Infof("PacketDispatcher: started on %s", d.conn.LocalAddr())
}

// Stop stops the dispatcher and all sessions
// This method is idempotent - calling it multiple times is safe
func (d *PacketDispatcher) Stop() error {
	d.stopOnce.Do(func() {
		d.logger.Info("PacketDispatcher: stopping...")
		d.cancel()

		// Close all sessions
		d.sessionsMu.Lock()
		for key, session := range d.sessions {
			d.logger.Debugf("PacketDispatcher: closing session %s", key)
			session.Close()
		}
		d.sessions = make(map[SessionKey]*Session)
		d.sessionsMu.Unlock()

		// Close pending connections channel
		close(d.pendingConns)

		// Wait for read loop to finish
		d.wg.Wait()

		d.logger.Info("PacketDispatcher: stopped")
	})
	return nil
}

// readLoop is the single centralized read loop
// This solves the core problem of multiple receivers competing
func (d *PacketDispatcher) readLoop() {
	defer d.wg.Done()

	buf := make([]byte, MaxUDPPacketSize)

	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		// Set read deadline to allow periodic context checks
		if err := d.conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
			d.logger.Errorf("PacketDispatcher: failed to set read deadline: %v", err)
			continue
		}

		n, remoteAddr, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Timeout is expected, continue
				continue
			}
			if d.ctx.Err() != nil {
				// Context cancelled, exit gracefully
				return
			}
			d.logger.Warnf("PacketDispatcher: read error: %v", err)
			continue
		}

		// Update statistics
		d.statsMu.Lock()
		d.stats.packetsReceived++
		d.stats.bytesReceived += uint64(n)
		d.statsMu.Unlock()

		// Parse packet header
		packet, err := DecodePacket(buf[:n])
		if err != nil {
			d.logger.Warnf("PacketDispatcher: failed to decode packet from %s: %v", remoteAddr, err)
			d.incrementDropped()
			continue
		}

		// 移除高频日志，避免日志噪音
		// d.logger.Debugf("PacketDispatcher: received %s packet from %s, session=%d, seq=%d",
		// 	packet.Header.Type, remoteAddr, packet.Header.SessionID, packet.Header.SequenceNum)

		// Route packet to appropriate session
		d.routePacket(packet, remoteAddr)
	}
}

// routePacket routes a packet to the appropriate session
func (d *PacketDispatcher) routePacket(packet *Packet, remoteAddr *net.UDPAddr) {
	key := SessionKey{
		RemoteAddr: remoteAddr.String(),
		SessionID:  packet.Header.SessionID,
	}

	d.sessionsMu.RLock()
	session, exists := d.sessions[key]
	d.sessionsMu.RUnlock()

	if exists {
		// Route to existing session
		session.HandlePacket(packet)
		return
	}

	// Handle new connection (SYN packet)
	if packet.Header.Type == PacketTypeSYN {
		d.handleNewConnection(packet, remoteAddr)
		return
	}

	// Unknown session and not a SYN packet, drop it
	// 移除高频日志，避免日志噪音（连接关闭后收到包是正常的）
	// 如需调试，可以临时启用此日志
	// d.logger.Warnf("PacketDispatcher: received non-SYN packet for unknown session %s", key)
	d.incrementDropped()
}

// handleNewConnection handles a new incoming connection (server side)
func (d *PacketDispatcher) handleNewConnection(packet *Packet, remoteAddr *net.UDPAddr) {
	key := SessionKey{
		RemoteAddr: remoteAddr.String(),
		SessionID:  packet.Header.SessionID,
	}

	d.logger.Infof("PacketDispatcher: new connection from %s, session=%d", remoteAddr, packet.Header.SessionID)

	// Create new session
	session := NewSession(d.conn, remoteAddr, packet.Header.SessionID, packet.Header.StreamID, false, d.logger)
	session.SetDispatcher(d)

	// Register session
	d.sessionsMu.Lock()
	d.sessions[key] = session
	d.sessionsMu.Unlock()

	// Handle the SYN packet (sends SYN-ACK)
	session.HandlePacket(packet)

	// Wait for connection establishment in a goroutine
	go d.waitForEstablishment(session, key)
}

// waitForEstablishment waits for the session to be established before queuing it
func (d *PacketDispatcher) waitForEstablishment(session *Session, key SessionKey) {
	// Wait for connection establishment (timeout 5 seconds)
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			d.logger.Warnf("PacketDispatcher: connection establishment timeout for %s", key)
			// Clean up session
			d.sessionsMu.Lock()
			delete(d.sessions, key)
			d.sessionsMu.Unlock()
			session.Close()
			return
		case <-ticker.C:
			if session.GetState() == StateEstablished {
				// Connection established, queue it
				select {
				case d.pendingConns <- session:
					d.logger.Infof("PacketDispatcher: connection established and queued for %s", key)
				case <-d.ctx.Done():
					return
				default:
					d.logger.Warnf("PacketDispatcher: pending connections queue full, dropping connection from %s", key)
					d.sessionsMu.Lock()
					delete(d.sessions, key)
					d.sessionsMu.Unlock()
					session.Close()
				}
				return
			}
		case <-d.ctx.Done():
			return
		}
	}
}

// RegisterSession registers a session with the dispatcher (client side)
func (d *PacketDispatcher) RegisterSession(session *Session) error {
	key := SessionKey{
		RemoteAddr: session.remoteAddr.String(),
		SessionID:  session.sessionID,
	}

	d.sessionsMu.Lock()
	defer d.sessionsMu.Unlock()

	if _, exists := d.sessions[key]; exists {
		return fmt.Errorf("session %s already exists", key)
	}

	d.sessions[key] = session
	session.SetDispatcher(d)

	d.logger.Infof("PacketDispatcher: registered session %s", key)
	return nil
}

// UnregisterSession removes a session from the dispatcher
func (d *PacketDispatcher) UnregisterSession(session *Session) {
	key := SessionKey{
		RemoteAddr: session.remoteAddr.String(),
		SessionID:  session.sessionID,
	}

	d.sessionsMu.Lock()
	delete(d.sessions, key)
	d.sessionsMu.Unlock()

	d.logger.Infof("PacketDispatcher: unregistered session %s", key)
}

// Accept waits for and returns a new incoming connection
func (d *PacketDispatcher) Accept() (*Session, error) {
	select {
	case session := <-d.pendingConns:
		if session == nil {
			return nil, fmt.Errorf("dispatcher closed")
		}
		return session, nil
	case <-d.ctx.Done():
		return nil, fmt.Errorf("dispatcher closed")
	}
}

// Send sends a packet through the dispatcher
func (d *PacketDispatcher) Send(packet *Packet, remoteAddr *net.UDPAddr) error {
	data := EncodePacket(packet)

	var n int
	var err error

	// Check if the connection is connected (client side) or unconnected (server side)
	// Connected UDP sockets (from DialUDP) must use Write()
	// Unconnected UDP sockets (from ListenUDP) must use WriteToUDP()
	if d.conn.RemoteAddr() != nil {
		// Connected UDP socket - use Write()
		n, err = d.conn.Write(data)
	} else {
		// Unconnected UDP socket - use WriteToUDP()
		n, err = d.conn.WriteToUDP(data, remoteAddr)
	}

	if err != nil {
		return fmt.Errorf("failed to send packet: %w", err)
	}

	if n != len(data) {
		return fmt.Errorf("incomplete write: wrote %d of %d bytes", n, len(data))
	}

	// 移除高频日志，避免日志噪音
	// d.logger.Debugf("PacketDispatcher: sent %s packet to %s, session=%d, seq=%d, size=%d",
	// 	packet.Header.Type, remoteAddr, packet.Header.SessionID, packet.Header.SequenceNum, n)

	return nil
}

// GetStats returns dispatcher statistics
func (d *PacketDispatcher) GetStats() (packetsReceived, packetsDropped, bytesReceived uint64) {
	d.statsMu.RLock()
	defer d.statsMu.RUnlock()
	return d.stats.packetsReceived, d.stats.packetsDropped, d.stats.bytesReceived
}

// incrementDropped increments the dropped packet counter
func (d *PacketDispatcher) incrementDropped() {
	d.statsMu.Lock()
	d.stats.packetsDropped++
	d.statsMu.Unlock()
}
