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

// UdpConnWrapper UDP连接包装器，实现io.Reader和io.Writer接口
type UdpConnWrapper struct {
	conn net.PacketConn
	addr net.Addr
	buf  []byte
	pos  int
}

// Read 实现io.Reader接口
func (u *UdpConnWrapper) Read(p []byte) (n int, err error) {
	if u.pos >= len(u.buf) {
		// 读取新的UDP数据包
		u.buf = make([]byte, 65507) // UDP最大包大小
		n, u.addr, err = u.conn.ReadFrom(u.buf)
		if err != nil {
			return 0, err
		}
		u.buf = u.buf[:n]
		u.pos = 0
	}

	// 从缓冲区复制数据
	copyLen := len(u.buf) - u.pos
	if copyLen > len(p) {
		copyLen = len(p)
	}
	copy(p, u.buf[u.pos:u.pos+copyLen])
	u.pos += copyLen
	return copyLen, nil
}

// Write 实现io.Writer接口
func (u *UdpConnWrapper) Write(p []byte) (n int, err error) {
	if u.addr == nil {
		return 0, fmt.Errorf("no target address set")
	}
	return u.conn.WriteTo(p, u.addr)
}

// UdpAdapter UDP协议适配器
type UdpAdapter struct {
	BaseAdapter
	conn        net.PacketConn
	clientConn  net.Conn
	active      bool
	connMutex   sync.RWMutex
	stream      stream.PackageStreamer
	streamMutex sync.RWMutex
	session     *ConnectionSession
}

// NewUdpAdapter 创建新的UDP适配器
func NewUdpAdapter(parentCtx context.Context, session *ConnectionSession) *UdpAdapter {
	adapter := &UdpAdapter{
		session: session,
	}
	adapter.SetName("udp")
	adapter.SetCtx(parentCtx, adapter.onClose)
	return adapter
}

// ConnectTo 连接到UDP服务器
func (u *UdpAdapter) ConnectTo(serverAddr string) error {
	u.connMutex.Lock()
	defer u.connMutex.Unlock()

	if u.clientConn != nil {
		return fmt.Errorf("already connected")
	}

	// 解析服务器地址
	addr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// 创建UDP连接
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to connect to UDP server: %w", err)
	}

	u.clientConn = conn
	u.SetAddr(serverAddr)

	// 创建数据流
	u.streamMutex.Lock()
	u.stream = stream.NewPackageStream(conn, conn, u.Ctx())
	u.streamMutex.Unlock()

	return nil
}

// ListenFrom 设置UDP监听地址
func (u *UdpAdapter) ListenFrom(listenAddr string) error {
	u.SetAddr(listenAddr)
	return nil
}

// Start 启动UDP服务器
func (u *UdpAdapter) Start(ctx context.Context) error {
	if u.Addr() == "" {
		return fmt.Errorf("address not set")
	}

	// 解析监听地址
	addr, err := net.ResolveUDPAddr("udp", u.Addr())
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// 创建UDP监听器
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	u.conn = conn
	u.active = true
	go u.receiveLoop()
	return nil
}

// receiveLoop UDP接收循环
func (u *UdpAdapter) receiveLoop() {
	buffer := make([]byte, 65507) // UDP最大包大小

	for u.active {
		n, addr, err := u.conn.ReadFrom(buffer)
		if err != nil {
			if !u.IsClosed() {
				utils.Errorf("UDP read error: %v", err)
			}
			return
		}

		// 为每个客户端创建独立的goroutine处理
		go u.handlePacket(buffer[:n], addr)
	}
}

// handlePacket 处理UDP数据包
func (u *UdpAdapter) handlePacket(data []byte, addr net.Addr) {
	utils.Infof("UDP adapter handling packet from %s, size: %d", addr, len(data))

	// 调用ConnectionSession.AcceptConnection处理连接
	if u.session != nil {
		// 创建虚拟连接包装器
		virtualConn := &UdpVirtualConn{
			data: data,
			addr: addr,
			conn: u.conn,
		}
		u.session.AcceptConnection(virtualConn, virtualConn)
	} else {
		// 如果没有session，使用默认的echo处理
		utils.Infof("Echoing UDP packet back to %s", addr)
		if _, err := u.conn.WriteTo(data, addr); err != nil {
			utils.Errorf("Failed to echo UDP packet: %v", err)
		}
	}
}

// UdpVirtualConn UDP虚拟连接，用于单次数据包处理
type UdpVirtualConn struct {
	data []byte
	addr net.Addr
	conn net.PacketConn
	pos  int
}

// Read 实现io.Reader接口
func (u *UdpVirtualConn) Read(p []byte) (n int, err error) {
	if u.pos >= len(u.data) {
		return 0, io.EOF
	}
	copyLen := len(u.data) - u.pos
	if copyLen > len(p) {
		copyLen = len(p)
	}
	copy(p, u.data[u.pos:u.pos+copyLen])
	u.pos += copyLen
	return copyLen, nil
}

// Write 实现io.Writer接口
func (u *UdpVirtualConn) Write(p []byte) (n int, err error) {
	return u.conn.WriteTo(p, u.addr)
}

// Stop 停止UDP适配器
func (u *UdpAdapter) Stop() error {
	u.active = false
	if u.conn != nil {
		u.conn.Close()
		u.conn = nil
	}
	u.connMutex.Lock()
	if u.clientConn != nil {
		u.clientConn.Close()
		u.clientConn = nil
	}
	u.connMutex.Unlock()
	u.streamMutex.Lock()
	if u.stream != nil {
		u.stream.Close()
		u.stream = nil
	}
	u.streamMutex.Unlock()
	return nil
}

// GetReader 获取读取器
func (u *UdpAdapter) GetReader() io.Reader {
	u.streamMutex.RLock()
	defer u.streamMutex.RUnlock()
	if u.stream != nil {
		return u.stream.GetReader()
	}
	return nil
}

// GetWriter 获取写入器
func (u *UdpAdapter) GetWriter() io.Writer {
	u.streamMutex.RLock()
	defer u.streamMutex.RUnlock()
	if u.stream != nil {
		return u.stream.GetWriter()
	}
	return nil
}

// Close 关闭适配器
func (u *UdpAdapter) Close() {
	_ = u.Stop()
	u.BaseAdapter.Close()
}

// onClose 关闭回调
func (u *UdpAdapter) onClose() {
	_ = u.Stop()
}
