package session

import (
	"encoding/json"
	"fmt"
	"net"
	"tunnox-core/internal/cloud/models"
	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
	"tunnox-core/internal/stream"
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

	// 获取控制连接
	clientConn := s.getControlConnectionByConnID(connPacket.ConnectionID)
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

	// ✅ 将认证成功的客户端添加到 controlConnMap 和 clientIDIndexMap
	if clientConn.Authenticated && clientConn.ClientID > 0 {
		s.controlConnLock.Lock()
		// 添加到controlConnMap
		s.controlConnMap[clientConn.ConnID] = clientConn
		// 添加到clientIDIndexMap用于快速查找
		s.clientIDIndexMap[clientConn.ClientID] = clientConn
		s.controlConnLock.Unlock()
		utils.Debugf("SessionManager: added client %d to clientIDIndexMap", clientConn.ClientID)
	}

	utils.Infof("Handshake succeeded for connection %s, ClientID=%d",
		connPacket.ConnectionID, clientConn.ClientID)
	return nil
}

// handleTunnelOpen 处理隧道打开请求
// 这个方法处理两种情况：
// 1. 源端客户端发起的隧道连接（需要创建bridge并通知目标端）
// 2. 目标端客户端响应的隧道连接（连接到已有的bridge）
func (s *SessionManager) handleTunnelOpen(connPacket *types.StreamPacket) error {
	if s.tunnelHandler == nil {
		return fmt.Errorf("tunnel handler not configured")
	}

	// 获取底层连接
	conn := s.getConnectionByConnID(connPacket.ConnectionID)
	if conn == nil {
		return fmt.Errorf("connection not found: %s", connPacket.ConnectionID)
	}

	// 解析隧道打开请求（从 Payload）
	req := &packet.TunnelOpenRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			utils.Errorf("Failed to parse tunnel open request: %v", err)
			s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
				TunnelID: "",
				Success:  false,
				Error:    fmt.Sprintf("invalid tunnel open request format: %v", err),
			})
			return fmt.Errorf("invalid tunnel open request format: %w", err)
		}
	}

	utils.Infof("Tunnel open request: TunnelID=%s, MappingID=%s, ConnID=%s",
		req.TunnelID, req.MappingID, connPacket.ConnectionID)

	// 检查是否已有bridge（目标端连接）
	s.bridgeLock.Lock()
	bridge, exists := s.tunnelBridges[req.TunnelID]
	s.bridgeLock.Unlock()

	if exists {
		// 这是目标端的连接，连接到已有的bridge
		utils.Infof("Tunnel[%s]: target connection arrived, attaching to bridge", req.TunnelID)

		// 发送成功响应
		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  true,
		})

		// 获取底层的net.Conn
		netConn := s.extractNetConn(conn)
		if netConn == nil {
			utils.Errorf("Tunnel[%s]: failed to extract net.Conn from target connection %s", req.TunnelID, conn.ID)
			return fmt.Errorf("failed to extract net.Conn from connection")
		}
		utils.Infof("Tunnel[%s]: extracted targetConn=%v (LocalAddr=%v, RemoteAddr=%v)",
			req.TunnelID, netConn, netConn.LocalAddr(), netConn.RemoteAddr())

		// 将目标端连接设置到bridge
		bridge.SetTargetConnection(netConn, conn.Stream)

		// ✅ 返回特殊错误，让ProcessPacketLoop停止处理
		return fmt.Errorf("tunnel target connected, switching to stream mode")
	}

	// 这是源端的连接，需要验证权限并创建bridge
	// 创建临时ControlConnection用于权限验证
	tempClientConn := &ControlConnection{
		ConnID: conn.ID,
		Stream: conn.Stream,
	}
	if err := s.tunnelHandler.HandleTunnelOpen(tempClientConn, req); err != nil {
		utils.Errorf("Tunnel open failed for connection %s: %v", connPacket.ConnectionID, err)
		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    err.Error(),
		})
		return err
	}

	// 发送成功响应给源端
	s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  true,
	})

	utils.Infof("Tunnel[%s]: source connection established, creating bridge", req.TunnelID)

	// 获取底层的net.Conn
	netConn := s.extractNetConn(conn)
	if netConn == nil {
		utils.Errorf("Tunnel[%s]: failed to extract net.Conn from connection %s", req.TunnelID, conn.ID)
		return fmt.Errorf("failed to extract net.Conn from connection")
	}
	utils.Infof("Tunnel[%s]: extracted sourceConn=%v (LocalAddr=%v, RemoteAddr=%v)",
		req.TunnelID, netConn, netConn.LocalAddr(), netConn.RemoteAddr())

	if err := s.startSourceBridge(req, netConn, conn.Stream); err != nil {
		utils.Errorf("Tunnel[%s]: failed to start bridge: %v", req.TunnelID, err)
		return err
	}

	// ✅ 返回特殊错误，让ProcessPacketLoop停止处理
	return fmt.Errorf("tunnel source connected, switching to stream mode")
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

	utils.Debugf("sendHandshakeResponse: respData=%s, len=%d", string(respData), len(respData))

	// 构造响应包
	respPacket := &packet.TransferPacket{
		PacketType: packet.HandshakeResp,
		Payload:    respData,
	}

	utils.Debugf("sendHandshakeResponse: PacketType=%d, Payload len=%d", respPacket.PacketType, len(respPacket.Payload))

	// 发送响应
	written, err := conn.Stream.WritePacket(respPacket, false, 0)
	if err != nil {
		return fmt.Errorf("failed to write handshake response: %w", err)
	}

	utils.Debugf("sendHandshakeResponse: wrote %d bytes", written)
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

