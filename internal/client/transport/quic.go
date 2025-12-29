//go:build !no_quic

// Package transport QUIC 传输层实现
// 提供 QUIC 协议的网络连接封装
// 使用 -tags no_quic 可以排除此协议以减小二进制体积
package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	corelog "tunnox-core/internal/core/log"

	"github.com/quic-go/quic-go"
)

func init() {
	RegisterProtocol("quic", 20, DialQUIC) // 优先级 20
}

// QUICStreamConn wraps a QUIC stream to implement net.Conn interface
type QUICStreamConn struct {
	stream    *quic.Stream
	conn      *quic.Conn
	closeOnce sync.Once
	closed    chan struct{}
}

// NewQUICStreamConn creates a new QUIC stream connection
func NewQUICStreamConn(ctx context.Context, address string) (*QUICStreamConn, error) {
	corelog.Debugf("QUIC: connecting to %s", address)

	// Create TLS config (skip verification for now, can be configured later)
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"tunnox-quic"},
	}

	// Create QUIC config
	quicConf := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}

	// Dial QUIC connection
	conn, err := quic.DialAddr(ctx, address, tlsConf, quicConf)
	if err != nil {
		return nil, fmt.Errorf("quic dial failed: %w", err)
	}

	corelog.Infof("QUIC: connection established to %s", address)

	// Open a stream
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		conn.CloseWithError(0, "failed to open stream")
		return nil, fmt.Errorf("quic open stream failed: %w", err)
	}

	corelog.Infof("QUIC: stream opened")

	qsc := &QUICStreamConn{
		stream: stream,
		conn:   conn,
		closed: make(chan struct{}),
	}

	return qsc, nil
}

// Read implements io.Reader
func (c *QUICStreamConn) Read(p []byte) (int, error) {
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
func (c *QUICStreamConn) Write(p []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.ErrClosedPipe
	default:
	}

	return c.stream.Write(p)
}

// Close implements io.Closer
func (c *QUICStreamConn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)

		// Close stream
		if c.stream != nil {
			c.stream.Close()
		}

		// Close connection
		if c.conn != nil {
			err = c.conn.CloseWithError(0, "normal closure")
		}

		corelog.Debugf("QUIC: connection closed")
	})
	return err
}

// LocalAddr implements net.Conn
func (c *QUICStreamConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr implements net.Conn
func (c *QUICStreamConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetDeadline implements net.Conn
func (c *QUICStreamConn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline implements net.Conn
func (c *QUICStreamConn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn
func (c *QUICStreamConn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}

// DialQUIC creates a QUIC connection to the server
func DialQUIC(ctx context.Context, address string) (net.Conn, error) {
	return NewQUICStreamConn(ctx, address)
}
