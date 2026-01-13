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
	adapter.SetProtocolAdapter(adapter) // 设置协议适配器引用，与 TCP/KCP 保持一致

	// Generate self-signed certificate
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

	// Create QUIC config
	quicConf := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
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

		corelog.Infof("QUIC: connection accepted from %s", conn.RemoteAddr())

		// Accept stream from connection
		go a.acceptStream(conn)
	}
}

// acceptStream accepts a stream from a QUIC connection
func (a *QuicAdapter) acceptStream(conn *quic.Conn) {
	ctx := a.Ctx()
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		corelog.Errorf("QUIC: accept stream error: %v", err)
		conn.CloseWithError(quic.ApplicationErrorCode(0), "failed to accept stream")
		return
	}

	// Wrap stream as connection
	streamConn := &QuicStreamConn{
		stream:     stream,
		connection: conn,
		remoteAddr: conn.RemoteAddr().String(),
		closed:     make(chan struct{}),
	}

	// Send to accept channel
	select {
	case a.connChan <- streamConn:
	case <-a.Ctx().Done():
		streamConn.Close()
		return
	case <-time.After(5 * time.Second):
		corelog.Errorf("QUIC: connection queue full, rejecting")
		streamConn.Close()
		return
	}
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

	// Close listener
	if a.listener != nil {
		if closeErr := a.listener.Close(); closeErr != nil {
			corelog.Errorf("QUIC listener close error: %v", closeErr)
			err = closeErr
		}
	}

	close(a.connChan)

	// 调用基类清理，与 TCP/KCP 适配器保持一致
	baseErr := a.BaseAdapter.onClose()
	if err == nil {
		err = baseErr
	}

	corelog.Infof("QUIC adapter closed")
	return err
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
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
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

		if streamErr := (*c.stream).Close(); streamErr != nil {
			err = streamErr
		}

		if connErr := c.connection.CloseWithError(0, "stream closed"); connErr != nil && err == nil {
			err = connErr
		}
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
