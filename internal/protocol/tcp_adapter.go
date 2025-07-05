package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"tunnox-core/internal/stream"
	"tunnox-core/internal/utils"
)

type TcpAdapter struct {
	BaseAdapter
	conn        net.Conn
	listener    net.Listener
	active      bool
	connMutex   sync.RWMutex
	stream      stream.PackageStreamer
	streamMutex sync.RWMutex
	session     *ConnectionSession
}

func NewTcpAdapter(parentCtx context.Context, session *ConnectionSession) *TcpAdapter {
	t := &TcpAdapter{
		session: session,
	}
	t.SetName("tcp")
	t.SetCtx(parentCtx, t.onClose)
	return t
}

func (t *TcpAdapter) ConnectTo(serverAddr string) error {
	t.connMutex.Lock()
	defer t.connMutex.Unlock()

	if t.conn != nil {
		return fmt.Errorf("already connected")
	}

	conn, err := net.Dial("tcp", serverAddr)
	if err != nil {
		return fmt.Errorf("failed to connect to tcp server: %w", err)
	}

	t.conn = conn
	t.SetAddr(serverAddr)

	t.streamMutex.Lock()
	t.stream = stream.NewPackageStream(conn, conn, t.Ctx())
	t.streamMutex.Unlock()

	return nil
}

func (t *TcpAdapter) ListenFrom(listenAddr string) error {
	t.SetAddr(listenAddr)
	return nil
}

func (t *TcpAdapter) Start(ctx context.Context) error {
	if t.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	ln, err := net.Listen("tcp", t.Addr())
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	t.listener = ln
	t.active = true
	go t.acceptLoop()
	return nil
}

func (t *TcpAdapter) acceptLoop() {
	for t.active {
		conn, err := t.listener.Accept()
		if err != nil {
			if !t.IsClosed() {
				utils.Errorf("TCP accept error: %v", err)
			}
			return
		}
		go t.handleConn(conn)
	}
}

func (t *TcpAdapter) handleConn(conn net.Conn) {
	defer conn.Close()
	utils.Infof("TCP adapter handling connection from %s", conn.RemoteAddr())

	// 调用ConnectionSession.AcceptConnection处理连接
	if t.session != nil {
		t.session.AcceptConnection(conn, conn)
	} else {
		// 如果没有session，使用默认的echo处理
		ctx, cancel := context.WithCancel(t.Ctx())
		defer cancel()
		ps := stream.NewPackageStream(conn, conn, ctx)
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

func (t *TcpAdapter) Stop() error {
	t.active = false
	if t.listener != nil {
		t.listener.Close()
		t.listener = nil
	}
	t.connMutex.Lock()
	if t.conn != nil {
		t.conn.Close()
		t.conn = nil
	}
	t.connMutex.Unlock()
	t.streamMutex.Lock()
	if t.stream != nil {
		t.stream.Close()
		t.stream = nil
	}
	t.streamMutex.Unlock()
	return nil
}

func (t *TcpAdapter) GetReader() io.Reader {
	t.streamMutex.RLock()
	defer t.streamMutex.RUnlock()
	if t.stream != nil {
		return t.stream.GetReader()
	}
	return nil
}

func (t *TcpAdapter) GetWriter() io.Writer {
	t.streamMutex.RLock()
	defer t.streamMutex.RUnlock()
	if t.stream != nil {
		return t.stream.GetWriter()
	}
	return nil
}

func (t *TcpAdapter) Close() {
	_ = t.Stop()
	t.BaseAdapter.Close()
}

func (t *TcpAdapter) onClose() {
	_ = t.Stop()
}
