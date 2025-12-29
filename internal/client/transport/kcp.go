//go:build !no_kcp

// Package transport KCP 传输层实现
// 提供 KCP 协议的网络连接封装
// 使用 -tags no_kcp 可以排除此协议以减小二进制体积
package transport

import (
	"context"
	"net"
	"time"

	"github.com/xtaci/kcp-go/v5"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

func init() {
	RegisterProtocol("kcp", 40, DialKCP) // 优先级 40（最低）
}

// KCP 配置常量（与服务端保持一致）
const (
	KCPDataShards       = 0
	KCPParityShards     = 0
	KCPSndWnd           = 1024
	KCPRcvWnd           = 1024
	KCPNoDelay          = 1
	KCPInterval         = 10
	KCPResend           = 2
	KCPNC               = 1
	KCPMTU              = 1400
	KCPStreamBufferSize = 4 * 1024 * 1024
)

// KCPConnWrapper 包装 KCP 连接以实现 net.Conn 接口
type KCPConnWrapper struct {
	conn *kcp.UDPSession
}

// NewKCPConnWrapper 创建 KCP 连接包装器
func NewKCPConnWrapper(conn *kcp.UDPSession) *KCPConnWrapper {
	return &KCPConnWrapper{conn: conn}
}

func (w *KCPConnWrapper) Read(b []byte) (int, error) {
	return w.conn.Read(b)
}

func (w *KCPConnWrapper) Write(b []byte) (int, error) {
	return w.conn.Write(b)
}

func (w *KCPConnWrapper) Close() error {
	return w.conn.Close()
}

func (w *KCPConnWrapper) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

func (w *KCPConnWrapper) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

func (w *KCPConnWrapper) SetDeadline(t time.Time) error {
	return w.conn.SetDeadline(t)
}

func (w *KCPConnWrapper) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

func (w *KCPConnWrapper) SetWriteDeadline(t time.Time) error {
	return w.conn.SetWriteDeadline(t)
}

// DialKCP 建立 KCP 连接
func DialKCP(_ context.Context, address string) (net.Conn, error) {
	corelog.Infof("Client: dialing KCP to %s", address)

	// 创建 KCP 连接（无加密，无 FEC）
	conn, err := kcp.DialWithOptions(address, nil, KCPDataShards, KCPParityShards)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError,
			"failed to dial KCP")
	}

	// 配置 KCP 参数
	conn.SetNoDelay(KCPNoDelay, KCPInterval, KCPResend, KCPNC)
	conn.SetWindowSize(KCPSndWnd, KCPRcvWnd)
	conn.SetMtu(KCPMTU)
	conn.SetReadBuffer(KCPStreamBufferSize)
	conn.SetWriteBuffer(KCPStreamBufferSize)
	conn.SetACKNoDelay(true)

	corelog.Infof("Client: KCP connection established to %s", address)

	return NewKCPConnWrapper(conn), nil
}
