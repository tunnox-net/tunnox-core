package protocol

import (
	"context"
	"net"
	"sync"
	"tunnox-core/internal/stream"
)

type TcpAdapter struct {
	BaseAdapter
	listener   net.Listener
	connWg     sync.WaitGroup
	active     bool
	activeLock sync.Mutex
}

func NewTcpAdapter(addr string, parentCtx context.Context) *TcpAdapter {
	t := &TcpAdapter{}
	t.SetName("tcp")
	t.SetAddr(addr)
	t.SetCtx(parentCtx, t.onClose)
	return t
}

func (t *TcpAdapter) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", t.Addr())
	if err != nil {
		return err
	}
	t.listener = ln
	t.activeLock.Lock()
	t.active = true
	t.activeLock.Unlock()

	go t.acceptLoop()
	return nil
}

func (t *TcpAdapter) acceptLoop() {
	for {
		t.activeLock.Lock()
		if !t.active {
			t.activeLock.Unlock()
			return
		}
		t.activeLock.Unlock()

		conn, err := t.listener.Accept()
		if err != nil {
			if !t.IsClosed() {
				// 可加日志
			}
			return
		}
		t.connWg.Add(1)
		go t.handleConn(conn)
	}
}

func (t *TcpAdapter) handleConn(conn net.Conn) {
	defer t.connWg.Done()
	ctx, cancel := context.WithCancel(t.Ctx())
	defer cancel()
	ps := stream.NewPackageStream(conn, conn, ctx)
	defer ps.Close()
	// 分层：交给ConnectionSession处理
	sess := NewConnectionSession(ps, ctx)
	sess.Run()
}

func (t *TcpAdapter) Close() error {
	t.activeLock.Lock()
	t.active = false
	t.activeLock.Unlock()
	if t.listener != nil {
		t.listener.Close()
	}
	t.Dispose.Close()
	t.connWg.Wait()
	return nil
}

func (t *TcpAdapter) onClose() {
	t.Close()
}
