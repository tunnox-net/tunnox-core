package session

import (
	"encoding/json"
	"fmt"
	"net"
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
		utils.Warnf("Unhandled packet type: %v", packetType)
		return fmt.Errorf("unhandled packet type: %v", packetType)
	}
}

// handleHandshake 处理握手请求
func (s *SessionManager) handleHandshake(connPacket *types.StreamPacket) error {
	if s.authHandler == nil {
		return fmt.Errorf("auth handler not configured")
	}

	// 解析握手请求（从 Payload）
	req := &packet.HandshakeRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			utils.Errorf("Failed to parse handshake request: %v", err)
			return fmt.Errorf("invalid handshake request format: %w", err)
		}
	}

	isControlConnection := req.ConnectionType != "tunnel"
	if req.ConnectionType == "" {
		isControlConnection = true
	}

	var clientConn ControlConnectionInterface
	if isControlConnection {
		// 获取或创建控制连接
		existingConn := s.getControlConnectionByConnID(connPacket.ConnectionID)
		if existingConn != nil {
			clientConn = existingConn
		} else {
			// 获取底层连接信息
			conn := s.getConnectionByConnID(connPacket.ConnectionID)
			if conn == nil {
				return fmt.Errorf("connection not found: %s", connPacket.ConnectionID)
			}

			// 创建控制连接
			enforcedProtocol := conn.Protocol
			if enforcedProtocol == "" {
				enforcedProtocol = "tcp" // 默认协议
			}
			// 从连接中提取远程地址
			var remoteAddr net.Addr
			if conn.RawConn != nil {
				remoteAddr = conn.RawConn.RemoteAddr()
			}
			newConn := NewControlConnection(conn.ID, conn.Stream, remoteAddr, enforcedProtocol)
			s.RegisterControlConnection(newConn)
			clientConn = newConn
		}
	} else {
		conn := s.getConnectionByConnID(connPacket.ConnectionID)
		if conn == nil {
			return fmt.Errorf("connection not found: %s", connPacket.ConnectionID)
		}
		enforcedProtocol := conn.Protocol
		if enforcedProtocol == "" {
			enforcedProtocol = "tcp"
		}
		var remoteAddr net.Addr
		if conn.RawConn != nil {
			remoteAddr = conn.RawConn.RemoteAddr()
		}
		newConn := NewControlConnection(conn.ID, conn.Stream, remoteAddr, enforcedProtocol)
		s.controlConnLock.Lock()
		s.controlConnMap[connPacket.ConnectionID] = newConn
		s.controlConnLock.Unlock()
		clientConn = newConn
	}

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

	if isControlConnection && clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
		s.controlConnLock.Lock()
		oldConn, exists := s.clientIDIndexMap[clientConn.GetClientID()]
		if exists && oldConn != nil && oldConn.GetConnID() != clientConn.GetConnID() {
			utils.Warnf("Client %d reconnected: oldConnID=%s, newConnID=%s, cleaning up old connection",
				clientConn.GetClientID(), oldConn.GetConnID(), clientConn.GetConnID())
			delete(s.controlConnMap, oldConn.GetConnID())
		}
		// 需要将接口转换为具体类型存储（内部实现需要）
		if concreteConn, ok := clientConn.(*ControlConnection); ok {
			s.clientIDIndexMap[clientConn.GetClientID()] = concreteConn
		}
		s.controlConnLock.Unlock()

		conn := s.getConnectionByConnID(connPacket.ConnectionID)
		if conn != nil && conn.Stream != nil {
			// 使用接口获取 Reader，而不是类型断言
			reader := conn.Stream.GetReader()

			// 协议特定的握手后处理（通过统一的回调接口）
			if handshakeHandler, ok := reader.(interface{ OnHandshakeComplete(clientID int64) }); ok {
				handshakeHandler.OnHandshakeComplete(clientConn.GetClientID())
			}
		}
	}

	utils.Infof("Handshake succeeded for connection %s, ClientID=%d",
		connPacket.ConnectionID, clientConn.GetClientID())
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
		// 这是目标端的连接，连接到本地已有的bridge

		// ✅ 隧道连接（有 MappingID）不应该被注册为控制连接
		// 由于现在在 Handshake 中已经通过 ConnectionType 识别，这里只需要清理可能的误注册
		if req.MappingID != "" {
			s.controlConnLock.Lock()
			if controlConn, exists := s.controlConnMap[connPacket.ConnectionID]; exists {
				// 如果这个连接被错误注册为控制连接，移除它
				delete(s.controlConnMap, connPacket.ConnectionID)
				if controlConn.IsAuthenticated() && controlConn.GetClientID() > 0 {
					// 如果 clientIDIndexMap 中也指向这个连接，移除它
					if currentControlConn, exists := s.clientIDIndexMap[controlConn.ClientID]; exists && currentControlConn.ConnID == connPacket.ConnectionID {
						delete(s.clientIDIndexMap, controlConn.ClientID)
					}
				}
			}
			s.controlConnLock.Unlock()
		}

		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  true,
		})

		netConn := s.extractNetConn(conn)
		if netConn == nil {
			utils.Errorf("Tunnel[%s]: failed to extract net.Conn from target connection %s", req.TunnelID, conn.ID)
			return fmt.Errorf("failed to extract net.Conn from connection")
		}

		bridge.SetTargetConnection(netConn, conn.Stream)

		if req.MappingID != "" {
			s.connLock.Lock()
			delete(s.connMap, connPacket.ConnectionID)
			s.connLock.Unlock()
		}

		return fmt.Errorf("tunnel target connected, switching to stream mode")
	}

	if s.tunnelRouting != nil {
		routingState, err := s.tunnelRouting.LookupWaitingTunnel(s.Ctx(), req.TunnelID)
		if err == nil {
			return s.handleCrossServerTargetConnection(conn, req, routingState)
		} else if err != ErrTunnelNotFound && err != ErrTunnelExpired {
			utils.Errorf("Tunnel[%s]: failed to lookup routing state: %v", req.TunnelID, err)
		}
	}

	clientConn := s.getControlConnectionByConnID(connPacket.ConnectionID)
	if clientConn == nil {
		utils.Warnf("Tunnel[%s]: control connection not found for connID %s", req.TunnelID, connPacket.ConnectionID)
		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    "connection not found or not authenticated",
		})
		return fmt.Errorf("control connection not found: %s", connPacket.ConnectionID)
	}

	if err := s.tunnelHandler.HandleTunnelOpen(clientConn, req); err != nil {
		utils.Errorf("Tunnel open failed for connection %s: %v", connPacket.ConnectionID, err)
		s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
			TunnelID: req.TunnelID,
			Success:  false,
			Error:    err.Error(),
		})
		return err
	}

	if req.MappingID != "" {
		s.controlConnLock.Lock()
		if _, exists := s.controlConnMap[connPacket.ConnectionID]; exists {
			delete(s.controlConnMap, connPacket.ConnectionID)
			if clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
				if currentControlConn, exists := s.clientIDIndexMap[clientConn.GetClientID()]; exists && currentControlConn.GetConnID() == connPacket.ConnectionID {
					delete(s.clientIDIndexMap, clientConn.GetClientID())
				}
			}
		}
		s.controlConnLock.Unlock()
	}

	s.sendTunnelOpenResponseDirect(conn, &packet.TunnelOpenAckResponse{
		TunnelID: req.TunnelID,
		Success:  true,
	})

	netConn := s.extractNetConn(conn)
	if netConn == nil {
		utils.Errorf("Tunnel[%s]: failed to extract net.Conn from connection %s", req.TunnelID, conn.ID)
		return fmt.Errorf("failed to extract net.Conn from connection")
	}

	if err := s.startSourceBridge(req, netConn, conn.Stream); err != nil {
		utils.Errorf("Tunnel[%s]: failed to start bridge: %v", req.TunnelID, err)
		return err
	}

	if req.MappingID != "" {
		s.controlConnLock.Lock()
		if controlConn, exists := s.controlConnMap[connPacket.ConnectionID]; exists {
			delete(s.controlConnMap, connPacket.ConnectionID)
			if controlConn.IsAuthenticated() && controlConn.GetClientID() > 0 {
				if currentControlConn, exists := s.clientIDIndexMap[controlConn.GetClientID()]; exists && currentControlConn.GetConnID() == connPacket.ConnectionID {
					delete(s.clientIDIndexMap, controlConn.GetClientID())
				}
			}
		}
		s.controlConnLock.Unlock()

		s.connLock.Lock()
		delete(s.connMap, connPacket.ConnectionID)
		s.connLock.Unlock()
	}

	return fmt.Errorf("tunnel source connected, switching to stream mode")
}

