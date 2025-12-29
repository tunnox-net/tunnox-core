//go:build !no_websocket

// Package client transport_websocket.go
// WebSocket 传输层 facade - 向后兼容层
// 实际实现已移至 internal/client/transport 子包
// 使用 -tags no_websocket 可以排除此协议以减小二进制体积
package client

import (
	"context"
	"net"

	"tunnox-core/internal/client/transport"
)

// websocketStreamConn WebSocket 连接封装
// Deprecated: 请使用 transport.WebSocketStreamConn
type websocketStreamConn = transport.WebSocketStreamConn

// newWebSocketStreamConn 创建 WebSocket 连接
// Deprecated: 请使用 transport.NewWebSocketStreamConn
func newWebSocketStreamConn(wsURL string) (*websocketStreamConn, error) {
	return transport.NewWebSocketStreamConn(wsURL)
}

// normalizeWebSocketURL 规范化 WebSocket URL
// Deprecated: 请使用 transport.NormalizeWebSocketURL
func normalizeWebSocketURL(address string) (string, error) {
	return transport.NormalizeWebSocketURL(address)
}

// dialWebSocket 建立 WebSocket 连接
// Deprecated: 请使用 transport.DialWebSocket
func dialWebSocket(ctx context.Context, address string) (net.Conn, error) {
	return transport.DialWebSocket(ctx, address)
}
