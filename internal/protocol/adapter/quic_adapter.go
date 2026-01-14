package adapter

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"sync"
	"time"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/protocol/session"

	"github.com/quic-go/quic-go"
)

// quicConnEntry 跟踪单个 QUIC 连接的状态
type quicConnEntry struct {
	conn       *quic.Conn
	remoteAddr string
	createdAt  time.Time
	lastActive time.Time
}

// QuicAdapter handles QUIC connections
type QuicAdapter struct {
	BaseAdapter
	listener  *quic.Listener
	connChan  chan io.ReadWriteCloser
	mu        sync.Mutex
	closed    bool
	tlsConfig *tls.Config

	// 连接跟踪
	activeConns   map[*quic.Conn]*quicConnEntry
	activeConnsMu sync.RWMutex
}

// NewQuicAdapter creates a new QUIC adapter
func NewQuicAdapter(parentCtx context.Context, sess session.Session) *QuicAdapter {
	adapter := &QuicAdapter{
		BaseAdapter: BaseAdapter{},
		connChan:    make(chan io.ReadWriteCloser, 100),
		activeConns: make(map[*quic.Conn]*quicConnEntry),
	}

	adapter.SetName("quic")
	adapter.SetSession(sess)
	adapter.SetCtx(parentCtx, adapter.onClose)
	adapter.SetProtocolAdapter(adapter)

	adapter.tlsConfig = generateTLSConfig()

	return adapter
}

// generateTLSConfig generates a self-signed TLS certificate for QUIC
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		corelog.Errorf("QUIC: failed to generate RSA key: %v", err)
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
		corelog.Errorf("QUIC: failed to create certificate: %v", err)
		return nil
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		corelog.Errorf("QUIC: failed to load key pair: %v", err)
		return nil
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"tunnox-quic"},
	}
}

// Listen 实现 ProtocolAdapter 接口，启动 QUIC 监听
func (a *QuicAdapter) Listen(addr string) error {
	corelog.Infof("QUIC adapter starting listener on %s", addr)

	if a.tlsConfig == nil {
		return coreerrors.New(coreerrors.CodeNotConfigured, "TLS config not initialized")
	}

	quicConf := &quic.Config{
		MaxIdleTimeout: 30 * time.Second,
	}

	// Create UDP listener
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to resolve UDP address")
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to listen on UDP")
	}

	// Create QUIC listener
	listener, err := quic.Listen(udpConn, a.tlsConfig, quicConf)
	if err != nil {
		udpConn.Close()
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError, "failed to create QUIC listener")
	}

	a.listener = listener

	// Start accepting QUIC connections and streams in background
	go a.acceptConnections()

	corelog.Infof("QUIC adapter listener started on %s", addr)
	return nil
}

// acceptConnections accepts incoming QUIC connections
func (a *QuicAdapter) acceptConnections() {
	go a.connectionCleanupLoop()

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
				corelog.Errorf("QUIC: accept error: %v", err)
				continue
			}
		}

		remoteAddr := conn.RemoteAddr().String()
		corelog.Infof("QUIC: connection accepted from %s", remoteAddr)

		a.trackConnection(conn, remoteAddr)

		go a.acceptStream(conn)
	}
}

func (a *QuicAdapter) trackConnection(conn *quic.Conn, remoteAddr string) {
	now := time.Now()
	a.activeConnsMu.Lock()
	a.activeConns[conn] = &quicConnEntry{
		conn:       conn,
		remoteAddr: remoteAddr,
		createdAt:  now,
		lastActive: now,
	}
	connCount := len(a.activeConns)
	a.activeConnsMu.Unlock()

	corelog.Infof("QUIC: tracking connection from %s, total active: %d", remoteAddr, connCount)
}

func (a *QuicAdapter) untrackConnection(conn *quic.Conn) {
	a.activeConnsMu.Lock()
	entry, exists := a.activeConns[conn]
	if exists {
		delete(a.activeConns, conn)
	}
	connCount := len(a.activeConns)
	a.activeConnsMu.Unlock()

	if exists {
		corelog.Infof("QUIC: untracking connection from %s, total active: %d", entry.remoteAddr, connCount)
	}
}

