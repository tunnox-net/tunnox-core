package reliable

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/sirupsen/logrus"
)

// Transport implements io.ReadWriteCloser for a reliable UDP session
// This is what the UdpAdapter returns to maintain interface compatibility
type Transport struct {
	session    *Session
	dispatcher *PacketDispatcher // Owned dispatcher (for client-side transports)
	logger     *logrus.Logger
}

// NewClientTransport creates a new client-side transport
func NewClientTransport(conn *net.UDPConn, remoteAddr *net.UDPAddr, dispatcher *PacketDispatcher, logger *logrus.Logger) (*Transport, error) {
	// Generate random session ID
	sessionID, err := generateSessionID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Generate random stream ID
	streamID, err := generateStreamID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate stream ID: %w", err)
	}

	// Create session
	session := NewSession(conn, remoteAddr, sessionID, streamID, true, logger)

	// Register with dispatcher
	if err := dispatcher.RegisterSession(session); err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to register session: %w", err)
	}

	// Initiate connection
	if err := session.Connect(); err != nil {
		session.Close()
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return &Transport{
		session:    session,
		dispatcher: dispatcher, // Store dispatcher so we can close it later
		logger:     logger,
	}, nil
}

// NewServerTransport creates a new server-side transport from an accepted session
func NewServerTransport(session *Session, logger *logrus.Logger) *Transport {
	return &Transport{
		session: session,
		logger:  logger,
	}
}

// Read implements io.Reader
func (t *Transport) Read(p []byte) (int, error) {
	return t.session.Read(p)
}

// Write implements io.Writer
func (t *Transport) Write(p []byte) (int, error) {
	return t.session.Write(p)
}

// Close implements io.Closer
func (t *Transport) Close() error {
	// Close session first
	if err := t.session.Close(); err != nil {
		t.logger.Errorf("Transport: failed to close session: %v", err)
	}
	
	// Close dispatcher if we own it (client-side transports)
	if t.dispatcher != nil {
		if err := t.dispatcher.Stop(); err != nil {
			t.logger.Errorf("Transport: failed to stop dispatcher: %v", err)
		}
	}
	
	return nil
}

// GetSession returns the underlying session (for testing/debugging)
func (t *Transport) GetSession() *Session {
	return t.session
}

// LocalAddr returns the local address
func (t *Transport) LocalAddr() net.Addr {
	return t.session.conn.LocalAddr()
}

// RemoteAddr returns the remote address
func (t *Transport) RemoteAddr() net.Addr {
	return t.session.GetRemoteAddr()
}

// SetDeadline sets the read and write deadlines
func (t *Transport) SetDeadline(deadline time.Time) error {
	// Note: Deadlines are handled at the session level via context
	// This is a placeholder for interface compatibility
	return nil
}

// SetReadDeadline sets the read deadline
func (t *Transport) SetReadDeadline(deadline time.Time) error {
	// Note: Deadlines are handled at the session level via context
	// This is a placeholder for interface compatibility
	return nil
}

// SetWriteDeadline sets the write deadline
func (t *Transport) SetWriteDeadline(deadline time.Time) error {
	// Note: Deadlines are handled at the session level via context
	// This is a placeholder for interface compatibility
	return nil
}

// generateSessionID generates a random session ID
func generateSessionID() (uint32, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fallback to timestamp if random fails
		return uint32(time.Now().Unix()), nil
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}

// generateStreamID generates a random stream ID
func generateStreamID() (uint32, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fallback to timestamp if random fails
		return uint32(time.Now().UnixNano()), nil
	}
	return binary.BigEndian.Uint32(buf[:]), nil
}
