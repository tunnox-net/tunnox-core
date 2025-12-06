package udp

import (
	"context"
	"io"
	"net"
	"sync"
	"time"
	"tunnox-core/internal/core/dispose"
	"tunnox-core/internal/utils"
)

// Transport 封装 UDPConn，对上提供 io.ReadWriteCloser
type Transport struct {
	*dispose.ManagerBase
	conn       *net.UDPConn
	remoteAddr *net.UDPAddr
	session    *SessionState
	sender     *Sender
	receiver   *Receiver

	// 上层读取缓冲区（按顺序存放完整 payload）
	readBufMu sync.Mutex
	readBuf   []byte
	readCond  *sync.Cond

	// 初始数据报缓冲区（用于 Accept 时传递第一个数据报）
	initialPacket   []byte
	initialPacketMu sync.Mutex

	closed  bool
	closeMu sync.Mutex
}

// NewTransport 创建 UDP 传输层，并启动 Receiver 读循环与 Sender 重传循环。
// - conn: 一个已连接到对端的 UDPConn（建议使用 net.DialUDP 返回的）
// - remoteAddr: 远程地址
// - sessionID: 会话 ID
// - initialPacket: 可选的初始数据报（用于 Accept 时传递第一个数据报）
func NewTransport(conn *net.UDPConn, remoteAddr *net.UDPAddr, sessionID uint32, parentCtx context.Context, initialPacket ...[]byte) *Transport {
	t := &Transport{
		ManagerBase: dispose.NewManager("UDPTransport", parentCtx),
		conn:        conn,
		remoteAddr:  remoteAddr,
		closed:      false,
		readBuf:     make([]byte, 0, 4096),
	}
	t.readCond = sync.NewCond(&t.readBufMu)

	// 保存初始数据报（如果有）
	if len(initialPacket) > 0 && len(initialPacket[0]) > 0 {
		t.initialPacket = make([]byte, len(initialPacket[0]))
		copy(t.initialPacket, initialPacket[0])
	}

	// 创建会话状态
	key := SessionKey{
		SessionID: sessionID,
		StreamID:  0,
	}
	t.session = NewSessionState(key)

	// 创建发送端和接收端
	cfg := DefaultConfig()
	t.sender = NewSender(conn, remoteAddr, t.session, cfg)
	t.receiver = NewReceiver(conn, remoteAddr, t.session, t.sender, t.onPacket)

	// 启动接收循环和重传循环
	t.receiver.StartReadLoop()
	t.sender.StartRetransmitLoop()

	// 如果有初始数据报，立即处理（在 receiver 启动后）
	// 注意：receiver 的 StartReadLoop 已经启动，但需要确保它已经开始读取
	// 这里直接处理初始数据报，不需要等待
	if len(initialPacket) > 0 && len(initialPacket[0]) > 0 {
		packet := make([]byte, len(initialPacket[0]))
		copy(packet, initialPacket[0])
		// 立即处理初始数据报（receiver 已经创建，可以直接调用）
		t.receiver.HandleDatagram(packet, t.remoteAddr)
	}

	// 注册清理函数
	t.AddCleanHandler(t.onClose)

	return t
}

// onPacket 接收端回调：将重组好的 payload 添加到读取缓冲区
func (t *Transport) onPacket(payload []byte) error {
	t.readBufMu.Lock()
	defer t.readBufMu.Unlock()

	if t.closed {
		return io.ErrClosedPipe
	}

	t.readBuf = append(t.readBuf, payload...)
	t.readCond.Signal()
	return nil
}

// Write 将数据写入 UDP 会话。
// 简化起见，可以把一次 Write 作为一个逻辑包发送。
func (t *Transport) Write(p []byte) (int, error) {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return 0, io.ErrClosedPipe
	}
	t.closeMu.Unlock()

	// 拷贝数据，避免外部修改
	payload := make([]byte, len(p))
	copy(payload, p)

	// 添加调试日志
	utils.Infof("UDP Transport.Write called, payload size=%d, sessionID=%d", len(payload), t.session.Key.SessionID)

	if err := t.sender.SendLogicalPacket(payload); err != nil {
		utils.Errorf("UDP Transport.Write: SendLogicalPacket failed: %v", err)
		return 0, err
	}

	utils.Infof("UDP Transport.Write: SendLogicalPacket succeeded, payload size=%d", len(payload))
	return len(p), nil
}

// Read 从内部缓冲区中读取数据。
// Receiver 每重组一个 payload，会 append 到 readBuf 并唤醒等待。
func (t *Transport) Read(p []byte) (int, error) {
	t.readBufMu.Lock()
	defer t.readBufMu.Unlock()

	for len(t.readBuf) == 0 {
		t.closeMu.Lock()
		closed := t.closed
		t.closeMu.Unlock()

		if closed {
			return 0, io.EOF
		}

		t.readCond.Wait()
	}

	n := copy(p, t.readBuf)
	t.readBuf = t.readBuf[n:]

	return n, nil
}

// Close 关闭 Sender/Receiver 和底层 conn。
func (t *Transport) Close() error {
	t.closeMu.Lock()
	if t.closed {
		t.closeMu.Unlock()
		return nil
	}
	t.closed = true
	t.closeMu.Unlock()

	// 唤醒等待的 Read
	t.readBufMu.Lock()
	t.readCond.Broadcast()
	t.readBufMu.Unlock()

	// 关闭 sender 和 receiver
	if t.sender != nil {
		_ = t.sender.Close()
	}
	if t.receiver != nil {
		_ = t.receiver.Close()
	}

	// 关闭底层连接
	if t.conn != nil {
		_ = t.conn.Close()
	}

	// 调用 dispose 清理
	result := t.Dispose.Close()
	if result.HasErrors() {
		return result
	}

	return nil
}

// onClose 清理回调
func (t *Transport) onClose() error {
	// 资源已在 Close() 中清理
	return nil
}

// LocalAddr 返回本地地址
func (t *Transport) LocalAddr() net.Addr {
	if t.conn != nil {
		return t.conn.LocalAddr()
	}
	return nil
}

// RemoteAddr 返回远程地址
func (t *Transport) RemoteAddr() net.Addr {
	return t.remoteAddr
}

// SetDeadline 设置读写截止时间
func (t *Transport) SetDeadline(deadline time.Time) error {
	// UDP Transport 不支持设置截止时间
	return nil
}

// SetReadDeadline 设置读截止时间
func (t *Transport) SetReadDeadline(deadline time.Time) error {
	// UDP Transport 不支持设置读截止时间
	return nil
}

// SetWriteDeadline 设置写截止时间
func (t *Transport) SetWriteDeadline(deadline time.Time) error {
	// UDP Transport 不支持设置写截止时间
	return nil
}
