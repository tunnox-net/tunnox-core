package session

import (
corelog "tunnox-core/internal/core/log"
	"fmt"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

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