// ============================================================================
// 辅助方法：响应发送
// ============================================================================

// sendHandshakeResponse 发送握手响应
func (s *SessionManager) sendHandshakeResponse(conn ControlConnectionInterface, resp *packet.HandshakeResponse) error {
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

	// 发送响应（统一通过 StreamProcessor 处理，所有协议都同步处理）
	utils.Debugf("Sending handshake response to connection %s, ClientID=%d", conn.GetConnID(), conn.GetClientID())

	if _, err := conn.GetStream().WritePacket(respPacket, true, 0); err != nil {
		utils.Errorf("Failed to write handshake response to connection %s: %v", conn.GetConnID(), err)
		return err
	}

	utils.Debugf("Handshake response written successfully to connection %s", conn.GetConnID())
	return nil
}

// sendTunnelOpenResponse 发送隧道打开响应
func (s *SessionManager) sendTunnelOpenResponse(conn ControlConnectionInterface, resp *packet.TunnelOpenAckResponse) error {
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
	if _, err := conn.GetStream().WritePacket(respPacket, false, 0); err != nil {
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
	if _, err := conn.Stream.WritePacket(respPacket, true, 0); err != nil {
		return fmt.Errorf("failed to write tunnel open response: %w", err)
	}

	return nil
}

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

// notifyTargetClientToOpenTunnel 通知目标客户端建立隧道连接
func (s *SessionManager) notifyTargetClientToOpenTunnel(req *packet.TunnelOpenRequest) {
	// 1. 获取映射配置
	if s.cloudControl == nil {
		utils.Errorf("Tunnel[%s]: CloudControl not configured, cannot notify target client", req.TunnelID)
		return
	}

	// ✅ 统一使用 GetPortMapping，直接返回 PortMapping
	mapping, err := s.cloudControl.GetPortMapping(req.MappingID)
	if err != nil {
		utils.Errorf("Tunnel[%s]: failed to get mapping %s: %v", req.TunnelID, req.MappingID, err)
		return
	}

	// 2. 找到目标客户端的控制连接（本地或跨服务器）
	targetControlConn := s.GetControlConnectionByClientID(mapping.TargetClientID)
	if targetControlConn == nil {
		// ✅ 本地未找到，尝试跨服务器转发
		if s.bridgeManager != nil {
			utils.Infof("Tunnel[%s]: target client %d not on this server, broadcasting to other nodes",
				req.TunnelID, mapping.TargetClientID)
			if err := s.bridgeManager.BroadcastTunnelOpen(req, mapping.TargetClientID); err != nil {
				utils.Errorf("Tunnel[%s]: failed to broadcast to other nodes: %v", req.TunnelID, err)
			} else {
				utils.Infof("Tunnel[%s]: broadcasted to other nodes for client %d",
					req.TunnelID, mapping.TargetClientID)
			}
		} else {
			utils.Errorf("Tunnel[%s]: target client %d not connected and BridgeManager not configured",
				req.TunnelID, mapping.TargetClientID)
		}
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
		"bandwidth_limit":    mapping.Config.BandwidthLimit,
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

	utils.Infof("Tunnel[%s]: sent TunnelOpenRequest to target client %d via control connection",
		req.TunnelID, mapping.TargetClientID)
}
