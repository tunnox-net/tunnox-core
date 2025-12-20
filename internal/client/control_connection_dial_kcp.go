package client

import (
	"context"
	"net"
	"time"

	"github.com/xtaci/kcp-go/v5"

	coreerrors "tunnox-core/internal/core/errors"
	corelog "tunnox-core/internal/core/log"
)

// KCP 配置常量（与服务端保持一致）
const (
	kcpDataShards       = 0
	kcpParityShards     = 0
	kcpSndWnd           = 1024
	kcpRcvWnd           = 1024
	kcpNoDelay          = 1
	kcpInterval         = 10
	kcpResend           = 2
	kcpNC               = 1
	kcpMTU              = 1400
	kcpStreamBufferSize = 4 * 1024 * 1024
)

// dialKCP 建立 KCP 连接
func dialKCP(_ context.Context, address string) (net.Conn, error) {
	corelog.Infof("Client: dialing KCP to %s", address)

	// 创建 KCP 连接（无加密，无 FEC）
	conn, err := kcp.DialWithOptions(address, nil, kcpDataShards, kcpParityShards)
	if err != nil {
		return nil, coreerrors.Wrap(err, coreerrors.CodeNetworkError,
			"failed to dial KCP")
	}

	// 配置 KCP 参数
	conn.SetNoDelay(kcpNoDelay, kcpInterval, kcpResend, kcpNC)
	conn.SetWindowSize(kcpSndWnd, kcpRcvWnd)
	conn.SetMtu(kcpMTU)
	conn.SetReadBuffer(kcpStreamBufferSize)
	conn.SetWriteBuffer(kcpStreamBufferSize)
	conn.SetACKNoDelay(true)

	corelog.Infof("Client: KCP connection established to %s", address)

	return &kcpConnWrapper{conn: conn}, nil
}

// kcpConnWrapper 包装 KCP 连接以实现 net.Conn 接口
type kcpConnWrapper struct {
	conn *kcp.UDPSession
}

func (w *kcpConnWrapper) Read(b []byte) (int, error) {
	return w.conn.Read(b)
}

func (w *kcpConnWrapper) Write(b []byte) (int, error) {
	return w.conn.Write(b)
}

func (w *kcpConnWrapper) Close() error {
	return w.conn.Close()
}

func (w *kcpConnWrapper) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

func (w *kcpConnWrapper) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

func (w *kcpConnWrapper) SetDeadline(t time.Time) error {
	return w.conn.SetDeadline(t)
}

func (w *kcpConnWrapper) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

func (w *kcpConnWrapper) SetWriteDeadline(t time.Time) error {
	return w.conn.SetWriteDeadline(t)
}
