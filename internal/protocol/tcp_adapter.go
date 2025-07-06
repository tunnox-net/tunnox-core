package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
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
	session     Session
}

func NewTcpAdapter(parentCtx context.Context, session Session) *TcpAdapter {
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
	t.stream = stream.NewStreamProcessor(conn, conn, t.Ctx())
	t.streamMutex.Unlock()

	return nil
}

func (t *TcpAdapter) ListenFrom(listenAddr string) error {
	t.SetAddr(listenAddr)
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

func (t *TcpAdapter) Start(ctx context.Context) error {

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

		if t.IsClosed() {
			utils.Warnf("TCP connection closed")
			return
		}

		go t.handleConn(conn)
	}
}

func (t *TcpAdapter) handleConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	utils.Infof("TCP adapter handling connection from %s", conn.RemoteAddr())

	// 使用新的 Session 接口处理连接
	if t.session != nil {
		// 初始化连接
		connInfo, err := t.session.InitConnection(conn, conn)
		if err != nil {
			utils.Errorf("Failed to initialize connection: %v", err)
			return
		}
		defer t.session.CloseConnection(connInfo.ID)

		// 处理数据流
		for {
			packet, bytesRead, err := connInfo.Stream.ReadPacket()
			if err != nil {
				if err == io.EOF {
					utils.Infof("Connection closed by peer: %s", connInfo.ID)
				} else {
					utils.Errorf("Failed to read packet: %v", err)
				}
				break
			}

			utils.Debugf("Read packet for connection %s: %d bytes, type: %s",
				connInfo.ID, bytesRead, packet.PacketType)

			// 包装成 StreamPacket
			connPacket := &StreamPacket{
				ConnectionID: connInfo.ID,
				Packet:       packet,
				Timestamp:    time.Now(),
			}

			// 处理数据包
			if err := t.session.HandlePacket(connPacket); err != nil {
				utils.Errorf("Failed to handle packet: %v", err)
				break
			}
		}
	} else {
		// 如果没有session，使用默认的echo处理
		ctx, cancel := context.WithCancel(t.Ctx())
		defer cancel()
		ps := stream.NewStreamProcessor(conn, conn, ctx)
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

func (t *TcpAdapter) onClose() {
	t.active = false
	if t.listener != nil {
		_ = t.listener.Close()
		t.listener = nil
	}
	t.connMutex.Lock()
	defer t.connMutex.Unlock()

	if t.conn != nil {
		_ = t.conn.Close()
		t.conn = nil
	}

	t.streamMutex.Lock()
	defer t.streamMutex.Unlock()
	if t.stream != nil {
		t.stream.Close()
		t.stream = nil
	}
	utils.Infof("TCP adapter closed")
}
