package session

import (
corelog "tunnox-core/internal/core/log"
	"encoding/json"
	"fmt"
	"net"

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

	if isControlConnection && clientConn.IsAuthenticated() && clientConn.GetClientID() > 0 {
		s.controlConnLock.Lock()
		oldConn, exists := s.clientIDIndexMap[clientConn.GetClientID()]
		if exists && oldConn != nil && oldConn.GetConnID() != clientConn.GetConnID() {
			corelog.Warnf("Client %d reconnected: oldConnID=%s, newConnID=%s, cleaning up old connection",
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

	corelog.Infof("Handshake succeeded for connection %s, ClientID=%d",
		connPacket.ConnectionID, clientConn.GetClientID())
	return nil
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

	// 调试：检查是否是 httppollStreamAdapter，如果是，检查其内部的 ServerStreamProcessor
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

