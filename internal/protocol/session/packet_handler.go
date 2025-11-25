package session

import (
	"encoding/json"
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

	// 解析握手请求（从 Payload）
	req := &packet.HandshakeRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			utils.Errorf("Failed to parse handshake request: %v", err)
			return fmt.Errorf("invalid handshake request format: %w", err)
		}
	}

	utils.Debugf("Handshake request: ClientID=%d, Version=%s, Protocol=%s",
		req.ClientID, req.Version, req.Protocol)

	// 调用 authHandler 处理
	resp, err := s.authHandler.HandleHandshake(clientConn, req)
	if err != nil {
		utils.Errorf("Handshake failed for connection %s: %v", connPacket.ConnectionID, err)
		// 发送失败响应
		s.sendHandshakeResponse(clientConn, &packet.HandshakeResponse{
			Success: false,
			Error:   err.Error(),
		})
		return err
	}

	// 发送成功响应
	if err := s.sendHandshakeResponse(clientConn, resp); err != nil {
		utils.Errorf("Failed to send handshake response: %v", err)
		return err
	}

	utils.Infof("Handshake succeeded for connection %s, ClientID=%d",
		connPacket.ConnectionID, req.ClientID)
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

	// 解析隧道打开请求（从 Payload）
	req := &packet.TunnelOpenRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			utils.Errorf("Failed to parse tunnel open request: %v", err)
			// 发送失败响应
			s.sendTunnelOpenResponse(clientConn, &packet.TunnelOpenAckResponse{
				TunnelID: "",
				Success:  false,
				Error:    fmt.Sprintf("invalid tunnel open request format: %v", err),
			})
			return fmt.Errorf("invalid tunnel open request format: %w", err)
		}
	}

	utils.Debugf("Tunnel open request: TunnelID=%s, MappingID=%s",
		req.TunnelID, req.MappingID)

	// 调用 tunnelHandler 处理
	if err := s.tunnelHandler.HandleTunnelOpen(clientConn, req); err != nil {
		utils.Errorf("Tunnel open failed for connection %s: %v", connPacket.ConnectionID, err)
		// 发送失败响应
		s.sendTunnelOpenResponse(clientConn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    err.Error(),
		})
		return err
	}

	utils.Infof("Tunnel open succeeded for connection %s, TunnelID=%s",
		connPacket.ConnectionID, req.TunnelID)
	return nil
}

// ============================================================================
// 辅助方法：响应发送
// ============================================================================

// sendHandshakeResponse 发送握手响应
func (s *SessionManager) sendHandshakeResponse(conn *ClientConnection, resp *packet.HandshakeResponse) error {
	// 序列化响应
	respData, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal handshake response: %w", err)
	}

	// 构造响应包
	respPacket := &packet.TransferPacket{
		PacketType: packet.HandshakeResp,
		Payload:    respData,
	}

	// 发送响应
	if _, err := conn.Stream.WritePacket(respPacket, false, 0); err != nil {
		return fmt.Errorf("failed to write handshake response: %w", err)
	}

	return nil
}

// sendTunnelOpenResponse 发送隧道打开响应
func (s *SessionManager) sendTunnelOpenResponse(conn *ClientConnection, resp *packet.TunnelOpenAckResponse) error {
	// 序列化响应
	respData, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal tunnel open response: %w", err)
	}

	// 构造响应包
	respPacket := &packet.TransferPacket{
		PacketType: packet.TunnelOpenAck,
		Payload:    respData,
	}

	// 发送响应
	if _, err := conn.Stream.WritePacket(respPacket, false, 0); err != nil {
		return fmt.Errorf("failed to write tunnel open response: %w", err)
	}

	return nil
}
