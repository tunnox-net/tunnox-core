package adapter

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"sync"
	"time"

	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"

	"github.com/quic-go/quic-go"
)

// QuicAdapter handles QUIC connections
type QuicAdapter struct {
	BaseAdapter
	listener  *quic.Listener
	connChan  chan io.ReadWriteCloser
	mu        sync.Mutex
	closed    bool
	tlsConfig *tls.Config
}

// NewQuicAdapter creates a new QUIC adapter
func NewQuicAdapter(parentCtx context.Context, sess session.Session) *QuicAdapter {
	adapter := &QuicAdapter{
		BaseAdapter: BaseAdapter{},
		connChan:    make(chan io.ReadWriteCloser, 100),
	}

	adapter.SetName("quic")
	adapter.SetSession(sess)
	adapter.SetCtx(parentCtx, adapter.onClose)

	// Generate self-signed certificate
	adapter.tlsConfig = generateTLSConfig()

	return adapter
}

// generateTLSConfig generates a self-signed TLS certificate for QUIC
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		utils.Errorf("QUIC: failed to generate RSA key: %v", err)
		return nil
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Tunnox"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		utils.Errorf("QUIC: failed to create certificate: %v", err)
		return nil
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		utils.Errorf("QUIC: failed to load key pair: %v", err)
		return nil
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"tunnox-quic"},
	}
}

// ListenFrom starts the QUIC server on the given address
func (a *QuicAdapter) ListenFrom(addr string) error {
	utils.Infof("QUIC adapter starting on %s", addr)

	if a.tlsConfig == nil {
		return fmt.Errorf("TLS config not initialized")
	}

	// Create QUIC config
	quicConf := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}

	// Create UDP listener
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	// Create QUIC listener
	listener, err := quic.Listen(udpConn, a.tlsConfig, quicConf)
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("failed to create QUIC listener: %w", err)
	}

	a.listener = listener

	// Start accepting connections
	go a.acceptConnections()

	// Start handling connections
	go a.handleConnections()

	utils.Infof("QUIC adapter started on %s", addr)
	return nil
}

// handleConnections processes incoming QUIC connections
func (a *QuicAdapter) handleConnections() {
	for {
		conn, err := a.Accept()
		if err != nil {
			select {
			case <-a.Ctx().Done():
				return
			default:
				utils.Errorf("QUIC: accept error: %v", err)
				continue
			}
		}

		go a.handleConnection(a, conn)
	}
}

// acceptConnections accepts incoming QUIC connections
func (a *QuicAdapter) acceptConnections() {
	for {
		select {
		case <-a.Ctx().Done():
			return
		default:
		}

		conn, err := a.listener.Accept(a.Ctx())
		if err != nil {
			select {
			case <-a.Ctx().Done():
				return
			default:
				utils.Errorf("QUIC: accept error: %v", err)
				continue
			}
		}

		utils.Infof("QUIC: connection accepted from %s", conn.RemoteAddr())

		// Accept stream from connection
		go a.acceptStream(conn)
	}
}

// acceptStream accepts a stream from a QUIC connection
func (a *QuicAdapter) acceptStream(conn *quic.Conn) {
	ctx := a.Ctx()
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		utils.Errorf("QUIC: accept stream error: %v", err)
		conn.CloseWithError(quic.ApplicationErrorCode(0), "failed to accept stream")
		return
	}

	utils.Debugf("QUIC: stream accepted from %s", conn.RemoteAddr())

	// Wrap stream as connection
	streamConn := &QuicServerStreamConn{
		stream:     stream,
		connection: conn,
		remoteAddr: conn.RemoteAddr().String(),
		closed:     make(chan struct{}),
	}

	// Send to accept channel
	select {
	case a.connChan <- streamConn:
		utils.Debugf("QUIC: stream queued for acceptance")
	case <-a.Ctx().Done():
		streamConn.Close()
		return
	case <-time.After(5 * time.Second):
		utils.Errorf("QUIC: connection queue full, rejecting")
		streamConn.Close()
		return
	}
}

// Accept accepts a new QUIC stream connection
func (a *QuicAdapter) Accept() (io.ReadWriteCloser, error) {
	select {
	case conn := <-a.connChan:
		return conn, nil
	case <-a.Ctx().Done():
		return nil, fmt.Errorf("quic adapter closed")
	}
}

// onClose handles cleanup when the adapter is closed
func (a *QuicAdapter) onClose() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()

	utils.Infof("QUIC adapter closing")

	// Close listener
	if a.listener != nil {
		if err := a.listener.Close(); err != nil {
			utils.Errorf("QUIC listener close error: %v", err)
		}
	}

	close(a.connChan)

	return nil
}

// Dial is not supported for QUIC adapter (server-side only)
func (a *QuicAdapter) Dial(address string) (io.ReadWriteCloser, error) {
	return nil, fmt.Errorf("dial not supported for QUIC adapter")
}

// Listen is not used for QUIC adapter (uses QUIC listener instead)
func (a *QuicAdapter) Listen(address string) error {
	return fmt.Errorf("listen not supported for QUIC adapter")
}

// getConnectionType returns the connection type for this adapter
func (a *QuicAdapter) getConnectionType() string {
	return "quic"
}

// QuicServerStreamConn wraps a QUIC stream for server side
type QuicServerStreamConn struct {
	stream     *quic.Stream
	connection *quic.Conn
	remoteAddr string
	closeOnce  sync.Once
	closed     chan struct{}
}

// Read implements io.Reader
func (c *QuicServerStreamConn) Read(p []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.EOF
	default:
	}

	n, err := c.stream.Read(p)
	if err != nil {
		select {
		case <-c.closed:
			return n, io.EOF
		default:
			return n, err
		}
	}

	return n, nil
}

// Write implements io.Writer
func (c *QuicServerStreamConn) Write(p []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.ErrClosedPipe
	default:
	}

	return c.stream.Write(p)
}

// Close implements io.Closer
func (c *QuicServerStreamConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)

		// Close stream
		c.stream.Close()

		utils.Debugf("QUIC: server stream closed")
	})
	return err
}

// LocalAddr implements net.Conn
func (c *QuicServerStreamConn) LocalAddr() net.Addr {
	return c.connection.LocalAddr()
}

// RemoteAddr implements net.Conn
func (c *QuicServerStreamConn) RemoteAddr() net.Addr {
	return c.connection.RemoteAddr()
}

// SetDeadline implements net.Conn
func (c *QuicServerStreamConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline implements net.Conn
func (c *QuicServerStreamConn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn
func (c *QuicServerStreamConn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}
