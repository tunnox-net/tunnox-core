package adapter

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/xtaci/kcp-go/v5"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
	"tunnox-core/internal/protocol/session"
)

// KCP 配置常量
const (
	// KcpDataShards FEC 数据分片数（0 表示禁用 FEC）
	KcpDataShards = 0

	// KcpParityShards FEC 校验分片数
	KcpParityShards = 0

	// KcpSndWnd 发送窗口大小
	KcpSndWnd = 1024

	// KcpRcvWnd 接收窗口大小
	KcpRcvWnd = 1024

	// KcpNoDelay 模式参数
	// nodelay=1, interval=10ms, resend=2, nc=1 (最快模式)
	KcpNoDelay  = 1
	KcpInterval = 10
	KcpResend   = 2
	KcpNC       = 1

	// KcpMTU 最大传输单元
	KcpMTU = 1400

	// KcpStreamBufferSize 流缓冲区大小
	KcpStreamBufferSize = 4 * 1024 * 1024 // 4MB
)

// KcpAdapter KCP 协议适配器
type KcpAdapter struct {
	BaseAdapter
	listener *kcp.Listener
}

// NewKcpAdapter 创建 KCP 适配器
func NewKcpAdapter(parentCtx context.Context, sess session.Session) *KcpAdapter {
	k := &KcpAdapter{
		BaseAdapter: BaseAdapter{},
	}
	k.SetName("kcp")
	k.SetSession(sess)
	k.SetCtx(parentCtx, k.onClose)
	k.SetProtocolAdapter(k)
	return k
}

// Dial 建立 KCP 连接（客户端）
func (k *KcpAdapter) Dial(addr string) (io.ReadWriteCloser, error) {
	corelog.Infof("KcpAdapter: dialing %s", addr)

	// 创建 KCP 连接（无加密，无 FEC）
	conn, err := kcp.DialWithOptions(addr, nil, KcpDataShards, KcpParityShards)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError,
			"failed to dial KCP")
	}

	// 配置 KCP 参数
	configureKCP(conn)

	corelog.Infof("KcpAdapter: connected to %s, local=%s",
		addr, conn.LocalAddr())

	return &kcpConn{conn: conn}, nil
}

// Listen 启动 KCP 监听（服务端）
func (k *KcpAdapter) Listen(addr string) error {
	corelog.Infof("KcpAdapter: listening on %s", addr)

	// 创建 KCP 监听器（无加密，无 FEC）
	listener, err := kcp.ListenWithOptions(addr, nil, KcpDataShards, KcpParityShards)
	if err != nil {
		return coreerrors.Wrap(err, coreerrors.CodeNetworkError,
			"failed to listen KCP")
	}

	// 配置监听器缓冲区
	if err := listener.SetReadBuffer(KcpStreamBufferSize); err != nil {
		corelog.Warnf("KcpAdapter: failed to set read buffer: %v", err)
	}
	if err := listener.SetWriteBuffer(KcpStreamBufferSize); err != nil {
		corelog.Warnf("KcpAdapter: failed to set write buffer: %v", err)
	}

	k.listener = listener

	corelog.Infof("KcpAdapter: listening started on %s", addr)
	return nil
}

// Accept 接受 KCP 连接（服务端）
func (k *KcpAdapter) Accept() (io.ReadWriteCloser, error) {
	if k.listener == nil {
		return nil, coreerrors.New(coreerrors.CodeNotConfigured,
			"KCP listener not initialized")
	}

	conn, err := k.listener.AcceptKCP()
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError,
			"failed to accept KCP connection")
	}

	// 配置 KCP 参数
	configureKCP(conn)

	corelog.Infof("KcpAdapter: accepted connection from %s", conn.RemoteAddr())

	return &kcpConn{conn: conn}, nil
}

// getConnectionType 返回连接类型
func (k *KcpAdapter) getConnectionType() string {
	return "KCP"
}

// onClose 清理资源
func (k *KcpAdapter) onClose() error {
	corelog.Info("KcpAdapter: closing...")

	var err error

	if k.listener != nil {
		if closeErr := k.listener.Close(); closeErr != nil {
			corelog.Errorf("KcpAdapter: failed to close listener: %v", closeErr)
			err = closeErr
		}
		k.listener = nil
	}

	// 调用基类清理
	baseErr := k.BaseAdapter.onClose()
	if err == nil {
		err = baseErr
	}

	corelog.Info("KcpAdapter: closed")
	return err
}

// configureKCP 配置 KCP 连接参数
func configureKCP(conn *kcp.UDPSession) {
	// 设置 NoDelay 模式（最快）
	// nodelay: 0=关闭, 1=开启
	// interval: 内部更新时钟间隔（毫秒）
	// resend: 快速重传触发次数
	// nc: 0=关闭拥塞控制, 1=开启
	conn.SetNoDelay(KcpNoDelay, KcpInterval, KcpResend, KcpNC)

	// 设置窗口大小
	conn.SetWindowSize(KcpSndWnd, KcpRcvWnd)

	// 设置 MTU
	conn.SetMtu(KcpMTU)

	// 设置缓冲区
	conn.SetReadBuffer(KcpStreamBufferSize)
	conn.SetWriteBuffer(KcpStreamBufferSize)

	// 设置 ACK 无延迟（立即发送 ACK）
	conn.SetACKNoDelay(true)
}

// kcpConn 包装 KCP 连接
type kcpConn struct {
	conn *kcp.UDPSession
}

func (c *kcpConn) Read(b []byte) (int, error) {
	return c.conn.Read(b)
}

func (c *kcpConn) Write(b []byte) (int, error) {
	return c.conn.Write(b)
}

func (c *kcpConn) Close() error {
	return c.conn.Close()
}

func (c *kcpConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

func (c *kcpConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

func (c *kcpConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

func (c *kcpConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *kcpConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
