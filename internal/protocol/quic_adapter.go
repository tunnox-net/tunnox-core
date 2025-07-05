package protocol

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
	"sync"
	"time"
	sm "tunnox-core/internal/stream"
	"tunnox-core/internal/utils"

	"github.com/quic-go/quic-go"
)

// QuicStreamWrapper QUIC流包装器，实现io.Reader和io.Writer接口
type QuicStreamWrapper struct {
	stream *quic.Stream
}

// Read 实现io.Reader接口
func (q *QuicStreamWrapper) Read(p []byte) (n int, err error) {
	return q.stream.Read(p)
}

// Write 实现io.Writer接口
func (q *QuicStreamWrapper) Write(p []byte) (n int, err error) {
	return q.stream.Write(p)
}

// Close 关闭流
func (q *QuicStreamWrapper) Close() error {
	return q.stream.Close()
}

// QuicAdapter QUIC协议适配器
type QuicAdapter struct {
	BaseAdapter
	listener    *quic.Listener
	connection  *quic.Conn
	active      bool
	connMutex   sync.RWMutex
	stream      sm.PackageStreamer
	streamMutex sync.RWMutex
	session     *ConnectionSession
	tlsConfig   *tls.Config
}

// NewQuicAdapter 创建新的QUIC适配器
func NewQuicAdapter(parentCtx context.Context, session *ConnectionSession) *QuicAdapter {
	adapter := &QuicAdapter{
		session: session,
	}
	adapter.SetName("quic")
	adapter.SetCtx(parentCtx, adapter.onClose)
	adapter.tlsConfig = generateTLSConfig()
	return adapter
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

// ConnectTo 连接到QUIC服务器
func (q *QuicAdapter) ConnectTo(serverAddr string) error {
	q.connMutex.Lock()
	defer q.connMutex.Unlock()

	if q.connection != nil {
		return fmt.Errorf("already connected")
	}

	// 连接到QUIC服务器
	conn, err := quic.DialAddr(context.Background(), serverAddr, q.tlsConfig, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to QUIC server: %w", err)
	}

	q.connection = conn
	q.SetAddr(serverAddr)

	// 打开流
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		conn.CloseWithError(0, "failed to open stream")
		return fmt.Errorf("failed to open QUIC stream: %w", err)
	}

	// 创建数据流
	q.streamMutex.Lock()
	q.stream = sm.NewPackageStream(stream, stream, q.Ctx())
	q.streamMutex.Unlock()

	return nil
}

// ListenFrom 设置QUIC监听地址
func (q *QuicAdapter) ListenFrom(listenAddr string) error {
	q.SetAddr(listenAddr)
	return nil
}

// Start 启动QUIC服务器
func (q *QuicAdapter) Start(ctx context.Context) error {
	if q.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	// 创建QUIC监听器
	listener, err := quic.ListenAddr(q.Addr(), q.tlsConfig, nil)
	if err != nil {
		return fmt.Errorf("failed to listen on QUIC: %w", err)
	}

	q.listener = listener
	q.active = true
	go q.acceptLoop()
	return nil
}

// acceptLoop QUIC接受连接循环
func (q *QuicAdapter) acceptLoop() {
	for q.active {
		conn, err := q.listener.Accept(context.Background())
		if err != nil {
			if !q.IsClosed() {
				utils.Errorf("QUIC accept error: %v", err)
			}
			return
		}
		go q.handleConnection(conn)
	}
}

// handleConnection 处理QUIC连接
func (q *QuicAdapter) handleConnection(conn *quic.Conn) {
	utils.Infof("QUIC adapter handling connection from %s", conn.RemoteAddr())

	// 接受流
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			if !q.IsClosed() {
				utils.Errorf("QUIC stream accept error: %v", err)
			}
			return
		}

		// 为每个流创建独立的goroutine处理
		go q.handleStream(stream)
	}
}

// handleStream 处理QUIC流
func (q *QuicAdapter) handleStream(stream *quic.Stream) {
	defer stream.Close()
	utils.Infof("QUIC adapter handling stream %d", stream.StreamID())

	// 调用ConnectionSession.AcceptConnection处理连接
	if q.session != nil {
		wrapper := &QuicStreamWrapper{stream: stream}
		q.session.AcceptConnection(wrapper, wrapper)
	} else {
		// 如果没有session，使用默认的echo处理
		ctx, cancel := context.WithCancel(q.Ctx())
		defer cancel()
		ps := sm.NewPackageStream(stream, stream, ctx)
		defer ps.Close()

		buf := make([]byte, 1024)
		for {
			n, err := ps.GetReader().Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				if _, err := ps.GetWriter().Write(buf[:n]); err != nil {
					break
				}
			}
		}
	}
}

// Stop 停止QUIC适配器
func (q *QuicAdapter) Stop() error {
	q.active = false
	if q.listener != nil {
		q.listener.Close()
		q.listener = nil
	}
	q.connMutex.Lock()
	if q.connection != nil {
		q.connection.CloseWithError(0, "server shutdown")
		q.connection = nil
	}
	q.connMutex.Unlock()
	q.streamMutex.Lock()
	if q.stream != nil {
		q.stream.Close()
		q.stream = nil
	}
	q.streamMutex.Unlock()
	return nil
}

// GetReader 获取读取器
func (q *QuicAdapter) GetReader() io.Reader {
	q.streamMutex.RLock()
	defer q.streamMutex.RUnlock()
	if q.stream != nil {
		return q.stream.GetReader()
	}
	return nil
}

// GetWriter 获取写入器
func (q *QuicAdapter) GetWriter() io.Writer {
	q.streamMutex.RLock()
	defer q.streamMutex.RUnlock()
	if q.stream != nil {
		return q.stream.GetWriter()
	}
	return nil
}

// Close 关闭适配器
func (q *QuicAdapter) Close() {
	_ = q.Stop()
	q.BaseAdapter.Close()
}

// onClose 关闭回调
func (q *QuicAdapter) onClose() {
	_ = q.Stop()
}
