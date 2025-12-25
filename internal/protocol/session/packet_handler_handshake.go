package session

import (
	"encoding/json"
	"fmt"
	"net"
	corelog "tunnox-core/internal/core/log"

	"tunnox-core/internal/core/types"
	"tunnox-core/internal/packet"
)

// handleHandshake 处理握手请求
func (s *SessionManager) handleHandshake(connPacket *types.StreamPacket) error {
	if s.authHandler == nil {
		return fmt.Errorf("auth handler not configured")
	}

	// 解析握手请求（从 Payload）
	req := &packet.HandshakeRequest{}
	if len(connPacket.Packet.Payload) > 0 {
		if err := json.Unmarshal(connPacket.Packet.Payload, req); err != nil {
			corelog.Errorf("Failed to parse handshake request: %v", err)
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
		corelog.Errorf("Handshake failed for connection %s: %v", connPacket.ConnectionID, err)
		// 发送失败响应
		s.sendHandshakeResponse(clientConn, &packet.HandshakeResponse{
			Success: false,
			Error:   err.Error(),
		})
		return err
	}

	// 发送成功响应
	if err := s.sendHandshakeResponse(clientConn, resp); err != nil {
		corelog.Errorf("Failed to send handshake response: %v", err)
		return err
	}

	// 调试日志
	corelog.Infof("Handshake: after sendHandshakeResponse - isControlConnection=%v, isAuthenticated=%v, clientID=%d, connID=%s",
		isControlConnection, clientConn.IsAuthenticated(), clientConn.GetClientID(), connPacket.ConnectionID)

	if isControlConnection && clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
		corelog.Infof("Handshake: updating clientIDIndexMap for client %d, isControlConnection=%v, isAuthenticated=%v",
			clientConn.GetClientID(), isControlConnection, clientConn.IsAuthenticated())
		s.controlConnLock.Lock()
		oldConn, exists := s.clientIDIndexMap[clientConn.GetClientID()]
		if exists && oldConn != nil && oldConn.GetConnID() != clientConn.GetConnID() {
			corelog.Warnf("Client %d reconnected: oldConnID=%s, newConnID=%s, cleaning up old connection",
				clientConn.GetClientID(), oldConn.GetConnID(), clientConn.GetConnID())
			delete(s.controlConnMap, oldConn.GetConnID())
			// 清理旧连接的 Redis 记录
			if s.connStateStore != nil {
				if err := s.connStateStore.UnregisterConnection(s.Ctx(), oldConn.GetConnID()); err != nil {
					corelog.Warnf("Failed to unregister old connection state: %v", err)
				}
			}
		}
		// 需要将接口转换为具体类型存储（内部实现需要）
		if concreteConn, ok := clientConn.(*ControlConnection); ok {
			s.clientIDIndexMap[clientConn.GetClientID()] = concreteConn
		}
		s.controlConnLock.Unlock()

		// ✅ 登记客户端位置到 Redis（用于跨节点查询）
		if s.connStateStore != nil {
			conn := s.getConnectionByConnID(connPacket.ConnectionID)
			protocol := "tcp"
			if conn != nil && conn.Protocol != "" {
				protocol = conn.Protocol
			}
			stateInfo := &ConnectionStateInfo{
				ConnectionID: connPacket.ConnectionID,
				ClientID:     clientConn.GetClientID(),
				NodeID:       s.nodeID,
				Protocol:     protocol,
				ConnType:     "control",
			}
			if err := s.connStateStore.RegisterConnection(s.Ctx(), stateInfo); err != nil {
				corelog.Warnf("Failed to register connection state: %v", err)
			} else {
				corelog.Infof("Registered client %d location to Redis (node=%s, connID=%s)",
					clientConn.GetClientID(), s.nodeID, connPacket.ConnectionID)
			}
		}

		conn := s.getConnectionByConnID(connPacket.ConnectionID)
		if conn != nil && conn.Stream != nil {
			// 使用接口获取 Reader，而不是类型断言
			reader := conn.Stream.GetReader()

			// 协议特定的握手后处理（通过统一的回调接口）
			if handshakeHandler, ok := reader.(interface{ OnHandshakeComplete(clientID int64) }); ok {
				handshakeHandler.OnHandshakeComplete(clientConn.GetClientID())
			}
		}
	} else {
		corelog.Warnf("Handshake: NOT updating clientIDIndexMap - isControlConnection=%v, isAuthenticated=%v, clientID=%d",
			isControlConnection, clientConn.IsAuthenticated(), clientConn.GetClientID())
	}

	corelog.Infof("Handshake succeeded for connection %s, ClientID=%d",
		connPacket.ConnectionID, clientConn.GetClientID())

	// ✅ 握手成功后，主动推送客户端的映射配置
	if isControlConnection && clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
		go s.pushConfigToClient(clientConn)
	}

	return nil
}

// pushConfigToClient 推送配置给客户端
func (s *SessionManager) pushConfigToClient(conn ControlConnectionInterface) {
	if s.authHandler == nil {
		corelog.Warnf("SessionManager: authHandler is nil, cannot push config to client %d", conn.GetClientID())
		return
	}

	configBody, err := s.authHandler.GetClientConfig(conn)
	if err != nil {
		corelog.Errorf("SessionManager: failed to get config for client %d: %v", conn.GetClientID(), err)
		return
	}

	// 发送配置
	responseCmd := &packet.CommandPacket{
		CommandType: packet.ConfigSet,
		CommandBody: configBody,
	}

	responsePacket := &packet.TransferPacket{
		PacketType:    packet.JsonCommand,
		CommandPacket: responseCmd,
	}

	stream := conn.GetStream()
	if stream == nil {
		corelog.Errorf("SessionManager: stream is nil for client %d", conn.GetClientID())
		return
	}

	if _, err := stream.WritePacket(responsePacket, true, 0); err != nil {
		corelog.Errorf("SessionManager: failed to push config to client %d: %v", conn.GetClientID(), err)
		return
	}

	corelog.Infof("SessionManager: pushed config to client %d (%d bytes)", conn.GetClientID(), len(configBody))
}

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
	corelog.Infof("Sending handshake response to connection %s, ClientID=%d", conn.GetConnID(), conn.GetClientID())

	stream := conn.GetStream()
	if stream == nil {
		corelog.Errorf("Failed to send handshake response: stream is nil for connection %s", conn.GetConnID())
		return fmt.Errorf("stream is nil")
	}

	// 调试：检查 stream 类型
	corelog.Infof("sendHandshakeResponse: stream type=%T, connID=%s", stream, conn.GetConnID())
	type streamProcessorGetter interface {
		GetStreamProcessor() interface {
			GetClientID() int64
			GetConnectionID() string
			GetMappingID() string
		}
	}
	if adapter, ok := stream.(streamProcessorGetter); ok {
		sp := adapter.GetStreamProcessor()
		if sp != nil {
			corelog.Infof("sendHandshakeResponse: adapter contains streamProcessor type=%T, connID=%s, clientID=%d", sp, conn.GetConnID(), sp.GetClientID())
		}
	}

	if _, err := stream.WritePacket(respPacket, true, 0); err != nil {
		corelog.Errorf("Failed to write handshake response to connection %s: %v", conn.GetConnID(), err)
		return err
	}

	corelog.Infof("Handshake response written successfully to connection %s", conn.GetConnID())
	return nil
}
