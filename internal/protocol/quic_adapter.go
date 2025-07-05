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
		// 使用带超时的Accept，以便能够响应上下文取消
		acceptCtx, cancel := context.WithTimeout(q.Ctx(), 100*time.Millisecond)
		conn, err := q.listener.Accept(acceptCtx)
		cancel()

		if err != nil {
			if q.IsClosed() || q.Ctx().Err() != nil {
				// 正常关闭，不记录错误
				utils.Infof("QUIC acceptLoop goroutine exited (normal close)")
				return
			}
			if err != context.DeadlineExceeded {
				utils.Errorf("QUIC accept error: %v", err)
			}
			continue
		}
		go q.handleConnection(conn)
	}
	utils.Infof("QUIC acceptLoop goroutine exited (active=false)")
}

// handleConnection 处理QUIC连接
func (q *QuicAdapter) handleConnection(conn *quic.Conn) {
	defer func() {
		utils.Infof("QUIC handleConnection goroutine exited for %s", conn.RemoteAddr())
		conn.CloseWithError(0, "connection closed")
	}()
	utils.Infof("QUIC adapter handling connection from %s", conn.RemoteAddr())

	for {
		select {
		case <-q.Ctx().Done():
			utils.Infof("QUIC handleConnection goroutine exited for %s (context done)", conn.RemoteAddr())
			return
		default:
			// 原有逻辑
			streamCtx, cancel := context.WithTimeout(q.Ctx(), 100*time.Millisecond)
			stream, err := conn.AcceptStream(streamCtx)
			cancel()

			if err != nil {
				if q.IsClosed() || q.Ctx().Err() != nil {
					utils.Infof("QUIC handleConnection stream accept exited (normal close)")
					return
				}
				if err != context.DeadlineExceeded {
					utils.Errorf("QUIC stream accept error: %v", err)
				}
				return
			}
			go q.handleStream(stream)
		}
	}
}

// handleStream 处理QUIC流
func (q *QuicAdapter) handleStream(stream *quic.Stream) {
	defer func() {
		utils.Infof("QUIC handleStream goroutine exited for stream %d", stream.StreamID())
		stream.Close()
	}()
	utils.Infof("QUIC adapter handling stream %d", stream.StreamID())

	for {
		select {
		case <-q.Ctx().Done():
			utils.Infof("QUIC handleStream goroutine exited for stream %d (context done)", stream.StreamID())
			return
		default:
			// 原有逻辑
			buf := make([]byte, 1024)
			n, err := stream.Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				if _, err := stream.Write(buf[:n]); err != nil {
					break
				}
			}
		}
	}
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

// onClose 关闭回调
func (q *QuicAdapter) onClose() {
	q.active = false
	if q.listener != nil {
		_ = q.listener.Close()
		q.listener = nil
	}
	q.connMutex.Lock()
	if q.connection != nil {
		_ = q.connection.CloseWithError(0, "server shutdown")
		q.connection = nil
	}
	q.connMutex.Unlock()
	q.streamMutex.Lock()
	if q.stream != nil {
		q.stream.Close()
		q.stream = nil
	}
	q.streamMutex.Unlock()
}
