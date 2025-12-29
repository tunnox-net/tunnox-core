package session

import (
	"fmt"
	"net"

	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// ============================================================================
// 辅助函数
// ============================================================================

// ToNetConn 统一接口：将适配层连接转换为 net.Conn
type ToNetConn interface {
	ToNetConn() net.Conn
}

// extractNetConn 从types.Connection中提取底层的net.Conn
func (s *SessionManager) extractNetConn(conn *types.Connection) net.Conn {
	if conn.RawConn != nil {
		return conn.RawConn
	}

	if conn.Stream != nil {
		// 使用接口获取 Reader，而不是类型断言
		reader := conn.Stream.GetReader()

		// 优先使用统一接口
		if toNetConn, ok := reader.(ToNetConn); ok {
			return toNetConn.ToNetConn()
		}

		// 回退：直接实现 net.Conn
		if netConn, ok := reader.(net.Conn); ok {
			return netConn
		}
	}
	return nil
}

// ============================================================================
// 数据包处理
// ============================================================================

// ProcessPacket 处理数据包（兼容旧接口）
func (s *SessionManager) ProcessPacket(connID string, pkt *packet.TransferPacket) error {
	// 转换为 StreamPacket
	streamPacket := &types.StreamPacket{
		ConnectionID: connID,
		Packet:       pkt,
	}

	return s.HandlePacket(streamPacket)
}

// HandlePacket 处理数据包（统一入口）
func (s *SessionManager) HandlePacket(connPacket *types.StreamPacket) error {
	if connPacket == nil || connPacket.Packet == nil {
		return fmt.Errorf("invalid packet: nil")
	}

	packetType := connPacket.Packet.PacketType

	// 根据数据包类型分发（忽略压缩/加密标志）
	switch {
	case packetType.IsJsonCommand() || packetType.IsCommandResp():
		return s.handleCommandPacket(connPacket)

	case packetType&0x3F == packet.Handshake:
		return s.handleHandshake(connPacket)

	case packetType&0x3F == packet.TunnelOpen:
		return s.handleTunnelOpen(connPacket)

	case packetType.IsHeartbeat():
		return s.handleHeartbeat(connPacket)

	default:
		corelog.Warnf("Unhandled packet type: %v", packetType)
		return fmt.Errorf("unhandled packet type: %v", packetType)
	}
}