func (a *QuicAdapter) connectionCleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.Ctx().Done():
			return
		case <-ticker.C:
			a.cleanupStaleConnections()
		}
	}
}

func (a *QuicAdapter) cleanupStaleConnections() {
	a.activeConnsMu.Lock()
	defer a.activeConnsMu.Unlock()

	now := time.Now()
	staleThreshold := 60 * time.Second
	var staleConns []*quic.Conn

	for conn, entry := range a.activeConns {
		if now.Sub(entry.lastActive) > staleThreshold {
			staleConns = append(staleConns, conn)
			corelog.Warnf("QUIC: connection from %s is stale (inactive for %v), will close", entry.remoteAddr, now.Sub(entry.lastActive))
		}
	}

	for _, conn := range staleConns {
		entry := a.activeConns[conn]
		delete(a.activeConns, conn)
		conn.CloseWithError(0, "connection idle timeout")
		corelog.Infof("QUIC: closed stale connection from %s", entry.remoteAddr)
	}

	if len(staleConns) > 0 {
		corelog.Infof("QUIC: cleaned up %d stale connections, %d remaining", len(staleConns), len(a.activeConns))
	}
}

// acceptStream accepts a stream from a QUIC connection
func (a *QuicAdapter) acceptStream(conn *quic.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	corelog.Infof("QUIC: acceptStream started for connection from %s", remoteAddr)

	ctx := a.Ctx()
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		corelog.Errorf("QUIC: accept stream error from %s: %v", remoteAddr, err)
		a.untrackConnection(conn)
		conn.CloseWithError(quic.ApplicationErrorCode(0), "failed to accept stream")
		corelog.Infof("QUIC: connection closed after stream accept error from %s", remoteAddr)
		return
	}

	corelog.Infof("QUIC: stream accepted from %s", remoteAddr)

	a.updateConnectionActivity(conn)

	streamConn := &QuicStreamConn{
		stream:     stream,
		connection: conn,
		remoteAddr: remoteAddr,
		closed:     make(chan struct{}),
		adapter:    a,
	}

	select {
	case a.connChan <- streamConn:
		corelog.Infof("QUIC: stream connection queued from %s", remoteAddr)
	case <-a.Ctx().Done():
		corelog.Infof("QUIC: context done while queueing connection from %s, closing", remoteAddr)
		streamConn.Close()
		return
	case <-time.After(5 * time.Second):
		corelog.Errorf("QUIC: connection queue full, rejecting connection from %s", remoteAddr)
		streamConn.Close()
		return
	}
}

func (a *QuicAdapter) updateConnectionActivity(conn *quic.Conn) {
	a.activeConnsMu.Lock()
	if entry, exists := a.activeConns[conn]; exists {
		entry.lastActive = time.Now()
	}
	a.activeConnsMu.Unlock()
}

