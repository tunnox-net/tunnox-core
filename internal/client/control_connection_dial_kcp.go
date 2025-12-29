//go:build !no_kcp

// Package client control_connection_dial_kcp.go
// KCP 传输层 facade - 向后兼容层
// 实际实现已移至 internal/client/transport 子包
// 使用 -tags no_kcp 可以排除此协议以减小二进制体积
package client

import (
	"context"
	"net"

	"tunnox-core/internal/client/transport"
)

// KCP 配置常量（向后兼容）
// Deprecated: 请使用 transport 包中的常量
const (
	kcpDataShards       = transport.KCPDataShards
	kcpParityShards     = transport.KCPParityShards
	kcpSndWnd           = transport.KCPSndWnd
	kcpRcvWnd           = transport.KCPRcvWnd
	kcpNoDelay          = transport.KCPNoDelay
	kcpInterval         = transport.KCPInterval
	kcpResend           = transport.KCPResend
	kcpNC               = transport.KCPNC
	kcpMTU              = transport.KCPMTU
	kcpStreamBufferSize = transport.KCPStreamBufferSize
)

// kcpConnWrapper KCP 连接包装器
// Deprecated: 请使用 transport.KCPConnWrapper
type kcpConnWrapper = transport.KCPConnWrapper

// dialKCP 建立 KCP 连接
// Deprecated: 请使用 transport.DialKCP
func dialKCP(ctx context.Context, address string) (net.Conn, error) {
	return transport.DialKCP(ctx, address)
}
