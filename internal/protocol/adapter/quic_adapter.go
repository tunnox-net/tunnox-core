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
	"tunnox-core/internal/core/errors"
	"tunnox-core/internal/protocol/session"
	"tunnox-core/internal/utils"

	"github.com/quic-go/quic-go"
)

const (
	// QUIC 相关常量
	quicMaxIdleTimeout     = 30 * time.Second
	quicKeepAlivePeriod    = 10 * time.Second
	quicMaxIncomingStreams = 100
	quicAcceptTimeout      = 100 * time.Millisecond
	quicDialTimeout        = 10 * time.Second
)

// QuicConn QUIC流连接包装器
type QuicConn struct {
	stream *quic.Stream
	conn   *quic.Conn
	mu     sync.RWMutex
	closed bool
}

func (q *QuicConn) Read(p []byte) (n int, err error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		return 0, io.EOF
	}

	return q.stream.Read(p)
}

func (q *QuicConn) Write(p []byte) (n int, err error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.closed {
		return 0, fmt.Errorf("connection closed")
	}

	return q.stream.Write(p)
}

func (q *QuicConn) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return nil
	}

	q.closed = true

	if q.stream != nil {
		return q.stream.Close()
	}

	return nil
}

// QuicAdapter QUIC协议适配器
// 只实现协议相关方法，其余继承 BaseAdapter
type QuicAdapter struct {
	BaseAdapter
	listener    *quic.Listener
	tlsConfig   *tls.Config
	quicConfig  *quic.Config
	connections map[string]*quic.Conn
	connLock    sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewQuicAdapter(parentCtx context.Context, session session.Session) *QuicAdapter {
	ctx, cancel := context.WithCancel(parentCtx)

	q := &QuicAdapter{
		connections: make(map[string]*quic.Conn),
		ctx:         ctx,
		cancel:      cancel,
	}

	q.BaseAdapter = BaseAdapter{} // 初始化 BaseAdapter
	q.tlsConfig = generateTLSConfig()
	q.quicConfig = generateQUICConfig()
	q.SetName("quic")
	q.SetSession(session)
	q.SetCtx(parentCtx, q.onClose)
	q.SetProtocolAdapter(q) // 设置协议适配器引用

	return q
}

func (q *QuicAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	// 创建客户端 TLS 配置
	clientTLSConfig := &tls.Config{
		InsecureSkipVerify: true, // 在生产环境中应该验证证书
		NextProtos:         []string{"tunnox-quic"},
	}

	dialCtx, cancel := context.WithTimeout(q.ctx, quicDialTimeout)
	defer cancel()

	conn, err := quic.DialAddr(dialCtx, addr, clientTLSConfig, q.quicConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to QUIC server: %w", err)
	}

	// 打开一个新的流
	streamCtx, streamCancel := context.WithTimeout(q.ctx, quicDialTimeout)
	defer streamCancel()

	stream, err := conn.OpenStreamSync(streamCtx)
	if err != nil {
		conn.CloseWithError(0, "failed to open stream")
		return nil, fmt.Errorf("failed to open QUIC stream: %w", err)
	}

	utils.Infof("QUIC connection established to %s", addr)

	return &QuicConn{
		stream: stream,
		conn:   conn,
	}, nil
}

func (q *QuicAdapter) Listen(addr string) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	listener, err := quic.Listen(udpConn, q.tlsConfig, q.quicConfig)
	if err != nil {
		udpConn.Close()
		return fmt.Errorf("failed to listen on QUIC: %w", err)
	}

	q.listener = listener

	// 启动连接管理 goroutine
	go q.manageConnections()

	utils.Infof("QUIC adapter listening on %s", addr)
	return nil
}