// Accept accepts a new QUIC stream connection
func (a *QuicAdapter) Accept() (io.ReadWriteCloser, error) {
	select {
	case conn, ok := <-a.connChan:
		if !ok {
			return nil, coreerrors.New(coreerrors.CodeResourceClosed, "quic adapter closed")
		}
		return conn, nil
	case <-a.Ctx().Done():
		return nil, coreerrors.New(coreerrors.CodeResourceClosed, "quic adapter closed")
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

	corelog.Infof("QUIC adapter closing")

	var err error

	a.closeAllTrackedConnections()

	if a.listener != nil {
		if closeErr := a.listener.Close(); closeErr != nil {
			corelog.Errorf("QUIC listener close error: %v", closeErr)
			err = closeErr
		}
	}

	close(a.connChan)

	baseErr := a.BaseAdapter.onClose()
	if err == nil {
		err = baseErr
	}

	corelog.Infof("QUIC adapter closed")
	return err
}

func (a *QuicAdapter) closeAllTrackedConnections() {
	a.activeConnsMu.Lock()
	defer a.activeConnsMu.Unlock()

	for conn, entry := range a.activeConns {
		conn.CloseWithError(0, "adapter closing")
		corelog.Infof("QUIC: closed tracked connection from %s on adapter shutdown", entry.remoteAddr)
	}
	a.activeConns = make(map[*quic.Conn]*quicConnEntry)
}

// Dial 建立 QUIC 连接（客户端）
// TODO: 跨节点通信场景下，考虑添加 TLS 配置支持（从 SessionManager 或配置文件获取）
// 当前使用 InsecureSkipVerify=true，业务数据已有端到端加密（AES-256-GCM）保护
func (a *QuicAdapter) Dial(address string) (io.ReadWriteCloser, error) {
	corelog.Infof("QUIC adapter dialing %s", address)

	tlsConf := &tls.Config{
		InsecureSkipVerify: true, // 自签名证书，跨节点通信暂用默认配置
		NextProtos:         []string{"tunnox-quic"},
	}

	quicConf := &quic.Config{
		MaxIdleTimeout: 30 * time.Second,
	}

	conn, err := quic.DialAddr(a.Ctx(), address, tlsConf, quicConf)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeConnectionError, "failed to dial QUIC")
	}

	// Open a stream
	stream, err := conn.OpenStreamSync(a.Ctx())
	if err != nil {
		conn.CloseWithError(quic.ApplicationErrorCode(0), "failed to open stream")
		return nil, coreerrors.Wrap(err, coreerrors.CodeConnectionError, "failed to open QUIC stream")
	}

	corelog.Infof("QUIC adapter connected to %s", address)

	return &QuicStreamConn{
		stream:     stream,
		connection: conn,
		remoteAddr: conn.RemoteAddr().String(),
		closed:     make(chan struct{}),
	}, nil
}

// getConnectionType 返回连接类型
func (a *QuicAdapter) getConnectionType() string {
	return "QUIC"
}

// QuicStreamConn wraps a QUIC stream as io.ReadWriteCloser
type QuicStreamConn struct {
	stream     *quic.Stream
	connection *quic.Conn
	remoteAddr string
	closeOnce  sync.Once
	closed     chan struct{}
	adapter    *QuicAdapter
}

// Read implements io.Reader
func (c *QuicStreamConn) Read(p []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.EOF
	default:
	}

	n, err := (*c.stream).Read(p)
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
func (c *QuicStreamConn) Write(p []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.ErrClosedPipe
	default:
	}

	return (*c.stream).Write(p)
}

// Close implements io.Closer
func (c *QuicStreamConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)

		corelog.Infof("QUIC: closing QuicStreamConn from %s", c.remoteAddr)

		if c.adapter != nil {
			c.adapter.untrackConnection(c.connection)
			c.adapter = nil
		}

		if c.stream != nil {
			if streamErr := (*c.stream).Close(); streamErr != nil {
				corelog.Warnf("QUIC: stream close error from %s: %v", c.remoteAddr, streamErr)
				err = streamErr
			}
			c.stream = nil
		}

		if c.connection != nil {
			if connErr := c.connection.CloseWithError(0, "stream closed"); connErr != nil && err == nil {
				corelog.Warnf("QUIC: connection close error from %s: %v", c.remoteAddr, connErr)
				err = connErr
			}
			c.connection = nil
		}

		corelog.Infof("QUIC: QuicStreamConn closed from %s", c.remoteAddr)
	})
	return err
}

// LocalAddr implements net.Conn
func (c *QuicStreamConn) LocalAddr() net.Addr {
	return c.connection.LocalAddr()
}

// RemoteAddr implements net.Conn
func (c *QuicStreamConn) RemoteAddr() net.Addr {
	return c.connection.RemoteAddr()
}

// SetDeadline implements net.Conn
func (c *QuicStreamConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline implements net.Conn
func (c *QuicStreamConn) SetReadDeadline(t time.Time) error {
	return (*c.stream).SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn
func (c *QuicStreamConn) SetWriteDeadline(t time.Time) error {
	return (*c.stream).SetWriteDeadline(t)
}

func (c *QuicStreamConn) GetNetConn() net.Conn {
	return c
}