// sendTunnelOpenResponseDirect 直接发送隧道打开响应（使用types.Connection）
func (s *SessionManager) sendTunnelOpenResponseDirect(conn *types.Connection, resp *packet.TunnelOpenAckResponse) error {
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

// extractNetConn 从types.Connection中提取底层的net.Conn
func (s *SessionManager) extractNetConn(conn *types.Connection) net.Conn {
	// ✅ 直接使用保存的原始连接
	if conn.RawConn != nil {
		return conn.RawConn
	}

	// 回退方案：尝试从Stream中获取
	if sp, ok := conn.Stream.(*stream.StreamProcessor); ok {
		if reader, ok := sp.GetReader().(net.Conn); ok {
			return reader
		}
	}
	return nil
}

// notifyTargetClientToOpenTunnel 通知目标客户端建立隧道连接
func (s *SessionManager) notifyTargetClientToOpenTunnel(req *packet.TunnelOpenRequest) {
	// 1. 获取映射配置
	if s.cloudControl == nil {
		utils.Errorf("Tunnel[%s]: CloudControl not configured, cannot notify target client", req.TunnelID)
		return
	}

	mappingInterface, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		utils.Errorf("Tunnel[%s]: failed to get mapping %s: %v", req.TunnelID, req.MappingID, err)
		return
	}

	// 类型断言为 *models.PortMapping
	mapping, ok := mappingInterface.(*models.PortMapping)
	if !ok {
		utils.Errorf("Tunnel[%s]: failed to cast mapping to *models.PortMapping, got type %T",
			req.TunnelID, mappingInterface)
		return
	}

	// 2. 找到目标客户端的控制连接
	targetControlConn := s.GetControlConnectionByClientID(mapping.TargetClientID)
	if targetControlConn == nil {
		utils.Errorf("Tunnel[%s]: target client %d not connected", req.TunnelID, mapping.TargetClientID)
		return
	}

	// 3. 构造TunnelOpenRequest命令
	cmdBody := map[string]interface{}{
		"tunnel_id":          req.TunnelID,
		"mapping_id":         req.MappingID,
		"secret_key":         mapping.SecretKey,
		"target_host":        mapping.TargetHost,
		"target_port":        mapping.TargetPort,
		"protocol":           string(mapping.Protocol),
		"enable_compression": mapping.Config.EnableCompression,
		"compression_level":  mapping.Config.CompressionLevel,
		"enable_encryption":  mapping.Config.EnableEncryption,
		"encryption_method":  mapping.Config.EncryptionMethod,
		"encryption_key":     mapping.Config.EncryptionKey,
		"bandwidth_limit":    mapping.Config.BandwidthLimit, // ✅ 添加带宽限制
	}

	cmdBodyJSON, err := json.Marshal(cmdBody)
	if err != nil {
		utils.Errorf("Tunnel[%s]: failed to marshal command body: %v", req.TunnelID, err)
		return
	}

	// 4. 通过控制连接发送命令
	cmd := &packet.CommandPacket{
		CommandType: packet.TunnelOpenRequestCmd, // 60
		CommandBody: string(cmdBodyJSON),
	}

	pkt := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: cmd,
	}

	_, err = targetControlConn.Stream.WritePacket(pkt, false, 0)
	if err != nil {
		utils.Errorf("Tunnel[%s]: failed to send tunnel open request to target client %d: %v",
			req.TunnelID, mapping.TargetClientID, err)
		return
	}

	utils.Infof("Tunnel[%s]: ✅ sent TunnelOpenRequest to target client %d via control connection",
		req.TunnelID, mapping.TargetClientID)
}
