package client

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"

	udpprotocol "tunnox-core/internal/protocol/udp"
)

// dialUDP 建立 UDP 控制连接
func dialUDP(ctx context.Context, address string) (net.Conn, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial UDP: %w", err)
	}

	// 生成 SessionID（简化实现，使用随机数）
	sessionID := generateSessionID()

	// 创建 UDP Transport（使用我们实现的 UDP 协议层）
	transport := udpprotocol.NewTransport(conn, udpAddr, sessionID, ctx)

	// 返回 Transport 作为 net.Conn（Transport 实现了 io.ReadWriteCloser）
	return transport, nil
}

// generateSessionID 生成随机的 SessionID
func generateSessionID() uint32 {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// 如果随机数生成失败，使用时间戳
		return uint32(0)
	}
	return binary.BigEndian.Uint32(buf[:])
}

