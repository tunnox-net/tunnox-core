package session

import (
	"fmt"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/utils"
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

	// 根据数据包类型分发
	switch packetType {
	case packet.JsonCommand, packet.CommandResp:
		return s.handleCommandPacket(connPacket)

	case packet.Handshake:
		return s.handleHandshake(connPacket)

	case packet.TunnelOpen:
		return s.handleTunnelOpen(connPacket)

	default:
		utils.Warnf("Unhandled packet type: %v", packetType)
		return fmt.Errorf("unhandled packet type: %v", packetType)
	}
}

// handleHandshake 处理握手请求
func (s *SessionManager) handleHandshake(connPacket *types.StreamPacket) error {
	if s.authHandler == nil {
		return fmt.Errorf("auth handler not configured")
	}

	// 获取或创建 ClientConnection
	s.connLock.Lock()
	clientConn, ok := s.clientConnMap[connPacket.ConnectionID]
	if !ok {
		// 从基础连接创建 ClientConnection
		if baseConn, exists := s.connMap[connPacket.ConnectionID]; exists {
			clientConn = &ClientConnection{
				ConnID:    baseConn.ID,
				Stream:    baseConn.Stream,
				CreatedAt: baseConn.CreatedAt,
				baseConn:  baseConn,
			}
			s.clientConnMap[connPacket.ConnectionID] = clientConn
		}
	}
	s.connLock.Unlock()

	if clientConn == nil {
		return fmt.Errorf("connection not found: %s", connPacket.ConnectionID)
	}

	// 构造握手请求（从 packet 解析）
	req := &packet.HandshakeRequest{}
	// TODO: 解析请求数据

	// 调用 authHandler 处理
	resp, err := s.authHandler.HandleHandshake(clientConn, req)
	if err != nil {
		return err
	}

	// 发送响应
	utils.Debugf("Handshake response: %+v", resp)
	return nil
}

// handleTunnelOpen 处理隧道打开请求
func (s *SessionManager) handleTunnelOpen(connPacket *types.StreamPacket) error {
	if s.tunnelHandler == nil {
		return fmt.Errorf("tunnel handler not configured")
	}

	// 获取 ClientConnection
	s.connLock.RLock()
	clientConn := s.clientConnMap[connPacket.ConnectionID]
	s.connLock.RUnlock()

	if clientConn == nil {
		return fmt.Errorf("client connection not found: %s", connPacket.ConnectionID)
	}

	// 构造隧道打开请求
	req := &packet.TunnelOpenRequest{}
	// TODO: 解析请求数据

	// 调用 tunnelHandler 处理
	return s.tunnelHandler.HandleTunnelOpen(clientConn, req)
}

