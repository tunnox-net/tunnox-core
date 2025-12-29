// Package client socks5_listener.go
// SOCKS5 监听器 facade - 向后兼容层
// 实际实现已移至 internal/client/socks5 子包
package client

import (
	"context"
	"net"

	"tunnox-core/internal/client/socks5"
)

// SOCKS5 协议常量（向后兼容）
// Deprecated: 请使用 socks5 包中的常量
const (
	socks5Version     = socks5.Version
	socks5AuthNone    = socks5.AuthNone
	socks5AuthNoMatch = socks5.AuthNoMatch
	socks5CmdConnect  = socks5.CmdConnect
	socks5AddrIPv4    = socks5.AddrIPv4
	socks5AddrDomain  = socks5.AddrDomain
	socks5AddrIPv6    = socks5.AddrIPv6
	socks5RepSuccess  = socks5.RepSuccess
	socks5RepFailure  = socks5.RepFailure
)

// SOCKS5ListenerConfig SOCKS5 监听器配置
// Deprecated: 请使用 socks5.ListenerConfig
type SOCKS5ListenerConfig = socks5.ListenerConfig

// SOCKS5TunnelCreator 隧道创建接口
// Deprecated: 请使用 socks5.TunnelCreator
type SOCKS5TunnelCreator = socks5.TunnelCreator

// SOCKS5Listener SOCKS5 代理监听器
// Deprecated: 请使用 socks5.Listener
type SOCKS5Listener = socks5.Listener

// NewSOCKS5Listener 创建 SOCKS5 监听器
// Deprecated: 请使用 socks5.NewListener
func NewSOCKS5Listener(
	ctx context.Context,
	config *SOCKS5ListenerConfig,
	tunnelCreator SOCKS5TunnelCreator,
) *SOCKS5Listener {
	return socks5.NewListener(ctx, config, tunnelCreator)
}

// socks5Handshake 类型适配（内部使用）
func socks5Handshake(l *SOCKS5Listener, conn net.Conn) (string, int, error) {
	return l.Handshake(conn)
}

// sendSOCKS5Success 发送成功响应（内部使用）
func sendSOCKS5Success(l *SOCKS5Listener, conn net.Conn) {
	l.SendSuccess(conn)
}

// sendSOCKS5Error 发送错误响应（内部使用）
func sendSOCKS5Error(l *SOCKS5Listener, conn net.Conn, rep byte) {
	l.SendError(conn, rep)
}