// manageConnections 管理 QUIC 连接
func (q *QuicAdapter) manageConnections() {
	for {
		select {
		case <-q.ctx.Done():
			return
		default:
		}

		acceptCtx, cancel := context.WithTimeout(q.ctx, quicAcceptTimeout)
		conn, err := q.listener.Accept(acceptCtx)
		cancel()

		if err != nil {
			if q.ctx.Err() != nil {
				return
			}
			// 超时是正常的，继续循环
			continue
		}

		// 记录连接
		remoteAddr := conn.RemoteAddr().String()
		q.connLock.Lock()
		q.connections[remoteAddr] = conn
		q.connLock.Unlock()

		utils.Infof("QUIC connection accepted from %s", remoteAddr)

		// 为每个连接启动流处理 goroutine
		go q.handleConnection(conn)
	}
}

// handleConnection 处理单个 QUIC 连接的流
func (q *QuicAdapter) handleConnection(conn *quic.Conn) {
	defer func() {
		remoteAddr := conn.RemoteAddr().String()
		q.connLock.Lock()
		delete(q.connections, remoteAddr)
		q.connLock.Unlock()
		conn.CloseWithError(0, "connection closed")
		utils.Infof("QUIC connection closed for %s", remoteAddr)
	}()

	for {
		select {
		case <-q.ctx.Done():
			return
		case <-conn.Context().Done():
			return
		default:
		}

		// 接受新的流
		streamCtx, cancel := context.WithTimeout(q.ctx, quicAcceptTimeout)
		stream, err := conn.AcceptStream(streamCtx)
		cancel()

		if err != nil {
			if q.ctx.Err() != nil || conn.Context().Err() != nil {
				return
			}
			// 超时是正常的，继续循环
			continue
		}

		utils.Infof("QUIC stream accepted from %s", conn.RemoteAddr())

		// 处理流
		quicConn := &QuicConn{
			stream: stream,
			conn:   conn,
		}

		// Session是系统关键组件，必须存在
		if q.GetSession() == nil {
			utils.Errorf("Session is required but not set for QUIC adapter")
			quicConn.Close()
			continue
		}

		// 在单独的 goroutine 中处理这个流
		go func() {
			defer quicConn.Close()

			_, err := q.GetSession().AcceptConnection(quicConn, quicConn)
			if err != nil {
				utils.Errorf("Failed to initialize QUIC stream connection: %v", err)
			}
		}()
	}
}

func (q *QuicAdapter) Accept() (io.ReadWriteCloser, error) {
	if q.listener == nil {
		return nil, fmt.Errorf("QUIC listener not initialized")
	}

	// QUIC适配器通过 manageConnections 处理连接，不需要传统的Accept方法
	// 返回超时错误以符合接口要求
	return nil, errors.NewProtocolTimeoutError("QUIC connection")
}

func (q *QuicAdapter) getConnectionType() string {
	return "QUIC"
}

// onClose QUIC 特定的资源清理
func (q *QuicAdapter) onClose() error {
	// 取消上下文，停止所有 goroutine
	if q.cancel != nil {
		q.cancel()
	}

	var err error

	// 关闭所有活动连接
	q.connLock.Lock()
	for addr, conn := range q.connections {
		if closeErr := conn.CloseWithError(0, "adapter closing"); closeErr != nil {
			utils.Warnf("Failed to close QUIC connection for %s: %v", addr, closeErr)
		}
		delete(q.connections, addr)
	}
	q.connLock.Unlock()

	// 关闭监听器
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

// generateQUICConfig 生成QUIC配置
func generateQUICConfig() *quic.Config {
	return &quic.Config{
		MaxIdleTimeout:          quicMaxIdleTimeout,
		KeepAlivePeriod:         quicKeepAlivePeriod,
		MaxIncomingStreams:      quicMaxIncomingStreams,
		MaxIncomingUniStreams:   quicMaxIncomingStreams,
		EnableDatagrams:         false,
		DisablePathMTUDiscovery: false,
		Allow0RTT:               false,
	}
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
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		utils.Errorf("Failed to generate serial number: %v", err)
		return nil
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Tunnox"},
			CommonName:   "Tunnox QUIC Server",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
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
		MinVersion:   tls.VersionTLS13, // QUIC 需要 TLS 1.3
	}
}
