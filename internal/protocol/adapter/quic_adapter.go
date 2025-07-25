package adapter

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"time"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"github.com/quic-go/quic-go"
)

// QuicConn QUIC连接包装器
type QuicConn struct {
	stream *quic.Stream
}

func (q *QuicConn) Read(p []byte) (n int, err error) {
	return q.stream.Read(p)
}

func (q *QuicConn) Write(p []byte) (n int, err error) {
	return q.stream.Write(p)
}

func (q *QuicConn) Close() error {
	return q.stream.Close()
}

// QuicAdapter QUIC协议适配器
// 只实现协议相关方法，其余继承 BaseAdapter
type QuicAdapter struct {
	BaseAdapter
	listener  *quic.Listener
	tlsConfig *tls.Config
}

func NewQuicAdapter(parentCtx context.Context, session session.Session) *QuicAdapter {
	q := &QuicAdapter{}
	q.BaseAdapter = BaseAdapter{} // 初始化 BaseAdapter
	q.tlsConfig = generateTLSConfig()
	q.SetName("quic")
	q.SetSession(session)
	q.SetCtx(parentCtx, q.onClose)
	return q
}

func (q *QuicAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	conn, err := quic.DialAddr(context.Background(), addr, q.tlsConfig, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to QUIC server: %w", err)
	}
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		conn.CloseWithError(0, "failed to open stream")
		return nil, fmt.Errorf("failed to open QUIC stream: %w", err)
	}
	return &QuicConn{stream: stream}, nil
}

func (q *QuicAdapter) Listen(addr string) error {
	listener, err := quic.ListenAddr(addr, q.tlsConfig, nil)
	if err != nil {
		return fmt.Errorf("failed to listen on QUIC: %w", err)
	}
	q.listener = listener
	return nil
}

func (q *QuicAdapter) Accept() (io.ReadWriteCloser, error) {
	if q.listener == nil {
		return nil, fmt.Errorf("QUIC listener not initialized")
	}
	acceptCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	conn, err := q.listener.Accept(acceptCtx)
	cancel()
	if err != nil {
		return nil, err
	}
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		conn.CloseWithError(0, "failed to open stream")
		return nil, fmt.Errorf("failed to open QUIC stream: %w", err)
	}
	return &QuicConn{stream: stream}, nil
}

func (q *QuicAdapter) getConnectionType() string {
	return "QUIC"
}

// ListenFrom 重写BaseAdapter的ListenFrom方法
func (q *QuicAdapter) ListenFrom(listenAddr string) error {
	q.SetAddr(listenAddr)
	if q.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	utils.Infof("QuicAdapter.ListenFrom called for adapter: %s, type: %T", q.Name(), q)

	// 直接使用自身作为ProtocolAdapter
	if err := q.Listen(q.Addr()); err != nil {
		return fmt.Errorf("failed to listen on %s: %w", q.getConnectionType(), err)
	}

	q.active = true
	go q.acceptLoop(q)
	return nil
}

// ConnectTo 重写BaseAdapter的ConnectTo方法
func (q *QuicAdapter) ConnectTo(serverAddr string) error {
	q.connMutex.Lock()
	defer q.connMutex.Unlock()

	if q.stream != nil {
		return fmt.Errorf("already connected")
	}

	// 直接使用自身作为ProtocolAdapter
	conn, err := q.Dial(serverAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s server: %w", q.getConnectionType(), err)
	}

	q.SetAddr(serverAddr)

	q.streamMutex.Lock()
	q.stream = stream.NewStreamProcessor(conn, conn, q.Ctx())
	q.streamMutex.Unlock()

	return nil
}

// onClose QUIC 特定的资源清理
func (q *QuicAdapter) onClose() error {
	var err error
	if q.listener != nil {
		err = q.listener.Close()
		q.listener = nil
	}
	baseErr := q.BaseAdapter.onClose()
	if err != nil {
		return err
	}
	return baseErr
}

// generateTLSConfig 生成TLS配置（用于QUIC）
func generateTLSConfig() *tls.Config {
	// 生成私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		utils.Errorf("Failed to generate private key: %v", err)
		return nil
	}

	// 生成证书模板
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	// 创建证书
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		utils.Errorf("Failed to create certificate: %v", err)
		return nil
	}

	// 编码证书
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// 解析TLS证书
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		utils.Errorf("Failed to parse TLS certificate: %v", err)
		return nil
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"tunnox-quic"},
	}
}
