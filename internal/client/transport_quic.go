//go:build !no_quic

// Package client transport_quic.go
// QUIC 传输层 facade - 向后兼容层
// 实际实现已移至 internal/client/transport 子包
// 使用 -tags no_quic 可以排除此协议以减小二进制体积
package client

import (
	"context"
	"net"

	"tunnox-core/internal/client/transport"
)

// quicStreamConn QUIC 连接封装
// Deprecated: 请使用 transport.QUICStreamConn
type quicStreamConn = transport.QUICStreamConn

// newQUICStreamConn 创建 QUIC 连接
// Deprecated: 请使用 transport.NewQUICStreamConn
func newQUICStreamConn(ctx context.Context, address string) (*quicStreamConn, error) {
	return transport.NewQUICStreamConn(ctx, address)
}

// dialQUIC 建立 QUIC 连接
// Deprecated: 请使用 transport.DialQUIC
func dialQUIC(ctx context.Context, address string) (net.Conn, error) {
	return transport.DialQUIC(ctx, address)
}
